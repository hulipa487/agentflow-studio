# AgentFlow Studio desktop on Wails v2 (spike)

A proof of concept for replacing the Fyne desktop app (`../desktop`) with the
Studio web app (`../web`) shown in the system webview by
[Wails v2](https://wails.io). The Fyne app is untouched; this directory builds
a second binary next to it.

## What it is

- **The UI is the web app.** `make web` builds `../web` and copies it into
  `frontend/dist`, which is embedded in the binary.
- **`proxy.go`** is the web app's local proxy (`../web/server/proxy.mjs`)
  ported to Go and served from Wails' asset handler, so the app reaches a
  runtime over HTTP, a Unix socket or a Windows named pipe with no CORS setup.
  The web app detects it through `/__af/__ping` and routes through it, as it
  does under the Vite dev server.
- **`window.go`** remembers window size, position and maximised state in
  `<config dir>/agentflow-studio/desktop/window.json`.
- Connections, language and theme are kept by the web app in the webview's
  localStorage, as in a browser.
- A second launch brings the running window forward (Wails' single-instance
  lock).
- `probe.go` (build tag `probe` only) is a test aid: a loopback control socket
  that runs JavaScript in the window and resizes it, used for the checks below.

## Build

    make build        # make web, then go build -tags desktop,production[,webkit2_41]
    make test         # proxy tests: TCP and Unix targets, refusals, streaming

No Wails CLI is needed. Linux builds need the WebKitGTK 4.1 development files
(`libwebkit2gtk-4.1-dev`) and run with `libwebkit2gtk-4.1-0`. Windows runs on
the WebView2 runtime (part of Windows 11 and current Windows 10); macOS uses
the system WKWebView. Only the Linux build was tried.

## Findings (Linux ARM64 dev board, 2026-11-20)

**It builds and runs here.** The board has the WebKitGTK 4.1 runtime but not
its headers and no root, so the headers were unpacked into a local prefix with
`apt-get download` and `PKG_CONFIG_PATH`. A normal machine installs
`libwebkit2gtk-4.1-dev` instead.

**Resizing.** All 14 screens (overview, agents list, builder, playground,
channels, models, memory, safety, users, usage, logs, config, docs, settings)
were opened at the minimum size, 960×600, and at 1700×1000. Every one reflows
the way it does in a browser: nothing clipped, overlapping or misaligned; the
overview grid goes from 2 to 4 columns, the docs gain their table of contents,
editors fill the space. See `docs-shots/`. Resizing is the browser's CSS
layout, so the Fyne class of resize bugs does not exist here.

**Proxy.** Login, every screen and the live log tail work through the Go proxy
against a local runtime; the log stream arrives line by line (WebKitGTK passes
streamed responses through Wails' asset handler). Named pipes are compiled for
Windows but untested.

**Persistence.** Closing and reopening the app restores the window size and
reconnects to the remembered runtime with no login.

| | Fyne (`../desktop`) | Wails |
|---|---|---|
| Binary | 34 MB | 12 MB (plus the system webview) |
| Memory, PSS of all processes | ~125 MB (login screen) | ~333 MB (connected, overview) |
| Processes | 1 | 3 (app, WebKit web process, WebKit network process) |
| Resize behaviour | custom layouts, known bugs | browser layout, no issues found |
| UI code to maintain | ~21,500 lines of Go, a second copy of every screen | ~300 lines of Go; the screens are the web app's |

The memory figures are not like for like (different screens), but a webview
app costs a few hundred MB more than Fyne, mostly in the WebKit web process.

## Gaps before it could replace the Fyne app

1. **File downloads.** The web app saves exports (user data, logs) with a blob
   link. In WebKitGTK under Wails that writes the file straight into the home
   directory with no dialog. Needs a small Go endpoint (for example
   `POST /__af/__save`) that opens `runtime.SaveFileDialog`, and `downloadText`
   in the web app using it when `/__af/__ping` reports `"shell": "wails"`.
2. **Open config folder.** The Fyne app can open the runtime's config directory
   in the file manager (`desktop/internal/ui/openfolder.go`); the web app has no
   equivalent. Needs a Go endpoint plus a button that appears only in the shell.
3. **Settings migration.** Fyne keeps its connection profiles in
   `appconfig` (`<config dir>/agentflow-studio/...`); the web app keeps them in
   localStorage. A first-run import from the Fyne file would carry users over.
4. **Webview profile location.** On Linux, Wails names the webview's data
   directory after the executable, so renaming the binary loses localStorage.
   Ship it under one fixed name (or set the data path explicitly once Wails
   exposes it on Linux). On Windows it is pinned to the app data dir here.
5. **Windows and macOS** were not built or run. Packaging (installer, icon,
   WebView2 bootstrapper or fixed runtime for offline installs, macOS bundle
   and signing) is not started.
6. **Background colour.** The window background is the light theme's, so in
   the dark theme a resize can briefly show light edges; it should follow the
   theme (`runtime.WindowSetBackgroundColour` when the theme changes).

Once those are done, `../desktop/internal/ui` (and the Fyne dependency) can go,
and every Studio feature is built once, in `../web`.
