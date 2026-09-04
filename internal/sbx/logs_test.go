package sbx

import (
	"slices"
	"testing"
)

func TestLogBufferResetAndTruncation(t *testing.T) {
	buffer := NewLogBuffer(3)
	buffer.Apply(LogBatch{Entries: []LogEntry{{Message: "one"}, {Message: "two"}}})
	buffer.Apply(LogBatch{Entries: []LogEntry{{Message: "three"}, {Message: "four"}}})
	if got := logMessages(buffer.Entries()); !slices.Equal(got, []string{"two", "three", "four"}) {
		t.Fatalf("messages = %v", got)
	}

	buffer.Apply(LogBatch{Reset: true, Entries: []LogEntry{{Message: "new"}}})
	if got := logMessages(buffer.Entries()); !slices.Equal(got, []string{"new"}) {
		t.Fatalf("messages after reset = %v", got)
	}
}

func logMessages(entries []LogEntry) []string {
	messages := make([]string, len(entries))
	for index, entry := range entries {
		messages[index] = entry.Message
	}
	return messages
}
