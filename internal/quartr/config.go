package quartr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Config is the on-disk shape of the persistent CLI config file.
// Empty fields are omitted from the serialized JSON so partial updates
// don't clobber values written by earlier `auth login` runs.
type Config struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Format  string `json:"format,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

// DefaultConfigPath returns the platform-appropriate config file path.
// QUARTR_CONFIG overrides; otherwise uses %APPDATA% on Windows,
// $XDG_CONFIG_HOME if set, or $HOME/.config/quartr/config.json.
func DefaultConfigPath() string {
	if p := os.Getenv("QUARTR_CONFIG"); strings.TrimSpace(p) != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "quartr", "config.json")
		}
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "quartr", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".quartr.json")
	}
	return filepath.Join(home, ".config", "quartr", "config.json")
}

// LoadConfig reads a JSON config file from path. A missing file returns
// the zero Config without error so first-run usage works seamlessly.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	if path == "" {
		path = DefaultConfigPath()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// SaveConfig writes cfg as pretty JSON to path with file mode 0600 and
// directory mode 0700, since the file holds the API key.
func SaveConfig(path string, cfg Config) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ") //nolint:gosec // G117: APIKey field is credential storage by design
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

// EffectiveTimeout parses timeout as a Go duration (e.g. "30s", "2m").
// Empty or invalid input returns the 30-second default.
func EffectiveTimeout(timeout string) time.Duration {
	if strings.TrimSpace(timeout) == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(timeout)
	if err != nil || d <= 0 {
		return 30 * time.Second
	}
	return d
}

// MaskKey returns key with all but the first and last 4 characters replaced
// by asterisks. Suitable for echoing the configured key back to the user
// without disclosing it.
func MaskKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
