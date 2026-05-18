package logcenter

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateEventAcceptsValidLogEvent(t *testing.T) {
	err := ValidateEvent(Event{
		EventID:     "evt_1",
		EventType:   EventTypeLogEvent,
		OccurredAt:  formatTime(time.Now()),
		Environment: "test",
		Service:     "orders-api",
		Level:       LevelInfo,
		Message:     "order created",
	})
	if err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestValidateEventRejectsInvalidLogEvent(t *testing.T) {
	err := ValidateEvent(Event{
		EventID:     "evt_1",
		EventType:   EventTypeLogEvent,
		OccurredAt:  formatTime(time.Now()),
		Environment: "test",
		Service:     "orders-api",
		Level:       "verbose",
		Message:     "order created",
	})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("ValidateEvent() error = %v, want ErrInvalidEvent", err)
	}
	if !strings.Contains(err.Error(), "level") {
		t.Fatalf("ValidateEvent() error = %q, want level", err.Error())
	}
}

func TestValidateEventRejectsInvalidAuditEvent(t *testing.T) {
	err := ValidateEvent(Event{
		EventID:     "evt_1",
		EventType:   EventTypeAuditEvent,
		OccurredAt:  formatTime(time.Now()),
		Environment: "test",
		Service:     "orders-api",
		Action:      "order.updated",
		EntityType:  "order",
		EntityID:    "order-123",
	})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("ValidateEvent() error = %v, want ErrInvalidEvent", err)
	}
	if !strings.Contains(err.Error(), "changes") {
		t.Fatalf("ValidateEvent() error = %q, want changes", err.Error())
	}
}

func TestValidateEventRejectsMismatchedIdempotencyKey(t *testing.T) {
	event := validLogEvent()
	event.IdempotencyKey = "different"

	err := ValidateEvent(event)
	if err == nil {
		t.Fatal("ValidateEvent() error = nil, want mismatched idempotency error")
	}
	if !strings.Contains(err.Error(), "idempotency_key") {
		t.Fatalf("error = %q, want idempotency_key", err.Error())
	}
}

func TestValidateEventRejectsInvalidClassificationAndRetentionHint(t *testing.T) {
	event := validLogEvent()
	event.Classification = "finance"
	if err := ValidateEvent(event); err == nil || !strings.Contains(err.Error(), "classification") {
		t.Fatalf("classification error = %v, want invalid classification", err)
	}

	event = validLogEvent()
	event.RetentionHint = "forever"
	if err := ValidateEvent(event); err == nil || !strings.Contains(err.Error(), "retention_hint") {
		t.Fatalf("retention error = %v, want invalid retention_hint", err)
	}
}

func TestValidateEventAcceptsFiscalProviderExchange(t *testing.T) {
	err := ValidateEvent(Event{
		EventID:     "evt_fiscal_provider",
		EventType:   EventTypeFiscalProviderExchange,
		OccurredAt:  formatTime(time.Now()),
		Environment: "test",
		Service:     "orders-api",
		Operation:   "provider.exchange",
		Data: Fields{
			"provider_request_payload_b64": strings.Repeat("A", 70*1024),
		},
	})
	if err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func validLogEvent() Event {
	return Event{
		EventID:     "evt_1",
		EventType:   EventTypeLogEvent,
		OccurredAt:  formatTime(time.Now()),
		Environment: "test",
		Service:     "orders-api",
		Level:       LevelInfo,
		Message:     "order created",
	}
}
