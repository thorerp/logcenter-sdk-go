package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestRoundTripperRecordsClientSpanForSuccessfulRequest(t *testing.T) {
	received := make(chan capturedBatch, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":1,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer collector.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      collector.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "roundtrip-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	httpClient := &http.Client{
		Transport: client.RoundTripper(nil, logcenter.RoundTripperMetadataFunc(func(req *http.Request, resp *http.Response, err error) logcenter.Fields {
			return logcenter.Fields{"peer": "upstream"}
		})),
	}
	ctx := logcenter.ContextWithRequest(context.Background(), logcenter.RequestContext{
		RequestID: "req_roundtrip",
		TraceID:   "trc_roundtrip",
		Operation: "call-upstream",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL+"/work", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	_ = resp.Body.Close()

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	event := (<-received).Events[0]
	if event.EventType != logcenter.EventTypeSpan || event.Kind != "client" || event.Name != "HTTP GET" {
		t.Fatalf("event = %#v, want client span", event)
	}
	if event.RequestID != "req_roundtrip" || event.TraceID != "trc_roundtrip" || event.Operation != "call-upstream" {
		t.Fatalf("context = %#v, want propagated request context", event)
	}
	if event.HTTPStatus == nil || *event.HTTPStatus != http.StatusNoContent || event.Status != logcenter.StatusSuccess {
		t.Fatalf("status = %#v/%s, want 204 success", event.HTTPStatus, event.Status)
	}
	if event.Metadata["peer"] != "upstream" || event.Metadata["path"] != "/work" {
		t.Fatalf("metadata = %#v, want upstream metadata", event.Metadata)
	}
}

func TestRoundTripperRecordsErrorEventForFailedRequest(t *testing.T) {
	received := make(chan capturedBatch, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":2,"accepted":2,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer collector.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      collector.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "roundtrip-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("upstream unavailable")
	})
	httpClient := &http.Client{
		Transport: client.RoundTripper(base, logcenter.RoundTripperErrorCode("UPSTREAM_FAILED")),
	}
	target := (&url.URL{Scheme: "https", Host: "upstream.test", Path: "/work"}).String()
	req, err := http.NewRequest(http.MethodPost, target, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if _, err := httpClient.Do(req); err == nil {
		t.Fatal("Do() error = nil, want upstream error")
	}

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	var span *logcenter.Event
	var errorEvent *logcenter.Event
	for i := range batch.Events {
		event := &batch.Events[i]
		if event.EventType == logcenter.EventTypeSpan {
			span = event
		}
		if event.EventType == logcenter.EventTypeErrorEvent {
			errorEvent = event
		}
	}
	if span == nil || span.Status != logcenter.StatusFailed || span.ErrorCode != "UPSTREAM_FAILED" {
		t.Fatalf("span = %#v, want failed span with error code", span)
	}
	if errorEvent == nil || errorEvent.ErrorCode != "UPSTREAM_FAILED" || errorEvent.ErrorType != "http_client" {
		t.Fatalf("error event = %#v, want correlated http client error", errorEvent)
	}
	if span.SpanID == "" || errorEvent.SpanID != span.SpanID {
		t.Fatalf("span/error correlation = %#v/%#v, want same span id", span, errorEvent)
	}
}
