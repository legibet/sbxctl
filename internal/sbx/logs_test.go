package sbx

import (
	"reflect"
	"testing"
)

func TestLogBufferResetAndTruncation(t *testing.T) {
	buffer := NewLogBuffer(3)
	buffer.Apply(LogBatch{Entries: []LogEntry{{Message: "one"}, {Message: "two"}}})
	buffer.Apply(LogBatch{Entries: []LogEntry{{Message: "three"}, {Message: "four"}}})
	if got := logMessages(buffer.Entries()); !reflect.DeepEqual(got, []string{"two", "three", "four"}) {
		t.Fatalf("messages = %v", got)
	}

	buffer.Apply(LogBatch{Reset: true, Entries: []LogEntry{{Message: "new"}}})
	if buffer.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", buffer.Len())
	}
	entries := buffer.Entries()
	entries[0].Message = "changed"
	if got := logMessages(buffer.Entries()); !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatalf("messages after copy mutation = %v", got)
	}
}

func logMessages(entries []LogEntry) []string {
	messages := make([]string, len(entries))
	for index, entry := range entries {
		messages[index] = entry.Message
	}
	return messages
}
