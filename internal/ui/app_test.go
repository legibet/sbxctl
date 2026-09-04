package ui

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

func asApp(t *testing.T, model tea.Model) app {
	t.Helper()
	a, ok := model.(app)
	if !ok {
		t.Fatalf("model is %T, want app", model)
	}
	return a
}

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
	a = asApp(t, model)
	if !strings.Contains(ansi.Strip(a.footer()), "Proceed? (y/n)") {
		t.Fatalf("confirmation footer = %q", a.footer())
	}
	model, command := a.Update(keyPress('y'))
	a = asApp(t, model)
	if a.confirm != nil || command == nil {
		t.Fatalf("confirmed state = %#v, command nil = %v", a.confirm, command == nil)
	}
	command()
	if !ran {
		t.Fatal("confirmation action did not run")
	}

	ran = false
	model, _ = a.Update(confirmMsg{question: "Proceed?", action: action})
	a = asApp(t, model)
	model, command = a.Update(keyPress('n'))
	a = asApp(t, model)
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

func TestAppViewDimensions(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {140, 40}} {
		t.Run(strconv.Itoa(size.width), func(t *testing.T) {
			a := newApp(sbx.Endpoint{URL: "http://127.0.0.1:9090"}, "home", &config.File{Targets: map[string]config.Target{}}, nil)
			a.overlay = overlayNone
			a.info = sbx.ServerInfo{Version: sbx.Version{Version: "1.14.0", APIVersion: 4}, StartedAt: time.Now().Add(-time.Hour)}
			a.connState = sbx.StateConnected
			a.width, a.height = size.width, size.height
			a.resizeWorkspaces()
			for active := range 3 {
				a.active = active
				assertViewDimensions(t, a.View().Content, size.width, size.height)
			}

			a.proxies.setGroups([]sbx.Group{{
				Tag: "proxy", Type: "selector", Selectable: true, Selected: "hk-01",
				Items: []sbx.Outbound{{Tag: "hk-01", Type: "shadowsocks", Delay: 142, TestedAt: time.Now().Add(-time.Minute)}},
			}})
			now := time.Now()
			a.connections.apply(sbx.ConnectionBatch{Reset: true, Events: []sbx.ConnectionEvent{
				newConnectionEvent(sbx.Connection{
					ID: "0123456789abcdef", Inbound: "mixed-in", InboundType: "mixed", Network: "tcp",
					Source: "127.0.0.1:12345", Destination: "1.1.1.1:443", Domain: "example.com",
					Rule: "domain_suffix", Outbound: "proxy", OutboundType: "selector",
					CreatedAt: now.Add(-time.Minute), Uplink: 1000, Downlink: 2000,
					UplinkTotal: 3000, DownlinkTotal: 4000, Chain: []string{"proxy", "hk-01"},
				}),
			}})
			a.logs.apply(sbx.LogBatch{Reset: true, Entries: []sbx.LogEntry{{Level: sbx.LevelInfo, Message: "\x1b[32mINFO[1]\x1b[0m ready"}}})
			for active := range 3 {
				a.active = active
				assertViewDimensions(t, a.View().Content, size.width, size.height)
			}
			a.active = 1
			a.overlay = overlayConnection
			assertViewDimensions(t, a.View().Content, size.width, size.height)
		})
	}
}

func assertViewDimensions(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("lines = %d, want %d", len(lines), height)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("line %d width = %d, want %d", i, got, width)
		}
	}
}
