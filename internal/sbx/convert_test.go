package sbx

import (
	"testing"
	"time"

	"github.com/legibet/sbxctl/internal/daemon"
)

func TestConvertTimestampUnits(t *testing.T) {
	outbound := convertOutbound(&daemon.GroupItem{UrlTestTime: 1_700_000_000})
	if !outbound.TestedAt.Equal(time.Unix(1_700_000_000, 0)) {
		t.Fatalf("outbound TestedAt = %v, want Unix seconds", outbound.TestedAt)
	}
	if !convertOutbound(&daemon.GroupItem{}).TestedAt.IsZero() {
		t.Fatal("missing UrlTestTime produced non-zero TestedAt")
	}

	connection := convertConnection(&daemon.Connection{
		CreatedAt: 1_700_000_000,
		ClosedAt:  1_700_000_001,
	})
	if !connection.CreatedAt.Equal(time.UnixMilli(1_700_000_000)) {
		t.Fatalf("connection CreatedAt = %v, want Unix milliseconds", connection.CreatedAt)
	}
	if !connection.ClosedAt.Equal(time.UnixMilli(1_700_000_001)) {
		t.Fatalf("connection ClosedAt = %v, want Unix milliseconds", connection.ClosedAt)
	}
	if !convertConnection(&daemon.Connection{}).CreatedAt.IsZero() {
		t.Fatal("missing CreatedAt produced non-zero time")
	}
}
