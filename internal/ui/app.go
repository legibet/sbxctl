package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

type workspace interface {
	setSize(width, height int)
	handleKey(msg tea.KeyPressMsg) tea.Cmd
	setFilter(text string)
	tick(now time.Time)
	view() string
	bindings() []key.Binding
}

type confirmMsg struct {
	question string
	action   tea.Cmd
}

type actionMsg struct {
	notice string
	err    error
}

type sessionEventMsg struct {
	session *sbx.Session
	event   sbx.Event
	ok      bool
}

type (
	tickMsg          time.Time
	noticeExpiredMsg uint64
)

type notice struct {
	text   string
	danger bool
	id     uint64
}

type app struct {
	endpoint sbx.Endpoint
	name     string
	file     *config.File
	session  *sbx.Session

	width, height int
	theme         theme
	keys          keyMap
	active        int
	overlay       overlayKind
	serverCursor  cursor
	serverForm    *serverForm
	serverDelete  *serverEntry
	serverError   string
	filter        textinput.Model
	filters       [3]string
	confirm       *confirmMsg
	notice        notice

	connState   sbx.ConnState
	connAttempt int
	connErr     error
	info        sbx.ServerInfo
	status      sbx.Status
	mode        string
	modes       []string

	proxies     proxiesWorkspace
	connections connectionsWorkspace
	logs        logsWorkspace
}

func newApp(ep sbx.Endpoint, name string, file *config.File, session *sbx.Session) app {
	if file == nil {
		file = &config.File{Servers: make(map[string]config.Server)}
	}
	t := newTheme()
	keys := newKeyMap()
	input := textinput.New()
	input.Prompt = "/"
	input.SetWidth(40)
	a := app{
		endpoint:  ep,
		name:      name,
		file:      file,
		session:   session,
		theme:     t,
		keys:      keys,
		connState: sbx.StateConnecting,
		filter:    input,
	}
	var client *sbx.Client
	if session != nil {
		client = session.Client()
	}
	a.proxies = newProxies(t, keys, client)
	a.connections = newConnections(t, keys, client)
	a.logs = newLogs(t, keys, client)
	if session == nil {
		a.overlay = overlayServers
	}
	return a
}

func (a app) Init() tea.Cmd {
	commands := []tea.Cmd{tick()}
	if a.session != nil {
		a.session.Start()
		commands = append(commands, waitSession(a.session))
	}
	return tea.Batch(commands...)
}

func (a app) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.resizeWorkspaces()
		a.filter.SetWidth(max(1, a.width-1))
		a.serverCursor.setHeight(a.serverBodyHeight() - 2)
		if a.serverForm != nil {
			a.serverForm.setWidth(a.serverWidth())
		}
		return a, nil
	case tea.ModeReportMsg:
		// Bubble Tea switches to grapheme-cluster widths (mode 2027) once the
		// terminal answers its startup query, but frames already drawn used
		// wcwidth and can have wrapped on emoji such as flags. Repaint from
		// scratch so nothing from those frames is left on screen.
		if msg.Mode == ansi.ModeUnicodeCore {
			return a, tea.ClearScreen
		}
		return a, nil
	case sessionEventMsg:
		if msg.session != a.session || !msg.ok {
			return a, nil
		}
		a.applySessionEvent(msg.event)
		a.syncStreams()
		return a, waitSession(msg.session)
	case tickMsg:
		a.currentWorkspace().tick(time.Time(msg))
		return a, tick()
	case confirmMsg:
		confirm := msg
		a.confirm = &confirm
		return a, nil
	case overlayKind:
		a.overlay = msg
		return a, nil
	case noticeExpiredMsg:
		if a.notice.id == uint64(msg) {
			a.notice.text = ""
			a.notice.danger = false
		}
		return a, nil
	case actionMsg:
		if msg.err != nil {
			return a.withNotice(msg.err.Error(), true)
		}
		return a.withNotice(msg.notice, false)
	}

	if a.overlay == overlayServers && a.serverForm != nil {
		command := a.handleServerForm(message)
		return a, command
	}
	msg, ok := message.(tea.KeyPressMsg)
	if !ok {
		return a, nil
	}
	if a.notice.text != "" {
		a.notice.text = ""
		a.notice.danger = false
	}
	if a.confirm != nil {
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		action := a.confirm.action
		a.confirm = nil
		if msg.String() == "y" {
			return a, action
		}
		return a, nil
	}
	if a.overlay != overlayNone {
		return a.handleOverlay(msg)
	}
	if a.filter.Focused() {
		return a.handleFilter(msg)
	}
	if command, matched := a.handleGlobal(msg); matched {
		return a, command
	}
	command := a.currentWorkspace().handleKey(msg)
	if a.active == 0 {
		a.syncStreams()
	}
	return a, command
}

func (a app) View() tea.View {
	var content string
	if a.width < 46 || a.height < 14 {
		content = lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, "terminal too small (46x14 minimum)")
		content = exactLines(strings.Split(content, "\n"), a.width, a.height)
	} else {
		workspaceView := a.currentWorkspace().view()
		switch a.overlay {
		case overlayHelp:
			workspaceView = helpOverlay(a.keys, a.currentWorkspace().bindings(), len(a.modes) > 0, a.width, a.height-3, a.theme)
		case overlayServers:
			workspaceView = a.serversView()
		case overlayConnection:
			detail := a.connections.detailView(max(1, a.width-8), max(1, a.height-7))
			workspaceView = renderOverlay("Connection", detail, a.width, a.height-3, a.theme)
		}
		lines := []string{a.topBar(), a.tabs()}
		lines = append(lines, strings.Split(workspaceView, "\n")...)
		lines = append(lines, a.footer())
		content = exactLines(lines, a.width, a.height)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "sbxctl"
	return view
}

func (a *app) applySessionEvent(event sbx.Event) {
	switch event := event.(type) {
	case *sbx.ConnEvent:
		a.connState = event.State
		a.connAttempt = event.Attempt
		a.connErr = event.Err
		if event.State == sbx.StateConnected {
			a.info = event.Info
			a.logs.setDefaultLevel(event.Info.DefaultLogLevel)
			a.connections.setUnavailable(nil)
		}
	case *sbx.StatusEvent:
		a.status = event.Status
	case *sbx.GroupsEvent:
		a.proxies.setGroups(event.Groups)
	case *sbx.OutboundsEvent:
		a.proxies.setOutbounds(event.Outbounds)
	case *sbx.ConnectionsEvent:
		a.connections.apply(event.Batch)
	case *sbx.LogsEvent:
		a.logs.apply(event.Batch)
	case *sbx.ClashModeEvent:
		a.mode = event.Mode
		if event.Modes != nil {
			a.modes = event.Modes
		}
	case *sbx.UnavailableEvent:
		switch event.Stream {
		case sbx.StreamClashMode:
			a.mode = ""
			a.modes = nil
		case sbx.StreamConnections:
			a.connections.setUnavailable(event.Err)
		}
	}
}

func (a *app) handleGlobal(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if msg.String() == "esc" && a.filters[a.active] != "" {
		a.filters[a.active] = ""
		a.filter.SetValue("")
		a.currentWorkspace().setFilter("")
		return nil, true
	}
	switch {
	case key.Matches(msg, a.keys.proxies):
		a.setActive(0)
		return nil, true
	case key.Matches(msg, a.keys.connections):
		a.setActive(1)
		return nil, true
	case key.Matches(msg, a.keys.logs):
		a.setActive(2)
		return nil, true
	case key.Matches(msg, a.keys.next):
		a.setActive((a.active + 1) % 3)
		return nil, true
	case key.Matches(msg, a.keys.previous):
		a.setActive((a.active + 2) % 3)
		return nil, true
	case key.Matches(msg, a.keys.filter):
		a.filter.SetValue(a.filters[a.active])
		return a.filter.Focus(), true
	case key.Matches(msg, a.keys.help):
		a.overlay = overlayHelp
		return nil, true
	case key.Matches(msg, a.keys.servers):
		a.overlay = overlayServers
		a.serverError = ""
		a.selectActiveServer()
		return nil, true
	case key.Matches(msg, a.keys.reconnect):
		if a.session != nil && a.connState != sbx.StateFailed {
			a.session.Reconnect()
			return nil, true
		}
		if a.endpoint.URL != "" {
			return a.replaceSession(a.endpoint, a.name), true
		}
		return nil, true
	case key.Matches(msg, a.keys.mode):
		return a.cycleMode(), true
	case key.Matches(msg, a.keys.quit):
		return tea.Quit, true
	}
	return nil, false
}

func (a *app) handleFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return *a, tea.Quit
	case "esc":
		a.filter.SetValue("")
		a.filter.Blur()
		a.filters[a.active] = ""
		a.currentWorkspace().setFilter("")
		return *a, nil
	case "enter":
		a.filter.Blur()
		return *a, nil
	}
	input, command := a.filter.Update(msg)
	a.filter = input
	a.filters[a.active] = input.Value()
	a.currentWorkspace().setFilter(input.Value())
	return *a, command
}

func (a *app) handleOverlay(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if a.overlay == overlayConnection {
		switch k {
		case "ctrl+c":
			return *a, tea.Quit
		case "esc", "enter", "q":
			a.overlay = overlayNone
			return *a, nil
		case "x":
			a.overlay = overlayNone
			return *a, a.connections.closeFocused()
		case "j", "k":
			a.connections.move(k)
		}
		return *a, nil
	}
	if a.overlay == overlayHelp {
		if k == "?" || k == "esc" || k == "q" {
			a.overlay = overlayNone
			return *a, nil
		}
		if k == "ctrl+c" {
			return *a, tea.Quit
		}
		return *a, nil
	}
	command := a.handleServers(k)
	return *a, command
}

func (a *app) replaceSession(endpoint sbx.Endpoint, name string) tea.Cmd {
	a.disconnect()
	a.endpoint = endpoint
	a.name = name
	session, err := sbx.NewSession(endpoint)
	if err != nil {
		a.connState = sbx.StateFailed
		a.connErr = err
		a.serverError = err.Error()
		_, command := a.withNotice(err.Error(), true)
		return command
	}
	a.session = session
	a.serverError = ""
	a.proxies.setClient(session.Client())
	a.connections.setClient(session.Client())
	a.logs.setClient(session.Client())
	a.overlay = overlayNone
	session.Start()
	a.syncStreams()
	return waitSession(session)
}

func (a *app) disconnect() {
	if a.session != nil {
		a.session.Close()
		a.session = nil
	}
	a.endpoint = sbx.Endpoint{}
	a.name = ""
	a.connState = sbx.StateConnecting
	a.connAttempt = 0
	a.connErr = nil
	a.info = sbx.ServerInfo{}
	a.status = sbx.Status{}
	a.mode = ""
	a.modes = nil
	a.filters = [3]string{}
	a.filter.SetValue("")
	a.filter.Blur()
	a.proxies.reset()
	a.proxies.setClient(nil)
	a.connections.reset()
	a.connections.setClient(nil)
	a.logs.reset()
	a.logs.setClient(nil)
}

func (a *app) cycleMode() tea.Cmd {
	if a.session == nil || len(a.modes) == 0 {
		return nil
	}
	next := 0
	for i, mode := range a.modes {
		if strings.EqualFold(mode, a.mode) {
			next = (i + 1) % len(a.modes)
			break
		}
	}
	mode := a.modes[next]
	client := a.session.Client()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.SetClashMode(ctx, mode)
		return actionMsg{notice: "mode " + mode, err: err}
	}
}

func (a *app) setActive(active int) {
	a.active = active
	a.filter.SetValue(a.filters[active])
	a.currentWorkspace().setFilter(a.filters[active])
	a.syncStreams()
}

func (a *app) syncStreams() {
	if a.session == nil {
		return
	}
	a.session.SetStream(sbx.StreamOutbounds, a.active == 0 && a.proxies.needsOutbounds())
	a.session.SetStream(sbx.StreamConnections, a.active == 1)
	a.session.SetStream(sbx.StreamLogs, a.active == 2)
}

func (a *app) selectActiveServer() {
	entries := a.serverEntries()
	a.serverCursor.setCount(len(entries))
	for i, entry := range entries {
		if entry.active {
			a.serverCursor.index = i
			return
		}
	}
}

func (a *app) currentWorkspace() workspace {
	switch a.active {
	case 1:
		return &a.connections
	case 2:
		return &a.logs
	default:
		return &a.proxies
	}
}

func (a *app) resizeWorkspaces() {
	height := max(0, a.height-3)
	a.proxies.setSize(a.width, height)
	a.connections.setSize(a.width, height)
	a.logs.setSize(a.width, height)
}

func (a app) topBar() string {
	server := a.name
	if server == "" && a.endpoint.URL != "" {
		server = "Unsaved"
	}
	if server == "" {
		return fitLine(a.theme.accentText.Render("sbxctl")+"  No server selected", a.width)
	}
	var state string
	switch a.connState {
	case sbx.StateConnected:
		state = a.theme.badgeOK.Render("●") + " " + a.info.Version.Version
	case sbx.StateReconnecting:
		state = a.theme.badgeWarn.Render("●") + " reconnecting (" + strconv.Itoa(a.connAttempt) + ")"
	case sbx.StateFailed:
		message := "failed"
		if a.connErr != nil {
			message = a.connErr.Error()
		}
		state = a.theme.badgeBad.Render("●") + " " + message
	default:
		state = a.theme.badgeWarn.Render("●") + " connecting"
	}
	// Uptime, mode and rates are dropped in that order when the bar does not fit.
	optional := make([]string, 0, 3)
	if !a.info.StartedAt.IsZero() {
		optional = append(optional, "up "+sbx.FormatDuration(time.Since(a.info.StartedAt)))
	}
	if a.mode != "" {
		optional = append(optional, a.mode)
	}
	if a.status.TrafficAvailable {
		optional = append(optional, "↑ "+sbx.FormatRate(a.status.Uplink)+" ↓ "+sbx.FormatRate(a.status.Downlink))
	}
	required := []string{a.theme.accentText.Render(server), state}
	line := strings.Join(slices.Concat(required, optional), "  ")
	for len(optional) > 0 && lipgloss.Width(line) > a.width {
		optional = optional[1:]
		line = strings.Join(slices.Concat(required, optional), "  ")
	}
	return fitLine(line, a.width)
}

func (a app) tabs() string {
	tabs := []string{"1 Proxies", "2 Connections", "3 Logs"}
	if a.status.TrafficAvailable {
		tabs[1] += " " + strconv.Itoa(a.status.ConnectionsIn)
	}
	for i := range tabs {
		if i == a.active {
			tabs[i] = a.theme.accentText.Render(tabs[i])
		} else {
			tabs[i] = a.theme.dimText.Render(tabs[i])
		}
	}
	return fitLine(strings.Join(tabs, "   "), a.width)
}

func (a app) footer() string {
	if a.overlay == overlayServers {
		return fitLine(a.theme.dimText.Render(configPath()), a.width)
	}
	if a.filter.Focused() {
		return fitLine(a.filter.View(), a.width)
	}
	if a.confirm != nil {
		return fitLine(a.theme.badgeBad.Render(a.confirm.question+" (y/n)"), a.width)
	}
	right := ""
	if a.notice.text != "" {
		if a.notice.danger {
			right = a.theme.badgeBad.Render(a.notice.text)
		} else {
			right = a.theme.badgeOK.Render(a.notice.text)
		}
	} else if a.active == 0 {
		right = a.proxies.filterSummary()
	}
	budget := a.width
	if right != "" {
		budget = max(0, a.width-lipgloss.Width(right)-2)
	}
	// Workspace keys go first when the hints do not fit, then servers. Help
	// always survives, because the help overlay lists every binding.
	workspace := a.currentWorkspace().bindings()
	if len(workspace) > 4 {
		workspace = workspace[:4]
	}
	tail := []key.Binding{a.keys.servers, a.keys.help}
	left := bindingHints(slices.Concat(workspace, tail))
	for lipgloss.Width(left) > budget && (len(workspace) > 0 || len(tail) > 1) {
		if len(workspace) > 0 {
			workspace = workspace[:len(workspace)-1]
		} else {
			tail = tail[1:]
		}
		left = bindingHints(slices.Concat(workspace, tail))
	}
	if right == "" {
		return fitLine(a.theme.dimText.Render(left), a.width)
	}
	left = a.theme.dimText.Render(left)
	gap := strings.Repeat(" ", max(2, a.width-lipgloss.Width(left)-lipgloss.Width(right)))
	return fitLine(left+gap+right, a.width)
}

func (a *app) withNotice(text string, danger bool) (tea.Model, tea.Cmd) {
	a.notice.id++
	a.notice.text = text
	a.notice.danger = danger
	id := a.notice.id
	return *a, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return noticeExpiredMsg(id) })
}

func waitSession(session *sbx.Session) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-session.Events()
		return sessionEventMsg{session: session, event: event, ok: ok}
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return tickMsg(now) })
}
