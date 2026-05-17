package logcenter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrClientClosed = errors.New("logcenter client is closed")

type Client struct {
	config   Config
	disabled bool
	redactor redactor
	limiter  limiter
	queue    *eventQueue
	notify   chan struct{}
	flushes  chan flushRequest
	closeCh  chan struct{}
	done     chan struct{}
	closed   atomic.Bool
	once     sync.Once
	stats    counters
	outbox   *durableOutbox
	tamper   *tamperEvidence
}

type flushRequest struct {
	ctx  context.Context
	done chan error
}

func NewClient(config Config) *Client {
	config = config.normalized()
	tamper, err := newTamperEvidence(config.TamperEvidence)
	client := &Client{
		config:   config,
		disabled: !config.isEnabled(),
		redactor: newRedactor(config.SensitiveKeyFragments),
		limiter:  newLimiter(config),
		queue:    newEventQueue(config.BufferSize),
		notify:   make(chan struct{}, 1),
		flushes:  make(chan flushRequest),
		closeCh:  make(chan struct{}),
		done:     make(chan struct{}),
		outbox:   newDurableOutbox(config.OutboxPath),
		tamper:   tamper,
	}
	if err != nil {
		client.stats.setError(err)
	}
	if client.disabled {
		client.closed.Store(true)
		close(client.done)
		return client
	}
	go client.run()
	return client
}

func NewNoopClient() *Client {
	return NewClient(Config{Enabled: Bool(false)})
}

func (client *Client) Enabled() bool {
	return !client.disabled
}

func (client *Client) Stats() Stats {
	return client.stats.snapshot()
}

func (client *Client) SendEvent(ctx context.Context, event Event) bool {
	event = withContextFields(ctx, event)
	return client.enqueue(event)
}

func (client *Client) SendEventSync(ctx context.Context, event Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if client.disabled {
		return nil
	}
	if client.closed.Load() {
		client.stats.dropped.Add(1)
		client.eventDropped(event, "client_closed", ErrClientClosed)
		return ErrClientClosed
	}
	event = withContextFields(ctx, event)
	event, err := client.prepareEvent(event)
	if err != nil {
		client.stats.dropped.Add(1)
		client.setError(err)
		client.eventDropped(event, eventDropReason(err), err)
		return err
	}
	err = client.sendBatchSync(ctx, []Event{event})
	if err != nil && isRetryable(err) {
		client.persistOutbox([]Event{event})
	}
	return err
}

func (client *Client) Flush(ctx context.Context) error {
	if client.disabled {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := contextWithOptionalTimeout(ctx, client.config.FlushTimeout)
	defer cancel()

	request := flushRequest{
		ctx:  ctx,
		done: make(chan error, 1),
	}

	select {
	case client.flushes <- request:
	case <-ctx.Done():
		return ctx.Err()
	case <-client.done:
		return nil
	}

	select {
	case err := <-request.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-client.done:
		return nil
	}
}

func (client *Client) Close(ctx context.Context) error {
	if client.disabled {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := contextWithOptionalTimeout(ctx, client.config.CloseTimeout)
	defer cancel()

	client.once.Do(func() {
		client.closed.Store(true)
		close(client.closeCh)
	})

	select {
	case <-client.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (client *Client) enqueue(event Event) bool {
	if client.disabled {
		return false
	}
	if client.closed.Load() {
		client.stats.dropped.Add(1)
		client.eventDropped(event, "client_closed", nil)
		return false
	}

	event, err := client.prepareEvent(event)
	if err != nil {
		client.stats.dropped.Add(1)
		client.setError(err)
		client.eventDropped(event, eventDropReason(err), err)
		return false
	}
	client.persistOutbox([]Event{event})
	result := client.queue.push(event)
	if result.dropped != nil {
		client.stats.dropped.Add(1)
		client.eventDropped(*result.dropped, result.dropReason, nil)
	}
	if !result.queued {
		return false
	}
	client.stats.queued.Add(1)
	client.signal()
	return true
}

func (client *Client) prepareEvent(event Event) (Event, error) {
	event = client.withDefaults(event)
	event = client.redactor.redactEvent(event)
	var err error
	event, err = client.limiter.limitEvent(event)
	if err != nil {
		return event, err
	}
	event, err = client.applyTamperEvidence(event)
	if err != nil {
		return event, err
	}
	if err := ValidateEvent(event); err != nil {
		return event, err
	}
	return event, nil
}

func (client *Client) applyTamperEvidence(event Event) (Event, error) {
	if client.tamper == nil {
		return event, nil
	}
	return client.tamper.Apply(event)
}

func (client *Client) withDefaults(event Event) Event {
	now := time.Now().UTC()
	event.IdempotencyKey = strings.TrimSpace(event.IdempotencyKey)
	if event.EventID == "" && event.IdempotencyKey != "" {
		event.EventID = event.IdempotencyKey
	}
	if event.EventID == "" {
		event.EventID = newID("evt_")
	}
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = event.EventID
	}
	if event.OccurredAt == "" {
		event.OccurredAt = formatTime(now)
	}
	if event.Environment == "" {
		event.Environment = client.config.Environment
	}
	if event.Service == "" {
		event.Service = client.config.Service
	}
	if event.ServiceVersion == "" {
		event.ServiceVersion = client.config.Version
	}
	return event
}

func (client *Client) signal() {
	select {
	case client.notify <- struct{}{}:
	default:
	}
}

func (client *Client) run() {
	defer close(client.done)

	ticker := time.NewTicker(client.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-client.notify:
			if client.queue.len() >= client.config.BatchSize {
				client.sendQueued(context.Background())
			}
		case <-ticker.C:
			client.sendQueued(context.Background())
		case request := <-client.flushes:
			request.done <- client.sendQueued(request.ctx)
		case <-client.closeCh:
			_ = client.sendQueued(context.Background())
			return
		}
	}
}

func (client *Client) sendQueued(ctx context.Context) error {
	var firstErr error
	for {
		events := client.queue.drain(client.config.BatchSize)
		if len(events) == 0 {
			break
		}
		if err := client.sendBatch(ctx, events); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}
	if err := client.sendOutbox(ctx); err != nil {
		return err
	}
	return nil
}

func (client *Client) sendBatch(ctx context.Context, events []Event) error {
	return client.sendBatchInternal(ctx, events, false)
}

func (client *Client) sendBatchSync(ctx context.Context, events []Event) error {
	return client.sendBatchInternal(ctx, events, true)
}

func (client *Client) sendBatchInternal(ctx context.Context, events []Event, rejectAsError bool) error {
	if len(events) == 0 {
		return nil
	}
	if strings.TrimSpace(client.config.Endpoint) == "" {
		err := fmt.Errorf("logcenter endpoint is empty")
		client.recordBatchFailure(err, events)
		return err
	}
	if strings.TrimSpace(client.config.APIKey) == "" {
		err := fmt.Errorf("logcenter api key is empty")
		client.recordBatchFailure(err, events)
		return err
	}

	payload := batchRequest{
		BatchID: newID("bt_"),
		SentAt:  formatTime(time.Now()),
		Source:  client.redactor.RedactFields(Fields(client.config.Source)),
		Events:  events,
	}

	var lastErr error
	attempts := client.config.RetryAttempts + 1
	for attempt := 0; attempt < attempts; attempt++ {
		response, err := client.postBatch(ctx, payload)
		if err == nil {
			client.stats.sentBatches.Add(1)
			client.stats.sentEvents.Add(uint64(len(events)))
			client.stats.accepted.Add(uint64(response.Accepted))
			client.stats.duplicated.Add(uint64(response.Duplicated))
			client.stats.rejected.Add(uint64(response.Rejected))
			client.recordEventRejections(events, response.Errors)
			client.removeOutbox(events)
			if rejectAsError && response.Rejected > 0 {
				err := rejectedEventsError{errors: response.Errors, rejected: response.Rejected}
				client.setError(err)
				return err
			}
			return nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == attempts-1 {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}

	client.recordBatchFailure(lastErr, events)
	return lastErr
}

func (client *Client) sendOutbox(ctx context.Context) error {
	if client.outbox == nil {
		return nil
	}
	for {
		events, err := client.outbox.Peek(client.config.BatchSize)
		if err != nil {
			client.setError(err)
			return err
		}
		if len(events) == 0 {
			return nil
		}
		if err := client.sendBatchInternal(ctx, events, false); err != nil {
			return err
		}
	}
}

func withContextFields(ctx context.Context, event Event) Event {
	if ctx == nil {
		ctx = context.Background()
	}
	request, _ := RequestFromContext(ctx)
	if event.RequestID == "" {
		event.RequestID = request.RequestID
	}
	if event.TraceID == "" {
		event.TraceID = request.TraceID
	}
	if event.SpanID == "" {
		event.SpanID = request.SpanID
	}
	if event.UserID == "" {
		event.UserID = request.UserID
	}
	if event.TenantID == "" {
		event.TenantID = request.TenantID
	}
	if event.Operation == "" {
		event.Operation = request.Operation
	}
	return event
}

func eventDropReason(err error) string {
	if errors.Is(err, ErrInvalidEvent) {
		return "validation"
	}
	return "payload_limit"
}

func (client *Client) postBatch(ctx context.Context, payload batchRequest) (BatchResponse, error) {
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return BatchResponse{}, err
	}

	requestContext, cancel := context.WithTimeout(ctx, client.config.SendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, client.config.Endpoint+"/v1/ingest/batch", bytes.NewReader(requestBody))
	if err != nil {
		return BatchResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.config.APIKey)

	resp, err := client.config.HTTPClient.Do(req)
	if err != nil {
		return BatchResponse{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BatchResponse{}, httpError{statusCode: resp.StatusCode, body: string(body)}
	}

	var response BatchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return BatchResponse{}, err
	}
	return response, nil
}

func contextWithOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func (client *Client) recordBatchFailure(err error, events []Event) {
	eventCount := len(events)
	client.stats.failedBatches.Add(1)
	client.stats.failedEvents.Add(uint64(eventCount))
	client.setError(err)
	if client.config.Hooks.OnBatchFailed != nil {
		events = append([]Event(nil), events...)
		callHook(func() {
			client.config.Hooks.OnBatchFailed(BatchFailure{
				Events:     events,
				EventCount: eventCount,
				Err:        err,
			})
		})
	}
}

func (client *Client) persistOutbox(events []Event) {
	if client.outbox == nil || len(events) == 0 {
		return
	}
	if err := client.outbox.Append(copyEvents(events)); err != nil {
		client.setError(err)
	}
}

func (client *Client) removeOutbox(events []Event) {
	if client.outbox == nil || len(events) == 0 {
		return
	}
	if err := client.outbox.Remove(eventIDs(events)); err != nil {
		client.setError(err)
	}
}

func (client *Client) eventDropped(event Event, reason string, err error) {
	if client.config.Hooks.OnEventDropped == nil {
		return
	}
	callHook(func() {
		client.config.Hooks.OnEventDropped(EventDrop{
			Event:  event,
			Reason: reason,
			Err:    err,
		})
	})
}

func (client *Client) recordEventRejections(events []Event, errors []EventError) {
	if client.config.Hooks.OnEventRejected == nil {
		return
	}
	for _, eventError := range errors {
		if eventError.Index < 0 || eventError.Index >= len(events) {
			continue
		}
		event := events[eventError.Index]
		callHook(func() {
			client.config.Hooks.OnEventRejected(EventRejection{
				Event: event,
				Error: eventError,
			})
		})
	}
}

func (client *Client) setError(err error) {
	lastError, changed := client.stats.setError(err)
	if !changed || client.config.Hooks.OnErrorChanged == nil {
		return
	}
	callHook(func() {
		client.config.Hooks.OnErrorChanged(ErrorChange{
			LastError: lastError,
			Err:       err,
		})
	})
}

type httpError struct {
	statusCode int
	body       string
}

type rejectedEventsError struct {
	errors   []EventError
	rejected int
}

func (err httpError) Error() string {
	if err.body == "" {
		return fmt.Sprintf("logcenter ingest failed with status %d", err.statusCode)
	}
	return fmt.Sprintf("logcenter ingest failed with status %d: %s", err.statusCode, err.body)
}

func (err rejectedEventsError) Error() string {
	if len(err.errors) > 0 && err.errors[0].Message != "" {
		return fmt.Sprintf("logcenter ingest rejected %d event(s): %s", err.rejected, err.errors[0].Message)
	}
	return fmt.Sprintf("logcenter ingest rejected %d event(s)", err.rejected)
}

func isRetryable(err error) bool {
	var httpErr httpError
	if errors.As(err, &httpErr) {
		return httpErr.statusCode == http.StatusTooManyRequests || httpErr.statusCode >= 500
	}
	return true
}
