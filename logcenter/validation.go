package logcenter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidEvent = errors.New("invalid logcenter event")

var allowedLevels = map[string]struct{}{
	LevelDebug: {},
	LevelInfo:  {},
	LevelWarn:  {},
	LevelError: {},
	LevelFatal: {},
}

var allowedStatuses = map[string]struct{}{
	StatusStarted:  {},
	StatusSuccess:  {},
	StatusFailed:   {},
	StatusTimeout:  {},
	StatusCanceled: {},
	StatusIgnored:  {},
	StatusRetrying: {},
}

var allowedClassifications = map[string]struct{}{
	ClassificationOperational: {},
	ClassificationSecurity:    {},
	ClassificationAudit:       {},
	ClassificationCritical:    {},
	ClassificationPrivacy:     {},
}

var allowedRetentionHints = map[string]struct{}{
	RetentionHintDefault:  {},
	RetentionHintShort:    {},
	RetentionHintStandard: {},
	RetentionHintLong:     {},
	RetentionHintAudit:    {},
	RetentionHintPrivacy:  {},
}

func ValidateEvent(event Event) error {
	if strings.TrimSpace(event.IdempotencyKey) != "" && strings.TrimSpace(event.EventID) != strings.TrimSpace(event.IdempotencyKey) {
		return fmt.Errorf("%w: idempotency_key must match event_id when both are set", ErrInvalidEvent)
	}
	if strings.TrimSpace(event.EventID) == "" {
		return requiredEventField("event_id")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return requiredEventField("event_type")
	}
	if _, err := parseRequiredEventTime("occurred_at", event.OccurredAt); err != nil {
		return err
	}
	if strings.TrimSpace(event.Environment) == "" {
		return requiredEventField("environment")
	}
	if strings.TrimSpace(event.Service) == "" {
		return requiredEventField("service")
	}
	if err := validateClassification(event.Classification); err != nil {
		return err
	}
	if err := validateRetentionHint(event.RetentionHint); err != nil {
		return err
	}
	if err := validateJSONValue("metadata", event.Metadata); err != nil {
		return err
	}
	if err := validateJSONValue("data", event.Data); err != nil {
		return err
	}

	switch event.EventType {
	case EventTypeRequestStarted:
		return validateRequestStartedEvent(event)
	case EventTypeRequestFinished:
		return validateRequestFinishedEvent(event)
	case EventTypeSpan:
		return validateSpanEvent(event)
	case EventTypeLogEvent:
		return validateLogEvent(event)
	case EventTypeErrorEvent:
		return validateErrorEvent(event)
	case EventTypeAuditEvent:
		return validateAuditEvent(event)
	case EventTypeFiscalProviderExchange:
		return validateFiscalProviderExchangeEvent(event)
	default:
		return fmt.Errorf("%w: unsupported event_type %q", ErrInvalidEvent, event.EventType)
	}
}

func validateRequestStartedEvent(event Event) error {
	if strings.TrimSpace(event.RequestID) == "" {
		return requiredEventField("request_id")
	}
	if strings.TrimSpace(event.TraceID) == "" {
		return requiredEventField("trace_id")
	}
	if strings.TrimSpace(event.Operation) == "" {
		return requiredEventField("operation")
	}
	_, err := parseRequiredEventTime("started_at", event.StartedAt)
	return err
}

func validateRequestFinishedEvent(event Event) error {
	if strings.TrimSpace(event.RequestID) == "" {
		return requiredEventField("request_id")
	}
	if strings.TrimSpace(event.TraceID) == "" {
		return requiredEventField("trace_id")
	}
	if err := validateStatus(event.Status); err != nil {
		return err
	}
	if _, err := parseRequiredEventTime("finished_at", event.FinishedAt); err != nil {
		return err
	}
	return validateDuration(event.DurationMS)
}

func validateSpanEvent(event Event) error {
	if strings.TrimSpace(event.TraceID) == "" {
		return requiredEventField("trace_id")
	}
	if strings.TrimSpace(event.SpanID) == "" {
		return requiredEventField("span_id")
	}
	if strings.TrimSpace(event.Name) == "" {
		return requiredEventField("name")
	}
	if _, err := parseRequiredEventTime("started_at", event.StartedAt); err != nil {
		return err
	}
	if _, err := parseRequiredEventTime("finished_at", event.FinishedAt); err != nil {
		return err
	}
	if err := validateDuration(event.DurationMS); err != nil {
		return err
	}
	return validateStatus(event.Status)
}

func validateLogEvent(event Event) error {
	if strings.TrimSpace(event.Level) == "" {
		return requiredEventField("level")
	}
	if _, ok := allowedLevels[event.Level]; !ok {
		return fmt.Errorf("%w: level is invalid", ErrInvalidEvent)
	}
	if strings.TrimSpace(event.Message) == "" {
		return requiredEventField("message")
	}
	return nil
}

func validateErrorEvent(event Event) error {
	if strings.TrimSpace(event.ErrorCode) == "" && strings.TrimSpace(event.ErrorMessage) == "" {
		return fmt.Errorf("%w: error_code or error_message is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(event.Severity) == "" {
		return requiredEventField("severity")
	}
	return nil
}

func validateAuditEvent(event Event) error {
	if strings.TrimSpace(event.Action) == "" {
		return requiredEventField("action")
	}
	if strings.TrimSpace(event.EntityType) == "" {
		return requiredEventField("entity_type")
	}
	if strings.TrimSpace(event.EntityID) == "" {
		return requiredEventField("entity_id")
	}
	if event.Changes == nil && event.OldValue == nil && event.NewValue == nil {
		return fmt.Errorf("%w: changes or old_value/new_value is required", ErrInvalidEvent)
	}
	if err := validateChanges(event.Changes); err != nil {
		return err
	}
	if err := validateJSONValue("old_value", event.OldValue); err != nil {
		return err
	}
	if err := validateJSONValue("new_value", event.NewValue); err != nil {
		return err
	}
	return nil
}

func validateFiscalProviderExchangeEvent(event Event) error {
	if strings.TrimSpace(event.Operation) == "" {
		return requiredEventField("operation")
	}
	if len(event.Data) == 0 {
		return requiredEventField("data")
	}
	return nil
}

func validateChanges(value any) error {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []Change:
		if len(typed) == 0 {
			return fmt.Errorf("%w: changes cannot be empty", ErrInvalidEvent)
		}
	case []any:
		if len(typed) == 0 {
			return fmt.Errorf("%w: changes cannot be empty", ErrInvalidEvent)
		}
	case map[string]any:
		if len(typed) == 0 {
			return fmt.Errorf("%w: changes cannot be empty", ErrInvalidEvent)
		}
	case Fields:
		if len(typed) == 0 {
			return fmt.Errorf("%w: changes cannot be empty", ErrInvalidEvent)
		}
	default:
		return fmt.Errorf("%w: changes must be object or array", ErrInvalidEvent)
	}
	return validateJSONValue("changes", value)
}

func validateDuration(duration *int64) error {
	if duration == nil {
		return requiredEventField("duration_ms")
	}
	if *duration < 0 {
		return fmt.Errorf("%w: duration_ms cannot be negative", ErrInvalidEvent)
	}
	return nil
}

func validateStatus(status string) error {
	if strings.TrimSpace(status) == "" {
		return requiredEventField("status")
	}
	if _, ok := allowedStatuses[status]; !ok {
		return fmt.Errorf("%w: status is invalid", ErrInvalidEvent)
	}
	return nil
}

func validateClassification(classification string) error {
	classification = strings.TrimSpace(classification)
	if classification == "" {
		return nil
	}
	if _, ok := allowedClassifications[classification]; !ok {
		return fmt.Errorf("%w: classification is invalid", ErrInvalidEvent)
	}
	return nil
}

func validateRetentionHint(retentionHint string) error {
	retentionHint = strings.TrimSpace(retentionHint)
	if retentionHint == "" {
		return nil
	}
	if _, ok := allowedRetentionHints[retentionHint]; !ok {
		return fmt.Errorf("%w: retention_hint is invalid", ErrInvalidEvent)
	}
	return nil
}

func validateJSONValue(field string, value any) error {
	if value == nil {
		return nil
	}
	if _, err := json.Marshal(value); err != nil {
		return fmt.Errorf("%w: %s must be valid JSON", ErrInvalidEvent, field)
	}
	return nil
}

func requiredEventField(field string) error {
	return fmt.Errorf("%w: %s is required", ErrInvalidEvent, field)
}

func parseRequiredEventTime(field, value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, requiredEventField(field)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s must be RFC3339", ErrInvalidEvent, field)
	}
	return parsed, nil
}
