package sbx

import (
	"fmt"
	"time"
)

type TestState int

const (
	TestPending TestState = iota
	TestOK
	TestFailed
	TestTimeout
)

func (s TestState) MarshalText() ([]byte, error) {
	switch s {
	case TestPending:
		return []byte("pending"), nil
	case TestOK:
		return []byte("ok"), nil
	case TestFailed:
		return []byte("failed"), nil
	case TestTimeout:
		return []byte("timeout"), nil
	default:
		return nil, fmt.Errorf("invalid test state %d", s)
	}
}

type TestResult struct {
	Tag   string    `json:"tag"`
	State TestState `json:"state"`
	Delay int       `json:"delay,omitempty"`
}

type TestTracker struct {
	results   []TestResult
	hadRecord map[string]bool
	startedAt time.Time
	deadline  time.Time
}

func NewTestTracker(tags []string, before map[string]Outbound, startedAt time.Time, timeout time.Duration) *TestTracker {
	results := make([]TestResult, len(tags))
	hadRecord := make(map[string]bool, len(tags))
	for index, tag := range tags {
		results[index] = TestResult{Tag: tag, State: TestPending}
		hadRecord[tag] = !before[tag].TestedAt.IsZero()
	}
	return &TestTracker{
		results:   results,
		hadRecord: hadRecord,
		startedAt: startedAt,
		deadline:  startedAt.Add(timeout),
	}
}

func (t *TestTracker) Observe(items []Outbound, now time.Time) {
	byTag := make(map[string]Outbound, len(items))
	for _, item := range items {
		byTag[item.Tag] = item
	}
	for index := range t.results {
		result := &t.results[index]
		if result.State != TestPending {
			continue
		}
		if item, ok := byTag[result.Tag]; ok {
			if item.TestedAt.After(t.startedAt) {
				result.State = TestOK
				result.Delay = item.Delay
				continue
			}
			if t.hadRecord[result.Tag] && item.TestedAt.IsZero() {
				result.State = TestFailed
				continue
			}
		}
		if !now.Before(t.deadline) {
			result.State = TestTimeout
		}
	}
}

func (t *TestTracker) Done() bool {
	for _, result := range t.results {
		if result.State == TestPending {
			return false
		}
	}
	return true
}

func (t *TestTracker) Results() []TestResult {
	return append([]TestResult(nil), t.results...)
}
