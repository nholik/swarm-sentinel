package healthcheck

import (
	"sync"
	"time"
)

// Snapshot describes the latest cycle timing details.
type Snapshot struct {
	LastCycleTime   *time.Time `json:"last_cycle_time"`
	CycleDurationMS int64      `json:"cycle_duration_ms"`
	StacksEvaluated int        `json:"stacks_evaluated"`
}

// Tracker records cycle timing for health endpoints.
type Tracker struct {
	mu              sync.RWMutex
	lastCycle       time.Time
	cycleDuration   time.Duration
	stacksEvaluated int
	ready           bool
}

// NewTracker constructs a new Tracker.
func NewTracker() *Tracker {
	return &Tracker{}
}

// RecordCycle updates cycle timing and readiness.
func (t *Tracker) RecordCycle(duration time.Duration, stacksEvaluated int) {
	if t == nil {
		return
	}
	now := time.Now().UTC()
	t.mu.Lock()
	t.lastCycle = now
	t.cycleDuration = duration
	t.stacksEvaluated = stacksEvaluated
	t.ready = true
	t.mu.Unlock()
}

// Snapshot returns the current tracker snapshot.
func (t *Tracker) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	var last *time.Time
	if !t.lastCycle.IsZero() {
		value := t.lastCycle
		last = &value
	}
	return Snapshot{
		LastCycleTime:   last,
		CycleDurationMS: int64(t.cycleDuration / time.Millisecond),
		StacksEvaluated: t.stacksEvaluated,
	}
}

// Ready reports whether at least one successful cycle has completed.
func (t *Tracker) Ready() bool {
	if t == nil {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.ready
}

// Healthy reports whether the last cycle completed within 2x the poll interval.
func (t *Tracker) Healthy(now time.Time, pollInterval time.Duration) bool {
	if t == nil {
		return false
	}
	if pollInterval <= 0 {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.lastCycle.IsZero() {
		return false
	}
	return now.Sub(t.lastCycle) <= 2*pollInterval
}
