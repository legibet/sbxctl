package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type connectionsWorkspace struct {
	width, height int
}

func (w *connectionsWorkspace) setSize(width, height int)       { w.width, w.height = width, height }
func (*connectionsWorkspace) handleKey(tea.KeyPressMsg) tea.Cmd { return nil }
func (*connectionsWorkspace) setFilter(string)                  {}
func (*connectionsWorkspace) bindings() []key.Binding           { return nil }
func (w *connectionsWorkspace) view() string {
	content := lipgloss.Place(w.width, w.height, lipgloss.Center, lipgloss.Center, "Connections")
	return exactLines(strings.Split(content, "\n"), w.width, w.height)
}
