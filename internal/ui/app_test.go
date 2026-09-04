package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/legibet/sbxctl/internal/sbx"
)

func TestConfirmationRunsOnlyOnYes(t *testing.T) {
	a := newApp(sbx.Endpoint{}, "", nil, nil)
	a.overlay = overlayNone
	a.width = 80
	ran := false
	action := func() tea.Msg {
		ran = true
		return actionMsg{notice: "done"}
	}

	model, _ := a.Update(confirmMsg{question: "Proceed?", action: action})
	a = model.(app)
	if !strings.Contains(ansi.Strip(a.footer()), "Proceed? (y/n)") {
		t.Fatalf("confirmation footer = %q", a.footer())
	}
	model, command := a.Update(keyPress('y'))
	a = model.(app)
	if a.confirm != nil || command == nil {
		t.Fatalf("confirmed state = %#v, command nil = %v", a.confirm, command == nil)
	}
	command()
	if !ran {
		t.Fatal("confirmation action did not run")
	}

	ran = false
	model, _ = a.Update(confirmMsg{question: "Proceed?", action: action})
	a = model.(app)
	model, command = a.Update(keyPress('n'))
	a = model.(app)
	if a.confirm != nil || command != nil || ran {
		t.Fatalf("cancelled confirmation = %#v, command nil = %v, ran = %v", a.confirm, command == nil, ran)
	}
}

func TestSessionEventsRouteToWorkspaces(t *testing.T) {
	a := newApp(sbx.Endpoint{}, "", nil, nil)
	a.applySessionEvent(&sbx.ConnectionsEvent{Batch: sbx.ConnectionBatch{Reset: true, Events: []sbx.ConnectionEvent{
		newConnectionEvent(sbx.Connection{ID: "connection"}),
	}}})
	a.applySessionEvent(&sbx.LogsEvent{Batch: sbx.LogBatch{Reset: true, Entries: []sbx.LogEntry{{Level: sbx.LevelDebug, Message: "debug"}}}})
	if len(a.connections.rows) != 1 || a.logs.buffer.Len() != 1 {
		t.Fatalf("routed rows = connections %d, logs %d", len(a.connections.rows), a.logs.buffer.Len())
	}

	unavailable := errors.New("unsupported")
	a.applySessionEvent(&sbx.UnavailableEvent{Stream: sbx.StreamConnections, Err: unavailable})
	if !errors.Is(a.connections.unavailable, unavailable) {
		t.Fatalf("unavailable = %v", a.connections.unavailable)
	}
	a.applySessionEvent(&sbx.ConnEvent{State: sbx.StateConnected, Info: sbx.ServerInfo{DefaultLogLevel: sbx.LevelDebug}})
	if a.connections.unavailable != nil || a.logs.level != sbx.LevelDebug {
		t.Fatalf("connected state = unavailable %v, log level %s", a.connections.unavailable, a.logs.level)
	}
}

func TestUnicodeModeReportRepaintsScreen(t *testing.T) {
	a := newApp(sbx.Endpoint{}, "", nil, nil)
	_, command := a.Update(tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeReset})
	if command == nil {
		t.Fatal("mode 2027 report returned no command")
	}
	if command() != tea.ClearScreen() {
		t.Fatalf("mode 2027 report command = %#v, want tea.ClearScreen", command())
	}
	if _, command := a.Update(tea.ModeReportMsg{Mode: ansi.ModeSynchronizedOutput, Value: ansi.ModeReset}); command != nil {
		t.Fatal("unrelated mode report produced a command")
	}
}
