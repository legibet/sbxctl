package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	missing, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if missing.Targets == nil {
		t.Fatal("missing config has nil Targets")
	}

	want := &File{
		Current: "home",
		Targets: map[string]Target{
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
	if names := got.Names(); !reflect.DeepEqual(names, []string{"home", "work"}) {
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
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	wantText := "current = 'home'"
	if len(data) == 0 || !(strings.Contains(string(data), wantText) || strings.Contains(string(data), `current = "home"`)) {
		t.Fatalf("config does not contain current target:\n%s", data)
	}
}
