package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestLoadRejectsUnknownFieldsWithoutExposingValues(t *testing.T) {
	const secret = "private-config-value"
	for _, document := range []string{
		"[targets.home]\nurl = 'http://localhost:9090'\nsecret = '" + secret + "'\n",
		"[servers.home]\nurl = 'http://localhost:9090'\nsecret = '" + secret + "'\nunknown = '" + secret + "'\n",
	} {
		t.Run(strings.SplitN(document, "\n", 2)[0], func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("HOME", t.TempDir())
			t.Setenv("AppData", t.TempDir())
			path, err := Path()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			file, err := Load()
			if err == nil || file != nil {
				t.Fatal("unknown fields produced a usable configuration")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("configuration error exposed a field value")
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())

	missing, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if missing.Servers == nil {
		t.Fatal("missing config has nil Servers")
	}

	want := &File{
		Current: "home",
		Servers: map[string]Server{
			"work": {URL: "https://api.example.com", TLS: TLS{ServerName: "server.example.com", Insecure: true}},
			"home": {URL: "http://127.0.0.1:9090", Secret: "secret", TLS: TLS{CAFile: "/path/ca.pem"}},
		},
	}
	if err := want.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	if names := got.Names(); !slices.Equal(names, []string{"home", "work"}) {
		t.Fatalf("Names() = %v", names)
	}

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config permissions = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("config directory permissions = %o, want 700", perm)
	}
}
