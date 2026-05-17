package logcenter

import (
	"strings"
	"testing"
)

func TestRedactFieldsMasksSensitiveKeys(t *testing.T) {
	fields := Fields{
		"password": "secret",
		"cpf":      "123",
		"email":    "user@example.invalid",
		"nested": Fields{
			"api_key": "test-secret-value",
			"safe":    "visible",
		},
	}

	redacted := RedactFields(fields)
	nested := redacted["nested"].(Fields)

	if redacted["password"] != redactedValue {
		t.Fatalf("password = %v, want redacted", redacted["password"])
	}
	if redacted["cpf"] != redactedValue || redacted["email"] != redactedValue {
		t.Fatalf("expanded sensitive fields were not redacted: %#v", redacted)
	}
	if nested["api_key"] != redactedValue {
		t.Fatalf("api_key = %v, want redacted", nested["api_key"])
	}
	if nested["safe"] != "visible" {
		t.Fatalf("safe = %v, want visible", nested["safe"])
	}
}

func TestRedactStringMasksAssignmentsAndBearer(t *testing.T) {
	redacted := RedactString(`token=abc Authorization: Bearer clear "api_key":"secret"`)

	if strings.Contains(redacted, "abc") || strings.Contains(redacted, "clear") || strings.Contains(redacted, "secret") {
		t.Fatalf("redacted string = %q, still contains secret", redacted)
	}
}

func TestRedactChangeMasksSensitiveFieldValues(t *testing.T) {
	redacted := RedactValue([]Change{{
		Field:    "api_key",
		OldValue: "old",
		NewValue: "new",
	}}).([]Change)

	if redacted[0].OldValue != redactedValue || redacted[0].NewValue != redactedValue {
		t.Fatalf("change = %#v, want redacted old/new", redacted[0])
	}
}
