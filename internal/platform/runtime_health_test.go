package platform

import (
	"strings"
	"testing"
	"time"
)

func TestRuntimeHealthTrackerKeepsBoundedFailureEvidence(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tracker := NewRuntimeHealthTracker(func() time.Time { return now })
	tracker.Started()
	tracker.Failed(strings.Repeat("x", 100))
	tracker.Failed("claim")
	snapshot := tracker.Snapshot()
	if !snapshot.Running || snapshot.ConsecutiveFailures != 2 || snapshot.FailureStage != "claim" || snapshot.LastFailureUTC != now {
		t.Fatalf("failed snapshot=%+v", snapshot)
	}
	now = now.Add(time.Minute)
	tracker.Succeeded()
	snapshot = tracker.Snapshot()
	if snapshot.ConsecutiveFailures != 0 || snapshot.FailureStage != "" || snapshot.LastSuccessUTC != now {
		t.Fatalf("successful snapshot=%+v", snapshot)
	}
	tracker.Stopped()
	if tracker.Snapshot().Running {
		t.Fatal("stopped tracker still reports running")
	}
}
