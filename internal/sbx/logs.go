package sbx

type LogBuffer struct {
	limit   int
	entries []LogEntry
}

func NewLogBuffer(limit int) *LogBuffer {
	if limit < 0 {
		limit = 0
	}
	return &LogBuffer{limit: limit}
}

func (b *LogBuffer) Apply(batch LogBatch) {
	if batch.Reset {
		b.entries = nil
	}
	b.entries = append(b.entries, batch.Entries...)
	if excess := len(b.entries) - b.limit; excess > 0 {
		copy(b.entries, b.entries[excess:])
		b.entries = b.entries[:b.limit]
	}
}

func (b *LogBuffer) Entries() []LogEntry {
	return append([]LogEntry(nil), b.entries...)
}

func (b *LogBuffer) Len() int {
	return len(b.entries)
}
