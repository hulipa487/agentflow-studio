# AgentFlow Studio desktop

The desktop app is the Studio web app (`../web`) shown in the system's
webview by [Wails v2](https://wails.io). Every screen is the web app's; this
directory adds only what a browser tab cannot do, in about 600 lines of Go.

It replaced a native Fyne app on 2026-11-20. That app is at the git tag
`fyne-desktop-final`.

## What the Go side does

- **Embeds the web app.** `make web` builds `../web` and copies it into
  `frontend/dist`, which is compiled into the binary.
- **The local proxy (`proxy.go`).** The web app's proxy
  (`../web/server/proxy.mjs`) in Go, served from Wails' asset handler. The app
  reaches a runtime over HTTP, a Unix socket or a Windows named pipe with no
  CORS setup; the web app finds the proxy through `/__af/__ping`, as it does
  under the Vite dev server.
- **The desktop bridge (`desktop.go`).** Answers under `/__af/__desktop/`. The
  web side is `../web/src/lib/desktop.ts`, used only when `/__af/__ping`
  reports `"shell": "wails"`:
  - `save`: exports (user data, usage CSV, docs files) open the native save
    dialog;
  - `open-folder`: the Channels screen's "Open config folder" button, when the
    runtime's config is on this machine;
  - `fyne-config`: on the first run, the old Fyne app's saved connections, and
    its language, theme and palette where the app has none of its own yet;
  - `background`: the window colour follows the page's theme, so a resize
    never shows mismatched edges;
  - `close`: the app's own close button (see the next point);
  - `update/status`, `update/check`, `update/apply`, `open-url`: self-updater
    against GitHub Releases (`SHA256SUMS.txt` verification, atomic replacement
    on Windows/macOS `.app`/writable Linux installs, and release-page fallback
    for `.deb` system installs).
  - `secrets/status`, `secrets/get`, `secrets/set`, `secrets/delete`: OS
    keychain storage (`windows-credman` via Win32 `advapi32.dll`,
    `macos-keychain` via `/usr/bin/security`, `linux-secret-service` via
    `secret-tool`) so remembered tokens are stored outside plaintext
    `localStorage`.
- **No native title bar on Windows.** The window is frameless there and the
  app's top bar is the title bar: its empty space drags the window, a
  double-click maximises, and minimise / maximise / close sit at its right end
  (`../web/src/components/shell/WindowControls.tsx`). The login and
  reconnecting screens get a thin strip with the same buttons. Linux and macOS
  keep the title bar their desktop draws. `AGENTFLOW_STUDIO_FRAMELESS=1` or `0`
  overrides either way.
- **The window (`window.go`)** remembers its size, position and maximised
  state in `<config dir>/agentflow-studio/desktop/window.json`. A second launch
  brings the running window forward.
- **Storage.** Connections (addresses and profile metadata), language and theme
  live in the webview's localStorage (`$XDG_DATA_HOME/agentflow-studio` on
  Linux, whatever the binary is called via `prgname_linux.go`, and the app data
  folder on Windows). Remembered admin tokens are stored in the OS keychain
  (`keychain.go` / `keychain_*.go`) and hydrated into memory at startup.
- **Icon.** `winres/` holds the icon (16 to 256 px), the Windows manifest
  (per-monitor DPI awareness, Common Controls v6, long paths) and the version
  info. `make winres` turns them into the committed
  `rsrc_windows_{amd64,arm64}.syso`, so a plain `go build` on Windows includes
  them. Linux and macOS get the icon through Wails' options.

`probe.go` (build tag `probe` only) is a test aid: a loopback control socket
that runs JavaScript in the window and resizes it, for scripted screen checks.

## Build

    make build        # make web, then go build -tags desktop,production[,webkit2_41]
    make run          # build and run
    make windows      # cross-compile Windows amd64 and arm64 .exe files (no cgo)
    make winres       # regenerate the Windows resources after changing winres/
    make test         # proxy and desktop-bridge tests

No Wails CLI is needed: a Wails v2 app builds with plain `go build` and the
`desktop,production` tags.

| OS | To build | To run |
|---|---|---|
| Linux | Go, a C toolchain, `libwebkit2gtk-4.1-dev` (Debian/Ubuntu) | `libwebkit2gtk-4.1-0` |
| Windows | Go only (or cross-compile with `make windows`) | the WebView2 runtime, part of Windows 11 and current Windows 10 |
| macOS | Go and Xcode's clang, on a Mac | nothing extra |

## Where it has been run

- **Linux** (ARM64): every screen at 960×600 and 1700×1000 (`docs-shots/`),
  the proxy and the live log tail, restart persistence, first-run import of a
  Fyne config, the save dialog, the title-bar buttons in frameless mode.
- **Windows 10/11**: `amd64` and `arm64` `CGO_ENABLED=0` binaries built cleanly;
  frameless WebView2 window verified via live `probe` build (initial `1280x860`
  and `1100x720` resize, `[data-window-drag]` strip, `Minimise` / `Maximise` /
  `Close` controls, clean exit via `/__af/__desktop/close`, and Win32
  Credential Manager `windows-credman` roundtrip).
- **macOS**: not yet built or run on real Mac hardware (CI validates compilation
  on `macos-latest`).

| | This app | The Fyne app it replaced |
|---|---|---|
| Binary | 13 MB (plus the system webview) | 34 MB |
| Memory, all processes | about 333 MB connected | about 125 MB at the login screen |
| Resizing | the browser's layout | custom layouts with known bugs |
| UI code | the web app's screens | 21,500 lines of Go, a second copy of every screen |

## Still open

1. **macOS** has to be built and run on a Mac.
2. **Packaging & signing.** Cross-platform packaging scripts live under
   `desktop/packaging/{windows,macos,linux}` and are wired into
   `.github/workflows/release.yml` (Windows NSIS installer with WebView2
   bootstrapper check, macOS `.app` / `.dmg` / `.tar.gz`, Linux `.deb` +
   user-local `.tar.gz`, plus `SHA256SUMS.txt` used by the built-in updater).
   Windows Authenticode signing and Apple Developer signing / notarisation are
   not configured yet (macOS builds use ad-hoc `codesign`).
3. **Native dialogs follow the OS language,** not the app's: the save dialog
   is in the system language.
