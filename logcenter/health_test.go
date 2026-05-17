package logcenter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthReportsOKDegradedAndDisabled(t *testing.T) {
	client := NewClient(Config{
		Endpoint:      "<logcenter-endpoint>",
		APIKey:        "test-api-key",
		Service:       "health-api",
		Environment:   "test",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	report := client.Health()
	if report.Status != HealthStatusOK || !report.Enabled {
		t.Fatalf("report = %#v, want enabled ok", report)
	}
	if report.Service != "health-api" || report.Environment != "test" {
		t.Fatalf("service/environment = %q/%q", report.Service, report.Environment)
	}

	client.SendEvent(context.Background(), Event{
		EventType: EventTypeLogEvent,
		Level:     LevelInfo,
	})
	report = client.Health()
	if report.Status != HealthStatusDegraded || report.Stats.Dropped != 1 {
		t.Fatalf("report = %#v, want degraded after dropped invalid event", report)
	}

	disabled := NewNoopClient()
	report = disabled.Health()
	if report.Status != HealthStatusDisabled || report.Enabled {
		t.Fatalf("disabled report = %#v, want disabled", report)
	}
}

func TestHealthHandlerWritesJSONAndOptionalStatusCode(t *testing.T) {
	client := NewNoopClient()
	handler := client.HealthHandler(HealthHandlerOptions{
		DisabledStatusCode: http.StatusServiceUnavailable,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content-type = %q, want application/json", rec.Header().Get("Content-Type"))
	}

	var report HealthReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != HealthStatusDisabled || report.SDKVersion == "" || report.Runtime == "" {
		t.Fatalf("report = %#v, want disabled report with version/runtime", report)
	}
}
