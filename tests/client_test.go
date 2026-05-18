package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestFlushHonorsDedicatedSendTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Timeout:       time.Second,
		SendTimeout:   10 * time.Millisecond,
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	client.Warn(context.Background(), "slow endpoint", nil)
	err := client.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want send timeout")
	}
}

func TestFlushHonorsDedicatedFlushTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		SendTimeout:   time.Second,
		FlushTimeout:  10 * time.Millisecond,
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	client.Warn(context.Background(), "slow flush", nil)
	err := client.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want flush timeout")
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

func TestSendEventSyncSendsImmediatelyWithoutFlush(t *testing.T) {
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
		Environment:   "test",
		Service:       "sync-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	ctx := logcenter.ContextWithRequest(context.Background(), logcenter.RequestContext{
		RequestID: "req_sync",
		TraceID:   "trc_sync",
		Operation: "sync-event",
	})
	err := client.SendEventSync(ctx, logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelInfo,
		Message:   "sync event",
	})
	if err != nil {
		t.Fatalf("SendEventSync() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(batch.Events))
	}
	event := batch.Events[0]
	if event.RequestID != "req_sync" || event.TraceID != "trc_sync" || event.Operation != "sync-event" {
		t.Fatalf("event = %#v, want context fields", event)
	}
	if stats := client.Stats(); stats.Queued != 0 || stats.SentEvents != 1 || stats.Accepted != 1 {
		t.Fatalf("stats = %#v, want direct send without queue", stats)
	}
}

func TestAuditSyncSendsAuditEvent(t *testing.T) {
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
		Environment:   "test",
		Service:       "sync-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	ctx := logcenter.ContextWithTenant(context.Background(), "tenant-from-context")
	err := client.AuditSync(ctx, logcenter.AuditEvent{
		ActorType:  "user",
		ActorID:    "user-123",
		Action:     "order.approved",
		EntityType: "order",
		EntityID:   "order-123",
		Changes: []logcenter.Change{
			{Field: "status", OldValue: "pending", NewValue: "approved"},
		},
	})
	if err != nil {
		t.Fatalf("AuditSync() error = %v", err)
	}

	event := (<-received).Events[0]
	if event.EventType != logcenter.EventTypeAuditEvent || event.Action != "order.approved" || event.TenantID != "tenant-from-context" {
		t.Fatalf("event = %#v, want audit event with context tenant", event)
	}
}

func TestSendEventSyncReturnsAPIRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":0,"duplicated":0,"rejected":1,"errors":[{"index":0,"event_id":"evt_rejected","code":"INVALID","message":"invalid event"}]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "sync-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	err := client.SendEventSync(context.Background(), logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelInfo,
		Message:   "rejected remotely",
	})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("SendEventSync() error = %v, want rejection error", err)
	}
	if stats := client.Stats(); stats.Rejected != 1 || !strings.Contains(stats.LastError, "rejected") {
		t.Fatalf("stats = %#v, want rejected sync event", stats)
	}
}

func TestNoopClientDoesNotQueueFlushOrFail(t *testing.T) {
	client := logcenter.NewNoopClient()

	if client.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if client.Info(context.Background(), "ignored", nil) {
		t.Fatal("Info() = true, want false for noop client")
	}
	if client.SendEvent(context.Background(), logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelInfo,
		Message:   "ignored",
	}) {
		t.Fatal("SendEvent() = true, want false for noop client")
	}

	ctx, request := client.StartRequest(context.Background(), logcenter.RequestStartOptions{
		Operation: "noop operation",
	})
	if client.Warn(ctx, "ignored", nil) {
		t.Fatal("Warn() = true, want false for noop client")
	}
	if request.End(logcenter.RequestEndOptions{}) {
		t.Fatal("Request.End() = true, want false for noop client")
	}
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	if stats := client.Stats(); stats != (logcenter.Stats{}) {
		t.Fatalf("stats = %#v, want zero values", stats)
	}
}

func TestDisabledConfigDoesNotRequireEndpointOrAPIKey(t *testing.T) {
	client := logcenter.NewClient(logcenter.Config{
		Enabled: logcenter.Bool(false),
	})

	if client.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if client.Error(context.Background(), errors.New("ignored"), logcenter.ErrorOptions{Code: "IGNORED"}) {
		t.Fatal("Error() = true, want false for disabled client")
	}
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}
}

func TestInvalidEventIsRejectedBeforeQueue(t *testing.T) {
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      "<logcenter-endpoint>",
		APIKey:        "test-api-key",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	queued := client.SendEvent(context.Background(), logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelInfo,
	})
	if queued {
		t.Fatal("SendEvent() = true, want false for invalid event")
	}

	stats := client.Stats()
	if stats.Dropped != 1 {
		t.Fatalf("Dropped = %d, want 1", stats.Dropped)
	}
	if !strings.Contains(stats.LastError, "message") {
		t.Fatalf("LastError = %q, want message validation error", stats.LastError)
	}
}

func TestFailureHooksObserveDroppedEventsBatchFailuresAndErrorChanges(t *testing.T) {
	droppedEvents := make(chan logcenter.EventDrop, 1)
	batchFailures := make(chan logcenter.BatchFailure, 1)
	errorChanges := make(chan logcenter.ErrorChange, 2)

	client := logcenter.NewClient(logcenter.Config{
		APIKey:        "test-api-key",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
		Hooks: logcenter.Hooks{
			OnEventDropped: func(drop logcenter.EventDrop) {
				droppedEvents <- drop
			},
			OnBatchFailed: func(failure logcenter.BatchFailure) {
				batchFailures <- failure
			},
			OnErrorChanged: func(change logcenter.ErrorChange) {
				errorChanges <- change
			},
		},
	})
	defer client.Close(context.Background())

	if client.SendEvent(context.Background(), logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelInfo,
	}) {
		t.Fatal("SendEvent() = true, want invalid event dropped")
	}
	drop := <-droppedEvents
	if drop.Reason != "validation" || drop.Err == nil {
		t.Fatalf("drop = %#v, want validation error", drop)
	}
	change := <-errorChanges
	if !strings.Contains(change.LastError, "message") {
		t.Fatalf("error change = %#v, want message validation error", change)
	}

	client.Warn(context.Background(), "missing endpoint", nil)
	if err := client.Flush(context.Background()); err == nil {
		t.Fatal("Flush() error = nil, want missing endpoint")
	}
	failure := <-batchFailures
	if failure.EventCount != 1 || failure.Err == nil {
		t.Fatalf("batch failure = %#v, want failed event count", failure)
	}
}

func TestFailureHookObservesRejectedEventsFromAPI(t *testing.T) {
	rejectedEvents := make(chan logcenter.EventRejection, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":0,"duplicated":0,"rejected":1,"errors":[{"index":0,"event_id":"evt_rejected","code":"INVALID","message":"invalid event"}]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
		Hooks: logcenter.Hooks{
			OnEventRejected: func(rejection logcenter.EventRejection) {
				rejectedEvents <- rejection
			},
		},
	})
	defer client.Close(context.Background())

	client.Info(context.Background(), "will be rejected by API", nil)
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	rejection := <-rejectedEvents
	if rejection.Event.EventType != logcenter.EventTypeLogEvent || rejection.Error.Code != "INVALID" {
		t.Fatalf("rejection = %#v, want rejected log event", rejection)
	}
}

func TestCustomSensitiveKeyFragmentsRedactClientEvents(t *testing.T) {
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
		Endpoint:              server.URL,
		APIKey:                "test-api-key",
		Environment:           "test",
		Service:               "orders-api",
		FlushInterval:         time.Hour,
		BufferSize:            10,
		BatchSize:             10,
		SensitiveKeyFragments: []string{"customer_code"},
	})
	defer client.Close(context.Background())

	client.Info(context.Background(), "created customer_code=secret", logcenter.Fields{
		"customer_code": "hidden",
	})
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	event := (<-received).Events[0]
	if event.Metadata["customer_code"] != "[REDACTED]" {
		t.Fatalf("customer_code = %v, want redacted", event.Metadata["customer_code"])
	}
	if strings.Contains(event.Message, "secret") {
		t.Fatalf("message = %q, want custom fragment redacted", event.Message)
	}
}

func TestPayloadLimitsTruncateStringsMetadataDataAndAuditValues(t *testing.T) {
	received := make(chan capturedBatch, 1)
	truncated := make(chan logcenter.EventTruncation, 2)
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
		Endpoint:           server.URL,
		APIKey:             "test-api-key",
		Environment:        "test",
		Service:            "orders-api",
		FlushInterval:      time.Hour,
		BufferSize:         10,
		BatchSize:          10,
		MaxStringBytes:     32,
		MaxMetadataBytes:   80,
		MaxDataBytes:       80,
		MaxAuditValueBytes: 80,
		MaxEventBytes:      2048,
		Hooks: logcenter.Hooks{
			OnEventTruncated: func(truncation logcenter.EventTruncation) {
				truncated <- truncation
			},
		},
	})
	defer client.Close(context.Background())

	longValue := strings.Repeat("x", 200)
	client.SendEvent(context.Background(), logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelInfo,
		Message:   longValue,
		Metadata: logcenter.Fields{
			"long_metadata": longValue,
			"other":         longValue,
		},
		Data: logcenter.Fields{
			"long_data": longValue,
			"other":     longValue,
		},
	})
	if !client.Audit(context.Background(), logcenter.AuditEvent{
		Action:     "order.updated",
		EntityType: "order",
		EntityID:   "order-123",
		OldValue:   longValue,
		NewValue:   longValue,
	}) {
		t.Fatalf("Audit() = false, stats = %#v", client.Stats())
	}

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if stats := client.Stats(); stats.Truncated != 2 {
		t.Fatalf("Truncated = %d, want 2", stats.Truncated)
	}
	for i := 0; i < 2; i++ {
		select {
		case event := <-truncated:
			if event.Reason != "payload_limit" {
				t.Fatalf("truncation reason = %q, want payload_limit", event.Reason)
			}
		default:
			t.Fatalf("missing truncation hook %d", i+1)
		}
	}

	batch := <-received
	logEvent := batch.Events[0]
	if len(logEvent.Message) > 32 || !strings.Contains(logEvent.Message, "[TRUNCATED]") {
		t.Fatalf("message = %q, want truncated to max string bytes", logEvent.Message)
	}
	if logEvent.Metadata["_truncated"] != true {
		t.Fatalf("metadata = %#v, want truncated placeholder", logEvent.Metadata)
	}
	if logEvent.Data["_truncated"] != true {
		t.Fatalf("data = %#v, want truncated placeholder", logEvent.Data)
	}

	auditEvent := batch.Events[1]
	if oldValue, ok := auditEvent.OldValue.(string); !ok || len(oldValue) > 32 || !strings.Contains(oldValue, "[TRUNCATED]") {
		t.Fatalf("old value = %#v, want truncated string", auditEvent.OldValue)
	}
}

func TestPayloadLimitsRejectEventThatStillExceedsMaxEventBytes(t *testing.T) {
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:         "<logcenter-endpoint>",
		APIKey:           "test-api-key",
		FlushInterval:    time.Hour,
		BufferSize:       10,
		BatchSize:        10,
		MaxStringBytes:   64,
		MaxEventBytes:    40,
		MaxMetadataBytes: 64,
		MaxDataBytes:     64,
	})
	defer client.Close(context.Background())

	queued := client.Info(context.Background(), "event too large", nil)
	if queued {
		t.Fatal("Info() = true, want false for event above max_event_bytes")
	}
	stats := client.Stats()
	if stats.Dropped != 1 {
		t.Fatalf("Dropped = %d, want 1", stats.Dropped)
	}
	if !strings.Contains(stats.LastError, "max_event_bytes") {
		t.Fatalf("LastError = %q, want max_event_bytes", stats.LastError)
	}
}

func TestPayloadLimitsAllowLargeFiscalProviderExchangeData(t *testing.T) {
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

	requestPayload := strings.Repeat("A", 70*1024)
	responsePayload := strings.Repeat("B", 80*1024)
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:           server.URL,
		APIKey:             "test-api-key",
		Environment:        "development",
		Service:            "fiscalpro-api",
		FlushInterval:      time.Hour,
		BufferSize:         10,
		BatchSize:          10,
		MaxDataBytes:       512 * 1024,
		MaxMetadataBytes:   256 * 1024,
		MaxAuditValueBytes: 256 * 1024,
		MaxEventBytes:      1024 * 1024,
		MaxBatchBytes:      2 * 1024 * 1024,
	})
	defer client.Close(context.Background())

	if !client.SendEvent(context.Background(), logcenter.Event{
		EventType: logcenter.EventTypeFiscalProviderExchange,
		Operation: "ACBr NFe autorizar",
		Data: logcenter.Fields{
			"provider_request_payload_b64":  requestPayload,
			"provider_response_payload_b64": responsePayload,
		},
	}) {
		t.Fatalf("SendEvent() = false, stats = %#v", client.Stats())
	}
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(batch.Events))
	}
	event := batch.Events[0]
	if event.EventType != logcenter.EventTypeFiscalProviderExchange {
		t.Fatalf("event type = %q", event.EventType)
	}
	if event.Data["provider_request_payload_b64"] != requestPayload || event.Data["provider_response_payload_b64"] != responsePayload {
		t.Fatal("provider payloads were not preserved")
	}
	if stats := client.Stats(); stats.Truncated != 0 || stats.Dropped != 0 {
		t.Fatalf("stats = %#v, want no truncation/drop", stats)
	}
}

func TestPayloadLimitsRejectBatchAboveMaxBatchBytes(t *testing.T) {
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:           "<logcenter-endpoint>",
		APIKey:             "test-api-key",
		FlushInterval:      time.Hour,
		BufferSize:         10,
		BatchSize:          10,
		MaxDataBytes:       4096,
		MaxMetadataBytes:   4096,
		MaxAuditValueBytes: 4096,
		MaxEventBytes:      4096,
		MaxBatchBytes:      128,
	})
	defer client.Close(context.Background())

	if !client.SendEvent(context.Background(), logcenter.Event{
		EventType: logcenter.EventTypeLogEvent,
		Level:     logcenter.LevelInfo,
		Message:   strings.Repeat("x", 256),
	}) {
		t.Fatalf("SendEvent() = false, stats = %#v", client.Stats())
	}
	err := client.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want max_batch_bytes error")
	}
	if !strings.Contains(err.Error(), "max_batch_bytes") ||
		!strings.Contains(err.Error(), "limit_bytes=128") ||
		!strings.Contains(err.Error(), "actual_bytes=") {
		t.Fatalf("error = %q, want clear batch size details", err.Error())
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

func TestSendEventMapsIdempotencyKeyAndCollectionHints(t *testing.T) {
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

	client.SendEvent(context.Background(), logcenter.Event{
		IdempotencyKey: "order-123:approved:v1",
		EventType:      logcenter.EventTypeLogEvent,
		Classification: logcenter.ClassificationCritical,
		RetentionHint:  logcenter.RetentionHintLong,
		Level:          logcenter.LevelInfo,
		Message:        "order approved",
	})
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	event := (<-received).Events[0]
	if event.EventID != "order-123:approved:v1" || event.IdempotencyKey != "order-123:approved:v1" {
		t.Fatalf("idempotency fields = %q/%q, want key mirrored to event_id", event.EventID, event.IdempotencyKey)
	}
	if event.Classification != logcenter.ClassificationCritical || event.RetentionHint != logcenter.RetentionHintLong {
		t.Fatalf("hints = %q/%q, want critical/long", event.Classification, event.RetentionHint)
	}
}

func TestAuditAndOperationEventsCarryCollectionHints(t *testing.T) {
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
		Environment:   "production",
		Service:       "orders-worker",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	client.Audit(context.Background(), logcenter.AuditEvent{
		IdempotencyKey: "audit-1",
		Classification: logcenter.ClassificationAudit,
		RetentionHint:  logcenter.RetentionHintAudit,
		Action:         "order.updated",
		EntityType:     "order",
		EntityID:       "order-123",
		Changes: []logcenter.Change{
			{Field: "status", OldValue: "pending", NewValue: "approved"},
		},
	})
	client.OperationEvent(context.Background(), logcenter.OperationEvent{
		IdempotencyKey: "step-1",
		Classification: logcenter.ClassificationOperational,
		RetentionHint:  logcenter.RetentionHintShort,
		Action:         "order.step",
		Description:    "step completed",
	})
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	auditEvent := batch.Events[0]
	stepEvent := batch.Events[1]
	if auditEvent.EventID != "audit-1" || auditEvent.Classification != logcenter.ClassificationAudit || auditEvent.RetentionHint != logcenter.RetentionHintAudit {
		t.Fatalf("audit event = %#v, want audit hints", auditEvent)
	}
	if stepEvent.EventID != "step-1" || stepEvent.Classification != logcenter.ClassificationOperational || stepEvent.RetentionHint != logcenter.RetentionHintShort {
		t.Fatalf("step event = %#v, want operation hints", stepEvent)
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

func TestHTTPMiddlewareCapturesAllowedRequestBodyAndRestoresBody(t *testing.T) {
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
		logcenter.HTTPRequestBodyCaptureFunc(func(r *http.Request) bool {
			return r.URL.Path == "/capture"
		}, 1024, "application/json"),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read restored body: %v", err)
		}
		if !strings.Contains(string(body), "secret") {
			t.Fatalf("handler body = %q, want original body restored", body)
		}
		client.Info(r.Context(), "body consumed", nil)
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/capture", strings.NewReader(`{"name":"visible","api_key":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	started := (<-received).Events[0]
	body := started.Data["request_body"].(map[string]any)
	value := body["value"].(map[string]any)
	if value["name"] != "visible" {
		t.Fatalf("captured body = %#v, want visible name", value)
	}
	if value["api_key"] != "[REDACTED]" {
		t.Fatalf("api_key = %v, want redacted", value["api_key"])
	}
	if body["truncated"] != false || body["encoding"] != "json" {
		t.Fatalf("request_body = %#v, want non-truncated json", body)
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
