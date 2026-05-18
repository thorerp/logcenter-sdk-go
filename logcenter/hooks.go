package logcenter

type Hooks struct {
	OnEventDropped   func(EventDrop)
	OnEventTruncated func(EventTruncation)
	OnBatchFailed    func(BatchFailure)
	OnEventRejected  func(EventRejection)
	OnErrorChanged   func(ErrorChange)
}

type EventDrop struct {
	Event  Event
	Reason string
	Err    error
}

type EventTruncation struct {
	Before Event
	After  Event
	Reason string
}

type BatchFailure struct {
	Events     []Event
	EventCount int
	Err        error
}

type EventRejection struct {
	Event Event
	Error EventError
}

type ErrorChange struct {
	LastError string
	Err       error
}

func callHook(fn func()) {
	if fn == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	fn()
}
