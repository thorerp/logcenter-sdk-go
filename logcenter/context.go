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
