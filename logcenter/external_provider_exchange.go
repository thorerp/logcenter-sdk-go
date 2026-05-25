package logcenter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type ExternalProviderExchange struct {
	IdempotencyKey string
	Classification string
	RetentionHint  string
	RequestID      string
	TraceID        string
	SpanID         string
	UserID         string
	TenantID       string
	Operation      string
	Provider       string
	ProviderEnv    any
	Method         string
	Endpoint       string
	URL            string
	Accept         string
	ContentType    string
	Attempt        int
	Status         string
	StatusCode     int
	Duration       time.Duration
	StartedAt      time.Time
	FinishedAt     time.Time
	ErrorCode      string
	ErrorMessage   string

	RequestPayloadBytes   []byte
	ResponsePayloadBytes  []byte
	RequestOmittedReason  string
	ResponseOmittedReason string
	Metadata              Fields
	Data                  Fields
}

func (client *Client) ExternalProviderExchange(ctx context.Context, exchange ExternalProviderExchange) bool {
	return client.SendEvent(ctx, NewExternalProviderExchangeEvent(exchange))
}

func (client *Client) ExternalProviderExchangeSync(ctx context.Context, exchange ExternalProviderExchange) error {
	return client.SendEventSync(ctx, NewExternalProviderExchangeEvent(exchange))
}

func NewExternalProviderExchangeEvent(exchange ExternalProviderExchange) Event {
	metadata := copyFields(exchange.Metadata)
	if metadata == nil {
		metadata = Fields{}
	}
	data := copyFields(exchange.Data)
	if data == nil {
		data = Fields{}
	}

	metadata["event_type"] = EventTypeExternalProviderExchange
	metadata["operation_kind"] = EventTypeExternalProviderExchange
	setFieldString(metadata, "provider", exchange.Provider)
	if exchange.ProviderEnv != nil {
		metadata["provider_environment"] = exchange.ProviderEnv
	}
	setFieldString(metadata, "provider_method", exchange.Method)
	setFieldString(metadata, "provider_endpoint", exchange.Endpoint)
	setFieldString(metadata, "provider_url", exchange.URL)
	setFieldString(metadata, "provider_accept", exchange.Accept)
	setFieldString(metadata, "provider_content_type", exchange.ContentType)
	setFieldInt(metadata, "provider_attempt", exchange.Attempt)
	setFieldInt(metadata, "provider_status_code", exchange.StatusCode)
	if exchange.StatusCode > 0 {
		metadata["provider_success"] = exchange.ErrorCode == "" && exchange.ErrorMessage == "" && exchange.StatusCode < 400
	}
	if exchange.Duration > 0 {
		metadata["provider_duration_ms"] = exchange.Duration.Milliseconds()
	}
	if !exchange.StartedAt.IsZero() {
		metadata["provider_started_at"] = formatTime(exchange.StartedAt)
	}
	if !exchange.FinishedAt.IsZero() {
		metadata["provider_finished_at"] = formatTime(exchange.FinishedAt)
	}
	addProviderPayloadMetadata(metadata, "provider_request", exchange.RequestPayloadBytes, exchange.RequestOmittedReason)
	addProviderPayloadMetadata(metadata, "provider_response", exchange.ResponsePayloadBytes, exchange.ResponseOmittedReason)
	setFieldString(metadata, "provider_error_code", exchange.ErrorCode)
	setFieldString(metadata, "provider_error", exchange.ErrorMessage)

	addProviderPayloadData(data, "provider_request", exchange.RequestPayloadBytes, exchange.RequestOmittedReason)
	addProviderPayloadData(data, "provider_response", exchange.ResponsePayloadBytes, exchange.ResponseOmittedReason)

	status := strings.TrimSpace(exchange.Status)
	if status == "" {
		status = providerExchangeStatus(exchange)
	}
	classification := strings.TrimSpace(exchange.Classification)
	if classification == "" {
		classification = ClassificationAudit
	}
	retentionHint := strings.TrimSpace(exchange.RetentionHint)
	if retentionHint == "" {
		retentionHint = RetentionHintAudit
	}
	operation := strings.TrimSpace(exchange.Operation)
	if operation == "" {
		operation = EventTypeExternalProviderExchange
	}

	event := Event{
		IdempotencyKey: exchange.IdempotencyKey,
		EventType:      EventTypeExternalProviderExchange,
		Classification: classification,
		RetentionHint:  retentionHint,
		RequestID:      exchange.RequestID,
		TraceID:        exchange.TraceID,
		SpanID:         exchange.SpanID,
		UserID:         exchange.UserID,
		TenantID:       exchange.TenantID,
		Operation:      operation,
		Status:         status,
		Method:         exchange.Method,
		Path:           exchange.Endpoint,
		RouteTemplate:  exchange.Endpoint,
		StartedAt:      formatOptionalTime(exchange.StartedAt),
		FinishedAt:     formatOptionalTime(exchange.FinishedAt),
		ErrorCode:      exchange.ErrorCode,
		ErrorMessage:   exchange.ErrorMessage,
		Metadata:       nilIfEmpty(metadata),
		Data:           nilIfEmpty(data),
	}
	if exchange.StatusCode > 0 {
		statusCode := exchange.StatusCode
		event.HTTPStatus = &statusCode
	}
	if exchange.Duration > 0 {
		durationMS := exchange.Duration.Milliseconds()
		event.DurationMS = &durationMS
	}
	return event
}

func addProviderPayloadMetadata(metadata Fields, prefix string, payload []byte, omittedReason string) {
	if len(payload) > 0 {
		metadata[prefix+"_payload_size_bytes"] = len(payload)
	}
	setFieldString(metadata, prefix+"_payload_omitted_reason", omittedReason)
}

func addProviderPayloadData(data Fields, prefix string, payload []byte, omittedReason string) {
	omittedReason = strings.TrimSpace(omittedReason)
	if omittedReason != "" {
		data[prefix+"_payload_omitted_reason"] = omittedReason
		return
	}
	if len(payload) == 0 {
		return
	}
	if json.Valid(payload) {
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err == nil {
			data[prefix+"_payload_json"] = value
			return
		}
	}
	data[prefix+"_payload_b64"] = base64.StdEncoding.EncodeToString(payload)
	data[prefix+"_payload_encoding"] = "base64"
}

func providerExchangeStatus(exchange ExternalProviderExchange) string {
	if exchange.ErrorCode != "" || exchange.ErrorMessage != "" || exchange.StatusCode >= 400 {
		return StatusFailed
	}
	if exchange.StatusCode > 0 {
		return StatusSuccess
	}
	return ""
}

func copyFields(fields Fields) Fields {
	if fields == nil {
		return nil
	}
	copied := make(Fields, len(fields))
	for key, value := range fields {
		copied[key] = value
	}
	return copied
}

func nilIfEmpty(fields Fields) Fields {
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func setFieldString(fields Fields, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		fields[key] = value
	}
}

func setFieldInt(fields Fields, key string, value int) {
	if value != 0 {
		fields[key] = value
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return formatTime(value)
}
