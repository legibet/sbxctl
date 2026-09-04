package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayTargets
	overlayConnection
)

type targetEntry struct {
	name   string
	url    string
	target sbx.Endpoint
	active bool
}

func renderOverlay(title, content string, width, height int, t theme) string {
	contentLines := strings.Split(content, "\n")
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
	leftWidth, rightWidth := 0, 0
	for _, binding := range global {
		help := binding.Help()
		leftWidth = max(leftWidth, 12+lipgloss.Width(help.Desc))
	}
	for _, binding := range workspace {
		help := binding.Help()
		rightWidth = max(rightWidth, 12+lipgloss.Width(help.Desc))
	}
	rows := max(len(global), len(workspace))
	lines := []string{t.title.Render(cell("Global", leftWidth, false)) + "    " + t.title.Render(cell("Workspace", rightWidth, false))}
	for i := range rows {
		left, right := "", ""
		if i < len(global) {
			help := global[i].Help()
			left = t.accentText.Render(cell(help.Key, 12, false)) + help.Desc
		}
		if i < len(workspace) {
			help := workspace[i].Help()
			right = t.accentText.Render(cell(help.Key, 12, false)) + help.Desc
		}
		lines = append(lines, cell(left, leftWidth, false)+"    "+cell(right, rightWidth, false))
	}
	return renderOverlay("Help", strings.Join(lines, "\n"), width, height, t)
}

func targetEntries(file *config.File, activeName string, activeEndpoint sbx.Endpoint) []targetEntry {
	entries := make([]targetEntry, 0, len(file.Targets)+1)
	if activeName == "" && activeEndpoint.URL != "" {
		entries = append(entries, targetEntry{name: "(url)", url: activeEndpoint.URL, target: activeEndpoint, active: true})
	}
	for _, name := range file.Names() {
		target := file.Targets[name]
		entries = append(entries, targetEntry{
			name: name,
			url:  target.URL,
			target: sbx.Endpoint{
				URL:        target.URL,
				Secret:     target.Secret,
				CAFile:     target.TLS.CAFile,
				ServerName: target.TLS.ServerName,
				Insecure:   target.TLS.Insecure,
			},
			active: name == activeName,
		})
	}
	return entries
}

func targetsOverlay(entries []targetEntry, selected cursor, width, height int, t theme) string {
	if len(entries) == 0 {
		return renderOverlay("Targets", "No targets. Add one with: sbxctl target add <name> <url>", width, height, t)
	}
	selected.setCount(len(entries))
	start, end := selected.visible()
	nameWidth := 0
	for _, entry := range entries {
		nameWidth = max(nameWidth, lipgloss.Width(entry.name))
	}
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		entry := entries[i]
		marker := "  "
		if entry.active {
			marker = t.accentText.Render("● ")
		}
		line := marker + cell(entry.name, nameWidth, false) + "  " + entry.url
		if i == selected.index {
			line = t.focusedRow.Render(ansi.Strip(line))
		}
		lines = append(lines, line)
	}
	return renderOverlay("Targets", strings.Join(lines, "\n"), width, height, t)
}
