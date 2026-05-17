package logcenter

import (
	"context"
	"sync"
	"time"
)

type RequestStartOptions struct {
	RequestID     string
	TraceID       string
	SpanID        string
	UserID        string
	TenantID      string
	Operation     string
	Method        string
	Path          string
	RouteTemplate string
	StartedAt     time.Time
	Metadata      Fields
	Data          Fields
}

type RequestEndOptions struct {
	Status     string
	HTTPStatus *int
	FinishedAt time.Time
	Metadata   Fields
	Data       Fields
}

type Request struct {
	client        *Client
	context       RequestContext
	method        string
	path          string
	routeTemplate string
	startedAt     time.Time

	mu    sync.Mutex
	ended bool
}

func (client *Client) StartRequest(ctx context.Context, options RequestStartOptions) (context.Context, *Request) {
	if ctx == nil {
		ctx = context.Background()
	}
	current, _ := RequestFromContext(ctx)
	requestContext := RequestContext{
		RequestID: firstNonEmpty(options.RequestID, current.RequestID),
		TraceID:   firstNonEmpty(options.TraceID, current.TraceID),
		SpanID:    firstNonEmpty(options.SpanID, current.SpanID),
		Operation: firstNonEmpty(options.Operation, current.Operation),
		UserID:    firstNonEmpty(options.UserID, current.UserID),
		TenantID:  firstNonEmpty(options.TenantID, current.TenantID),
	}
	if requestContext.RequestID == "" {
		requestContext.RequestID = newID("req_")
	}
	if requestContext.TraceID == "" {
		requestContext.TraceID = newID("trc_")
	}
	if requestContext.Operation == "" && (options.Method != "" || options.Path != "") {
		requestContext.Operation = options.Method + " " + options.Path
	}

	startedAt := options.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if options.RouteTemplate == "" {
		options.RouteTemplate = options.Path
	}

	request := &Request{
		client:        client,
		context:       requestContext,
		method:        options.Method,
		path:          options.Path,
		routeTemplate: options.RouteTemplate,
		startedAt:     startedAt,
	}

	client.enqueue(Event{
		EventType:     EventTypeRequestStarted,
		RequestID:     requestContext.RequestID,
		TraceID:       requestContext.TraceID,
		SpanID:        requestContext.SpanID,
		UserID:        requestContext.UserID,
		TenantID:      requestContext.TenantID,
		Operation:     requestContext.Operation,
		Method:        options.Method,
		Path:          options.Path,
		RouteTemplate: options.RouteTemplate,
		StartedAt:     formatTime(startedAt),
		Metadata:      options.Metadata,
		Data:          options.Data,
	})

	return ContextWithRequest(ctx, requestContext), request
}

func (request *Request) End(options RequestEndOptions) bool {
	request.mu.Lock()
	if request.ended {
		request.mu.Unlock()
		return false
	}
	request.ended = true
	request.mu.Unlock()

	status := options.Status
	if status == "" {
		status = StatusSuccess
	}
	finishedAt := options.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	duration := finishedAt.Sub(request.startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	return request.client.enqueue(Event{
		EventType:     EventTypeRequestFinished,
		RequestID:     request.context.RequestID,
		TraceID:       request.context.TraceID,
		SpanID:        request.context.SpanID,
		UserID:        request.context.UserID,
		TenantID:      request.context.TenantID,
		Operation:     request.context.Operation,
		Status:        status,
		Method:        request.method,
		Path:          request.path,
		RouteTemplate: request.routeTemplate,
		HTTPStatus:    options.HTTPStatus,
		DurationMS:    &duration,
		FinishedAt:    formatTime(finishedAt),
		Metadata:      options.Metadata,
		Data:          options.Data,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
