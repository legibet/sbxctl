package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/legibet/sbxctl/internal/sbx"
)

func TestConnectionsApplySortFilterAndSelection(t *testing.T) {
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	w := newConnections(newTheme(), newKeyMap(), nil)
	w.setSize(140, 37)
	w.tick(now)
	w.apply(sbx.ConnectionBatch{Reset: true, Events: []sbx.ConnectionEvent{
		newConnectionEvent(sbx.Connection{ID: "alpha", CreatedAt: now.Add(-3 * time.Minute), Domain: "Example.COM", Source: "10.0.0.1:1000", Destination: "1.1.1.1:443", Outbound: "direct"}),
		newConnectionEvent(sbx.Connection{ID: "bravo", CreatedAt: now.Add(-time.Minute), Source: "10.0.0.2:2000", Destination: "2.2.2.2:443", Outbound: "proxy", UplinkTotal: 500}),
		newConnectionEvent(sbx.Connection{ID: "charlie", CreatedAt: now.Add(-2 * time.Minute), Source: "10.0.0.3:3000", Destination: "3.3.3.3:443", Outbound: "proxy"}),
	}})
	w.apply(sbx.ConnectionBatch{Events: []sbx.ConnectionEvent{
		{Type: sbx.ConnectionUpdate, ID: "alpha", UplinkDelta: 100},
		{Type: sbx.ConnectionUpdate, ID: "bravo", UplinkDelta: 10},
		{Type: sbx.ConnectionUpdate, ID: "charlie", UplinkDelta: 50},
		{Type: sbx.ConnectionClosed, ID: "charlie", ClosedAt: now},
	}})

	assertConnectionOrder(t, w.rows, "bravo,alpha")
	w.selectedID = "bravo"
	w.sort = connSortRate
	w.rebuild()
	assertConnectionOrder(t, w.rows, "alpha,bravo")
	if got := w.rows[w.cursor.index].ID; got != "bravo" {
		t.Fatalf("selected row after rate sort = %q, want bravo", got)
	}
	w.sort = connSortTotal
	w.rebuild()
	assertConnectionOrder(t, w.rows, "bravo,alpha")

	w.setFilter("example.com")
	assertConnectionOrder(t, w.rows, "alpha")
	w.setFilter("")
	w.state = connClosed
	w.sort = connSortTime
	w.rebuild()
	assertConnectionOrder(t, w.rows, "charlie")
}

// The table is truncated to the terminal width when it is drawn, so columns
// that overflow disappear silently. Check the widths themselves instead.
func TestConnectionColumnsDropByPriority(t *testing.T) {
	for _, want := range []struct {
		width   int
		columns string
	}{
		{46, "destination outbound down"},
		{54, "destination outbound down age"},
		{66, "destination outbound up down age"},
		{90, "source destination outbound up down age"},
		{102, "inbound source destination outbound up down age"},
		{122, "inbound source destination rule outbound up down age"},
	} {
		c := (&connectionsWorkspace{width: want.width}).columns()
		names := make([]string, 0, 8)
		for _, column := range []struct {
			name  string
			width int
		}{
			{"inbound", c.inbound},
			{"source", c.source},
			{"destination", c.destination},
			{"rule", c.rule},
			{"outbound", c.outbound},
			{"up", c.up},
			{"down", c.down},
			{"age", c.age},
		} {
			if column.width > 0 {
				names = append(names, column.name)
			}
		}
		if got := strings.Join(names, " "); got != want.columns {
			t.Errorf("width %d: columns = %q, want %q", want.width, got, want.columns)
		}
		if c.width() != want.width {
			t.Errorf("width %d: columns occupy %d", want.width, c.width())
		}
	}
}

func newConnectionEvent(connection sbx.Connection) sbx.ConnectionEvent {
	return sbx.ConnectionEvent{Type: sbx.ConnectionNew, ID: connection.ID, Connection: &connection}
}

func assertConnectionOrder(t *testing.T, rows []sbx.Connection, want string) {
	t.Helper()
	ids := make([]string, len(rows))
	for index := range rows {
		ids[index] = rows[index].ID
	}
	if got := strings.Join(ids, ","); got != want {
		t.Fatalf("connection order = %q, want %q", got, want)
	}
}
