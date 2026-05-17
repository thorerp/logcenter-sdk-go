package logcenter

import (
	"sync"
	"sync/atomic"
)

type Stats struct {
	Queued        uint64
	Dropped       uint64
	SentEvents    uint64
	SentBatches   uint64
	FailedEvents  uint64
	FailedBatches uint64
	Accepted      uint64
	Duplicated    uint64
	Rejected      uint64
	LastError     string
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

func (counters *counters) setError(err error) {
	if err == nil {
		return
	}
	counters.mu.Lock()
	counters.lastError = err.Error()
	counters.mu.Unlock()
}
