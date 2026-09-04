package sbx

import (
	"slices"
	"testing"
	"time"
)

func TestConnectionTableApply(t *testing.T) {
	created := time.Unix(100, 0)
	closed := time.Unix(200, 0)
	table := NewConnectionTable(2, 2*time.Second)
	table.Apply(ConnectionBatch{Reset: true, Events: []ConnectionEvent{
		{Type: ConnectionNew, ID: "active-1", Connection: &Connection{ID: "active-1", CreatedAt: created}},
		{Type: ConnectionNew, ID: "closed-1", Connection: &Connection{ID: "closed-1", CreatedAt: created, ClosedAt: closed}},
	}})
	if len(table.Active()) != 1 || len(table.Closed()) != 1 {
		t.Fatalf("after reset snapshot: active %d closed %d", len(table.Active()), len(table.Closed()))
	}

	table.Apply(ConnectionBatch{Events: []ConnectionEvent{
		{Type: ConnectionNew, ID: "active-1", Connection: &Connection{ID: "active-1", UplinkTotal: 9999}},
	}})
	connection, _ := table.Get("active-1")
	if connection.UplinkTotal != 0 {
		t.Fatalf("duplicate NEW overwrote totals = %#v", connection)
	}

	table.Apply(ConnectionBatch{Events: []ConnectionEvent{
		{Type: ConnectionUpdate, ID: "active-1", UplinkDelta: 200, DownlinkDelta: 100},
	}})
	connection, _ = table.Get("active-1")
	if connection.Uplink != 100 || connection.Downlink != 50 || connection.UplinkTotal != 200 || connection.DownlinkTotal != 100 {
		t.Fatalf("updated connection = %#v", connection)
	}

	closedAt := time.Unix(300, 0)
	table.Apply(ConnectionBatch{Events: []ConnectionEvent{{Type: ConnectionClosed, ID: "active-1", ClosedAt: closedAt}}})
	connection, ok := table.Get("active-1")
	if !ok || !connection.ClosedAt.Equal(closedAt) || connection.Uplink != 0 || connection.Downlink != 0 {
		t.Fatalf("closed connection = %#v, %v", connection, ok)
	}
}

func TestConnectionTableClosedLimitAndOrder(t *testing.T) {
	table := NewConnectionTable(2, time.Second)
	for index, id := range []string{"oldest", "middle", "newest"} {
		closedAt := time.Unix(int64(index+1), 0)
		table.Apply(ConnectionBatch{Events: []ConnectionEvent{{
			Type:       ConnectionClosed,
			ID:         id,
			Connection: &Connection{ID: id, CreatedAt: closedAt.Add(-time.Second)},
			ClosedAt:   closedAt,
		}}})
	}
	closed := table.Closed()
	if got := []string{closed[0].ID, closed[1].ID}; !slices.Equal(got, []string{"newest", "middle"}) {
		t.Fatalf("closed IDs = %v", got)
	}
	if _, ok := table.Get("oldest"); ok {
		t.Fatal("oldest closed connection was retained")
	}
}

func TestConnectionTableGetPrefix(t *testing.T) {
	table := NewConnectionTable(10, time.Second)
	table.Apply(ConnectionBatch{Events: []ConnectionEvent{
		{Type: ConnectionNew, ID: "abcd-1111", Connection: &Connection{}},
		{Type: ConnectionNew, ID: "abcd-2222", Connection: &Connection{}},
		{Type: ConnectionNew, ID: "wxyz-3333", Connection: &Connection{}},
	}})
	if matches := table.Find("abc"); len(matches) != 0 {
		t.Fatalf("Find(abc) = %d matches, want 0 (prefix too short)", len(matches))
	}
	if matches := table.Find("abcd"); len(matches) != 2 {
		t.Fatalf("Find(abcd) = %d matches, want 2", len(matches))
	}
	if matches := table.Find("wxyz"); len(matches) != 1 || matches[0].ID != "wxyz-3333" {
		t.Fatalf("Find(wxyz) = %#v", matches)
	}
	if matches := table.Find("abcd-1111"); len(matches) != 1 || matches[0].ID != "abcd-1111" {
		t.Fatalf("Find(exact) = %#v", matches)
	}
}

func TestConnectionTableReset(t *testing.T) {
	table := NewConnectionTable(10, time.Second)
	table.Apply(ConnectionBatch{Events: []ConnectionEvent{{Type: ConnectionNew, ID: "first", Connection: &Connection{}}}})
	table.Apply(ConnectionBatch{Reset: true, Events: []ConnectionEvent{{Type: ConnectionNew, ID: "second", Connection: &Connection{}}}})
	if _, ok := table.Get("first"); ok {
		t.Fatal("reset retained old connection")
	}
	if _, ok := table.Get("second"); !ok {
		t.Fatal("reset missing new connection")
	}
}
