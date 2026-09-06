package ui

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

type serverEntry struct {
	name   string
	server config.Server
	active bool
}

func serverEndpoint(s config.Server) sbx.Endpoint {
	return sbx.Endpoint{
		URL:        s.URL,
		Secret:     s.Secret,
		CAFile:     s.TLS.CAFile,
		ServerName: s.TLS.ServerName,
		Insecure:   s.TLS.Insecure,
	}
}

func (a app) serverEntries() []serverEntry {
	entries := make([]serverEntry, 0, len(a.file.Servers)+1)
	if a.name == "" && a.endpoint.URL != "" {
		ep := a.endpoint
		entries = append(entries, serverEntry{server: config.Server{
			URL: ep.URL, Secret: ep.Secret,
			TLS: config.TLS{CAFile: ep.CAFile, ServerName: ep.ServerName, Insecure: ep.Insecure},
		}, active: true})
	}
	for _, name := range a.file.Names() {
		entries = append(entries, serverEntry{name: name, server: a.file.Servers[name], active: name == a.name})
	}
	return entries
}

func (a *app) handleServers(k string) tea.Cmd {
	if k == "ctrl+c" {
		return tea.Quit
	}
	if a.serverDelete != nil {
		entry := *a.serverDelete
		if k == "y" {
			next := *a.file
			next.Servers = maps.Clone(a.file.Servers)
			delete(next.Servers, entry.name)
			if next.Current == entry.name {
				next.Current = ""
			}
			if err := next.Save(); err != nil {
				a.serverError = "Could not delete server: " + err.Error()
				return nil
			}
			a.file = &next
			if entry.active {
				a.disconnect()
			}
		} else if k != "n" && k != "esc" {
			return nil
		}
		a.serverDelete = nil
		a.serverError = ""
		return nil
	}
	switch k {
	case "esc", "q":
		a.overlay = overlayNone
		return nil
	case "a":
		return a.openServerForm(serverEntry{})
	}
	entries := a.serverEntries()
	a.serverCursor.setCount(len(entries))
	if a.serverCursor.handleKey(k) || len(entries) == 0 {
		return nil
	}
	entry := entries[a.serverCursor.index]
	switch k {
	case "enter":
		if entry.name != "" {
			next := *a.file
			next.Current = entry.name
			if err := next.Save(); err != nil {
				a.serverError = "Could not remember server: " + err.Error()
				return nil
			}
			a.file = &next
		}
		if entry.active && a.session != nil && a.connState != sbx.StateFailed {
			a.overlay = overlayNone
			return nil
		}
		return a.replaceSession(serverEndpoint(entry.server), entry.name)
	case "e":
		return a.openServerForm(entry)
	case "d":
		if entry.name != "" {
			a.serverError = ""
			a.serverDelete = &entry
		}
	}
	return nil
}

const (
	serverNameField = iota
	serverURLField
	serverSecretField
	serverCAField
	serverTLSNameField
	serverTLSControl
	serverInsecureControl
	serverSaveControl
	serverConnectControl
)

type serverForm struct {
	inputs     [5]textinput.Model
	original   serverEntry
	focus      int
	expanded   bool
	insecure   bool
	discard    bool
	errorField int
	err        string
}

func (a *app) openServerForm(entry serverEntry) tea.Cmd {
	f := &serverForm{original: entry, insecure: entry.server.TLS.Insecure, errorField: -1}
	values := []string{
		entry.name,
		entry.server.URL,
		entry.server.Secret,
		entry.server.TLS.CAFile,
		entry.server.TLS.ServerName,
	}
	for i, value := range values {
		input := textinput.New()
		input.Prompt = ""
		input.SetVirtualCursor(true)
		input.SetStyles(textinput.Styles{
			Focused: textinput.StyleState{Text: a.theme.accentText, Placeholder: a.theme.dimText},
			Blurred: textinput.StyleState{Placeholder: a.theme.dimText},
			Cursor:  textinput.CursorStyle{Color: a.theme.accent},
		})
		input.SetValue(value)
		f.inputs[i] = input
	}
	f.inputs[serverURLField].Placeholder = "http://127.0.0.1:9090"
	f.inputs[serverSecretField].Placeholder = "Optional"
	f.inputs[serverSecretField].EchoMode = textinput.EchoPassword
	f.inputs[serverSecretField].EchoCharacter = '•'
	f.expanded = entry.server.TLS != (config.TLS{})
	f.setWidth(a.serverWidth())
	a.serverForm = f
	a.serverError = ""
	return f.inputs[serverNameField].Focus()
}

func (f *serverForm) setWidth(width int) {
	for i := range f.inputs {
		f.inputs[i].SetWidth(max(1, width-15))
	}
}

func (f serverForm) value() (string, config.Server) {
	s := config.Server{
		URL:    strings.TrimSpace(f.inputs[serverURLField].Value()),
		Secret: f.inputs[serverSecretField].Value(),
	}
	if f.https() {
		s.TLS = config.TLS{
			CAFile:     strings.TrimSpace(f.inputs[serverCAField].Value()),
			ServerName: strings.TrimSpace(f.inputs[serverTLSNameField].Value()),
			Insecure:   f.insecure,
		}
	}
	return strings.TrimSpace(f.inputs[serverNameField].Value()), s
}

func (f serverForm) https() bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(f.inputs[serverURLField].Value())), "https://")
}

func (f serverForm) changed() bool {
	name, server := f.value()
	return name != f.original.name || server != f.original.server
}

func (f serverForm) controls() []int {
	controls := []int{serverNameField, serverURLField, serverSecretField}
	if f.https() {
		controls = append(controls, serverTLSControl)
		if f.expanded {
			controls = append(controls, serverCAField, serverTLSNameField, serverInsecureControl)
		}
	}
	controls = append(controls, serverSaveControl)
	if f.original.name == "" {
		controls = append(controls, serverConnectControl)
	}
	return controls
}

func (f *serverForm) moveFocus(back bool) tea.Cmd {
	controls := f.controls()
	index := slices.Index(controls, f.focus)
	step := 1
	if back {
		step = -1
	}
	next := controls[(index+step+len(controls))%len(controls)]
	return f.focusControl(next)
}

func (f *serverForm) focusControl(control int) tea.Cmd {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
	f.focus = control
	if control < len(f.inputs) {
		return f.inputs[control].Focus()
	}
	return nil
}

func (a *app) handleServerForm(message tea.Msg) tea.Cmd {
	f := a.serverForm
	if msg, ok := message.(tea.KeyPressMsg); ok {
		k := msg.String()
		if k == "ctrl+c" {
			return tea.Quit
		}
		if f.discard {
			switch k {
			case "y":
				a.serverForm = nil
			case "n", "esc":
				f.discard = false
			}
			return nil
		}
		switch k {
		case "esc":
			if f.changed() {
				f.discard = true
			} else {
				a.serverForm = nil
			}
			return nil
		case "ctrl+r":
			if f.focus == serverSecretField {
				input := &f.inputs[serverSecretField]
				if input.EchoMode == textinput.EchoPassword {
					input.EchoMode = textinput.EchoNormal
				} else {
					input.EchoMode = textinput.EchoPassword
				}
			}
			return nil
		case "tab", "shift+tab":
			return f.moveFocus(k == "shift+tab")
		case "enter", "space":
			switch f.focus {
			case serverTLSControl:
				f.expanded = !f.expanded
				return nil
			case serverInsecureControl:
				f.insecure = !f.insecure
				return nil
			case serverSaveControl, serverConnectControl:
				return a.saveServer(f.focus == serverConnectControl)
			}
			if k == "enter" {
				cmd := f.moveFocus(false)
				if f.focus == serverSaveControl && len(a.file.Servers) == 0 {
					cmd = f.focusControl(serverConnectControl)
				}
				return cmd
			}
		}
	}
	if f.focus < len(f.inputs) && !f.discard {
		input, cmd := f.inputs[f.focus].Update(message)
		f.inputs[f.focus] = input
		return cmd
	}
	return nil
}

func (a *app) saveServer(connect bool) tea.Cmd {
	f := a.serverForm
	name, server := f.value()
	f.err, f.errorField = "", -1
	_, exists := a.file.Servers[name]
	switch {
	case name == "" || strings.ContainsFunc(name, unicode.IsControl):
		f.err, f.errorField = "Enter a name without control characters.", serverNameField
	case name != f.original.name && exists:
		f.err, f.errorField = "Name already exists. Choose another name.", serverNameField
	default:
		if err := sbx.ParseURL(server.URL); err != nil {
			f.err, f.errorField = err.Error(), serverURLField
		}
	}
	if f.err != "" {
		return f.focusControl(f.errorField)
	}
	next := *a.file
	next.Servers = maps.Clone(a.file.Servers)
	if f.original.name != "" {
		delete(next.Servers, f.original.name)
		if next.Current == f.original.name {
			next.Current = name
		}
	}
	next.Servers[name] = server
	if connect {
		next.Current = name
	}
	if err := next.Save(); err != nil {
		f.err = "Could not save server: " + err.Error()
		return nil
	}
	a.file = &next
	a.serverForm = nil
	a.serverError = ""
	if f.original.active {
		a.name = name
	}
	if connect || (f.original.active && server != f.original.server) {
		return a.replaceSession(serverEndpoint(server), name)
	}
	for i, entry := range a.serverEntries() {
		if entry.name == name {
			a.serverCursor.index = i
			break
		}
	}
	return nil
}

func (a app) serverWidth() int { return min(68, max(1, a.width-8)) }

// The body sits above a blank line, two status lines and the hints, inside a
// border, below the top bar, tabs and footer.
func (a app) serverBodyHeight() int { return max(3, min(12, a.height-9)) }

// configPath is the file the server manager writes, with the home directory
// shortened so it stays readable in the footer.
func configPath() string {
	path, err := config.Path()
	if err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return path
}

// fitHints joins the hints, dropping them from the end until the line fits. The
// last one always survives: it is how to leave the current view.
func fitHints(hints []string, width int) string {
	for len(hints) > 1 && lipgloss.Width(strings.Join(hints, "  ")) > width {
		hints = slices.Delete(hints, len(hints)-2, len(hints)-1)
	}
	return strings.Join(hints, "  ")
}

func (a app) serversView() string {
	w, h := a.serverWidth(), a.serverBodyHeight()
	title := "Servers"
	hints := []string{"Enter connect", "a add", "e edit", "d delete", "Esc back"}
	status := a.serverError
	statusStyle := a.theme.badgeBad
	if f := a.serverForm; f != nil {
		status = f.err
		if f.discard {
			status = "Discard unsaved changes?  y yes / n no"
			statusStyle = a.theme.badgeWarn
		}
	} else if a.serverDelete != nil {
		status = fmt.Sprintf("Delete local record %q?  y yes / n no", a.serverDelete.name)
		if a.serverDelete.active {
			status = fmt.Sprintf("Delete %q locally and disconnect?  y yes / n no", a.serverDelete.name)
		}
		if a.serverError != "" {
			status = a.serverError + "\n" + status
		}
	}
	statusLines := strings.Split(ansi.Wrap(status, w, ""), "\n")
	statusHeight := max(2, len(statusLines))
	h = min(h, max(3, a.height-statusHeight-7))
	var lines []string
	if f := a.serverForm; f != nil {
		title = "Add server"
		if f.original.name != "" {
			title = "Edit server"
		} else if f.original.active {
			title = "Save server"
		}
		lines = f.view(w, h, a.theme)
		hints = []string{"Tab next", "Shift+Tab previous", "Esc cancel"}
		if f.focus == serverSecretField {
			hints = []string{"Ctrl+R show/hide secret", "Tab next", "Esc cancel"}
		}
	} else {
		entries := a.serverEntries()
		if len(entries) == 0 {
			lines = []string{"", a.theme.title.Render("No servers yet"), "", "Add an existing sing-box API server to get started.", "", a.theme.accentText.Render("a  Add server")}
			hints = []string{"a add", "Esc back", "Ctrl+C quit"}
		} else {
			selected := a.serverCursor
			selected.setHeight(h - 2)
			selected.setCount(len(entries))
			start, end := selected.visible()
			nameWidth := 12
			for _, entry := range entries {
				nameWidth = min(w/3, max(nameWidth, lipgloss.Width(entry.name)))
			}
			lines = []string{a.theme.dimText.Render("  " + cell("NAME", nameWidth, false) + "  URL"), ""}
			for i := start; i < end; i++ {
				entry := entries[i]
				name, marker := entry.name, "  "
				if name == "" {
					name = "Unsaved"
				}
				if entry.active {
					marker = "● "
				}
				line := cell(marker+cell(name, nameWidth, false)+"  "+entry.server.URL, w, false)
				if i == selected.index {
					line = a.theme.focusedRow.Render(line)
					if entry.name == "" {
						hints = []string{"Enter connect", "a add", "e save", "Esc back"}
					}
				} else if entry.active {
					line = a.theme.accentText.Render(line)
				}
				lines = append(lines, line)
			}
			if status == "" && len(entries) > h-2 {
				statusLines = []string{fmt.Sprintf("%d–%d of %d", start+1, end, len(entries))}
				statusStyle = a.theme.dimText
			}
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	lines = append(lines, "")
	for _, line := range statusLines {
		lines = append(lines, statusStyle.Render(line))
	}
	if len(statusLines) < 2 {
		lines = append(lines, "")
	}
	lines = append(lines, a.theme.dimText.Render(fitHints(hints, w)))
	for i := range lines {
		lines[i] = cell(lines[i], w, false)
	}
	return renderOverlay(title, strings.Join(lines, "\n"), a.width, a.height-3, a.theme)
}

func (f serverForm) view(width, height int, t theme) []string {
	labels := []string{"Name", "URL", "Secret", "CA file", "Server name"}
	var lines []string
	focusLine := 0
	for _, control := range f.controls() {
		if control == serverConnectControl {
			continue
		}
		var line string
		switch control {
		case serverTLSControl:
			line = "▸ TLS settings"
			if f.expanded {
				line = "▾ TLS settings"
			}
		case serverInsecureControl:
			line = "[ ] Skip certificate verification (unsafe)"
			if f.insecure {
				line = "[x] Skip certificate verification (unsafe)"
			}
		case serverSaveControl:
			lines = append(lines, "")
			saveLabel := "Save"
			_, server := f.value()
			if f.original.active && server != f.original.server {
				saveLabel = "Save & reconnect"
			}
			save := "[ " + saveLabel + " ]"
			if f.focus == serverSaveControl {
				save = t.focusedRow.Render(save)
			}
			line = save
			if f.original.name == "" {
				connect := "[ Save & connect ]"
				if f.focus == serverConnectControl {
					connect = t.focusedRow.Render(connect)
				}
				line += "  " + connect
			}
		default:
			label := cell(labels[control], 13, false)
			switch control {
			case f.errorField:
				label = t.badgeBad.Render(label)
			case f.focus:
				label = t.accentText.Render(label)
			}
			line = label + f.inputs[control].View()
		}
		if control == f.focus || (control == serverSaveControl && f.focus == serverConnectControl) {
			focusLine = len(lines)
			if control == serverTLSControl || control == serverInsecureControl {
				line = t.focusedRow.Render(line)
			}
		}
		lines = append(lines, cell(line, width, false))
	}
	start := max(0, focusLine-height+1)
	return lines[start:min(len(lines), start+height)]
}
