package logcenter

import (
	"context"
	"sync"
	"time"
)

type Span struct {
	client       *Client
	request      RequestContext
	name         string
	kind         string
	spanID       string
	parentSpanID string
	startedAt    time.Time
	metadata     Fields
	data         Fields

	mu           sync.Mutex
	errorCode    string
	errorMessage string
	ended        bool
}

type SpanOption func(*Span)

func SpanKind(kind string) SpanOption {
	return func(span *Span) {
		span.kind = kind
	}
}

func SpanMetadata(fields Fields) SpanOption {
	return func(span *Span) {
		span.metadata = fields
	}
}

func SpanData(fields Fields) SpanOption {
	return func(span *Span) {
		span.data = fields
	}
}

func (client *Client) StartSpan(ctx context.Context, name string, options ...SpanOption) (context.Context, *Span) {
	ctx, request := ensureRequestContext(ctx)
	parentSpanID := request.SpanID
	spanID := newID("spn_")
	request.SpanID = spanID

	span := &Span{
		client:       client,
		request:      request,
		name:         name,
		kind:         "internal",
		spanID:       spanID,
		parentSpanID: parentSpanID,
		startedAt:    time.Now().UTC(),
	}
	for _, option := range options {
		option(span)
	}

	return ContextWithRequest(ctx, request), span
}

func (span *Span) RecordError(err error, code string) {
	if err == nil {
		return
	}

	span.mu.Lock()
	span.errorCode = code
	span.errorMessage = err.Error()
	span.mu.Unlock()

	span.client.RecordError(ContextWithRequest(context.Background(), span.request), err, ErrorOptions{
		Code:     code,
		Severity: SeverityError,
	})
}

func (span *Span) End(status string) bool {
	span.mu.Lock()
	if span.ended {
		span.mu.Unlock()
		return false
	}
	span.ended = true
	errorCode := span.errorCode
	errorMessage := span.errorMessage
	span.mu.Unlock()

	if status == "" {
		status = StatusSuccess
	}
	finishedAt := time.Now().UTC()
	duration := finishedAt.Sub(span.startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	return span.client.enqueue(Event{
		EventType:    EventTypeSpan,
		RequestID:    span.request.RequestID,
		TraceID:      span.request.TraceID,
		SpanID:       span.spanID,
		ParentSpanID: span.parentSpanID,
		Name:         span.name,
		Kind:         span.kind,
		Status:       status,
		StartedAt:    formatTime(span.startedAt),
		FinishedAt:   formatTime(finishedAt),
		DurationMS:   &duration,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Metadata:     span.metadata,
		Data:         span.data,
	})
}
