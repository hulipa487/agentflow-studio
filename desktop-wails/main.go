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

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// onReadyHooks run once the page has loaded; only test builds add any.
var onReadyHooks []func(context.Context)

func main() {
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

	err = wails.Run(&options.App{
		Title:     "AgentFlow Studio",
		Width:     state.Width,
		Height:    state.Height,
		MinWidth:  minWidth,
		MinHeight: minHeight,
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
			About: &mac.AboutInfo{Title: "AgentFlow Studio", Message: version},
		},
		// The Linux program name (which also names the webview's data folder)
		// is set in prgname_linux.go, before GTK starts.
		Linux: &linux.Options{},
	})
	if err != nil {
		log.Fatal(err)
	}
}
