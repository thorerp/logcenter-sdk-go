package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

func TestDurableOutboxPersistsAndRemovesAcceptedEvents(t *testing.T) {
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

	outboxPath := filepath.Join(t.TempDir(), "outbox.jsonl")
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "outbox-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
		OutboxPath:    outboxPath,
	})
	defer client.Close(context.Background())

	client.Info(context.Background(), "persisted before send", nil)
	if _, err := os.Stat(outboxPath); err != nil {
		t.Fatalf("outbox file should exist before flush: %v", err)
	}
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	<-received
	if _, err := os.Stat(outboxPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outbox stat = %v, want removed after accepted send", err)
	}
}

func TestDurableOutboxRetriesPersistedEventsAfterFailure(t *testing.T) {
	received := make(chan capturedBatch, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`temporary failure`))
			return
		}
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":1,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	outboxPath := filepath.Join(t.TempDir(), "outbox.jsonl")
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "outbox-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
		OutboxPath:    outboxPath,
	})
	defer client.Close(context.Background())

	client.Info(context.Background(), "retry later", nil)
	if err := client.Flush(context.Background()); err == nil {
		t.Fatal("Flush() error = nil, want first send failure")
	}
	first := <-received
	if _, err := os.Stat(outboxPath); err != nil {
		t.Fatalf("outbox file should remain after failure: %v", err)
	}

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("second Flush() error = %v", err)
	}
	second := <-received
	if len(first.Events) != 1 || len(second.Events) != 1 || first.Events[0].EventID != second.Events[0].EventID {
		t.Fatalf("retry batches = %#v/%#v, want same event retried from outbox", first.Events, second.Events)
	}
	if _, err := os.Stat(outboxPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outbox stat = %v, want removed after retry succeeds", err)
	}
}
