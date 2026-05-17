package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

func TestGenericOperationLifecycleAndSteps(t *testing.T) {
	received := make(chan capturedBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":4,"accepted":4,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "worker-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	ctx, operation := client.StartOperation(context.Background(), "process-order", logcenter.OperationStartOptions{
		RequestID: "req_operation",
		TraceID:   "trc_operation",
		Kind:      "job",
		Metadata:  logcenter.Fields{"queue": "orders"},
	})
	ctx = logcenter.ContextWithUser(ctx, "user-123")
	ctx = logcenter.ContextWithTenant(ctx, "tenant-123")

	client.Info(ctx, "operation running", nil)
	operation.StepWithContext(ctx, logcenter.OperationEvent{
		Action:      "order.validated",
		EntityType:  "order",
		EntityID:    "order-123",
		Description: "order validated",
		Status:      logcenter.StatusSuccess,
		Metadata:    logcenter.Fields{"step": "validate"},
		Data:        logcenter.Fields{"attempt": 1},
	})
	operation.EndWithContext(ctx, logcenter.OperationEndOptions{
		Status:   logcenter.StatusSuccess,
		Metadata: logcenter.Fields{"result": "processed"},
	})

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 4 {
		t.Fatalf("events = %d, want start, log, step, finish", len(batch.Events))
	}
	started := batch.Events[0]
	if started.EventType != logcenter.EventTypeRequestStarted || started.Operation != "process-order" || started.Kind != "job" {
		t.Fatalf("started = %#v, want generic operation start", started)
	}
	if started.Metadata["queue"] != "orders" {
		t.Fatalf("started metadata = %#v, want queue", started.Metadata)
	}

	step := batch.Events[2]
	if step.EventType != logcenter.EventTypeLogEvent || step.Kind != "operation_event" {
		t.Fatalf("step = %#v, want operation log event", step)
	}
	if step.Message != "order validated" || step.Action != "order.validated" || step.EntityType != "order" || step.EntityID != "order-123" {
		t.Fatalf("step fields = %#v, want action/entity/description", step)
	}
	if step.UserID != "user-123" || step.TenantID != "tenant-123" {
		t.Fatalf("step identity = %#v, want enriched user/tenant", step)
	}
	if step.Status != logcenter.StatusSuccess || step.Data["attempt"].(float64) != 1 {
		t.Fatalf("step status/data = %#v, want status and data", step)
	}

	finished := batch.Events[3]
	if finished.EventType != logcenter.EventTypeRequestFinished || finished.Status != logcenter.StatusSuccess {
		t.Fatalf("finished = %#v, want operation finish", finished)
	}
	if finished.UserID != "user-123" || finished.TenantID != "tenant-123" {
		t.Fatalf("finished identity = %#v, want enriched user/tenant", finished)
	}
}

func TestOperationEventUsesContextWithoutOperationHandle(t *testing.T) {
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
		Service:       "worker-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	ctx := logcenter.ContextWithRequest(context.Background(), logcenter.RequestContext{
		RequestID: "req_existing",
		TraceID:   "trc_existing",
		Operation: "existing-operation",
	})
	client.OperationEvent(ctx, logcenter.OperationEvent{
		Action:      "order.queued",
		EntityType:  "order",
		EntityID:    "order-123",
		Description: "order queued",
	})

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	event := (<-received).Events[0]
	if event.RequestID != "req_existing" || event.TraceID != "trc_existing" || event.Operation != "existing-operation" {
		t.Fatalf("event context = %#v, want existing context", event)
	}
	if event.Action != "order.queued" || event.Message != "order queued" {
		t.Fatalf("event = %#v, want operation event fields", event)
	}
}
