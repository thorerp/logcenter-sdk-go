package logcenter

import "sync"

type eventQueue struct {
	mu       sync.Mutex
	events   []Event
	capacity int
}

type queuePushResult struct {
	queued     bool
	dropped    *Event
	dropReason string
}

func newEventQueue(capacity int) *eventQueue {
	return &eventQueue{
		events:   make([]Event, 0, capacity),
		capacity: capacity,
	}
}

func (queue *eventQueue) push(event Event) queuePushResult {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	if len(queue.events) < queue.capacity {
		queue.events = append(queue.events, event)
		return queuePushResult{queued: true}
	}

	if isDroppable(event) {
		dropped := event
		return queuePushResult{dropped: &dropped, dropReason: "buffer_full"}
	}

	if index := queue.indexOfDroppable(LevelDebug); index >= 0 {
		dropped := queue.events[index]
		queue.events[index] = event
		return queuePushResult{queued: true, dropped: &dropped, dropReason: "buffer_replaced"}
	}
	if index := queue.indexOfDroppable(LevelInfo); index >= 0 {
		dropped := queue.events[index]
		queue.events[index] = event
		return queuePushResult{queued: true, dropped: &dropped, dropReason: "buffer_replaced"}
	}
	dropped := event
	return queuePushResult{dropped: &dropped, dropReason: "buffer_full"}
}

func (queue *eventQueue) drain(max int) []Event {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	if len(queue.events) == 0 {
		return nil
	}
	if max <= 0 || max > len(queue.events) {
		max = len(queue.events)
	}
	events := make([]Event, max)
	copy(events, queue.events[:max])
	copy(queue.events, queue.events[max:])
	queue.events = queue.events[:len(queue.events)-max]
	return events
}

func (queue *eventQueue) len() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return len(queue.events)
}

func (queue *eventQueue) indexOfDroppable(level string) int {
	for i, event := range queue.events {
		if event.EventType == EventTypeLogEvent && event.Level == level {
			return i
		}
	}
	return -1
}

func isDroppable(event Event) bool {
	return event.EventType == EventTypeLogEvent && (event.Level == LevelDebug || event.Level == LevelInfo)
}
