package client

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Server string `json:"server"`
	Device string `json:"device,omitempty"`
}

// Resolve picks the server address: flag > environment > config file.
func Resolve(flagServer, envServer string, file Config) (string, bool) {
	for _, s := range []string{flagServer, envServer, file.Server} {
		if strings.TrimSpace(s) != "" {
			return NormalizeServer(s), true
		}
	}
	return "", false
}

// NormalizeServer adds a scheme (https for *.ts.net, http otherwise) and drops a trailing slash.
func NormalizeServer(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if strings.Contains(s, "://") {
		return s
	}
	host := s
	if i := strings.IndexAny(host, ":/"); i >= 0 {
		host = host[:i]
	}
	if strings.HasSuffix(host, ".ts.net") {
		return "https://" + s
	}
	return "http://" + s
}

// DefaultConfigPath is ~/.config/clip/config.json on every system. Windows' %APPDATA%
// is avoided on purpose: apps run from a package see a private copy of it.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "clip", "config.json"), nil
}

// LoadConfig reads the config file; a missing file is an empty config.
func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

func SaveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, b, 0o644)
}
