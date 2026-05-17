package logcenter

import (
	"context"
	"strings"
	"sync"
	"time"
)

type OperationStartOptions struct {
	RequestID string
	TraceID   string
	SpanID    string
	UserID    string
	TenantID  string
	Kind      string
	StartedAt time.Time
	Metadata  Fields
	Data      Fields
}

type OperationEndOptions struct {
	Status     string
	FinishedAt time.Time
	Metadata   Fields
	Data       Fields
}

type OperationEvent struct {
	IdempotencyKey string
	Classification string
	RetentionHint  string
	Action         string
	EntityType     string
	EntityID       string
	Description    string
	Status         string
	Level          string
	Kind           string
	Metadata       Fields
	Data           Fields
}

type Operation struct {
	client    *Client
	request   RequestContext
	name      string
	kind      string
	startedAt time.Time

	mu    sync.Mutex
	ended bool
}

func (client *Client) StartOperation(ctx context.Context, name string, options ...OperationStartOptions) (context.Context, *Operation) {
	if ctx == nil {
		ctx = context.Background()
	}
	startOptions := OperationStartOptions{}
	if len(options) > 0 {
		startOptions = options[0]
	}

	ctx, requestContext := ensureRequestContext(ctx)
	requestContext.RequestID = firstNonEmpty(startOptions.RequestID, requestContext.RequestID)
	requestContext.TraceID = firstNonEmpty(startOptions.TraceID, requestContext.TraceID)
	requestContext.SpanID = firstNonEmpty(startOptions.SpanID, requestContext.SpanID)
	requestContext.UserID = firstNonEmpty(startOptions.UserID, requestContext.UserID)
	requestContext.TenantID = firstNonEmpty(startOptions.TenantID, requestContext.TenantID)

	operationName := strings.TrimSpace(name)
	if operationName == "" {
		operationName = strings.TrimSpace(requestContext.Operation)
	}
	if operationName == "" {
		operationName = "operation"
	}
	requestContext.Operation = operationName

	kind := strings.TrimSpace(startOptions.Kind)
	if kind == "" {
		kind = "operation"
	}
	startedAt := startOptions.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	operation := &Operation{
		client:    client,
		request:   requestContext,
		name:      operationName,
		kind:      kind,
		startedAt: startedAt,
	}

	client.enqueue(Event{
		EventType:     EventTypeRequestStarted,
		RequestID:     requestContext.RequestID,
		TraceID:       requestContext.TraceID,
		SpanID:        requestContext.SpanID,
		UserID:        requestContext.UserID,
		TenantID:      requestContext.TenantID,
		Operation:     requestContext.Operation,
		Kind:          kind,
		Method:        kind,
		Path:          operationName,
		RouteTemplate: operationName,
		StartedAt:     formatTime(startedAt),
		Metadata:      startOptions.Metadata,
		Data:          startOptions.Data,
	})

	return ContextWithRequest(ctx, requestContext), operation
}

func (operation *Operation) End(options ...OperationEndOptions) bool {
	return operation.EndWithContext(context.Background(), options...)
}

func (operation *Operation) EndWithContext(ctx context.Context, options ...OperationEndOptions) bool {
	endOptions := OperationEndOptions{}
	if len(options) > 0 {
		endOptions = options[0]
	}

	operation.mu.Lock()
	if operation.ended {
		operation.mu.Unlock()
		return false
	}
	operation.ended = true
	requestContext := operation.request
	operation.mu.Unlock()

	requestContext = mergeRequestContext(requestContext, ctx)

	status := endOptions.Status
	if status == "" {
		status = StatusSuccess
	}
	finishedAt := endOptions.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	duration := finishedAt.Sub(operation.startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	return operation.client.enqueue(Event{
		EventType:     EventTypeRequestFinished,
		RequestID:     requestContext.RequestID,
		TraceID:       requestContext.TraceID,
		SpanID:        requestContext.SpanID,
		UserID:        requestContext.UserID,
		TenantID:      requestContext.TenantID,
		Operation:     requestContext.Operation,
		Kind:          operation.kind,
		Status:        status,
		Method:        operation.kind,
		Path:          operation.name,
		RouteTemplate: operation.name,
		FinishedAt:    formatTime(finishedAt),
		DurationMS:    &duration,
		Metadata:      endOptions.Metadata,
		Data:          endOptions.Data,
	})
}

func (operation *Operation) Event(event OperationEvent) bool {
	return operation.EventWithContext(context.Background(), event)
}

func (operation *Operation) EventWithContext(ctx context.Context, event OperationEvent) bool {
	requestContext := mergeRequestContext(operation.request, ctx)
	return operation.client.enqueue(operationEventToLogEvent(requestContext, event))
}

func (operation *Operation) Step(event OperationEvent) bool {
	return operation.Event(event)
}

func (operation *Operation) StepWithContext(ctx context.Context, event OperationEvent) bool {
	return operation.EventWithContext(ctx, event)
}

func (client *Client) OperationEvent(ctx context.Context, event OperationEvent) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	requestContext, _ := RequestFromContext(ctx)
	return client.enqueue(operationEventToLogEvent(requestContext, event))
}

func operationEventToLogEvent(requestContext RequestContext, event OperationEvent) Event {
	level := event.Level
	if level == "" {
		level = LevelInfo
	}
	message := strings.TrimSpace(event.Description)
	if message == "" {
		message = strings.TrimSpace(event.Action)
	}
	if message == "" {
		message = "operation event"
	}
	kind := strings.TrimSpace(event.Kind)
	if kind == "" {
		kind = "operation_event"
	}

	return Event{
		IdempotencyKey: event.IdempotencyKey,
		EventType:      EventTypeLogEvent,
		Classification: event.Classification,
		RetentionHint:  event.RetentionHint,
		RequestID:      requestContext.RequestID,
		TraceID:        requestContext.TraceID,
		SpanID:         requestContext.SpanID,
		UserID:         requestContext.UserID,
		TenantID:       requestContext.TenantID,
		Operation:      requestContext.Operation,
		Kind:           kind,
		Level:          level,
		Message:        message,
		Status:         event.Status,
		Action:         event.Action,
		EntityType:     event.EntityType,
		EntityID:       event.EntityID,
		Metadata:       event.Metadata,
		Data:           event.Data,
	}
}

func mergeRequestContext(base RequestContext, ctx context.Context) RequestContext {
	if ctx == nil {
		return base
	}
	contextValues, ok := RequestFromContext(ctx)
	if !ok {
		return base
	}
	if contextValues.RequestID != "" {
		base.RequestID = contextValues.RequestID
	}
	if contextValues.TraceID != "" {
		base.TraceID = contextValues.TraceID
	}
	if contextValues.SpanID != "" {
		base.SpanID = contextValues.SpanID
	}
	if contextValues.UserID != "" {
		base.UserID = contextValues.UserID
	}
	if contextValues.TenantID != "" {
		base.TenantID = contextValues.TenantID
	}
	if contextValues.Operation != "" {
		base.Operation = contextValues.Operation
	}
	return base
}
