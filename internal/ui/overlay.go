package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayServers
	overlayConnection
)

func renderOverlay(title, content string, width, height int, t theme) string {
	contentLines := strings.Split(content, "\n")
	// Trim the content rather than the finished box, so the border stays closed.
	if limit := max(0, height-2); len(contentLines) > limit {
		contentLines = contentLines[:limit]
	}
	contentWidth := 0
	for _, line := range contentLines {
		contentWidth = max(contentWidth, lipgloss.Width(line))
	}
	innerWidth := max(contentWidth+2, lipgloss.Width(title)+4)
	innerWidth = min(innerWidth, max(1, width-4))
	topFill := max(0, innerWidth-lipgloss.Width(title)-3)
	top := t.dimText.Foreground(t.rule).Render("╭─ ") + t.title.Render(title) + t.dimText.Foreground(t.rule).Render(" "+strings.Repeat("─", topFill)+"╮")
	box := []string{fitLine(top, innerWidth+2)}
	for _, line := range contentLines {
		body := " " + cell(line, max(0, innerWidth-2), false) + " "
		box = append(box, t.dimText.Foreground(t.rule).Render("│")+body+t.dimText.Foreground(t.rule).Render("│"))
	}
	box = append(box, t.dimText.Foreground(t.rule).Render("╰"+strings.Repeat("─", innerWidth)+"╯"))
	placed := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, strings.Join(box, "\n"))
	return exactLines(strings.Split(placed, "\n"), width, height)
}

func helpOverlay(keys keyMap, workspace []key.Binding, modeAvailable bool, width, height int, t theme) string {
	global := globalBindings(keys, modeAvailable)
	entry := func(binding key.Binding) string {
		help := binding.Help()
		return t.accentText.Render(cell(help.Key, 12, false)) + help.Desc
	}
	columnWidth := func(bindings []key.Binding) int {
		result := 0
		for _, binding := range bindings {
			result = max(result, 12+lipgloss.Width(binding.Help().Desc))
		}
		return result
	}
	leftWidth, rightWidth := columnWidth(global), columnWidth(workspace)

	var lines []string
	if leftWidth+4+rightWidth <= max(0, width-4) {
		lines = append(lines, t.title.Render(cell("Global", leftWidth, false))+"    "+t.title.Render(cell("Workspace", rightWidth, false)))
		for i := range max(len(global), len(workspace)) {
			left, right := "", ""
			if i < len(global) {
				left = entry(global[i])
			}
			if i < len(workspace) {
				right = entry(workspace[i])
			}
			lines = append(lines, cell(left, leftWidth, false)+"    "+cell(right, rightWidth, false))
		}
	} else {
		lines = append(lines, t.title.Render("Global"))
		for _, binding := range global {
			lines = append(lines, entry(binding))
		}
		lines = append(lines, "", t.title.Render("Workspace"))
		for _, binding := range workspace {
			lines = append(lines, entry(binding))
		}
	}
	return renderOverlay("Help", strings.Join(lines, "\n"), width, height, t)
}
