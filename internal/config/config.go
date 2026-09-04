package config

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

type TLS struct {
	CAFile     string `toml:"ca_file,omitempty"`
	ServerName string `toml:"server_name,omitempty"`
	Insecure   bool   `toml:"insecure,omitempty"`
}

type Target struct {
	URL    string `toml:"url"`
	Secret string `toml:"secret,omitempty"`
	TLS    TLS    `toml:"tls,omitempty"`
}

type File struct {
	Current string            `toml:"current,omitempty"`
	Targets map[string]Target `toml:"targets"`
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sbxctl", "config.toml"), nil
}

func Load() (*File, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &File{Targets: make(map[string]Target)}, nil
	}
	if err != nil {
		return nil, err
	}

	var file File
	if err := toml.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Targets == nil {
		file.Targets = make(map[string]Target)
	}
	return &file, nil
}

func (f *File) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := toml.Marshal(f)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".config.toml-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (f *File) Names() []string {
	names := make([]string, 0, len(f.Targets))
	for name := range f.Targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
