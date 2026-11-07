// Package appconfig persists the desktop app's connection settings.
package appconfig

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Profile is one saved connection: a runtime URL and the admin token that
// opens it. The active profile is mirrored into Config.URL/Token so every
// screen keeps reading one pair; the list exists so switching cells is a click
// rather than retyping a token.
type Profile struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	ForgetToken bool   `json:"forget_token,omitempty"`
}

type Config struct {
	// URL and Token are the active connection. They are the fields the rest of
	// the app reads and writes; Profiles is the saved list behind them.
	URL     string `json:"url"`
	Token   string `json:"token"`
	Theme   string `json:"theme,omitempty"`   // "light" or "dark"; empty follows the OS
	Palette string `json:"palette,omitempty"` // "trans" or "obsidian"; empty defaults to "trans"
	// ForgetToken keeps the admin token out of this file: the URL is still
	// remembered, the token is asked for at every launch. The zero value
	// (remember) is the historical behaviour.
	ForgetToken bool      `json:"forget_token,omitempty"`
	Profiles    []Profile `json:"profiles,omitempty"`
	Active      string    `json:"active,omitempty"`
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
// The old single-URL shape is migrated into one profile on read.
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
	c.normalize()
	return c
}

// normalize migrates the pre-profile shape and keeps the active mirror in
// sync. It is safe to call more than once.
func (c *Config) normalize() {
	if len(c.Profiles) == 0 {
		if c.URL == "" {
			return
		}
		c.Profiles = []Profile{{Name: profileName(c.URL), URL: c.URL, Token: c.Token, ForgetToken: c.ForgetToken}}
		c.Active = c.Profiles[0].Name
		return
	}
	if c.Active == "" {
		c.Active = c.Profiles[0].Name
	}
	for _, p := range c.Profiles {
		if p.Name == c.Active {
			c.URL, c.Token, c.ForgetToken = p.URL, p.Token, p.ForgetToken
			return
		}
	}
	// Active names a profile that is gone (hand-edited file): fall back to the
	// first one rather than presenting an empty URL.
	c.Active = c.Profiles[0].Name
	c.URL, c.Token, c.ForgetToken = c.Profiles[0].URL, c.Profiles[0].Token, c.Profiles[0].ForgetToken
}

// syncActive copies the live URL/token back into the active profile, adding it
// when the list is empty. Save calls it, so a login or a token change persists
// without every caller knowing about Profiles.
func (c *Config) syncActive() {
	if c.URL == "" {
		return
	}
	name := c.Active
	if name == "" {
		name = profileName(c.URL)
	}
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			c.Profiles[i].URL = c.URL
			c.Profiles[i].Token = c.Token
			c.Profiles[i].ForgetToken = c.ForgetToken
			c.Active = name
			return
		}
	}
	c.Profiles = append(c.Profiles, Profile{Name: name, URL: c.URL, Token: c.Token, ForgetToken: c.ForgetToken})
	c.Active = name
}

// SelectProfile makes a saved profile the active one, reporting whether it
// exists.
func (c *Config) SelectProfile(name string) bool {
	for _, p := range c.Profiles {
		if p.Name == name {
			c.Active = p.Name
			c.URL, c.Token, c.ForgetToken = p.URL, p.Token, p.ForgetToken
			return true
		}
	}
	return false
}

// RememberConnection records a successful login as a saved profile, named by
// its host and deduplicated by URL. It returns the profile name.
func (c *Config) RememberConnection(rawURL, token string, remember bool) string {
	if !remember {
		// "Forget the token" means exactly that: keep the URL, store no secret.
		token = ""
	}
	name := profileName(rawURL)
	for i := range c.Profiles {
		if c.Profiles[i].URL == rawURL {
			name = c.Profiles[i].Name
			c.Profiles[i].Token = token
			c.Profiles[i].ForgetToken = !remember
			c.Active = name
			c.URL, c.Token, c.ForgetToken = rawURL, token, !remember
			return name
		}
	}
	c.Profiles = append(c.Profiles, Profile{Name: name, URL: rawURL, Token: token, ForgetToken: !remember})
	c.Active = name
	c.URL, c.Token, c.ForgetToken = rawURL, token, !remember
	return name
}

// profileName derives a short label from a URL: its host (and port when one is
// written), or "default" for something unparseable.
func profileName(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return "default"
	}
	return u.Host
}

// Save writes the config with owner-only permissions. The token is a secret
// and is stored as plain text in the user's config directory — the same trust
// level as a shell history file.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	c.syncActive()
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
