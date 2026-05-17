package logcenter

import (
	"sync"
	"sync/atomic"
)

type Stats struct {
	Queued        uint64 `json:"queued"`
	Dropped       uint64 `json:"dropped"`
	SentEvents    uint64 `json:"sent_events"`
	SentBatches   uint64 `json:"sent_batches"`
	FailedEvents  uint64 `json:"failed_events"`
	FailedBatches uint64 `json:"failed_batches"`
	Accepted      uint64 `json:"accepted"`
	Duplicated    uint64 `json:"duplicated"`
	Rejected      uint64 `json:"rejected"`
	LastError     string `json:"last_error,omitempty"`
}

type counters struct {
	queued        atomic.Uint64
	dropped       atomic.Uint64
	sentEvents    atomic.Uint64
	sentBatches   atomic.Uint64
	failedEvents  atomic.Uint64
	failedBatches atomic.Uint64
	accepted      atomic.Uint64
	duplicated    atomic.Uint64
	rejected      atomic.Uint64

	mu        sync.Mutex
	lastError string
}

func (counters *counters) snapshot() Stats {
	counters.mu.Lock()
	lastError := counters.lastError
	counters.mu.Unlock()

	return Stats{
		Queued:        counters.queued.Load(),
		Dropped:       counters.dropped.Load(),
		SentEvents:    counters.sentEvents.Load(),
		SentBatches:   counters.sentBatches.Load(),
		FailedEvents:  counters.failedEvents.Load(),
		FailedBatches: counters.failedBatches.Load(),
		Accepted:      counters.accepted.Load(),
		Duplicated:    counters.duplicated.Load(),
		Rejected:      counters.rejected.Load(),
		LastError:     lastError,
	}
}

func (counters *counters) setError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	next := err.Error()
	counters.mu.Lock()
	changed := counters.lastError != next
	counters.lastError = next
	counters.mu.Unlock()
	return next, changed
}
