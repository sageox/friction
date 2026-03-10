// Package throttle provides a flush rate limiter to prevent thundering herd.
package throttle

import (
	"sync"
	"time"
)

// Throttle prevents thundering herd flushes by enforcing a minimum cooldown
// between flush operations. Without throttling, every Record() call above a batch
// threshold can spawn a new flush goroutine, creating unbounded HTTP POSTs.
//
// Usage: call TryFlush() before spawning a flush goroutine. It atomically claims
// a flush slot if the cooldown has elapsed. Call RecordFlush() from the flush
// function itself (e.g., ticker-triggered flushes that bypass TryFlush).
type Throttle struct {
	mu        sync.Mutex
	lastFlush time.Time
	cooldown  time.Duration
}

// New creates a Throttle with the given minimum interval between flushes.
func New(cooldown time.Duration) *Throttle {
	return &Throttle{cooldown: cooldown}
}

// TryFlush atomically checks if the cooldown has elapsed and claims the flush slot.
// Returns true if the caller should proceed with flushing.
func (t *Throttle) TryFlush() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Since(t.lastFlush) < t.cooldown {
		return false
	}
	t.lastFlush = time.Now()
	return true
}

// RecordFlush updates the last flush timestamp. Use this for flushes triggered
// by the background ticker (which bypass TryFlush).
func (t *Throttle) RecordFlush() {
	t.mu.Lock()
	t.lastFlush = time.Now()
	t.mu.Unlock()
}
