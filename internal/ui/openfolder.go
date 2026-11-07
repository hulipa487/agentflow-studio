package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// openConfigFolder opens the directory containing the channels config. The
// runtime may be on another machine; the local stat check turns that into a
// clear error instead of an empty file-manager window.
func openConfigFolder(path string) error {
	dir := filepath.Dir(path)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		dir = path
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("config folder is not available on this machine: %s", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("config path is not a directory: %s", dir)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open config folder: %w", err)
	}
	return nil
}
