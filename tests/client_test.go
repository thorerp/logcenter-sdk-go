package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

type capturedBatch struct {
	BatchID string            `json:"batch_id"`
	SentAt  string            `json:"sent_at"`
	Source  map[string]any    `json:"source"`
	Events  []logcenter.Event `json:"events"`
}

func TestFlushSendsBatchToServer(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Fatalf("Authorization = %q, want bearer key", got)
		}
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":1,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "production",
		Service:       "orders-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	client.Info(context.Background(), "created token=secret", logcenter.Fields{"api_key": "hidden", "safe": "visible"})
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(batch.Events))
	}
	event := batch.Events[0]
	if event.EventType != logcenter.EventTypeLogEvent || event.Level != logcenter.LevelInfo {
		t.Fatalf("event = %#v, want info log", event)
	}
	if event.Metadata["api_key"] != "[REDACTED]" {
		t.Fatalf("metadata api_key = %v, want redacted", event.Metadata["api_key"])
	}
	if event.Message == "created token=secret" {
		t.Fatal("message was not redacted")
	}

	stats := client.Stats()
	if stats.SentBatches != 1 || stats.Accepted != 1 {
		t.Fatalf("stats = %#v, want sent/accepted", stats)
	}
}

func TestFlushHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Timeout:       10 * time.Millisecond,
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	client.Warn(context.Background(), "slow endpoint", nil)
	err := client.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want timeout")
	}

	stats := client.Stats()
	if stats.FailedBatches != 1 || stats.FailedEvents != 1 {
		t.Fatalf("stats = %#v, want failed batch/event", stats)
	}
}

func TestFlushRequiresEndpoint(t *testing.T) {
	client := logcenter.NewClient(logcenter.Config{
		APIKey:        "test-api-key",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	client.Warn(context.Background(), "missing endpoint", nil)
	err := client.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want missing endpoint error")
	}
	if err.Error() != "logcenter endpoint is empty" {
		t.Fatalf("Flush() error = %q, want missing endpoint error", err.Error())
	}
	if stats := client.Stats(); stats.FailedBatches != 1 || stats.FailedEvents != 1 {
		t.Fatalf("stats = %#v, want failed batch/event", stats)
	}
}

func TestBufferFullDropsDebugButPreservesError(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":1,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		FlushInterval: time.Hour,
		BufferSize:    1,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	client.Debug(context.Background(), "debug", nil)
	client.RecordError(context.Background(), errors.New("boom"), logcenter.ErrorOptions{Code: "BOOM"})
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 1 || batch.Events[0].EventType != logcenter.EventTypeErrorEvent {
		t.Fatalf("events = %#v, want preserved error event", batch.Events)
	}
	if stats := client.Stats(); stats.Dropped != 1 {
		t.Fatalf("Dropped = %d, want 1", stats.Dropped)
	}
}

func TestSendEventCoversRawCollectionFields(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":1,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "production",
		Service:       "orders-worker",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	ctx := logcenter.ContextWithUser(context.Background(), "user-123")
	ctx = logcenter.ContextWithTenant(ctx, "tenant-123")
	ctx = logcenter.ContextWithOperation(ctx, "process order")
	client.SendEvent(ctx, logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelFatal,
		Message:   "worker stopped",
		Metadata:  logcenter.Fields{"safe": "visible"},
		Data:      logcenter.Fields{"payload_id": "payload-123", "api_key": "hidden"},
	})

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	event := batch.Events[0]
	if event.Level != logcenter.LevelFatal {
		t.Fatalf("level = %q, want fatal", event.Level)
	}
	if event.UserID != "user-123" || event.TenantID != "tenant-123" || event.Operation != "process order" {
		t.Fatalf("context fields were not applied: %#v", event)
	}
	if event.Data["payload_id"] != "payload-123" {
		t.Fatalf("data payload_id = %v, want payload-123", event.Data["payload_id"])
	}
	if event.Data["api_key"] != "[REDACTED]" {
		t.Fatalf("data api_key = %v, want redacted", event.Data["api_key"])
	}
}

func TestManualRequestLifecycleCollectsRouteAndIdentity(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":3,"accepted":3,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "production",
		Service:       "orders-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	ctx, request := client.StartRequest(context.Background(), logcenter.RequestStartOptions{
		RequestID:     "req_manual",
		TraceID:       "trc_manual",
		UserID:        "user-123",
		TenantID:      "tenant-123",
		Method:        http.MethodPost,
		Path:          "/orders/123",
		RouteTemplate: "/orders/{id}",
		Metadata:      logcenter.Fields{"request_meta": "visible"},
	})
	client.Info(ctx, "inside manual request", nil)
	status := http.StatusAccepted
	request.End(logcenter.RequestEndOptions{
		Status:     logcenter.StatusSuccess,
		HTTPStatus: &status,
		Metadata:   logcenter.Fields{"finish_meta": "visible"},
	})

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 3 {
		t.Fatalf("events = %d, want request start, log, request finish", len(batch.Events))
	}
	started := batch.Events[0]
	if started.EventType != logcenter.EventTypeRequestStarted || started.RouteTemplate != "/orders/{id}" {
		t.Fatalf("started event = %#v, want route template", started)
	}
	if started.UserID != "user-123" || started.TenantID != "tenant-123" {
		t.Fatalf("started identity = %#v, want user/tenant", started)
	}
	logEvent := batch.Events[1]
	if logEvent.UserID != "user-123" || logEvent.TenantID != "tenant-123" {
		t.Fatalf("log identity = %#v, want user/tenant from context", logEvent)
	}
	finished := batch.Events[2]
	if finished.EventType != logcenter.EventTypeRequestFinished || finished.RouteTemplate != "/orders/{id}" {
		t.Fatalf("finished event = %#v, want route template", finished)
	}
	if finished.HTTPStatus == nil || *finished.HTTPStatus != http.StatusAccepted {
		t.Fatalf("http status = %v, want 202", finished.HTTPStatus)
	}
}

func TestHTTPMiddlewareDoesNotWaitForRemoteInHandler(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":2,"accepted":2,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "http-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	handler := client.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := client.StartSpan(r.Context(), "work")
		client.Info(ctx, "inside handler", nil)
		span.End(logcenter.StatusSuccess)
		w.WriteHeader(http.StatusCreated)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/orders", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	batch := <-received
	if len(batch.Events) != 4 {
		t.Fatalf("events = %d, want request start, log, span, request finish", len(batch.Events))
	}
	if batch.Events[0].EventType != logcenter.EventTypeRequestStarted {
		t.Fatalf("first event = %s, want request_started", batch.Events[0].EventType)
	}
	if batch.Events[len(batch.Events)-1].EventType != logcenter.EventTypeRequestFinished {
		t.Fatalf("last event = %s, want request_finished", batch.Events[len(batch.Events)-1].EventType)
	}
}

func TestHTTPMiddlewareOptionsCollectRouteTemplateAndIdentity(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":2,"accepted":2,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "http-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	handler := client.HTTPMiddleware(
		logcenter.HTTPRouteTemplate("/orders/{id}"),
		logcenter.HTTPUserIDFunc(func(r *http.Request) string { return r.Header.Get("X-User-ID") }),
		logcenter.HTTPTenantIDFunc(func(r *http.Request) string { return r.Header.Get("X-Tenant-ID") }),
		logcenter.HTTPMetadataFunc(func(r *http.Request) logcenter.Fields {
			return logcenter.Fields{"request_class": "api"}
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPatch, "/orders/123", nil)
	req.Header.Set("X-User-ID", "user-123")
	req.Header.Set("X-Tenant-ID", "tenant-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	batch := <-received
	started := batch.Events[0]
	if started.RouteTemplate != "/orders/{id}" || started.UserID != "user-123" || started.TenantID != "tenant-123" {
		t.Fatalf("started event = %#v, want route/user/tenant", started)
	}
	if started.Metadata["request_class"] != "api" {
		t.Fatalf("metadata request_class = %v, want api", started.Metadata["request_class"])
	}
}

func TestHTTPMiddlewareFlushesFullInvestigableRequest(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":6,"accepted":6,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "production",
		Service:       "orders-api",
		FlushInterval: time.Hour,
		BufferSize:    20,
		BatchSize:     20,
	})
	defer client.Close(context.Background())

	handler := client.HTTPMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := client.StartSpan(r.Context(), "call_payment_provider")
		client.Info(ctx, "Order payload created", logcenter.Fields{"document_type": "order"})
		span.RecordError(errors.New("provider timeout"), "PROVIDER_TIMEOUT")
		span.End(logcenter.StatusFailed)
		client.Audit(ctx, logcenter.AuditEvent{
			ActorType:  "user",
			ActorID:    "user-456",
			TenantID:   "tenant-123",
			Action:     "order.rejected",
			EntityType: "order",
			EntityID:   "order-789",
			Changes: []logcenter.Change{
				{Field: "status", OldValue: "processing", NewValue: "rejected"},
			},
		})
		w.WriteHeader(http.StatusBadGateway)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/orders", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	seen := map[string]bool{}
	for _, event := range batch.Events {
		seen[event.EventType] = true
	}
	for _, eventType := range []string{
		logcenter.EventTypeRequestStarted,
		logcenter.EventTypeSpan,
		logcenter.EventTypeLogEvent,
		logcenter.EventTypeErrorEvent,
		logcenter.EventTypeAuditEvent,
		logcenter.EventTypeRequestFinished,
	} {
		if !seen[eventType] {
			t.Fatalf("event type %s was not sent in batch: %#v", eventType, batch.Events)
		}
	}
}
