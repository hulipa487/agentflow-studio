// Package appconfig persists the desktop app's connection settings.
package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	URL     string `json:"url"`
	Token   string `json:"token"`
	Theme   string `json:"theme,omitempty"`   // "light" or "dark"; empty follows the OS
	Palette string `json:"palette,omitempty"` // "trans" or "obsidian"; empty defaults to "trans"
}

// Path returns the config file. AGENTFLOW_STUDIO_CONFIG overrides it (useful
// for tests and for keeping a second profile); otherwise it lives under the
// OS config directory.
func Path() (string, error) {
	if p := os.Getenv("AGENTFLOW_STUDIO_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agentflow-studio", "config.json"), nil
}

// Load returns the saved config, or a zero Config when none exists. A
// malformed file is treated as absent: the app should show login, not crash.
func Load() Config {
	p, err := Path()
	if err != nil {
		return Config{}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Config{}
	}
	var c Config
	if json.Unmarshal(b, &c) != nil {
		return Config{}
	}
	return c
}

// Save writes the config with owner-only permissions. The token is a secret
// and is stored as plain text in the user's config directory — the same trust
// level as a shell history file.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
