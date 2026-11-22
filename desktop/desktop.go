package main

// What the desktop shell offers the web app beyond the proxy, under
// /__af/__desktop/. The web app finds out it is in the shell from
// /__af/__ping ("shell": "wails") and uses these instead of the browser's
// stand-ins:
//
//   POST save         {name, content}   save dialog, then write the file
//   POST open-folder  {path}            show a folder in the file manager
//   GET  fyne-config                     the old Fyne app's saved connections
//   POST background   {r, g, b}          window colour behind the page
//   POST close                           the app's own close button
//   GET  update/status                   cached release/updater status
//   POST update/check                    query GitHub Releases for a newer version
//   POST update/apply                    download, verify SHA-256 and relaunch
//   POST open-url     {url}              open an http(s) URL in the OS browser
//   GET  secrets/status                  OS keychain availability and backend name
//   POST secrets/get    {account}        read a token from the OS keychain
//   POST secrets/set    {account, secret} store a token in the OS keychain
//   POST secrets/delete {account}        remove a token from the OS keychain
//
// Like the proxy, this is reachable only from the app's own webview.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// maxSaveBytes bounds a file saved through the dialog: a user export or a
// usage CSV, never more than a few MB in practice.
const maxSaveBytes = 64 << 20

type desktop struct {
	mu  sync.Mutex
	ctx context.Context // the Wails app context, set at startup
	// frameless is reported by /__af/__ping, so the web app knows to draw the
	// window controls; close is what its close button runs.
	frameless bool
	close     func(ctx context.Context)
	quit      func(ctx context.Context)
	updater   *updater
	secrets   secretStore
	// The window calls, swappable in tests.
	saveDialog func(ctx context.Context, name string) (string, error)
	setBg      func(ctx context.Context, r, g, b uint8)
	openDir    func(dir string) error
	openURL    func(ctx context.Context, target string)
}

func newDesktop() *desktop {
	return &desktop{
		quit:    runtime.Quit,
		updater: newUpdater(version),
		secrets: newSecretStore(),
		saveDialog: func(ctx context.Context, name string) (string, error) {
			return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{DefaultFilename: name, CanCreateDirectories: true})
		},
		setBg: func(ctx context.Context, r, g, b uint8) {
			runtime.WindowSetBackgroundColour(ctx, r, g, b, 255)
		},
		openDir: openInFileManager,
		openURL: runtime.BrowserOpenURL,
	}
}

func (d *desktop) setContext(ctx context.Context) {
	d.mu.Lock()
	d.ctx = ctx
	d.mu.Unlock()
}

func (d *desktop) context() context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ctx
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, limit int64, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, limit)).Decode(v)
}

// serve handles one /__af/__desktop/<action> request.
func (d *desktop) serve(w http.ResponseWriter, r *http.Request, action string) {
	ctx := d.context()
	needsWindow := action == "save" || action == "background" || action == "close" || action == "open-url" || action == "update/apply"
	if needsWindow && ctx == nil {
		refuse(w, http.StatusServiceUnavailable, "the window is not ready")
		return
	}
	switch {
	case action == "save" && r.Method == http.MethodPost:
		var in struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := decode(r, maxSaveBytes+4096, &in); err != nil {
			refuse(w, http.StatusBadRequest, "bad save request: "+err.Error())
			return
		}
		path, err := d.saveDialog(ctx, filepath.Base(in.Name))
		if err != nil {
			reply(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if path == "" {
			reply(w, map[string]any{"ok": false, "cancelled": true})
			return
		}
		if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
			reply(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		reply(w, map[string]any{"ok": true, "path": path, "name": filepath.Base(path)})

	case action == "open-folder" && r.Method == http.MethodPost:
		var in struct {
			Path string `json:"path"`
		}
		if err := decode(r, 8192, &in); err != nil || in.Path == "" {
			refuse(w, http.StatusBadRequest, "a path is required")
			return
		}
		dir, problem := folderOf(in.Path)
		if problem != "" {
			reply(w, map[string]any{"ok": false, "problem": problem, "dir": dir})
			return
		}
		if err := d.openDir(dir); err != nil {
			reply(w, map[string]any{"ok": false, "problem": "failed", "dir": dir, "error": err.Error()})
			return
		}
		reply(w, map[string]any{"ok": true, "dir": dir})

	case action == "fyne-config" && r.Method == http.MethodGet:
		reply(w, readFyneConfig())

	case action == "background" && r.Method == http.MethodPost:
		var in struct{ R, G, B int }
		if err := decode(r, 256, &in); err != nil {
			refuse(w, http.StatusBadRequest, "bad colour")
			return
		}
		d.setBg(ctx, clampByte(in.R), clampByte(in.G), clampByte(in.B))
		reply(w, map[string]any{"ok": true})

	case action == "close" && r.Method == http.MethodPost:
		reply(w, map[string]any{"ok": true})
		if d.close != nil {
			// After the reply, so the page is not left waiting on a request
			// the quitting app will never finish.
			go d.close(ctx)
		}

	case action == "update/status" && r.Method == http.MethodGet:
		reply(w, d.updater.status())

	case action == "update/check" && r.Method == http.MethodPost:
		st, err := d.updater.check(r.Context())
		if err != nil {
			refuse(w, http.StatusBadGateway, err.Error())
			return
		}
		reply(w, st)

	case action == "update/apply" && r.Method == http.MethodPost:
		quitFn := func() {
			if d.close != nil {
				d.close(ctx)
			} else if d.quit != nil {
				d.quit(ctx)
			}
		}
		if err := d.updater.apply(r.Context(), quitFn); err != nil {
			refuse(w, http.StatusBadGateway, err.Error())
			return
		}
		reply(w, map[string]any{"ok": true, "restarting": true})

	case action == "open-url" && r.Method == http.MethodPost:
		var in struct {
			URL string `json:"url"`
		}
		if err := decode(r, 4096, &in); err != nil || in.URL == "" {
			refuse(w, http.StatusBadRequest, "url is required")
			return
		}
		parsed, err := url.Parse(in.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			refuse(w, http.StatusBadRequest, "only http and https URLs are allowed")
			return
		}
		if d.openURL != nil {
			d.openURL(ctx, parsed.String())
		}
		reply(w, map[string]any{"ok": true})

	case action == "secrets/status" && r.Method == http.MethodGet:
		if d.secrets == nil {
			reply(w, map[string]any{"ok": true, "available": false, "backend": "none"})
			return
		}
		reply(w, map[string]any{"ok": true, "available": d.secrets.Available(), "backend": d.secrets.Backend()})

	case action == "secrets/get" && r.Method == http.MethodPost:
		var in struct {
			Account string `json:"account"`
		}
		if err := decode(r, 4096, &in); err != nil {
			refuse(w, http.StatusBadRequest, "bad secret lookup request")
			return
		}
		if err := validateAccount(in.Account); err != nil {
			refuse(w, http.StatusBadRequest, err.Error())
			return
		}
		if d.secrets == nil || !d.secrets.Available() {
			refuse(w, http.StatusServiceUnavailable, errKeychainUnavailable.Error())
			return
		}
		secret, found, err := d.secrets.Get(in.Account)
		if err != nil {
			if errors.Is(err, errKeychainUnavailable) {
				refuse(w, http.StatusServiceUnavailable, err.Error())
				return
			}
			refuse(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := map[string]any{"ok": true, "found": found}
		if found {
			out["secret"] = secret
		}
		reply(w, out)

	case action == "secrets/set" && r.Method == http.MethodPost:
		var in struct {
			Account string `json:"account"`
			Secret  string `json:"secret"`
		}
		if err := decode(r, 65536, &in); err != nil {
			refuse(w, http.StatusBadRequest, "bad secret store request")
			return
		}
		if err := validateAccount(in.Account); err != nil {
			refuse(w, http.StatusBadRequest, err.Error())
			return
		}
		if in.Secret == "" {
			refuse(w, http.StatusBadRequest, "secret is required")
			return
		}
		if d.secrets == nil || !d.secrets.Available() {
			refuse(w, http.StatusServiceUnavailable, errKeychainUnavailable.Error())
			return
		}
		if err := d.secrets.Set(in.Account, in.Secret); err != nil {
			if errors.Is(err, errKeychainUnavailable) {
				refuse(w, http.StatusServiceUnavailable, err.Error())
				return
			}
			refuse(w, http.StatusInternalServerError, err.Error())
			return
		}
		reply(w, map[string]any{"ok": true})

	case action == "secrets/delete" && r.Method == http.MethodPost:
		var in struct {
			Account string `json:"account"`
		}
		if err := decode(r, 4096, &in); err != nil {
			refuse(w, http.StatusBadRequest, "bad secret delete request")
			return
		}
		if err := validateAccount(in.Account); err != nil {
			refuse(w, http.StatusBadRequest, err.Error())
			return
		}
		if d.secrets == nil || !d.secrets.Available() {
			refuse(w, http.StatusServiceUnavailable, errKeychainUnavailable.Error())
			return
		}
		if err := d.secrets.Delete(in.Account); err != nil {
			if errors.Is(err, errKeychainUnavailable) {
				refuse(w, http.StatusServiceUnavailable, err.Error())
				return
			}
			refuse(w, http.StatusInternalServerError, err.Error())
			return
		}
		reply(w, map[string]any{"ok": true})

	default:
		refuse(w, http.StatusNotFound, "no such desktop action")
	}
}

func clampByte(v int) uint8 { return uint8(min(max(v, 0), 255)) }

// folderOf is the folder to show for a config path: the path itself when it
// is a directory, else the directory holding it. The runtime may run on
// another machine, so a path that does not exist here is reported, not opened.
func folderOf(path string) (dir, problem string) {
	dir = filepath.Dir(path)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		dir = path
	}
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		return dir, "missing"
	case !info.IsDir():
		return dir, "notdir"
	}
	return dir, ""
}

func openInFileManager(dir string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap it; explorer exits non-zero even on success
	return nil
}

// fyneProfile mirrors a saved connection in the config.json of the Fyne
// desktop app this one replaced (its internal/appconfig, at the git tag
// fyne-desktop-final). Reading it lets existing users start signed in.
type fyneProfile struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	ForgetToken bool   `json:"forget_token,omitempty"`
}

type fyneConfig struct {
	URL         string        `json:"url"`
	Token       string        `json:"token"`
	ForgetToken bool          `json:"forget_token,omitempty"`
	Theme       string        `json:"theme,omitempty"`
	Palette     string        `json:"palette,omitempty"`
	Language    string        `json:"language,omitempty"`
	Profiles    []fyneProfile `json:"profiles,omitempty"`
	Active      string        `json:"active,omitempty"`
}

// fyneConfigPath is where the Fyne app keeps its settings, by the same rule
// it uses: AGENTFLOW_STUDIO_CONFIG, else the OS config directory.
func fyneConfigPath() (string, error) {
	if p := os.Getenv("AGENTFLOW_STUDIO_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agentflow-studio", "config.json"), nil
}

// readFyneConfig returns the Fyne app's connections, with a config written
// before profiles existed (a bare url and token) read as one profile.
func readFyneConfig() map[string]any {
	empty := map[string]any{"found": false, "profiles": []fyneProfile{}}
	path, err := fyneConfigPath()
	if err != nil {
		return empty
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || err != nil {
		return empty
	}
	var c fyneConfig
	if json.Unmarshal(b, &c) != nil {
		return empty
	}
	profiles := c.Profiles
	if len(profiles) == 0 && c.URL != "" {
		profiles = []fyneProfile{{URL: c.URL, Token: c.Token, ForgetToken: c.ForgetToken}}
	}
	kept := profiles[:0]
	for _, p := range profiles {
		if p.URL == "" {
			continue
		}
		if p.ForgetToken {
			p.Token = ""
		}
		kept = append(kept, p)
	}
	return map[string]any{
		"found": true, "profiles": kept, "active": c.Active,
		"theme": c.Theme, "palette": c.Palette, "language": c.Language,
	}
}
