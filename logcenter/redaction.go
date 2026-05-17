package logcenter

import (
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var defaultSensitiveKeyFragments = []string{
	"password",
	"senha",
	"token",
	"authorization",
	"cookie",
	"secret",
	"secret_key",
	"client_secret",
	"api_key",
	"apikey",
	"private_key",
	"pfx",
	"certificate",
	"certificate_password",
	"certificado",
	"senha_certificado",
	"base64",
	"file",
	"arquivo",
	"document",
	"pdf",
	"xml",
	"logo",
	"stripe",
	"csrt",
	"chave_acesso",
	"cpf",
	"cnpj",
	"email",
	"phone",
	"telefone",
	"cvv",
}

var (
	defaultRedactor = newRedactor(nil)
	bearerPattern   = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

type redactor struct {
	fragments   []string
	pairPattern *regexp.Regexp
}

func newRedactor(extraFragments []string) redactor {
	fragments := normalizeFragments(append(defaultSensitiveKeyFragments, extraFragments...))
	return redactor{
		fragments:   fragments,
		pairPattern: sensitivePairPattern(fragments),
	}
}

func RedactFields(fields Fields) Fields {
	return defaultRedactor.RedactFields(fields)
}

func (redactor redactor) RedactFields(fields Fields) Fields {
	if fields == nil {
		return nil
	}
	redacted := make(Fields, len(fields))
	for key, value := range fields {
		if redactor.IsSensitiveKey(key) {
			redacted[key] = redactedValue
			continue
		}
		redacted[key] = redactor.RedactValue(value)
	}
	return redacted
}

func RedactValue(value any) any {
	return defaultRedactor.RedactValue(value)
}

func (redactor redactor) RedactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactor.RedactFields(Fields(typed))
	case Fields:
		return redactor.RedactFields(typed)
	case []any:
		values := make([]any, len(typed))
		for i, item := range typed {
			values[i] = redactor.RedactValue(item)
		}
		return values
	case []Change:
		values := make([]Change, len(typed))
		for i, change := range typed {
			values[i] = Change{
				Field:    change.Field,
				OldValue: redactor.RedactNamedValue(change.Field, change.OldValue),
				NewValue: redactor.RedactNamedValue(change.Field, change.NewValue),
			}
		}
		return values
	case string:
		return redactor.RedactString(typed)
	default:
		return value
	}
}

func RedactNamedValue(name string, value any) any {
	return defaultRedactor.RedactNamedValue(name, value)
}

func (redactor redactor) RedactNamedValue(name string, value any) any {
	if value == nil {
		return nil
	}
	if redactor.IsSensitiveKey(name) {
		return redactedValue
	}
	return redactor.RedactValue(value)
}

func RedactString(value string) string {
	return defaultRedactor.RedactString(value)
}

func (redactor redactor) RedactString(value string) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer "+redactedValue)
	if redactor.pairPattern == nil {
		return value
	}
	value = redactor.pairPattern.ReplaceAllStringFunc(value, func(match string) string {
		separatorIndex := strings.IndexAny(match, ":=")
		if separatorIndex == -1 {
			return redactedValue
		}
		return strings.TrimSpace(match[:separatorIndex]) + match[separatorIndex:separatorIndex+1] + redactedValue
	})
	return value
}

func IsSensitiveKey(key string) bool {
	return defaultRedactor.IsSensitiveKey(key)
}

func (redactor redactor) IsSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range redactor.fragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func redactEvent(event Event) Event {
	return defaultRedactor.redactEvent(event)
}

func (redactor redactor) redactEvent(event Event) Event {
	event.Message = redactor.RedactString(event.Message)
	event.ErrorMessage = redactor.RedactString(event.ErrorMessage)
	event.StackTrace = redactor.RedactString(event.StackTrace)
	event.Reason = redactor.RedactString(event.Reason)
	event.Metadata = redactor.RedactFields(event.Metadata)
	event.Data = redactor.RedactFields(event.Data)
	event.Changes = redactor.RedactValue(event.Changes)
	event.OldValue = redactor.RedactNamedValue(event.FieldName, event.OldValue)
	event.NewValue = redactor.RedactNamedValue(event.FieldName, event.NewValue)
	return event
}

func normalizeFragments(fragments []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		fragment = strings.ToLower(strings.TrimSpace(fragment))
		if fragment == "" {
			continue
		}
		if _, ok := seen[fragment]; ok {
			continue
		}
		seen[fragment] = struct{}{}
		normalized = append(normalized, fragment)
	}
	return normalized
}

func sensitivePairPattern(fragments []string) *regexp.Regexp {
	quoted := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		quoted = append(quoted, regexp.QuoteMeta(fragment))
	}
	if len(quoted) == 0 {
		return nil
	}
	pattern := `(?i)(["']?(?:` + strings.Join(quoted, "|") + `)["']?)\s*[:=]\s*("[^"]*"|'[^']*'|[^,;\s}\]]+)`
	return regexp.MustCompile(pattern)
}
