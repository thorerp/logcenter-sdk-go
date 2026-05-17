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

type Client struct {
	config  Config
	queue   *eventQueue
	notify  chan struct{}
	flushes chan flushRequest
	closeCh chan struct{}
	done    chan struct{}
	closed  atomic.Bool
	once    sync.Once
	stats   counters
}

type flushRequest struct {
	ctx  context.Context
	done chan error
}

func NewClient(config Config) *Client {
	config = config.normalized()
	client := &Client{
		config:  config,
		queue:   newEventQueue(config.BufferSize),
		notify:  make(chan struct{}, 1),
		flushes: make(chan flushRequest),
		closeCh: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go client.run()
	return client
}

func (client *Client) Stats() Stats {
	return client.stats.snapshot()
}

func (client *Client) SendEvent(ctx context.Context, event Event) bool {
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
	return client.enqueue(event)
}

func (client *Client) Flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ctx == nil {
		ctx = context.Background()
	}
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
	if client.closed.Load() {
		client.stats.dropped.Add(1)
		return false
	}

	event = client.withDefaults(redactEvent(event))
	queued, dropped := client.queue.push(event)
	if dropped {
		client.stats.dropped.Add(1)
	}
	if !queued {
		return false
	}
	client.stats.queued.Add(1)
	client.signal()
	return true
}

func (client *Client) withDefaults(event Event) Event {
	now := time.Now().UTC()
	if event.EventID == "" {
		event.EventID = newID("evt_")
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
			return firstErr
		}
		if err := client.sendBatch(ctx, events); err != nil && firstErr == nil {
			firstErr = err
		}
	}
}

func (client *Client) sendBatch(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	if strings.TrimSpace(client.config.Endpoint) == "" {
		err := fmt.Errorf("logcenter endpoint is empty")
		client.recordBatchFailure(err, len(events))
		return err
	}
	if strings.TrimSpace(client.config.APIKey) == "" {
		err := fmt.Errorf("logcenter api key is empty")
		client.recordBatchFailure(err, len(events))
		return err
	}

	payload := batchRequest{
		BatchID: newID("bt_"),
		SentAt:  formatTime(time.Now()),
		Source:  RedactFields(Fields(client.config.Source)),
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
			return nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == attempts-1 {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}

	client.recordBatchFailure(lastErr, len(events))
	return lastErr
}

func (client *Client) postBatch(ctx context.Context, payload batchRequest) (BatchResponse, error) {
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return BatchResponse{}, err
	}

	requestContext, cancel := context.WithTimeout(ctx, client.config.Timeout)
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

func (client *Client) recordBatchFailure(err error, eventCount int) {
	client.stats.failedBatches.Add(1)
	client.stats.failedEvents.Add(uint64(eventCount))
	client.stats.setError(err)
}

type httpError struct {
	statusCode int
	body       string
}

func (err httpError) Error() string {
	if err.body == "" {
		return fmt.Sprintf("logcenter ingest failed with status %d", err.statusCode)
	}
	return fmt.Sprintf("logcenter ingest failed with status %d: %s", err.statusCode, err.body)
}

func isRetryable(err error) bool {
	var httpErr httpError
	if errors.As(err, &httpErr) {
		return httpErr.statusCode == http.StatusTooManyRequests || httpErr.statusCode >= 500
	}
	return true
}
