package ui

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/legibet/sbxctl/internal/sbx"
)

type proxySort string

const (
	sortUpstream proxySort = "upstream"
	sortName     proxySort = "name"
	sortDelay    proxySort = "delay"
)

type proxiesWorkspace struct {
	width, height int
	theme         theme
	keys          keyMap
	client        *sbx.Client

	groups    []sbx.Group
	outbounds []sbx.Outbound
	left      cursor
	right     cursor
	focus     int
	sort      proxySort
	reverse   bool
	filter    string
	pending   map[string]time.Time
	now       time.Time
}

func newProxies(t theme, keys keyMap, client *sbx.Client) proxiesWorkspace {
	return proxiesWorkspace{
		theme:   t,
		keys:    keys,
		client:  client,
		sort:    sortUpstream,
		pending: make(map[string]time.Time),
		now:     time.Now(),
	}
}

func (w *proxiesWorkspace) setSize(width, height int) {
	w.width, w.height = width, height
	w.left.setHeight(max(0, height-1))
	w.right.setHeight(max(0, height-1))
	w.updateCounts()
}

func (w *proxiesWorkspace) setClient(client *sbx.Client) { w.client = client }

func (w *proxiesWorkspace) setGroups(groups []sbx.Group) {
	w.groups = groups
	w.finishPendingGroups(groups)
	w.updateCounts()
}

func (w *proxiesWorkspace) setOutbounds(outbounds []sbx.Outbound) {
	w.outbounds = outbounds
	w.finishPending(outbounds)
	w.updateCounts()
}

func (w *proxiesWorkspace) reset() {
	w.groups = nil
	w.outbounds = nil
	w.left = cursor{height: max(0, w.height-1)}
	w.right = cursor{height: max(0, w.height-1)}
	w.focus = 0
	w.filter = ""
	w.pending = make(map[string]time.Time)
}

func (w *proxiesWorkspace) setFilter(text string) {
	w.filter = text
	w.updateCounts()
}

func (w *proxiesWorkspace) bindings() []key.Binding {
	return []key.Binding{w.keys.move, w.keys.selectItem, w.keys.test, w.keys.testGroup, w.keys.pane, w.keys.sort, w.keys.reverse}
}

func (w *proxiesWorkspace) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	narrow := w.width < 100
	if narrow && w.focus == 1 && (k == "esc" || k == "h" || k == "left") {
		w.focus = 0
		w.updateCounts()
		return nil
	}

	switch k {
	case "h", "left":
		w.focus = 0
		w.updateCounts()
		return nil
	case "l", "right":
		w.focusRight()
		return nil
	case "enter":
		if w.focus == 0 {
			w.focusRight()
			w.right.index = 0
			w.right.offset = 0
			return nil
		}
		return w.selectOutbound()
	case "t":
		return w.testFocused()
	case "T":
		return w.testCurrentGroup()
	case "s":
		switch w.sort {
		case sortUpstream:
			w.sort = sortName
		case sortName:
			w.sort = sortDelay
		default:
			w.sort = sortUpstream
		}
		w.right.index = 0
		w.right.offset = 0
		return nil
	case "S":
		w.reverse = !w.reverse
		w.right.index = 0
		w.right.offset = 0
		return nil
	}

	before := w.left.index
	side := &w.right
	if w.focus == 0 {
		side = &w.left
	}
	if side.handleKey(k) && w.focus == 0 && w.left.index != before {
		w.right.index = 0
		w.right.offset = 0
		w.updateCounts()
	}
	return nil
}

func (w *proxiesWorkspace) view() string {
	w.updateCounts()
	if w.width < 100 {
		if w.focus == 0 {
			return w.leftView(w.width)
		}
		return w.rightView(w.width)
	}
	leftWidth := max(26, w.width/3)
	rightWidth := w.width - leftWidth - 1
	left := strings.Split(w.leftView(leftWidth), "\n")
	right := strings.Split(w.rightView(rightWidth), "\n")
	lines := make([]string, w.height)
	rule := w.theme.dimText.Foreground(w.theme.rule).Render("│")
	for i := range lines {
		lines[i] = left[i] + rule + right[i]
	}
	return strings.Join(lines, "\n")
}

func (w *proxiesWorkspace) leftView(width int) string {
	rows := w.leftRows()
	header := "Groups"
	if w.filter != "" && w.focus == 0 {
		header += "  " + countSummary(len(rows), len(w.groups)+1)
	}
	if w.focus == 0 {
		header = w.theme.title.Render(header)
	} else {
		header = w.theme.dimText.Render(header)
	}
	countWidth := 5
	typeWidth := min(10, max(0, width/4))
	remaining := max(0, width-countWidth-typeWidth-2)
	// Group tags are usually shorter than the outbound they select, so give the
	// name column what the tags actually need and leave the rest to the outbound.
	nameWidth := 0
	for _, group := range w.groups {
		nameWidth = max(nameWidth, ansi.StringWidth(group.Tag))
	}
	nameWidth = min(nameWidth, remaining/2)
	selectedWidth := max(0, remaining-nameWidth)

	lines := []string{fitLine(header, width)}
	start, end := w.left.visible()
	for position := start; position < end; position++ {
		groupIndex := rows[position]
		var line string
		if groupIndex == len(w.groups) {
			name := "All outbounds"
			if position == w.left.index {
				name = w.theme.accentText.Render(name)
			}
			line = cell(name, max(0, width-6), false) + w.theme.dimText.Render(cell("", 1, false)+cell(strconv.Itoa(len(w.outbounds)), 5, true))
		} else {
			group := w.groups[groupIndex]
			name := group.Tag
			if position == w.left.index {
				name = w.theme.accentText.Render(name)
			}
			line = cell(name, nameWidth, false) + " " +
				w.theme.dimText.Render(cell(group.Type, typeWidth, false)) + " " +
				cell(group.Selected, selectedWidth, false) +
				w.theme.dimText.Render(cell(strconv.Itoa(len(group.Items)), countWidth, true))
		}
		line = fitLine(line, width)
		if position == w.left.index && w.focus == 0 {
			line = w.theme.focusedRow.Render(ansi.Strip(line))
		}
		lines = append(lines, line)
	}
	return exactLines(lines, width, w.height)
}

func (w *proxiesWorkspace) rightView(width int) string {
	items := w.rightItems()
	total := w.rightTotal()
	group, all := w.currentGroup()
	header := "All outbounds  " + strconv.Itoa(len(items))
	if !all && group != nil {
		header = group.Tag + "  " + group.Type + "  " + strconv.Itoa(len(items)) + " items  sort " + string(w.sort) + w.sortArrow()
	} else if all {
		header += "  sort " + string(w.sort) + w.sortArrow()
	}
	if w.filter != "" && w.focus == 1 {
		header += "  " + countSummary(len(items), total)
	}
	if w.focus == 1 {
		header = w.theme.title.Render(header)
	} else {
		header = w.theme.dimText.Render(header)
	}
	lines := []string{fitLine(header, width)}
	start, end := w.right.visible()
	for position := start; position < end; position++ {
		outbound := items[position]
		marker := "  "
		if group != nil && !all && outbound.Tag == group.Selected {
			marker = w.theme.accentText.Render("● ")
		}
		typeWidth, delayWidth, agoWidth := 12, 8, 8
		if width < 60 {
			agoWidth = 0
		}
		nameWidth := max(0, width-2-typeWidth-delayWidth-agoWidth)
		name := outbound.Tag
		if group != nil && !all && outbound.Tag == group.Selected {
			name = w.theme.selectedRow.Render(name)
		}
		line := marker + cell(name, nameWidth, false) +
			w.theme.dimText.Render(cell(outbound.Type, typeWidth, false)) +
			cell(delayCell(w.theme, outbound, w.isPending(outbound.Tag)), delayWidth, true)
		if agoWidth > 0 {
			line += cell(agoCell(w.theme, outbound, w.now), agoWidth, true)
		}
		line = fitLine(line, width)
		if position == w.right.index && w.focus == 1 {
			line = w.theme.focusedRow.Render(ansi.Strip(line))
		}
		lines = append(lines, line)
	}
	return exactLines(lines, width, w.height)
}

func (w *proxiesWorkspace) leftRows() []int {
	rows := make([]int, 0, len(w.groups)+1)
	query := strings.ToLower(w.filter)
	for i, group := range w.groups {
		if query == "" || w.focus != 0 || strings.Contains(strings.ToLower(group.Tag), query) || strings.Contains(strings.ToLower(group.Type), query) || strings.Contains(strings.ToLower(group.Selected), query) {
			rows = append(rows, i)
		}
	}
	if query == "" || w.focus != 0 || strings.Contains("all outbounds", query) {
		rows = append(rows, len(w.groups))
	}
	return rows
}

func (w *proxiesWorkspace) currentGroup() (*sbx.Group, bool) {
	rows := w.leftRows()
	if len(rows) == 0 {
		return nil, false
	}
	position := min(w.left.index, len(rows)-1)
	index := rows[position]
	if index == len(w.groups) {
		return nil, true
	}
	return &w.groups[index], false
}

func (w *proxiesWorkspace) rightItems() []sbx.Outbound {
	group, all := w.currentGroup()
	var source []sbx.Outbound
	if all {
		source = w.outbounds
	} else if group != nil {
		source = group.Items
	}
	items := append([]sbx.Outbound(nil), source...)
	if w.reverse && w.sort == sortUpstream {
		slices.Reverse(items)
	} else if w.sort != sortUpstream {
		sort.SliceStable(items, func(i, j int) bool {
			a, b := items[i], items[j]
			if w.sort == sortDelay {
				aMissing, bMissing := a.TestedAt.IsZero(), b.TestedAt.IsZero()
				if aMissing != bMissing {
					return !aMissing
				}
				if aMissing {
					return false
				}
				if w.reverse {
					return a.Delay > b.Delay
				}
				return a.Delay < b.Delay
			}
			if w.reverse {
				return strings.ToLower(a.Tag) > strings.ToLower(b.Tag)
			}
			return strings.ToLower(a.Tag) < strings.ToLower(b.Tag)
		})
	}
	if w.filter != "" && w.focus == 1 {
		query := strings.ToLower(w.filter)
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Tag), query) || strings.Contains(strings.ToLower(item.Type), query) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	return items
}

func (w *proxiesWorkspace) rightTotal() int {
	group, all := w.currentGroup()
	if all {
		return len(w.outbounds)
	}
	if group != nil {
		return len(group.Items)
	}
	return 0
}

func (w *proxiesWorkspace) updateCounts() {
	w.left.setCount(len(w.leftRows()))
	w.right.setCount(len(w.rightItems()))
}

func (w *proxiesWorkspace) needsOutbounds() bool {
	_, all := w.currentGroup()
	return all
}

func (w *proxiesWorkspace) focusRight() {
	group, all := w.currentGroup()
	tag := ""
	if group != nil {
		tag = group.Tag
	}
	w.focus = 1
	rows := w.leftRows()
	for position, index := range rows {
		if (all && index == len(w.groups)) || (!all && index < len(w.groups) && w.groups[index].Tag == tag) {
			w.left.index = position
			break
		}
	}
	w.updateCounts()
}

func (w *proxiesWorkspace) filterSummary() string {
	if w.filter == "" {
		return ""
	}
	if w.focus == 0 {
		return "/" + w.filter + "  " + countSummary(len(w.leftRows()), len(w.groups)+1)
	}
	return "/" + w.filter + "  " + countSummary(len(w.rightItems()), w.rightTotal())
}

func (w *proxiesWorkspace) tick(now time.Time) {
	w.now = now
	for tag, triggeredAt := range w.pending {
		if !now.Before(triggeredAt.Add(10 * time.Second)) {
			delete(w.pending, tag)
		}
	}
}

func (w *proxiesWorkspace) selectOutbound() tea.Cmd {
	group, all := w.currentGroup()
	items := w.rightItems()
	if all || group == nil || !group.Selectable || len(items) == 0 || w.client == nil {
		return nil
	}
	outbound := items[min(w.right.index, len(items)-1)]
	client := w.client
	groupTag := group.Tag
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.SelectOutbound(ctx, groupTag, outbound.Tag)
		return actionMsg{notice: "selected " + outbound.Tag, err: err}
	}
}

func (w *proxiesWorkspace) testFocused() tea.Cmd {
	if w.focus == 0 {
		return w.testCurrentGroup()
	}
	items := w.rightItems()
	if len(items) == 0 || w.client == nil {
		return nil
	}
	outbound := items[min(w.right.index, len(items)-1)]
	w.pending[outbound.Tag] = w.now
	return urlTestCmd(w.client, outbound.Tag)
}

func (w *proxiesWorkspace) testCurrentGroup() tea.Cmd {
	group, all := w.currentGroup()
	if all || group == nil {
		return func() tea.Msg { return actionMsg{err: errors.New("no group selected")} }
	}
	if w.client == nil {
		return nil
	}
	for _, item := range group.Items {
		w.pending[item.Tag] = w.now
	}
	return urlTestCmd(w.client, group.Tag)
}

func urlTestCmd(client *sbx.Client, tag string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.URLTest(ctx, tag)
		return actionMsg{notice: "url test triggered", err: err}
	}
}

func (w *proxiesWorkspace) finishPendingGroups(groups []sbx.Group) {
	for _, group := range groups {
		w.finishPending(group.Items)
	}
}

func (w *proxiesWorkspace) finishPending(items []sbx.Outbound) {
	for _, item := range items {
		if triggeredAt, ok := w.pending[item.Tag]; ok && item.TestedAt.After(triggeredAt) {
			delete(w.pending, item.Tag)
		}
	}
}

func (w *proxiesWorkspace) isPending(tag string) bool {
	_, ok := w.pending[tag]
	return ok
}

func (w *proxiesWorkspace) sortArrow() string {
	if w.reverse {
		return "↓"
	}
	return "↑"
}

func countSummary(count, total int) string {
	return strconv.Itoa(count) + "/" + strconv.Itoa(total)
}
