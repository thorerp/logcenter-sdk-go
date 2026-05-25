package logcenter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewExternalProviderExchangeEventBuildsCanonicalStructuredPayload(t *testing.T) {
	startedAt := time.Date(2026, 5, 25, 10, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	finishedAt := startedAt.Add(250 * time.Millisecond)

	event := NewExternalProviderExchangeEvent(ExternalProviderExchange{
		Provider:             "acbr",
		ProviderEnv:          "homologacao",
		Method:               http.MethodPost,
		Endpoint:             "/nfe",
		URL:                  "https://hom.acbr.example/nfe",
		Accept:               "application/json",
		ContentType:          "application/json",
		Attempt:              2,
		StatusCode:           http.StatusOK,
		Duration:             250 * time.Millisecond,
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
		RequestPayloadBytes:  []byte(`{"id":123,"token":"secret","items":[{"sku":"ABC"}]}`),
		ResponsePayloadBytes: []byte(`[{"status":"autorizado"}]`),
		Metadata:             Fields{"custom_metadata": "keep"},
		Data:                 Fields{"custom_data": true},
	})

	if event.EventType != EventTypeExternalProviderExchange ||
		event.Operation != EventTypeExternalProviderExchange ||
		event.Classification != ClassificationAudit ||
		event.RetentionHint != RetentionHintAudit ||
		event.Status != StatusSuccess {
		t.Fatalf("event canonico inesperado: %#v", event)
	}
	if event.HTTPStatus == nil || *event.HTTPStatus != http.StatusOK {
		t.Fatalf("http status = %#v", event.HTTPStatus)
	}
	if event.DurationMS == nil || *event.DurationMS != 250 {
		t.Fatalf("duration ms = %#v", event.DurationMS)
	}
	if event.Metadata["data"] != nil {
		t.Fatalf("metadata nao deve duplicar data: %#v", event.Metadata["data"])
	}
	if event.Metadata["event_type"] != EventTypeExternalProviderExchange ||
		event.Metadata["operation_kind"] != EventTypeExternalProviderExchange ||
		event.Metadata["provider"] != "acbr" ||
		event.Metadata["provider_success"] != true ||
		event.Metadata["provider_request_payload_size_bytes"] != len(`{"id":123,"token":"secret","items":[{"sku":"ABC"}]}`) ||
		event.Metadata["custom_metadata"] != "keep" {
		t.Fatalf("metadata inesperada: %#v", event.Metadata)
	}

	requestPayload, ok := event.Data["provider_request_payload_json"].(map[string]any)
	if !ok {
		t.Fatalf("request payload json = %#v", event.Data["provider_request_payload_json"])
	}
	if requestPayload["id"].(json.Number).String() != "123" ||
		requestPayload["token"] != "secret" ||
		event.Data["custom_data"] != true {
		t.Fatalf("request payload inesperado: %#v", requestPayload)
	}
	responsePayload, ok := event.Data["provider_response_payload_json"].([]any)
	if !ok || len(responsePayload) != 1 {
		t.Fatalf("response payload json = %#v", event.Data["provider_response_payload_json"])
	}
}

func TestNewExternalProviderExchangeEventStoresNonJSONPayloadAsBase64AndOmittedReason(t *testing.T) {
	requestPayload := []byte("plain text payload")

	event := NewExternalProviderExchangeEvent(ExternalProviderExchange{
		Provider:              "migrate",
		Method:                http.MethodPost,
		Endpoint:              "/nfce",
		RequestPayloadBytes:   requestPayload,
		ResponseOmittedReason: "too_large",
	})

	if event.Data["provider_request_payload_b64"] != base64.StdEncoding.EncodeToString(requestPayload) ||
		event.Data["provider_request_payload_encoding"] != "base64" ||
		event.Data["provider_response_payload_omitted_reason"] != "too_large" {
		t.Fatalf("data inesperado: %#v", event.Data)
	}
	if event.Metadata["provider_request_payload_size_bytes"] != len(requestPayload) ||
		event.Metadata["provider_response_payload_omitted_reason"] != "too_large" {
		t.Fatalf("metadata inesperada: %#v", event.Metadata)
	}
}

func TestExternalProviderExchangeRedactsStructuredPayloadOnPrepare(t *testing.T) {
	client := NewClient(Config{Enabled: Bool(false)})
	event := NewExternalProviderExchangeEvent(ExternalProviderExchange{
		Provider:            "acbr",
		RequestPayloadBytes: []byte(`{"token":"secret","nested":{"cpf":"41004110812"},"normal":"ok"}`),
	})

	prepared, err := client.prepareEvent(event)
	if err != nil {
		t.Fatalf("prepareEvent() error = %v", err)
	}

	requestPayload, ok := prepared.Data["provider_request_payload_json"].(Fields)
	if !ok {
		t.Fatalf("request payload json = %#v", prepared.Data["provider_request_payload_json"])
	}
	nested, ok := requestPayload["nested"].(Fields)
	if !ok {
		t.Fatalf("nested = %#v", requestPayload["nested"])
	}
	if requestPayload["token"] != redactedValue ||
		nested["cpf"] != redactedValue ||
		requestPayload["normal"] != "ok" {
		t.Fatalf("payload redigido inesperado: %#v", requestPayload)
	}
}

func TestExternalProviderExchangeUsesDataLimiterOnPrepare(t *testing.T) {
	client := NewClient(Config{
		Enabled:      Bool(false),
		MaxDataBytes: 96,
	})
	event := NewExternalProviderExchangeEvent(ExternalProviderExchange{
		Provider:            "acbr",
		RequestPayloadBytes: []byte(`{"description":"` + strings.Repeat("A", 500) + `"}`),
	})

	prepared, err := client.prepareEvent(event)
	if err != nil {
		t.Fatalf("prepareEvent() error = %v", err)
	}
	if prepared.Data["_truncated"] != true ||
		prepared.Data["_truncated_field"] != "data" {
		t.Fatalf("data deveria ser limitada: %#v", prepared.Data)
	}
}

func TestExternalProviderExchangeSyncPropagatesContextFields(t *testing.T) {
	received := make(chan Event, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch struct {
			Events []Event `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch.Events[0]
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":1,"accepted":1,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		Endpoint:      server.URL,
		APIKey:        "test-api-key",
		FlushInterval: time.Hour,
	})
	defer client.Close(context.Background())

	ctx := ContextWithRequest(context.Background(), RequestContext{
		RequestID: "req-1",
		TraceID:   "trace-1",
		SpanID:    "span-1",
		UserID:    "user-1",
		TenantID:  "tenant-1",
		Operation: "POST /nfe/transmitir",
	})
	err := client.ExternalProviderExchangeSync(ctx, ExternalProviderExchange{
		Provider:            "acbr",
		RequestPayloadBytes: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("ExternalProviderExchangeSync() error = %v", err)
	}

	event := <-received
	if event.RequestID != "req-1" ||
		event.TraceID != "trace-1" ||
		event.SpanID != "span-1" ||
		event.UserID != "user-1" ||
		event.TenantID != "tenant-1" {
		t.Fatalf("contexto nao propagado: %#v", event)
	}
	if event.Operation != EventTypeExternalProviderExchange {
		t.Fatalf("operation = %q", event.Operation)
	}
}
