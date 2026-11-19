package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultWidth  = 1280
	defaultHeight = 860
	// The web app's layout holds together down to this size; below it the
	// sidebar and a split editor no longer fit side by side.
	minWidth  = 960
	minHeight = 600
)

// windowState is what the window looked like when it was last closed.
type windowState struct {
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	X           int  `json:"x"`
	Y           int  `json:"y"`
	HasPosition bool `json:"has_position"`
	Maximised   bool `json:"maximised"`
}

// appDataDir is where the app keeps its own files: the window state and, on
// Windows, the webview's profile.
func appDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "agentflow-studio", "desktop")
	return dir, os.MkdirAll(dir, 0o700)
}

func statePath(dir string) string { return filepath.Join(dir, "window.json") }

// loadWindowState reads the saved state; anything missing or unreadable falls
// back to the defaults, and a saved size is never below the minimum.
func loadWindowState(dir string) windowState {
	s := windowState{Width: defaultWidth, Height: defaultHeight}
	if b, err := os.ReadFile(statePath(dir)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	s.Width = max(s.Width, minWidth)
	s.Height = max(s.Height, minHeight)
	return s
}

func saveWindowState(dir string, s windowState) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp := statePath(dir) + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, statePath(dir))
	}
}

// currentWindowState reads the window as it is now. A maximised window keeps
// the size and position it had before, so un-maximising next time returns
// to it.
func currentWindowState(ctx context.Context, prev windowState) windowState {
	s := prev
	s.Maximised = runtime.WindowIsMaximised(ctx)
	if s.Maximised || runtime.WindowIsMinimised(ctx) {
		return s
	}
	s.Width, s.Height = runtime.WindowGetSize(ctx)
	s.X, s.Y = runtime.WindowGetPosition(ctx)
	s.HasPosition = true
	return s
}
