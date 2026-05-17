package logcenter

import "context"

type contextKey string

const requestContextKey contextKey = "logcenter_request_context"

type RequestContext struct {
	RequestID string
	TraceID   string
	SpanID    string
	Operation string
	UserID    string
	TenantID  string
}

func ContextWithRequest(ctx context.Context, values RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey, values)
}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	values, _ := RequestFromContext(ctx)
	values.RequestID = requestID
	return ContextWithRequest(ctx, values)
}

func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	values, _ := RequestFromContext(ctx)
	values.TraceID = traceID
	return ContextWithRequest(ctx, values)
}

func ContextWithSpanID(ctx context.Context, spanID string) context.Context {
	values, _ := RequestFromContext(ctx)
	values.SpanID = spanID
	return ContextWithRequest(ctx, values)
}

func ContextWithOperation(ctx context.Context, operation string) context.Context {
	values, _ := RequestFromContext(ctx)
	values.Operation = operation
	return ContextWithRequest(ctx, values)
}

func ContextWithUser(ctx context.Context, userID string) context.Context {
	values, _ := RequestFromContext(ctx)
	values.UserID = userID
	return ContextWithRequest(ctx, values)
}

func ContextWithTenant(ctx context.Context, tenantID string) context.Context {
	values, _ := RequestFromContext(ctx)
	values.TenantID = tenantID
	return ContextWithRequest(ctx, values)
}

func RequestFromContext(ctx context.Context) (RequestContext, bool) {
	values, ok := ctx.Value(requestContextKey).(RequestContext)
	return values, ok
}

func ensureRequestContext(ctx context.Context) (context.Context, RequestContext) {
	values, _ := RequestFromContext(ctx)
	if values.RequestID == "" {
		values.RequestID = newID("req_")
	}
	if values.TraceID == "" {
		values.TraceID = newID("trc_")
	}
	return ContextWithRequest(ctx, values), values
}
