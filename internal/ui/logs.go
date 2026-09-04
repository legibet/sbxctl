package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type logsWorkspace struct {
	width, height int
}

func (w *logsWorkspace) setSize(width, height int)       { w.width, w.height = width, height }
func (*logsWorkspace) handleKey(tea.KeyPressMsg) tea.Cmd { return nil }
func (*logsWorkspace) setFilter(string)                  {}
func (*logsWorkspace) bindings() []key.Binding           { return nil }
func (w *logsWorkspace) view() string {
	content := lipgloss.Place(w.width, w.height, lipgloss.Center, lipgloss.Center, "Logs")
	return exactLines(strings.Split(content, "\n"), w.width, w.height)
}
