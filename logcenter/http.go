package logcenter

import (
	"net/http"
	"strings"
)

type HTTPMiddlewareOption func(*httpMiddlewareConfig)

type httpMiddlewareConfig struct {
	routeTemplate     string
	routeTemplateFunc func(*http.Request) string
	userIDFunc        func(*http.Request) string
	tenantIDFunc      func(*http.Request) string
	metadataFunc      func(*http.Request) Fields
	dataFunc          func(*http.Request) Fields
}

func HTTPRouteTemplate(routeTemplate string) HTTPMiddlewareOption {
	return func(config *httpMiddlewareConfig) {
		config.routeTemplate = routeTemplate
	}
}

func HTTPRouteTemplateFunc(fn func(*http.Request) string) HTTPMiddlewareOption {
	return func(config *httpMiddlewareConfig) {
		config.routeTemplateFunc = fn
	}
}

func HTTPUserIDFunc(fn func(*http.Request) string) HTTPMiddlewareOption {
	return func(config *httpMiddlewareConfig) {
		config.userIDFunc = fn
	}
}

func HTTPTenantIDFunc(fn func(*http.Request) string) HTTPMiddlewareOption {
	return func(config *httpMiddlewareConfig) {
		config.tenantIDFunc = fn
	}
}

func HTTPMetadataFunc(fn func(*http.Request) Fields) HTTPMiddlewareOption {
	return func(config *httpMiddlewareConfig) {
		config.metadataFunc = fn
	}
}

func HTTPDataFunc(fn func(*http.Request) Fields) HTTPMiddlewareOption {
	return func(config *httpMiddlewareConfig) {
		config.dataFunc = fn
	}
}

func (client *Client) HTTPMiddleware(options ...HTTPMiddlewareOption) func(http.Handler) http.Handler {
	config := httpMiddlewareConfig{}
	for _, option := range options {
		option(&config)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := firstHeader(r, "X-LogCenter-Request-Id", "X-Request-Id")
			traceID, parentSpanID := traceFromHeaders(r)
			ctx, request := client.StartRequest(r.Context(), RequestStartOptions{
				RequestID:     requestID,
				TraceID:       traceID,
				SpanID:        parentSpanID,
				UserID:        config.userID(r),
				TenantID:      config.tenantID(r),
				Method:        r.Method,
				Path:          r.URL.Path,
				RouteTemplate: config.routeTemplateFor(r),
				Metadata: mergeFields(Fields{
					"remote_addr": r.RemoteAddr,
					"user_agent":  r.UserAgent(),
				}, config.metadata(r)),
				Data: config.data(r),
			})

			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r.WithContext(ctx))

			status := StatusSuccess
			if recorder.status >= 500 {
				status = StatusFailed
			}
			request.End(RequestEndOptions{
				Status:     status,
				HTTPStatus: &recorder.status,
			})
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(statusCode int) {
	recorder.status = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}

func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		value := strings.TrimSpace(r.Header.Get(name))
		if value != "" {
			return value
		}
	}
	return ""
}

func traceFromHeaders(r *http.Request) (string, string) {
	traceID := firstHeader(r, "X-LogCenter-Trace-Id", "X-Trace-Id")
	if traceID != "" {
		return traceID, ""
	}

	traceparent := firstHeader(r, "Traceparent")
	parts := strings.Split(traceparent, "-")
	if len(parts) == 4 && strings.EqualFold(parts[0], "00") {
		return parts[1], parts[2]
	}
	return "", ""
}

func (config httpMiddlewareConfig) routeTemplateFor(r *http.Request) string {
	if config.routeTemplateFunc != nil {
		if routeTemplate := strings.TrimSpace(config.routeTemplateFunc(r)); routeTemplate != "" {
			return routeTemplate
		}
	}
	if config.routeTemplate != "" {
		return config.routeTemplate
	}
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.URL.Path
}

func (config httpMiddlewareConfig) userID(r *http.Request) string {
	if config.userIDFunc == nil {
		return ""
	}
	return config.userIDFunc(r)
}

func (config httpMiddlewareConfig) tenantID(r *http.Request) string {
	if config.tenantIDFunc == nil {
		return ""
	}
	return config.tenantIDFunc(r)
}

func (config httpMiddlewareConfig) metadata(r *http.Request) Fields {
	if config.metadataFunc == nil {
		return nil
	}
	return config.metadataFunc(r)
}

func (config httpMiddlewareConfig) data(r *http.Request) Fields {
	if config.dataFunc == nil {
		return nil
	}
	return config.dataFunc(r)
}

func mergeFields(left, right Fields) Fields {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	merged := make(Fields, len(left)+len(right))
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		merged[key] = value
	}
	return merged
}
