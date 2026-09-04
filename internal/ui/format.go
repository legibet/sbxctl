package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/legibet/sbxctl/internal/sbx"
)

func fitLine(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = ansi.Truncate(text, width, "")
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

func cell(text string, width int, right bool) string {
	text = ansi.Truncate(text, max(0, width), "")
	padding := strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
	if right {
		return padding + text
	}
	return text + padding
}

func exactLines(lines []string, width, height int) string {
	result := make([]string, height)
	for i := range result {
		if i < len(lines) {
			result[i] = fitLine(lines[i], width)
		} else {
			result[i] = strings.Repeat(" ", max(0, width))
		}
	}
	return strings.Join(result, "\n")
}

func delayCell(t theme, outbound sbx.Outbound, pending bool) string {
	if pending {
		return t.dimText.Render("testing")
	}
	if outbound.TestedAt.IsZero() {
		return t.dimText.Render("-")
	}
	value := fmt.Sprintf("%dms", outbound.Delay)
	switch {
	case outbound.Delay <= 200:
		return t.badgeOK.Render(value)
	case outbound.Delay <= 500:
		return t.badgeWarn.Render(value)
	default:
		return t.badgeBad.Render(value)
	}
}

func agoCell(t theme, outbound sbx.Outbound, now time.Time) string {
	return t.dimText.Render(sbx.FormatAgo(outbound.TestedAt, now))
}
