package logcenter

import (
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var sensitiveKeyFragments = []string{
	"password",
	"senha",
	"token",
	"authorization",
	"cookie",
	"secret",
	"api_key",
	"apikey",
	"private_key",
	"pfx",
	"certificate_password",
	"cvv",
}

var (
	sensitivePairPattern = regexp.MustCompile(`(?i)\b(password|senha|token|secret|api[_-]?key|apikey|authorization|cookie)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^,;\s]+)`)
	bearerPattern        = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func RedactFields(fields Fields) Fields {
	if fields == nil {
		return nil
	}
	redacted := make(Fields, len(fields))
	for key, value := range fields {
		if IsSensitiveKey(key) {
			redacted[key] = redactedValue
			continue
		}
		redacted[key] = RedactValue(value)
	}
	return redacted
}

func RedactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return RedactFields(Fields(typed))
	case Fields:
		return RedactFields(typed)
	case []any:
		values := make([]any, len(typed))
		for i, item := range typed {
			values[i] = RedactValue(item)
		}
		return values
	case []Change:
		values := make([]Change, len(typed))
		for i, change := range typed {
			values[i] = Change{
				Field:    change.Field,
				OldValue: RedactNamedValue(change.Field, change.OldValue),
				NewValue: RedactNamedValue(change.Field, change.NewValue),
			}
		}
		return values
	default:
		return value
	}
}

func RedactNamedValue(name string, value any) any {
	if value == nil {
		return nil
	}
	if IsSensitiveKey(name) {
		return redactedValue
	}
	return RedactValue(value)
}

func RedactString(value string) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer "+redactedValue)
	value = sensitivePairPattern.ReplaceAllStringFunc(value, func(match string) string {
		separatorIndex := strings.IndexAny(match, ":=")
		if separatorIndex == -1 {
			return redactedValue
		}
		return strings.TrimSpace(match[:separatorIndex]) + match[separatorIndex:separatorIndex+1] + redactedValue
	})
	return value
}

func IsSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range sensitiveKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func redactEvent(event Event) Event {
	event.Message = RedactString(event.Message)
	event.ErrorMessage = RedactString(event.ErrorMessage)
	event.StackTrace = RedactString(event.StackTrace)
	event.Reason = RedactString(event.Reason)
	event.Metadata = RedactFields(event.Metadata)
	event.Changes = RedactValue(event.Changes)
	event.OldValue = RedactNamedValue(event.FieldName, event.OldValue)
	event.NewValue = RedactNamedValue(event.FieldName, event.NewValue)
	return event
}
