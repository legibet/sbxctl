package ui

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/legibet/sbxctl/internal/sbx"
)

type connState string

const (
	connActive connState = "active"
	connClosed connState = "closed"
	connAll    connState = "all"
)

type connSort string

const (
	connSortTime  connSort = "time"
	connSortRate  connSort = "rate"
	connSortTotal connSort = "total"
)

type connectionsWorkspace struct {
	width, height int
	theme         theme
	keys          keyMap
	client        *sbx.Client

	table       *sbx.ConnectionTable
	unavailable error
	state       connState
	sort        connSort
	reverse     bool
	filter      string
	cursor      cursor
	selectedID  string
	rows        []sbx.Connection
	now         time.Time
}

func newConnections(t theme, keys keyMap, client *sbx.Client) connectionsWorkspace {
	w := connectionsWorkspace{
		theme:  t,
		keys:   keys,
		client: client,
		table:  sbx.NewConnectionTable(1000, time.Second),
		state:  connActive,
		sort:   connSortTime,
		now:    time.Now(),
	}
	w.rebuild()
	return w
}

func (w *connectionsWorkspace) setSize(width, height int) {
	w.width, w.height = width, height
	w.cursor.setHeight(max(0, height-2))
}

func (w *connectionsWorkspace) setClient(client *sbx.Client) { w.client = client }

func (w *connectionsWorkspace) reset() {
	height := w.cursor.height
	*w = connectionsWorkspace{
		width:  w.width,
		height: w.height,
		theme:  w.theme,
		keys:   w.keys,
		client: w.client,
		table:  sbx.NewConnectionTable(1000, time.Second),
		state:  connActive,
		sort:   connSortTime,
		now:    time.Now(),
		cursor: cursor{height: height},
	}
}

func (w *connectionsWorkspace) apply(batch sbx.ConnectionBatch) {
	w.table.Apply(batch)
	w.rebuild()
}

func (w *connectionsWorkspace) setUnavailable(err error) { w.unavailable = err }

func (w *connectionsWorkspace) setFilter(text string) {
	w.filter = text
	w.rebuild()
}

func (w *connectionsWorkspace) tick(now time.Time) { w.now = now }

func (w *connectionsWorkspace) bindings() []key.Binding {
	return []key.Binding{w.keys.move, w.keys.details, w.keys.state, w.keys.closeConn, w.keys.closeAll, w.keys.sort, w.keys.reverse}
}

func (w *connectionsWorkspace) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if len(w.rows) > 0 {
			return func() tea.Msg { return overlayConnection }
		}
	case "a":
		switch w.state {
		case connActive:
			w.state = connClosed
		case connClosed:
			w.state = connAll
		default:
			w.state = connActive
		}
		w.rebuild()
	case "x":
		return w.closeFocused()
	case "X":
		command := w.closeAll()
		return func() tea.Msg { return confirmMsg{question: "Close all connections?", action: command} }
	case "s":
		switch w.sort {
		case connSortTime:
			w.sort = connSortRate
		case connSortRate:
			w.sort = connSortTotal
		default:
			w.sort = connSortTime
		}
		w.rebuild()
	case "S":
		w.reverse = !w.reverse
		w.rebuild()
	default:
		w.move(msg.String())
	}
	return nil
}

func (w *connectionsWorkspace) move(k string) {
	if w.cursor.handleKey(k) {
		w.syncSelected()
	}
}

func (w *connectionsWorkspace) rebuild() {
	w.rows = w.stateRows()
	query := strings.ToLower(w.filter)
	if query != "" {
		filtered := w.rows[:0]
		for _, connection := range w.rows {
			if connectionMatches(connection, query) {
				filtered = append(filtered, connection)
			}
		}
		w.rows = filtered
	}
	sort.Slice(w.rows, func(i, j int) bool {
		a, b := w.rows[i], w.rows[j]
		order := w.compare(a, b)
		if order == 0 {
			return a.ID < b.ID
		}
		if w.reverse {
			return order < 0
		}
		return order > 0
	})
	w.cursor.setCount(len(w.rows))
	for index := range w.rows {
		if w.rows[index].ID == w.selectedID {
			w.cursor.index = index
			w.cursor.visible()
			break
		}
	}
	w.syncSelected()
}

func (w *connectionsWorkspace) stateRows() []sbx.Connection {
	switch w.state {
	case connClosed:
		return w.table.Closed()
	case connAll:
		return w.table.All()
	default:
		return w.table.Active()
	}
}

func (w *connectionsWorkspace) compare(a, b sbx.Connection) int {
	switch w.sort {
	case connSortRate:
		return compareInt64(a.Uplink+a.Downlink, b.Uplink+b.Downlink)
	case connSortTotal:
		return compareInt64(a.UplinkTotal+a.DownlinkTotal, b.UplinkTotal+b.DownlinkTotal)
	default:
		aTime, bTime := a.CreatedAt, b.CreatedAt
		if w.state == connClosed {
			aTime, bTime = a.ClosedAt, b.ClosedAt
		}
		switch {
		case aTime.After(bTime):
			return 1
		case aTime.Before(bTime):
			return -1
		default:
			return 0
		}
	}
}

func compareInt64(a, b int64) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}

func connectionMatches(connection sbx.Connection, query string) bool {
	values := []string{
		connection.Source,
		connection.Destination,
		connection.Domain,
		connection.Inbound,
		connection.Outbound,
		connection.Rule,
	}
	if connection.Process != nil {
		values = append(values, connection.Process.Path)
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func (w *connectionsWorkspace) syncSelected() {
	if len(w.rows) == 0 {
		w.selectedID = ""
		return
	}
	w.selectedID = w.rows[w.cursor.index].ID
}

func (w *connectionsWorkspace) view() string {
	if w.unavailable != nil {
		content := lipgloss.Place(w.width, w.height, lipgloss.Center, lipgloss.Center,
			w.theme.dimText.Render("connection tracking is not available on this server"))
		return exactLines(strings.Split(content, "\n"), w.width, w.height)
	}

	stateName := strings.ToUpper(string(w.state[:1])) + string(w.state[1:])
	header := stateName + " " + strconv.Itoa(len(w.rows)) + "  sort " + string(w.sort) + w.sortArrow()
	if w.filter != "" {
		header += "  " + countSummary(len(w.rows), len(w.stateRows()))
	}
	lines := []string{fitLine(w.theme.title.Render(header), w.width), w.columnHeader()}
	if len(w.rows) == 0 {
		content := lipgloss.Place(w.width, max(0, w.height-2), lipgloss.Center, lipgloss.Center,
			w.theme.dimText.Render("no connections"))
		lines = append(lines, strings.Split(content, "\n")...)
		return exactLines(lines, w.width, w.height)
	}

	start, end := w.cursor.visible()
	for position := start; position < end; position++ {
		line := w.connectionRow(w.rows[position])
		if position == w.cursor.index {
			line = w.theme.focusedRow.Render(ansi.Strip(line))
		} else if !w.rows[position].ClosedAt.IsZero() {
			line = w.theme.dimText.Render(ansi.Strip(line))
		}
		lines = append(lines, fitLine(line, w.width))
	}
	return exactLines(lines, w.width, w.height)
}

type connectionColumns struct {
	inbound, source, destination, rule, outbound, up, down, age int
}

func (w *connectionsWorkspace) columns() connectionColumns {
	columns := connectionColumns{source: 22, destination: 16, outbound: 18, up: 10, down: 10, age: 6}
	count := 6
	if w.width >= 110 {
		columns.inbound = 10
		count++
	}
	if w.width >= 130 {
		columns.rule = 18
		count++
	}
	fixed := columns.inbound + columns.source + columns.rule + columns.outbound + columns.up + columns.down + columns.age + (count-1)*2
	columns.destination = max(16, w.width-fixed)
	if overflow := fixed + columns.destination - w.width; overflow > 0 {
		reduction := min(overflow, columns.source-16)
		columns.source -= reduction
		overflow -= reduction
		columns.outbound -= min(overflow, columns.outbound-12)
	}
	return columns
}

func (w *connectionsWorkspace) columnHeader() string {
	c := w.columns()
	parts := make([]string, 0, 8)
	if c.inbound > 0 {
		parts = append(parts, cell("INBOUND", c.inbound, false))
	}
	parts = append(parts, cell("SOURCE", c.source, false), cell("DESTINATION", c.destination, false))
	if c.rule > 0 {
		parts = append(parts, cell("RULE", c.rule, false))
	}
	parts = append(parts,
		cell("OUTBOUND", c.outbound, false),
		cell("UP", c.up, true),
		cell("DOWN", c.down, true),
		cell("AGE", c.age, true),
	)
	return fitLine(w.theme.dimText.Render(strings.Join(parts, "  ")), w.width)
}

func (w *connectionsWorkspace) connectionRow(connection sbx.Connection) string {
	c := w.columns()
	destination := connection.Destination
	if connection.Domain != "" {
		destination = connection.Domain
	}
	up, down := connection.Uplink, connection.Downlink
	if w.state != connActive {
		up, down = connection.UplinkTotal, connection.DownlinkTotal
	}
	age := w.now.Sub(connection.CreatedAt)
	if !connection.ClosedAt.IsZero() {
		age = connection.ClosedAt.Sub(connection.CreatedAt)
	}
	parts := make([]string, 0, 8)
	if c.inbound > 0 {
		parts = append(parts, cell(connection.Inbound, c.inbound, false))
	}
	parts = append(parts, cell(connection.Source, c.source, false), cell(destination, c.destination, false))
	if c.rule > 0 {
		parts = append(parts, w.theme.dimText.Render(cell(connection.Rule, c.rule, false)))
	}
	parts = append(parts,
		cell(connection.Outbound, c.outbound, false),
		cell(w.trafficValue(up), c.up, true),
		cell(w.trafficValue(down), c.down, true),
		cell(sbx.FormatShortDuration(age), c.age, true),
	)
	return strings.Join(parts, "  ")
}

func (w *connectionsWorkspace) trafficValue(value int64) string {
	if value == 0 && w.state == connActive {
		return w.theme.dimText.Render("-")
	}
	if w.state == connActive {
		return sbx.FormatRate(value)
	}
	return sbx.FormatBytes(value)
}

func (w *connectionsWorkspace) sortArrow() string {
	if w.reverse {
		return "↑"
	}
	return "↓"
}

func (w *connectionsWorkspace) closeFocused() tea.Cmd {
	if len(w.rows) == 0 {
		return nil
	}
	connection := w.rows[w.cursor.index]
	if !connection.ClosedAt.IsZero() {
		return func() tea.Msg { return actionMsg{notice: "connection already closed"} }
	}
	if w.client == nil {
		return nil
	}
	client := w.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.CloseConnection(ctx, connection.ID)
		return actionMsg{notice: "closed " + shortConnectionID(connection.ID), err: err}
	}
}

func (w *connectionsWorkspace) closeAll() tea.Cmd {
	if w.client == nil {
		return nil
	}
	client := w.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.CloseAllConnections(ctx)
		return actionMsg{notice: "closed all connections", err: err}
	}
}

func shortConnectionID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func (w *connectionsWorkspace) detailView(width, height int) string {
	if len(w.rows) == 0 {
		return w.theme.dimText.Render("no connection selected")
	}
	connection := w.rows[w.cursor.index]
	closed := !connection.ClosedAt.IsZero()
	state := "active"
	if closed {
		state = "closed"
	}
	duration := w.now.Sub(connection.CreatedAt)
	if closed {
		duration = connection.ClosedAt.Sub(connection.CreatedAt)
	}
	lines := make([]string, 0, 22)
	add := func(label, value string) {
		if value == "" {
			return
		}
		line := w.theme.dimText.Render(cell(label, 14, false)) + "  " + value
		lines = append(lines, fitLine(line, width))
	}
	add("id", connection.ID)
	add("state", state)
	add("inbound", typedName(connection.Inbound, connection.InboundType))
	add("network", connection.Network)
	add("source", connection.Source)
	add("destination", connection.Destination)
	add("domain", connection.Domain)
	add("protocol", connection.Protocol)
	add("user", connection.User)
	add("rule", connection.Rule)
	add("outbound", typedName(connection.Outbound, connection.OutboundType))
	add("chain", strings.Join(connection.Chain, " > "))
	add("from outbound", connection.FromOutbound)
	add("created", connection.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	closedAt := "-"
	if closed {
		closedAt = connection.ClosedAt.Local().Format("2006-01-02 15:04:05")
	}
	add("closed", closedAt)
	add("duration", sbx.FormatDuration(duration))
	if closed {
		add("uplink", sbx.FormatBytes(connection.UplinkTotal))
		add("downlink", sbx.FormatBytes(connection.DownlinkTotal))
	} else {
		add("uplink", sbx.FormatRate(connection.Uplink)+"  "+sbx.FormatBytes(connection.UplinkTotal))
		add("downlink", sbx.FormatRate(connection.Downlink)+"  "+sbx.FormatBytes(connection.DownlinkTotal))
	}
	if process := connection.Process; process != nil {
		if process.PID != 0 {
			add("process pid", strconv.FormatUint(uint64(process.PID), 10))
		}
		add("process user", process.UserName)
		add("process path", process.Path)
		add("process packages", strings.Join(process.PackageNames, ", "))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func typedName(name, kind string) string {
	if kind == "" {
		return name
	}
	return name + " (" + kind + ")"
}
