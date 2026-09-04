package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/legibet/sbxctl/internal/sbx"
)

func TestLogsLevelPauseLatestAndFilter(t *testing.T) {
	w := newLogs(newTheme(), newKeyMap(), nil)
	w.setSize(80, 21)
	w.setDefaultLevel(sbx.LevelDebug)
	for _, want := range []sbx.LogLevel{sbx.LevelInfo, sbx.LevelWarn, sbx.LevelError, sbx.LevelDebug} {
		w.handleKey(keyPress('L'))
		if w.level != want {
			t.Fatalf("level after cycle = %s, want %s", w.level, want)
		}
	}

	w.apply(sbx.LogBatch{Reset: true, Entries: []sbx.LogEntry{
		{Level: sbx.LevelInfo, Message: "\x1b[32mINFO[1]\x1b[0m ready"},
		{Level: sbx.LevelWarn, Message: "\x1b[33mWARN[2]\x1b[0m first Needle"},
	}})
	w.handleKey(keyPress(' '))
	w.apply(sbx.LogBatch{Entries: []sbx.LogEntry{
		{Level: sbx.LevelDebug, Message: "DEBUG[3] second needle"},
		{Level: sbx.LevelError, Message: "ERROR[4] failed"},
	}})
	if w.follow || w.unread != 2 {
		t.Fatalf("paused state = follow %v unread %d, want false and 2", w.follow, w.unread)
	}
	w.handleKey(keyPress('G'))
	if !w.follow || w.unread != 0 || w.cursor.index != len(w.rows)-1 {
		t.Fatalf("latest state = follow %v unread %d index %d", w.follow, w.unread, w.cursor.index)
	}

	w.setFilter("needle")
	view := w.view()
	if !strings.Contains(view, w.theme.accentText.Render("Needle")) {
		t.Fatalf("filtered view does not contain accent-styled match: %q", view)
	}
	stripped := ansi.Strip(view)
	for _, message := range []string{"WARN[2] first Needle", "DEBUG[3] second needle"} {
		if !strings.Contains(stripped, message) {
			t.Fatalf("filtered view does not contain %q", message)
		}
	}
}

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: string(code)}
}
