package platform

import (
	"strings"
	"sync"
	"time"
)

// RuntimeHealthSnapshot is bounded, process-local evidence about a background
// manager. Durable work receipts remain authoritative; this snapshot exists so
// a manager failure cannot disappear between database inspections.
type RuntimeHealthSnapshot struct {
	Running             bool
	LastAttemptUTC      time.Time
	LastSuccessUTC      time.Time
	LastFailureUTC      time.Time
	ConsecutiveFailures int64
	FailureStage        string
}

// RuntimeHealthTracker stores only timestamps, a counter, and a controlled
// stage label. It deliberately does not retain arbitrary error text or work
// payloads.
type RuntimeHealthTracker struct {
	mu       sync.RWMutex
	now      func() time.Time
	snapshot RuntimeHealthSnapshot
}

func NewRuntimeHealthTracker(now func() time.Time) *RuntimeHealthTracker {
	if now == nil {
		now = time.Now
	}
	return &RuntimeHealthTracker{now: now}
}

func (tracker *RuntimeHealthTracker) Started() {
	if tracker == nil {
		return
	}
	tracker.mu.Lock()
	tracker.snapshot.Running = true
	tracker.mu.Unlock()
}

func (tracker *RuntimeHealthTracker) Stopped() {
	if tracker == nil {
		return
	}
	tracker.mu.Lock()
	tracker.snapshot.Running = false
	tracker.mu.Unlock()
}

func (tracker *RuntimeHealthTracker) Succeeded() {
	if tracker == nil {
		return
	}
	now := tracker.now().UTC()
	tracker.mu.Lock()
	tracker.snapshot.LastAttemptUTC = now
	tracker.snapshot.LastSuccessUTC = now
	tracker.snapshot.ConsecutiveFailures = 0
	tracker.snapshot.FailureStage = ""
	tracker.mu.Unlock()
}

func (tracker *RuntimeHealthTracker) Failed(stage string) {
	if tracker == nil {
		return
	}
	now := tracker.now().UTC()
	tracker.mu.Lock()
	tracker.snapshot.LastAttemptUTC = now
	tracker.snapshot.LastFailureUTC = now
	tracker.snapshot.ConsecutiveFailures++
	tracker.snapshot.FailureStage = boundedStage(stage)
	tracker.mu.Unlock()
}

func (tracker *RuntimeHealthTracker) Snapshot() RuntimeHealthSnapshot {
	if tracker == nil {
		return RuntimeHealthSnapshot{}
	}
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()
	return tracker.snapshot
}

func boundedStage(stage string) string {
	stage = strings.TrimSpace(stage)
	if len(stage) > 64 {
		stage = stage[:64]
	}
	return stage
}
