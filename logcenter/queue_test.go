package logcenter

import "testing"

func TestEventQueueDropsDebugBeforeImportantEvent(t *testing.T) {
	queue := newEventQueue(1)

	result := queue.push(Event{
		EventType: EventTypeLogEvent,
		Level:     LevelDebug,
		Message:   "debug",
	})
	if !result.queued || result.dropped != nil {
		t.Fatalf("push debug result=%#v, want queued without drop", result)
	}

	result = queue.push(Event{
		EventType:    EventTypeErrorEvent,
		ErrorCode:    "BOOM",
		ErrorMessage: "boom",
		Severity:     SeverityError,
	})
	if !result.queued || result.dropped == nil || result.dropReason != "buffer_replaced" {
		t.Fatalf("push error result=%#v, want queued with debug dropped", result)
	}

	events := queue.drain(10)
	if len(events) != 1 || events[0].EventType != EventTypeErrorEvent {
		t.Fatalf("events = %#v, want preserved error event", events)
	}
}
