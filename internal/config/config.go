package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type TLS struct {
	CAFile     string `toml:"ca_file,omitempty"`
	ServerName string `toml:"server_name,omitempty"`
	Insecure   bool   `toml:"insecure,omitempty"`
}

type Server struct {
	URL    string `toml:"url"`
	Secret string `toml:"secret,omitempty"`
	TLS    TLS    `toml:"tls,omitempty"`
}

type File struct {
	Current string            `toml:"current,omitempty"`
	Servers map[string]Server `toml:"servers"`
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
		return &File{Servers: make(map[string]Server)}, nil
	}
	if err != nil {
		return nil, err
	}

	var file File
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&file); err != nil {
		if unknown, ok := errors.AsType[*toml.StrictMissingError](err); ok {
			keys := make([]string, 0, len(unknown.Errors))
			for _, field := range unknown.Errors {
				keys = append(keys, strings.Join(field.Key(), "."))
			}
			return nil, fmt.Errorf("read %s: unknown fields %s; use current and [servers.<name>] with url, secret and tls settings", path, strings.Join(keys, ", "))
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if file.Servers == nil {
		file.Servers = make(map[string]Server)
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
	defer func() { _ = os.Remove(tempPath) }()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (f *File) Names() []string {
	names := make([]string, 0, len(f.Servers))
	for name := range f.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
