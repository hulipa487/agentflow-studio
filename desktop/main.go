// AgentFlow Studio as a desktop app: the Studio web app, shown in the
// system's webview by Wails v2.
//
// The web app is the whole UI. This program only adds what a browser tab
// cannot do on its own:
//   - the local proxy (proxy.go), so the app reaches a runtime over HTTP, a
//     Unix socket or a Windows named pipe without any CORS setup;
//   - a window that remembers its size, position and maximised state
//     (window.go).
//
// Everything else the app remembers (connections, language, theme) the web
// app keeps in the webview's localStorage, exactly as it does in a browser.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The production build of ../web, copied here by `make web`.
//
//go:embed all:frontend/dist
var embedded embed.FS

// The app icon. Windows takes it from the .exe's resources
// (rsrc_windows_*.syso, made from winres/ with `make winres`); Linux and
// macOS are handed it here.
//
//go:embed winres/icon256.png
var appIcon []byte

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// onReadyHooks run once the page has loaded; only test builds add any.
var onReadyHooks []func(context.Context)

// wantFrameless decides whether the window drops the native title bar. On
// Windows it does: its white caption bar clashes with the app's own top bar,
// which then carries the minimise, maximise and close buttons and drags the
// window. Linux and macOS keep the title bar the desktop draws.
// AGENTFLOW_STUDIO_FRAMELESS=1 or 0 overrides either way.
func wantFrameless() bool {
	switch os.Getenv("AGENTFLOW_STUDIO_FRAMELESS") {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return goruntime.GOOS == "windows"
}

func main() {
	cleanupOldUpdateFiles()

	assets, err := fs.Sub(embedded, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		log.Fatal("the web app is not embedded: run `make web` before building")
	}

	dataDir, err := appDataDir()
	if err != nil {
		log.Fatal(err)
	}
	state := loadWindowState(dataDir)
	var ctx context.Context
	proxy := newProxy()
	frameless := wantFrameless()
	proxy.desktop.frameless = frameless
	// The app's own close button: save the window like the native one does,
	// then quit.
	proxy.desktop.close = func(c context.Context) {
		saveWindowState(dataDir, currentWindowState(c, state))
		runtime.Quit(c)
	}

	err = wails.Run(&options.App{
		Title:     "AgentFlow Studio",
		Width:     state.Width,
		Height:    state.Height,
		MinWidth:  minWidth,
		MinHeight: minHeight,
		// Without the native title bar the web app draws the window controls
		// in its own top bar (see wantFrameless).
		Frameless: frameless,
		WindowStartState: func() options.WindowStartState {
			if state.Maximised {
				return options.Maximised
			}
			return options.Normal
		}(),
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: proxy,
		},
		// The light theme's page background, for the first paint. The web app
		// sends its own background whenever the theme changes, so a resize
		// never shows a different colour at the edges.
		BackgroundColour: &options.RGBA{R: 246, G: 249, B: 252, A: 255},
		OnStartup: func(c context.Context) {
			ctx = c
			proxy.desktop.setContext(c)
		},
		OnDomReady: func(c context.Context) {
			if state.HasPosition && !state.Maximised {
				runtime.WindowSetPosition(c, state.X, state.Y)
			}
			for _, hook := range onReadyHooks {
				hook(c)
			}
		},
		OnBeforeClose: func(c context.Context) bool {
			saveWindowState(dataDir, currentWindowState(c, state))
			return false
		},
		// A second launch brings the running window forward instead of
		// opening another.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "dev.agentflow.studio",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if ctx == nil {
					return
				}
				runtime.WindowUnminimise(ctx)
				runtime.WindowShow(ctx)
			},
		},
		Windows: &windows.Options{
			// localStorage (connections, language, theme) lives here rather
			// than in a folder named after the executable.
			WebviewUserDataPath: dataDir,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{Title: "AgentFlow Studio", Message: version, Icon: appIcon},
		},
		// The Linux program name (which also names the webview's data folder)
		// is set in prgname_linux.go, before GTK starts.
		Linux: &linux.Options{Icon: appIcon},
	})
	if err != nil {
		log.Fatal(err)
	}
}
