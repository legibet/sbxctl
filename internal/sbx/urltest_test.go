package sbx

import (
	"slices"
	"testing"
	"time"
)

func TestTestTracker(t *testing.T) {
	startedAt := time.Unix(100, 0)
	tracker := NewTestTracker(
		[]string{"ok", "failed", "timeout"},
		map[string]Outbound{
			"ok":     {Tag: "ok", TestedAt: startedAt.Add(-time.Minute)},
			"failed": {Tag: "failed", TestedAt: startedAt.Add(-time.Minute)},
		},
		startedAt,
		10*time.Second,
	)
	tracker.Observe([]Outbound{
		{Tag: "ok", TestedAt: startedAt.Add(time.Second), Delay: 42},
		{Tag: "failed"},
		{Tag: "timeout"},
	}, startedAt.Add(time.Second))
	if tracker.Done() {
		t.Fatal("Done() before timeout = true")
	}

	tracker.Observe(nil, startedAt.Add(10*time.Second))
	want := []TestResult{
		{Tag: "ok", State: TestOK, Delay: 42},
		{Tag: "failed", State: TestFailed},
		{Tag: "timeout", State: TestTimeout},
	}
	if got := tracker.Results(); !slices.Equal(got, want) {
		t.Fatalf("Results() = %#v, want %#v", got, want)
	}
	if !tracker.Done() {
		t.Fatal("Done() after timeout = false")
	}
}
