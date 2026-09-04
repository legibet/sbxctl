package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	proxies, connections, logs  key.Binding
	next, previous              key.Binding
	filter, help, targets       key.Binding
	reconnect, mode, quit       key.Binding
	move, selectItem, details   key.Binding
	test, testGroup             key.Binding
	pane, sort, reverse         key.Binding
	state, closeConn, closeAll  key.Binding
	pause, latest, level, clear key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		proxies:     key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "proxies")),
		connections: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "connections")),
		logs:        key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "logs")),
		next:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next workspace")),
		previous:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous workspace")),
		filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		targets:     key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "targets")),
		reconnect:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reconnect")),
		mode:        key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "cycle mode")),
		quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		move:        key.NewBinding(key.WithKeys("j", "k", "up", "down", "g", "G", "ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "pgup", "pgdown", "home", "end"), key.WithHelp("j/k", "move")),
		selectItem:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		details:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		test:        key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "test")),
		testGroup:   key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "test group")),
		pane:        key.NewBinding(key.WithKeys("h", "l", "left", "right"), key.WithHelp("h/l", "switch pane")),
		sort:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		reverse:     key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "reverse")),
		state:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "active/closed/all")),
		closeConn:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "close")),
		closeAll:    key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "close all")),
		pause:       key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "pause/resume")),
		latest:      key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "latest")),
		level:       key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "level")),
		clear:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
	}
}

func globalBindings(keys keyMap, modeAvailable bool) []key.Binding {
	bindings := []key.Binding{
		keys.proxies, keys.connections, keys.logs, keys.next, keys.previous,
		keys.filter, keys.help, keys.targets, keys.reconnect,
	}
	if modeAvailable {
		bindings = append(bindings, keys.mode)
	}
	return append(bindings, keys.quit)
}
