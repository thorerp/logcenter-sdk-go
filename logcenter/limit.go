package logcenter

import (
	"encoding/json"
	"fmt"
)

const truncatedSuffix = "...[TRUNCATED]"

type limiter struct {
	maxStringBytes     int
	maxMetadataBytes   int
	maxDataBytes       int
	maxAuditValueBytes int
	maxEventBytes      int
}

func newLimiter(config Config) limiter {
	return limiter{
		maxStringBytes:     config.MaxStringBytes,
		maxMetadataBytes:   config.MaxMetadataBytes,
		maxDataBytes:       config.MaxDataBytes,
		maxAuditValueBytes: config.MaxAuditValueBytes,
		maxEventBytes:      config.MaxEventBytes,
	}
}

func (limiter limiter) limitEvent(event Event) (Event, error) {
	event.IdempotencyKey = limiter.limitString(event.IdempotencyKey)
	event.ServiceVersion = limiter.limitString(event.ServiceVersion)
	event.Classification = limiter.limitString(event.Classification)
	event.RetentionHint = limiter.limitString(event.RetentionHint)
	event.UserID = limiter.limitString(event.UserID)
	event.TenantID = limiter.limitString(event.TenantID)
	event.Operation = limiter.limitString(event.Operation)
	event.Method = limiter.limitString(event.Method)
	event.Path = limiter.limitString(event.Path)
	event.RouteTemplate = limiter.limitString(event.RouteTemplate)
	event.Kind = limiter.limitString(event.Kind)
	event.Name = limiter.limitString(event.Name)
	event.Message = limiter.limitString(event.Message)
	event.ErrorCode = limiter.limitString(event.ErrorCode)
	event.ErrorMessage = limiter.limitString(event.ErrorMessage)
	event.ErrorType = limiter.limitString(event.ErrorType)
	event.Severity = limiter.limitString(event.Severity)
	event.Fingerprint = limiter.limitString(event.Fingerprint)
	event.StackTrace = limiter.limitString(event.StackTrace)
	event.ActorType = limiter.limitString(event.ActorType)
	event.ActorID = limiter.limitString(event.ActorID)
	event.Action = limiter.limitString(event.Action)
	event.EntityType = limiter.limitString(event.EntityType)
	event.EntityID = limiter.limitString(event.EntityID)
	event.FieldName = limiter.limitString(event.FieldName)
	event.Reason = limiter.limitString(event.Reason)
	event.Metadata = limiter.limitFields("metadata", event.Metadata, limiter.maxMetadataBytes)
	event.Data = limiter.limitFields("data", event.Data, limiter.maxDataBytes)
	event.Changes = limiter.limitJSONValue("changes", event.Changes, limiter.maxAuditValueBytes)
	event.OldValue = limiter.limitJSONValue("old_value", event.OldValue, limiter.maxAuditValueBytes)
	event.NewValue = limiter.limitJSONValue("new_value", event.NewValue, limiter.maxAuditValueBytes)

	if limiter.maxEventBytes > 0 {
		encoded, err := json.Marshal(event)
		if err != nil {
			return event, fmt.Errorf("%w: event must be valid JSON: %w", ErrInvalidEvent, err)
		}
		if len(encoded) > limiter.maxEventBytes {
			return event, fmt.Errorf("%w: event exceeds max_event_bytes", ErrInvalidEvent)
		}
	}
	return event, nil
}

func (limiter limiter) limitFields(field string, fields Fields, maxBytes int) Fields {
	if fields == nil {
		return nil
	}
	limited := limiter.limitValue(fields)
	fields, ok := limited.(Fields)
	if !ok {
		return nil
	}
	limited = limiter.limitJSONValue(field, fields, maxBytes)
	if limitedFields, ok := limited.(Fields); ok {
		return limitedFields
	}
	return nil
}

func (limiter limiter) limitJSONValue(field string, value any, maxBytes int) any {
	if value == nil {
		return nil
	}
	limited := limiter.limitValue(value)
	if jsonSize(limited) <= maxBytes {
		return limited
	}
	placeholder := Fields{
		"_truncated":       true,
		"_truncated_field": field,
	}
	if jsonSize(placeholder) <= maxBytes {
		return placeholder
	}
	return nil
}

func (limiter limiter) limitValue(value any) any {
	switch typed := value.(type) {
	case string:
		return limiter.limitString(typed)
	case map[string]any:
		return limiter.limitFieldsMap(Fields(typed))
	case Fields:
		return limiter.limitFieldsMap(typed)
	case []any:
		values := make([]any, len(typed))
		for i, item := range typed {
			values[i] = limiter.limitValue(item)
		}
		return values
	case []string:
		values := make([]string, len(typed))
		for i, item := range typed {
			values[i] = limiter.limitString(item)
		}
		return values
	case []Change:
		values := make([]Change, len(typed))
		for i, change := range typed {
			values[i] = Change{
				Field:    limiter.limitString(change.Field),
				OldValue: limiter.limitValue(change.OldValue),
				NewValue: limiter.limitValue(change.NewValue),
			}
		}
		return values
	default:
		return value
	}
}

func (limiter limiter) limitFieldsMap(fields Fields) Fields {
	limited := make(Fields, len(fields))
	for key, value := range fields {
		limited[limiter.limitString(key)] = limiter.limitValue(value)
	}
	return limited
}

func (limiter limiter) limitString(value string) string {
	return truncateStringBytes(value, limiter.maxStringBytes)
}

func truncateStringBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	if maxBytes <= len(truncatedSuffix) {
		return validStringPrefix(value, maxBytes)
	}
	return validStringPrefix(value, maxBytes-len(truncatedSuffix)) + truncatedSuffix
}

func validStringPrefix(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := 0
	for index := range value {
		if index > maxBytes {
			break
		}
		end = index
	}
	return value[:end]
}

func jsonSize(value any) int {
	if value == nil {
		return 0
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return MaxJSONValueBytes + 1
	}
	return len(encoded)
}
