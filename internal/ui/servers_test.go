package ui

import (
	"maps"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

func newServerTestApp(t *testing.T) *app {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Reserve a loopback endpoint; these state checks do not wait for RPCs.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ep := sbx.Endpoint{URL: "http://" + listener.Addr().String()}
	file := &config.File{Current: "home", Servers: map[string]config.Server{
		"home":  {URL: ep.URL},
		"other": {URL: ep.URL, Secret: "other-secret"},
		"spare": {URL: ep.URL, Secret: "spare-secret"},
	}}
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	session, err := sbx.NewSession(ep)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(ep, "home", file, session)
	a.overlay = overlayServers
	a.status.TrafficAvailable = true
	t.Cleanup(a.disconnect)
	return &a
}

func assertSavedServers(t *testing.T, want *config.File) {
	t.Helper()
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("saved configuration differs from expected records and selection")
	}
}

func TestServerWriteFailurePreservesState(t *testing.T) {
	for _, operation := range []string{"add", "edit", "delete", "select"} {
		t.Run(operation, func(t *testing.T) {
			a := newServerTestApp(t)
			before := &config.File{Current: a.file.Current, Servers: maps.Clone(a.file.Servers)}
			session, endpoint := a.session, a.endpoint
			switch operation {
			case "add", "edit":
				if operation == "add" {
					a.openServerForm(serverEntry{})
				} else {
					a.handleServers("e")
				}
				a.serverForm.inputs[serverNameField].SetValue("new-name")
				a.serverForm.inputs[serverURLField].SetValue(endpoint.URL)
			case "delete":
				a.handleServers("d")
			case "select":
				a.serverCursor.index = 1
			}
			form := a.serverForm
			// A regular file cannot be a config directory, even when tests run as root.
			dir := os.Getenv("XDG_CONFIG_HOME")
			path, err := config.Path()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CONFIG_HOME", path)
			switch operation {
			case "add", "edit":
				a.saveServer(true)
			case "delete":
				a.handleServers("y")
			case "select":
				a.handleServers("enter")
			}
			t.Setenv("XDG_CONFIG_HOME", dir)
			assertSavedServers(t, before)
			if !reflect.DeepEqual(a.file, before) || a.name != before.Current || a.endpoint != endpoint || a.session != session || !a.status.TrafficAvailable {
				t.Fatal("failed write changed configuration or active session state")
			}
			if form != nil {
				if a.serverForm != form || form.inputs[serverNameField].Value() != "new-name" || form.inputs[serverURLField].Value() != endpoint.URL || form.err == "" {
					t.Fatal("failed save lost form input or did not report the error")
				}
			} else if a.serverError == "" {
				t.Fatal("failed operation did not report the error")
			}
			if operation == "delete" && a.serverDelete == nil {
				t.Fatal("failed deletion dismissed confirmation")
			}
		})
	}
}

func TestSaveFirstServerDoesNotSelect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newApp(sbx.Endpoint{}, "", nil, nil)
	t.Cleanup(a.disconnect)
	a.openServerForm(serverEntry{})
	a.serverForm.inputs[serverNameField].SetValue("home")
	a.serverForm.inputs[serverURLField].SetValue("https://example.invalid")
	a.serverForm.inputs[serverSecretField].SetValue("saved-secret")
	a.saveServer(false)
	want := &config.File{Servers: map[string]config.Server{
		"home": {URL: "https://example.invalid", Secret: "saved-secret"},
	}}
	assertSavedServers(t, want)
	if !reflect.DeepEqual(a.file, want) || a.name != "" || a.session != nil || a.serverForm != nil {
		t.Fatal("save-only selected a server or failed to finish saving")
	}
}

func TestServerEditsSelectionAndDeletion(t *testing.T) {
	a := newServerTestApp(t)
	spare := a.file.Servers["spare"]
	original := a.session
	a.handleServers("e")
	a.serverForm.inputs[serverNameField].SetValue("renamed")
	a.saveServer(false)
	if a.session != original || a.name != "renamed" || a.file.Current != "renamed" || !a.status.TrafficAvailable {
		t.Fatal("rename disturbed the session or lost the remembered name")
	}
	if _, exists := a.file.Servers["home"]; exists || len(a.file.Servers) != 3 {
		t.Fatal("rename left the old record or changed another record")
	}
	assertSavedServers(t, a.file)

	a.selectActiveServer()
	a.handleServers("e")
	a.serverForm.inputs[serverSecretField].SetValue("new-secret")
	a.saveServer(false)
	if a.session == nil || a.session == original || a.endpoint.Secret != "new-secret" || a.file.Current != "renamed" || a.status.TrafficAvailable {
		t.Fatal("connection edit failed to replace the session and clear old status")
	}
	select {
	case _, open := <-original.Events():
		if open {
			t.Fatal("old session events remain open")
		}
	default:
		t.Fatal("old session was not closed")
	}
	assertSavedServers(t, a.file)

	previous := a.session
	a.serverCursor.index = 0 // other precedes renamed.
	a.handleServers("enter")
	if a.name != "other" || a.file.Current != "other" || a.session == nil || a.session == previous || a.endpoint != serverEndpoint(a.file.Servers["other"]) {
		t.Fatal("selection failed to connect and remember the chosen record")
	}
	assertSavedServers(t, a.file)

	selected := a.session
	a.serverCursor.index = 1
	a.handleServers("d")
	a.handleServers("y")
	if a.session != selected || a.file.Current != "other" || len(a.file.Servers) != 2 {
		t.Fatal("deleting an inactive record disturbed the current server")
	}
	assertSavedServers(t, a.file)

	a.logs.apply(sbx.LogBatch{Entries: []sbx.LogEntry{{Level: sbx.LevelInfo, Message: "old session log"}}})
	a.selectActiveServer()
	a.handleServers("d")
	a.handleServers("y")
	if a.session != nil || a.name != "" || a.endpoint != (sbx.Endpoint{}) || a.file.Current != "" {
		t.Fatal("deleting the active record failed to disconnect and clear selection")
	}
	if len(a.file.Servers) != 1 || a.file.Servers["spare"] != spare {
		t.Fatal("deleting the active record changed an unrelated record")
	}
	if a.logs.buffer.Len() != 0 || a.proxies.client != nil || a.connections.client != nil || a.logs.client != nil {
		t.Fatal("disconnected workspaces retained old data or clients")
	}
	assertSavedServers(t, a.file)
}

func TestServerInitializationFailureCanBeEdited(t *testing.T) {
	a := newServerTestApp(t)
	url := a.endpoint.URL
	a.openServerForm(serverEntry{})
	a.serverForm.inputs[serverNameField].SetValue("bad-ca")
	a.serverForm.inputs[serverURLField].SetValue("https://example.invalid")
	a.serverForm.inputs[serverCAField].SetValue(filepath.Join(t.TempDir(), "missing.pem"))
	a.saveServer(true)
	if a.session != nil || a.name != "bad-ca" || a.file.Current != "bad-ca" || a.connState != sbx.StateFailed || a.connErr == nil || a.status.TrafficAvailable {
		t.Fatal("initialization failure lost the selection or retained the old session state")
	}
	assertSavedServers(t, a.file)

	a.selectActiveServer()
	a.handleServers("e")
	if a.serverForm == nil {
		t.Fatal("failed server cannot be edited")
	}
	a.serverForm.inputs[serverURLField].SetValue(url)
	a.saveServer(false)
	if a.session == nil || a.name != "bad-ca" || a.file.Current != "bad-ca" || a.connErr != nil || a.endpoint != (sbx.Endpoint{URL: url}) {
		t.Fatal("editing failed server did not start a new session with corrected settings")
	}
	assertSavedServers(t, a.file)
}
