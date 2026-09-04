package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// theme maps roles onto the terminal's own 16-color palette so sbxctl follows
// whatever scheme the terminal is using. Text uses the default foreground.
type theme struct {
	accent  color.Color
	success color.Color
	warn    color.Color
	danger  color.Color
	dim     color.Color
	rule    color.Color

	title       lipgloss.Style
	dimText     lipgloss.Style
	accentText  lipgloss.Style
	selectedRow lipgloss.Style
	focusedRow  lipgloss.Style
	badgeOK     lipgloss.Style
	badgeWarn   lipgloss.Style
	badgeBad    lipgloss.Style
	border      lipgloss.Style
}

func newTheme() theme {
	t := theme{
		accent:  lipgloss.Color("4"),
		success: lipgloss.Color("2"),
		warn:    lipgloss.Color("3"),
		danger:  lipgloss.Color("1"),
		dim:     lipgloss.Color("8"),
		rule:    lipgloss.Color("8"),
	}
	t.title = lipgloss.NewStyle().Bold(true)
	t.dimText = lipgloss.NewStyle().Foreground(t.dim)
	t.accentText = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
	t.selectedRow = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
	t.focusedRow = lipgloss.NewStyle().Reverse(true)
	t.badgeOK = lipgloss.NewStyle().Foreground(t.success)
	t.badgeWarn = lipgloss.NewStyle().Foreground(t.warn)
	t.badgeBad = lipgloss.NewStyle().Foreground(t.danger)
	t.border = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.rule).Padding(0, 1)
	return t
}
