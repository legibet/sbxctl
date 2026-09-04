package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/legibet/sbxctl/internal/sbx"
)

type logsWorkspace struct {
	width, height int
	theme         theme
	keys          keyMap
	client        *sbx.Client

	buffer       *sbx.LogBuffer
	defaultLevel sbx.LogLevel
	level        sbx.LogLevel
	levelChanged bool
	filter       string
	rows         []sbx.LogEntry
	cursor       cursor
	follow       bool
	unread       int
}

func newLogs(t theme, keys keyMap, client *sbx.Client) logsWorkspace {
	return logsWorkspace{
		theme:        t,
		keys:         keys,
		client:       client,
		buffer:       sbx.NewLogBuffer(5000),
		defaultLevel: sbx.LevelInfo,
		level:        sbx.LevelInfo,
		follow:       true,
	}
}

func (w *logsWorkspace) setSize(width, height int) {
	w.width, w.height = width, height
	w.cursor.setHeight(max(0, height-1))
}

func (w *logsWorkspace) setClient(client *sbx.Client) { w.client = client }

func (w *logsWorkspace) reset() {
	height := w.cursor.height
	*w = logsWorkspace{
		width:        w.width,
		height:       w.height,
		theme:        w.theme,
		keys:         w.keys,
		client:       w.client,
		buffer:       sbx.NewLogBuffer(5000),
		defaultLevel: sbx.LevelInfo,
		level:        sbx.LevelInfo,
		cursor:       cursor{height: height},
		follow:       true,
	}
}

func (w *logsWorkspace) apply(batch sbx.LogBatch) {
	newVisible := 0
	if !batch.Reset && !w.follow {
		for _, entry := range batch.Entries {
			if w.matches(entry) {
				newVisible++
			}
		}
	}
	if batch.Reset {
		w.unread = 0
		w.cursor = cursor{height: w.cursor.height}
	}
	w.buffer.Apply(batch)
	w.rebuild()
	if w.follow {
		w.latest()
	} else {
		w.unread += newVisible
	}
}

func (w *logsWorkspace) rebuild() {
	entries := w.buffer.Entries()
	w.rows = make([]sbx.LogEntry, 0, len(entries))
	for _, entry := range entries {
		if w.matches(entry) {
			w.rows = append(w.rows, entry)
		}
	}
	w.cursor.setCount(len(w.rows))
}

func (w *logsWorkspace) matches(entry sbx.LogEntry) bool {
	return entry.Level <= w.level && strings.Contains(strings.ToLower(sbx.StripANSI(entry.Message)), strings.ToLower(w.filter))
}

func (w *logsWorkspace) setDefaultLevel(level sbx.LogLevel) {
	w.defaultLevel = level
	if !w.levelChanged {
		w.level = level
		w.rebuild()
		if w.follow {
			w.latest()
		}
	}
}

func (w *logsWorkspace) setFilter(text string) {
	w.filter = text
	w.rebuild()
	if w.follow {
		w.latest()
	}
}

func (*logsWorkspace) tick(time.Time) {}

func (w *logsWorkspace) bindings() []key.Binding {
	return []key.Binding{w.keys.move, w.keys.pause, w.keys.latest, w.keys.level, w.keys.clear}
}

func (w *logsWorkspace) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	switch k {
	case "space":
		w.follow = !w.follow
		if w.follow {
			w.latest()
		}
		return nil
	case "G", "end":
		w.follow = true
		w.latest()
		return nil
	case "L":
		w.levelChanged = true
		if w.level > sbx.LevelError {
			w.level--
		} else {
			w.level = w.defaultLevel
		}
		w.rebuild()
		if w.follow {
			w.latest()
		}
		return nil
	case "c":
		command := w.clearLogs()
		return func() tea.Msg { return confirmMsg{question: "Clear logs?", action: command} }
	}
	if isUpwardLogKey(k) {
		w.follow = false
	}
	w.cursor.handleKey(k)
	return nil
}

func isUpwardLogKey(k string) bool {
	switch k {
	case "k", "up", "ctrl+u", "ctrl+b", "pgup", "g", "home":
		return true
	default:
		return false
	}
}

func (w *logsWorkspace) latest() {
	w.cursor.setCount(len(w.rows))
	if len(w.rows) > 0 {
		w.cursor.index = len(w.rows) - 1
		w.cursor.visible()
	}
	w.unread = 0
}

func (w *logsWorkspace) clearLogs() tea.Cmd {
	if w.client == nil {
		return nil
	}
	client := w.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.ClearLogs(ctx)
		return actionMsg{notice: "logs cleared", err: err}
	}
}

func (w *logsWorkspace) view() string {
	header := "Logs  " + w.level.String() + "  " + strconv.Itoa(len(w.rows)) + " lines"
	if w.filter != "" {
		header += "  " + countSummary(len(w.rows), w.levelCount())
	}
	if w.follow {
		header += "  " + w.theme.dimText.Render("following")
	} else {
		paused := "paused  +" + strconv.Itoa(w.unread)
		header += "  " + w.theme.badgeWarn.Render(paused)
	}
	lines := []string{fitLine(w.theme.title.Render(header), w.width)}
	if len(w.rows) == 0 {
		content := lipgloss.Place(w.width, max(0, w.height-1), lipgloss.Center, lipgloss.Center,
			w.theme.dimText.Render("no logs"))
		lines = append(lines, strings.Split(content, "\n")...)
		return exactLines(lines, w.width, w.height)
	}
	start, end := w.cursor.visible()
	for position := start; position < end; position++ {
		message := w.rows[position].Message
		if w.filter != "" {
			message = fitLine(ansi.Truncate(sbx.StripANSI(message), w.width, ""), w.width)
			if position == w.cursor.index {
				message = highlightMatches(message, w.filter, w.theme.focusedRow, w.theme.accentText.Reverse(true))
			} else {
				message = highlightMatches(message, w.filter, lipgloss.NewStyle(), w.theme.accentText)
			}
		} else {
			message = fitLine(ansi.Truncate(message, w.width, ""), w.width)
			if position == w.cursor.index {
				message = w.theme.focusedRow.Render(ansi.Strip(message))
			}
		}
		lines = append(lines, message)
	}
	return exactLines(lines, w.width, w.height)
}

func (w *logsWorkspace) levelCount() int {
	count := 0
	for _, entry := range w.buffer.Entries() {
		if entry.Level <= w.level {
			count++
		}
	}
	return count
}

func highlightMatches(text, search string, normal, match lipgloss.Style) string {
	lowerText := strings.ToLower(text)
	lowerSearch := strings.ToLower(search)
	var highlighted strings.Builder
	for len(lowerText) > 0 {
		index := strings.Index(lowerText, lowerSearch)
		if index < 0 {
			highlighted.WriteString(normal.Render(text))
			break
		}
		highlighted.WriteString(normal.Render(text[:index]))
		highlighted.WriteString(match.Render(text[index : index+len(search)]))
		text = text[index+len(search):]
		lowerText = lowerText[index+len(lowerSearch):]
	}
	return highlighted.String()
}
