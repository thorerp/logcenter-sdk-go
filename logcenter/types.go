package logcenter

import "time"

const (
	EventTypeRequestStarted           = "request_started"
	EventTypeRequestFinished          = "request_finished"
	EventTypeSpan                     = "span"
	EventTypeLogEvent                 = "log_event"
	EventTypeErrorEvent               = "error_event"
	EventTypeAuditEvent               = "audit_event"
	EventTypeExternalProviderExchange = "external_provider_exchange"
	// Deprecated: use EventTypeExternalProviderExchange.
	EventTypeFiscalProviderExchange = "fiscal_provider_exchange"

	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
	LevelFatal = "fatal"

	StatusStarted  = "started"
	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusTimeout  = "timeout"
	StatusCanceled = "canceled"
	StatusIgnored  = "ignored"
	StatusRetrying = "retrying"

	SeverityError = "error"

	ClassificationOperational = "operational"
	ClassificationSecurity    = "security"
	ClassificationAudit       = "audit"
	ClassificationCritical    = "critical"
	ClassificationPrivacy     = "privacy"

	RetentionHintDefault  = "default"
	RetentionHintShort    = "short"
	RetentionHintStandard = "standard"
	RetentionHintLong     = "long"
	RetentionHintAudit    = "audit"
	RetentionHintPrivacy  = "privacy"
)

type Fields map[string]any

type Event struct {
	EventID        string `json:"event_id"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	EventType      string `json:"event_type"`
	OccurredAt     string `json:"occurred_at"`
	Environment    string `json:"environment"`
	Service        string `json:"service"`
	ServiceVersion string `json:"service_version,omitempty"`
	Classification string `json:"classification,omitempty"`
	RetentionHint  string `json:"retention_hint,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	SpanID         string `json:"span_id,omitempty"`
	ParentSpanID   string `json:"parent_span_id,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	TenantID       string `json:"tenant_id,omitempty"`
	Operation      string `json:"operation,omitempty"`
	Status         string `json:"status,omitempty"`
	Method         string `json:"method,omitempty"`
	Path           string `json:"path,omitempty"`
	RouteTemplate  string `json:"route_template,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Name           string `json:"name,omitempty"`
	Level          string `json:"level,omitempty"`
	Message        string `json:"message,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	ErrorType      string `json:"error_type,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	StackTrace     string `json:"stack_trace,omitempty"`
	HTTPStatus     *int   `json:"http_status,omitempty"`
	DurationMS     *int64 `json:"duration_ms,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	FinishedAt     string `json:"finished_at,omitempty"`
	ActorType      string `json:"actor_type,omitempty"`
	ActorID        string `json:"actor_id,omitempty"`
	Action         string `json:"action,omitempty"`
	EntityType     string `json:"entity_type,omitempty"`
	EntityID       string `json:"entity_id,omitempty"`
	FieldName      string `json:"field_name,omitempty"`
	Changes        any    `json:"changes,omitempty"`
	OldValue       any    `json:"old_value,omitempty"`
	NewValue       any    `json:"new_value,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Metadata       Fields `json:"metadata,omitempty"`
	Data           Fields `json:"data,omitempty"`
}

type Change struct {
	Field    string `json:"field"`
	OldValue any    `json:"old_value"`
	NewValue any    `json:"new_value"`
}

type ErrorOptions struct {
	IdempotencyKey string
	Classification string
	RetentionHint  string
	Code           string
	Type           string
	Severity       string
	Fingerprint    string
	StackTrace     string
	Message        string
	Metadata       Fields
	Data           Fields
}

type AuditEvent struct {
	IdempotencyKey string
	Classification string
	RetentionHint  string
	ActorType      string
	ActorID        string
	TenantID       string
	Operation      string
	Action         string
	EntityType     string
	EntityID       string
	FieldName      string
	OldValue       any
	NewValue       any
	Changes        []Change
	Reason         string
	Metadata       Fields
	Data           Fields
}

type batchRequest struct {
	BatchID string         `json:"batch_id"`
	SentAt  string         `json:"sent_at"`
	Source  map[string]any `json:"source"`
	Events  []Event        `json:"events"`
}

type BatchResponse struct {
	BatchID    string       `json:"batch_id"`
	Received   int          `json:"received"`
	Accepted   int          `json:"accepted"`
	Duplicated int          `json:"duplicated"`
	Rejected   int          `json:"rejected"`
	Errors     []EventError `json:"errors"`
}

type EventError struct {
	Index   int    `json:"index"`
	EventID string `json:"event_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
