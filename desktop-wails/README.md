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
- **`desktop.go`** answers under `/__af/__desktop/` with what a browser tab
  cannot do (`web/src/lib/desktop.ts` is the web side, used only when
  `/__af/__ping` reports `"shell": "wails"`):
  - `save`: exports (user data, usage CSV, docs files) open the native save
    dialog instead of the webview silently writing into the home folder;
  - `open-folder`: the Channels screen gets an "Open config folder" button, as
    in the Fyne app, when the runtime's config is on this machine;
  - `fyne-config`: on the first run the web app imports the Fyne app's saved
    connections (tokens only where Fyne remembered them), and its language,
    theme and palette where the web app has none of its own yet;
  - `background`: the window colour follows the page's theme, so a resize
    never shows mismatched edges.
- Connections, language and theme are kept by the web app in the webview's
  localStorage, as in a browser. On Linux that storage lives in
  `$XDG_DATA_HOME/agentflow-studio` whatever the binary is called
  (`prgname_linux.go`); on Windows in the app data folder.
- **No native title bar on Windows.** The window is frameless there and the
  app's own top bar is the title bar: its empty space drags the window, a
  double-click maximises, and minimise / maximise / close buttons sit at its
  right end (`web/src/components/shell/WindowControls.tsx`); the login and
  reconnecting screens get a thin strip with the same. The close button saves
  the window state like the native one. Linux and macOS keep the title bar
  their desktop draws; `AGENTFLOW_STUDIO_FRAMELESS=1` or `0` overrides either
  way.
- A second launch brings the running window forward (Wails' single-instance
  lock).
- `probe.go` (build tag `probe` only) is a test aid: a loopback control socket
  that runs JavaScript in the window and resizes it, used for the checks below.

## Build

    make build        # make web, then go build -tags desktop,production[,webkit2_41]
    make windows      # cross-compile Windows amd64 and arm64 .exe files (no cgo)
    make test         # proxy and desktop-bridge tests

No Wails CLI is needed. Linux builds need the WebKitGTK 4.1 development files
(`libwebkit2gtk-4.1-dev`) and run with `libwebkit2gtk-4.1-0`. Windows runs on
the WebView2 runtime (part of Windows 11 and current Windows 10); macOS uses
the system WKWebView. The Linux build was run; the Windows builds compile
(amd64 and arm64) but were not run; macOS needs cgo and the macOS SDK, so it
must be built on a Mac.

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

## Closed since the first spike

Checked in the running app with a fresh profile and a Fyne `config.json`:

- **First-run import:** the app opened in 繁體中文 and the dark theme taken
  from the Fyne config, and reconnected to the imported runtime with no login.
- **Save dialog:** the docs "Download" button opened the GTK save dialog with
  the file name filled in; cancelling it wrote nothing and showed no
  "downloaded" message. (Callers now toast only once the file is saved.)
- **Open config folder:** the button appears beside the config path on the
  Channels screen in the app, and not in a browser. A path that is not on this
  machine is reported instead of opening an empty window. Actually opening the
  file manager was not exercised here.
- **Storage location:** the webview's data folder is `agentflow-studio` for a
  binary named `studio-probe`.
- **Background colour:** covered by tests of the endpoint; the colour change on
  screen during a resize was not captured.

- **Windows icon, manifest and version info:** `rsrc_windows_amd64.syso` and
  `rsrc_windows_arm64.syso`, generated from `winres/` by `make winres`
  (go-winres), give the .exe the AgentFlow icon (16 to 256 px), a manifest with
  per-monitor DPI awareness, Common Controls v6 (native-styled dialogs) and
  long-path support, and version info ("AgentFlow Studio" 0.1.0). Extracting
  the resources back out of the built amd64 .exe confirmed all three. Linux
  and macOS get the same icon through Wails' options; on Linux it was not
  confirmed from the running window.

## Still open

1. **Windows and macOS were not run.** The Windows .exe files build here; they
   need a run on Windows 10/11 (WebView2, dialogs, named pipes). macOS has to
   be built and run on a Mac.
2. **Packaging.** No installer (NSIS/MSI with the WebView2 bootstrapper, or the
   fixed runtime for offline installs), no macOS bundle, signing or
   notarisation, no Linux .desktop file or package.
3. **Native dialogs follow the OS language,** not the app's: the GTK save dialog
   was in English while the app was in 繁體中文.

Once those are done, `../desktop/internal/ui` (and the Fyne dependency) can go,
and every Studio feature is built once, in `../web`.
