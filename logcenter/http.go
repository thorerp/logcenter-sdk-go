package logcenter

import (
	"net/http"
	"strings"
	"time"
)

func (client *Client) HTTPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now().UTC()
			requestID := firstHeader(r, "X-LogCenter-Request-Id", "X-Request-Id")
			if requestID == "" {
				requestID = newID("req_")
			}
			traceID := firstHeader(r, "X-LogCenter-Trace-Id", "X-Trace-Id", "Traceparent")
			if traceID == "" || strings.HasPrefix(strings.ToLower(traceID), "00-") {
				traceID = newID("trc_")
			}
			operation := r.Method + " " + r.URL.Path
			requestContext := RequestContext{
				RequestID: requestID,
				TraceID:   traceID,
				Operation: operation,
			}
			ctx := ContextWithRequest(r.Context(), requestContext)

			client.enqueue(Event{
				EventType:     EventTypeRequestStarted,
				RequestID:     requestID,
				TraceID:       traceID,
				Operation:     operation,
				Method:        r.Method,
				Path:          r.URL.Path,
				RouteTemplate: r.URL.Path,
				StartedAt:     formatTime(startedAt),
				Metadata: Fields{
					"remote_addr": r.RemoteAddr,
					"user_agent":  r.UserAgent(),
				},
			})

			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r.WithContext(ctx))

			finishedAt := time.Now().UTC()
			duration := finishedAt.Sub(startedAt).Milliseconds()
			status := StatusSuccess
			if recorder.status >= 500 {
				status = StatusFailed
			}
			client.enqueue(Event{
				EventType:   EventTypeRequestFinished,
				RequestID:   requestID,
				TraceID:     traceID,
				Operation:   operation,
				Status:      status,
				HTTPStatus:  &recorder.status,
				FinishedAt:  formatTime(finishedAt),
				DurationMS:  &duration,
				Method:      r.Method,
				Path:        r.URL.Path,
				Metadata:    Fields{},
				OccurredAt:  formatTime(finishedAt),
				Environment: client.config.Environment,
				Service:     client.config.Service,
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
