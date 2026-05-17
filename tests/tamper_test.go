package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

func TestTamperEvidenceAddsHashChainMetadata(t *testing.T) {
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

	statePath := filepath.Join(t.TempDir(), "tamper-state.json")
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "tamper-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
		TamperEvidence: logcenter.TamperEvidenceConfig{
			Enabled:   true,
			ChainID:   "test-chain",
			Secret:    "test-secret",
			StatePath: statePath,
		},
	})
	defer client.Close(context.Background())

	client.SendEvent(context.Background(), logcenter.Event{
		IdempotencyKey: "evt-1",
		EventType:      logcenter.EventTypeLogEvent,
		Level:          logcenter.LevelInfo,
		Message:        "first",
	})
	client.SendEvent(context.Background(), logcenter.Event{
		IdempotencyKey: "evt-2",
		EventType:      logcenter.EventTypeLogEvent,
		Level:          logcenter.LevelInfo,
		Message:        "second",
	})
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(batch.Events))
	}
	first := batch.Events[0].Metadata["logcenter_integrity"].(map[string]any)
	second := batch.Events[1].Metadata["logcenter_integrity"].(map[string]any)
	if first["algorithm"] != "hmac-sha256" || first["chain_id"] != "test-chain" || first["sequence"].(float64) != 1 {
		t.Fatalf("first integrity = %#v, want hmac chain sequence 1", first)
	}
	if second["sequence"].(float64) != 2 || second["previous_hash"] != first["hash"] {
		t.Fatalf("second integrity = %#v, want sequence 2 linked to first hash %v", second, first["hash"])
	}
	if first["canonical_hash"] == "" || first["hash"] == "" || second["hash"] == "" {
		t.Fatalf("integrity hashes missing: %#v / %#v", first, second)
	}
}

func TestTamperEvidenceCanBeFilteredByEventType(t *testing.T) {
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
		Service:       "tamper-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
		TamperEvidence: logcenter.TamperEvidenceConfig{
			Enabled:    true,
			ChainID:    "audit-chain",
			EventTypes: []string{logcenter.EventTypeAuditEvent},
		},
	})
	defer client.Close(context.Background())

	client.Info(context.Background(), "not chained", nil)
	client.Audit(context.Background(), logcenter.AuditEvent{
		Action:     "order.updated",
		EntityType: "order",
		EntityID:   "order-123",
		Changes: []logcenter.Change{
			{Field: "status", OldValue: "pending", NewValue: "approved"},
		},
	})
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if _, ok := batch.Events[0].Metadata["logcenter_integrity"]; ok {
		t.Fatalf("log event metadata = %#v, want no integrity metadata", batch.Events[0].Metadata)
	}
	if _, ok := batch.Events[1].Metadata["logcenter_integrity"]; !ok {
		t.Fatalf("audit event metadata = %#v, want integrity metadata", batch.Events[1].Metadata)
	}
}
