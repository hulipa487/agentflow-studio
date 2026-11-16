# AgentFlow Studio (desktop)

A native desktop client for AgentFlow: build an agent, apply it live, chat with
it, and watch every step of the turn — without a browser and without CORS.

Built with [Fyne](https://fyne.io/) (Go + native widgets). It talks to the
runtime's existing `/admin/api/studio/*` API; the runtime remains the source of
truth for config, persistence, memory and traces.

## Features

- **Overview** — the landing screen: instance, region, version, uptime and
  config epoch; live sessions (busy or idle); configured agents and channels;
  whether the credential store is on; and sparklines of the runtime's sampled
  counters (LLM calls and tokens, inbound messages, safety drops). Refreshes
  every five seconds while open.
- **Agents** — list configured agents with model, capabilities, live sessions
  and whether the definition is persisted; create from a template or a blank
  definition, edit, delete.
- **Builder** — form for model, safety, capabilities, skills, history budget,
  daemon flag, `can_contact`, credentials and extras; a real Luau code editor
  with syntax highlighting, line numbers, wavy diagnostics and hover errors,
  automatic completion with ghost text, inline symbol docs and IDE keybindings;
  instructions editor; raw YAML. **Validate** checks the prelude directive, compiles the loop
  and runs boot-equivalent validation. **Apply** installs it live; **Apply &
  save** persists into the deployment config.
- **Playground** — threaded chat, one session per thread, optional act-as-user
  tenant, streamed replies, and a live trace of turns, ops, model/tool calls,
  tokens, durations, safety decisions and errors.
- **Documentation** — the full AgentFlow guide set (getting started, architecture,
  configuration reference, Luau loops, extension points) is bundled in the app
  and rendered natively from the Help button. Search, section navigation and
  internal links work offline, including when the runtime uses only a Unix
  socket or named pipe.
- **Channels** — list the gateway's webhook, Telegram, GitHub-webhook and bridge
  channels; add, edit and delete them; validate against the deployment; and
  persist into `channels.yaml` (configdir) or `gateway.channels` (single file).
  Secrets are write-only and preserved when left blank; changes bind at the next
  runtime start, and the screen marks which definitions are live.
- **Models** — manage the deployment's LLM endpoints in place: add, edit and
  delete models, test them with a real one-token call, and save the live set back
  to `models:` in the config or revert to the file. API keys are write-only and
  preserved when left blank; the screen flags runtime-only and unsaved entries.
- **Safety** — edit the core-owned filter surface: named `profiles.safety`
  chains (ordered builtin filters and `classifier:<name>` references) and
  `safety.classifiers` policies (model, prompt, mode, directions, sources,
  threshold, label actions, failure policy, cache and budget). Dry runs use the
  same validation boot uses; usage on agents and spawn profiles is shown, and an
  in-use profile or classifier cannot be deleted. Changes bind at the next
  runtime start.
- **Credentials** — browse the encrypted credential store by bucket (engine-wide
  or per-user), create, edit and revoke secrets, and edit the header/scheme used
  to inject them. Values are write-only: the screen shows only the last-four
  fingerprint, and leaving the secret field blank keeps the stored value.
- **Users** — the identity layer's profiles: search, today's quota against the
  limit, linked channel handles (with trust and last seen, and unlink), file
  projects, recent metered calls and the audit trail. Set a profile's daily
  token limit and its model and instruction layer, issue single-use
  registration invites, export everything held for a person to a JSON file, or
  purge a profile (typed confirmation).
- **Tenancy** — groups (create, add and remove members, hand over the master,
  disband) and named memory pools (create with an owner and an immutable
  retention, then grant agents read or read/write access per tenant or for
  every member). Live: the registry is in the runtime store.
- **Usage** — the token ledger for any UTC day: billable totals, calls and
  failures, cached and reasoning tokens, grouped by tenant, agent or model with
  each one's share, or every slice.
- **Logs** — the runtime's live log tail with a level floor, text filter, pause
  with a pending count, copy one line or all visible lines, and automatic
  reconnect.
- **Config** — the raw config file for everything without its own screen
  (memory, prompts, tools, MCP, triggers, search, files, net, runtime):
  boot-equivalent **Validate**, an mtime-guarded atomic **Save** that keeps a
  `.bak`, and a read-only masked view of what the runtime parsed. A
  config-directory deployment is explained instead of edited.
- **Go to (Ctrl+K)** — a command palette over every screen, every agent
  ("Edit echo", "Chat with assistant") and the global actions (new agent,
  refresh, light/dark, invite, lock).
- **Connection health** — the rail's dot follows a 15-second heartbeat and a
  toast announces when the runtime goes away or comes back.
- **Native** — the admin token is sent directly; there is no browser origin, so
  no `runtime.admin.cors_origins` entry is needed. On the same machine the
  connection can use a Unix socket or Windows named pipe instead of a TCP port.
- **Designed for the work** — a trans-pride light/dark shell with a navigation
  rail, breadcrumbs, a compact sidebar footer, an inspector beside the builder, state chips,
  inline banners, loading states, a responsive chat pane and a searchable agent
  list. Mixed-height rows (chips, dots, labels and icon buttons) are vertically
  centred so headers and cards stay on one baseline.
- **Settings** — Appearance and connection in one place, reached from the gear
  in the sidebar footer. The Light/Dark switch and palette picker live here.
  The choice is saved in the app config; on first launch
  the app follows the OS variant. Switching remaps the current screen in place,
  so unsaved builder edits, scroll positions and the current playground thread
  survive.

## Loop editor: an IDE, not a textbox

The Luau loop editor is backed by a language server that lives **inside
Studio**. Nothing is added to the AgentFlow core binary and no external process
is started: `internal/lua` implements the analyzer and `internal/lua/lsp`
implements the LSP 3.17 subset, and the editor connects to it over an in-memory
pipe (`net.Pipe`) exactly as it would to an external server.

The editor itself is a purpose-built Fyne widget: a line-number gutter,
syntax-highlighted Luau, current-line and selection rendering, horizontal and
vertical scrolling, auto-indent, auto-paired brackets/quotes, multi-line paste,
undo/redo, and IDE keybindings (Ctrl+S, Ctrl+C/X/V/A/Z/Y, Ctrl+Home/End,
word movement, Shift+arrow selection, Ctrl+Backspace/Delete word deletion,
double-click word/whitespace selection, occurrence highlighting for the
selected text, Alt+Up/Down to move a line, Tab/Shift+Tab indent).

**Diagnostics** are published as you type (`textDocument/publishDiagnostics`,
full sync) and drawn *inside* the editor: a severity marker in the gutter and a
wavy squiggle under the affected token. The message appears in a compact card
only while the pointer is over that squiggle, so it never covers unrelated
code.

Diagnostics cover:

- syntax and structure: unterminated strings, mismatched brackets, unclosed
  blocks, `repeat`/`until` mismatches;
- the chunk contract: exactly one `-- af-prelude-version: N`, compared with the
  runtime's `prelude_version` from the catalog, and a global `function loop()`
  (a `local function loop` is called out specifically);
- API surface: unknown prelude globals and fields with "did you mean"
  suggestions (`llm.chta` → `llm.chat`), the five support-chunk globals
  (`token_budget`, `memory_*`, `fact_extractor_turn`), and sandbox omissions
  (`io`, `debug`, `require`, ...);
- capability checks against the agent's **effective** capabilities — the
  selected list, or the deployment defaults when the list is empty. Toggling a
  capability in the form re-lints immediately via
  `workspace/didChangeConfiguration`;
- AgentFlow-specific hazards: `llm.stream` in a generic `for`, yielding ops at
  chunk top level, `return` from `loop()`, `af.op`, and maintenance-only calls
  such as `user.get` / `store.scopes`.

**Completion is automatic.** After an identifier or a `.`/`:` the editor asks
the language server for candidates, filters them by the typed prefix, and shows
an inline popup anchored at the caret; the selected candidate is previewed as
ghost text. Up/Down move the selection, Enter or Tab accepts, Esc dismisses, and
Ctrl+Space opens it manually. Insertions replace the typed prefix (snippets are
reduced to their default text), so `llm.ch` completes to `llm.chat` without
duplicating text. After `session.` or `llm.` it lists that namespace's fields.

Hovering a symbol (or a diagnostic squiggle) shows a compact card at the
pointer with the signature, summary, required capability and yield/maintenance
notes. Nothing is pushed into a footer below the code.

Chunks that declare `-- af-import: <package>` are treated leniently for unknown
globals: a package's exports are concatenated before the chunk at boot and are
not visible to the analyzer, so reporting every package call as a typo would be
wrong. The import directive itself is still checked by the runtime's validator.

The analyzer only knows the documented prelude; the runtime's **Validate** is
still the authoritative compile and boot-equivalence check. The example loops
in `examples/loops` are part of the test suite (`internal/lua/testdata`) and
must lint clean.

## Channels

The **Channels** screen edits the deployment's `gateway.channels`, which is the
front door for webhook, Telegram, GitHub-webhook and bridge traffic. Unlike agents,
channels are constructed at boot: the screen validates and persists an edit, and
the runtime binds it at the next start. Each card shows whether the definition
is `live` (the boot set still matches) or `restart required`, and the header says
how many saved changes are pending.

The editor covers the full `config.Channel` surface: name, type, target agent,
route path, Telegram mode and allowed user IDs, webhook timeout/async, media
policy, and the type-specific secrets. Validation is the runtime's own
`config.ValidateChannels` plus Studio's name/uniqueness rule, so a save cannot
produce a config the next boot would refuse (unknown agent, duplicate path,
bad timeout, unauthenticated ghhook, and so on).

Secrets are never sent to the desktop app. The API returns `has_token` /
`has_secret` / `has_secret_token` and an env/credential reference such as
`${TELEGRAM_BOT_TOKEN}` when the stored value is one; leaving a secret field
blank in the editor keeps the stored value, and each existing secret has a
**remove** checkbox. In configdir mode the write is atomic, keeps
`channels.yaml.bak`, and reloads the whole directory to validate cross-file
references; in single-file mode the `gateway.channels` node is replaced in
place, preserving comments, and written through the same validate-then-swap
path as the config editor. An mtime check refuses a save if the file changed
under you.

## Safety

The **Safety** screen edits `profiles.safety` and `safety.classifiers`. A
profile is an ordered chain: the five compiled-in builtin filters
(`source-attribution`, `signal-gate`, `steady-directive`, `support-offer`,
`affect-guard`) plus any number of `classifier:<name>` filters. Order matters,
and the preview shows the chain the dispatcher will build.

A classifier is a model-backed policy: the runtime asks an engine-keyed model
for a label and a score and applies this screen's `threshold`, `categories`
(`allow`, `review`, or `block`), directions and sources. Failure knobs
(`on_error`, `on_saturated`, `on_budget_exhausted`) default to `block`, and
`mode: shadow` records verdicts without enforcing them. The editor covers the
prompt, timeout, cache, daily classifier budget, refusal and fallback text too.

Validation runs the runtime's own safety checks against the config on disk — an
unknown filter, an unknown or user-keyed classifier model, or a missing `allow`
label is refused. A profile still named by an agent or spawn profile, and a
classifier still referenced by a profile, cannot be deleted. Unlike models,
safety dispatch is built when an agent starts, so saved edits are durable but
bind at the **next runtime start**; the screen marks each entry `live` or
`restart required`. In configdir mode the write replaces only
`profiles.safety` and `safety` in `system.yaml`, keeps a `.bak`, and reloads the
whole directory before accepting the result. Single-file mode preserves the rest
of the document and comments through validate-then-swap.

## Credentials

The **Credentials** screen manages the encrypted store that backs
`cred:<service>`, `credential.get()`, HTTP `auth`, mail passwords and
`key_source: user` models. Pick the engine-wide bucket or a user UUID (a new
bucket can be opened before it holds anything), then add, edit or revoke a
credential. Each card shows the service, kind, last-four fingerprint, the
`header: scheme <secret>` shape used for injection, and when it last changed.

Secrets are write-only end to end: `GET` returns metadata only, and there is no
form field that can reveal a stored value. Leaving the secret blank on an edit
keeps the encrypted value and updates only the metadata. Writes are live
immediately (the store is consulted at request time), which makes this screen
different from Safety and Channels. If `runtime.credentials.enabled` is false
or no master key was supplied, the screen says so and links the required
setting instead of failing.

## Documentation

The Help button in the header opens the documentation screen. Content lives in
[`docs/`](docs) as ordinary Markdown files, and the app embeds the directory at
build time — there is no documentation server and no runtime `/docs` dependency.
Pages are grouped into Start here, Build and Reference in the sidebar; search
filters the page list and shows a matching line for each hit. Internal
`*.md` links navigate inside the viewer, external links open in the browser,
and the OpenAPI contract and Luau type definitions open with their OS handler.

`docs/openapi/users.yaml` and `docs/luau/af.d.luau` mirror
`api/openapi/users.yaml` and `api/luau/af.d.luau` in the runtime repo (both
contracts are versioned with the code there). Run `make docs-sync` (set
`AGENTFLOW_DIR` if the runtime checkout is not a sibling) when either changes.

## Theme

The palette uses the trans flag's blue, pink and white: a five-band accent line
under the header, a flag-gradient logo, blue/pink
selection states and tone-matched status chips. Both variants are contrast
checked for text, inputs and buttons.

The **Light** / **Dark** switch is in **Settings**, alongside the palette
picker. The setting is stored as `theme` in the app config
(`agentflow-studio/config.json` under the OS config directory); an empty value
follows the OS theme. The switch updates canvas colors and widget themes in
place rather than rebuilding the screen.

## Keyboard shortcuts

| Shortcut | Action |
|---|---|
| `Ctrl+K` (or `Ctrl+Shift+P`) | Go to: screens, agents and actions |
| `Ctrl+N` | New agent |
| `Ctrl+1` … `Ctrl+9` | Overview, Agents, Playground, Channels, Models, Credentials, Safety, Users, Logs |
| `F1` | Documentation |
| `F5` | Refresh the current screen |
| `Ctrl+S` | Apply in the builder; validate and save in Config |
| `Ctrl+Space` | Prelude API completion in the loop editor |
| `Ctrl+Enter` | Send in the playground |
| `Esc` | Back (from the builder to Agents, from any other screen to Overview) |

The rail groups the screens as **Home** (Overview), **Build** (Agents,
Playground), **Connect** (Channels, Models, Credentials), **Govern** (Safety,
Users, Tenancy), **Operate** (Usage, Logs, Config) and **Studio**
(Documentation, Settings). Tenancy, Usage and Config have no number key; use
the rail or `Ctrl+K`.

A dirty builder window marks the title with `•` and asks before discarding.

## Window

On Windows the window is **visually frameless with native window behavior**. The
window keeps its normal resize frame and caption styles internally, but the app
subclasses the window procedure:

- `WM_NCCALCSIZE` makes the client area cover the native frame, so nothing is
  painted and the content reaches every edge.
- `WM_WINDOWPOSCHANGING` removes the hidden frame from GLFW's programmatic
  resize requests (they carry `SWP_NOACTIVATE`). Without this, every Fyne
  `Resize` — including the one a modal dialog triggers — would grow the window
  by the frame and leave unpainted strips. Windows-initiated layout (Aero Snap,
  maximize/restore) is left untouched. The one size requested before the
  subclass is installed (the first window) is corrected once at startup.
- `WM_NCHITTEST` classifies the surface: edges and corners are native resize
  handles; the top bar is `HTCAPTION` (drag to move, double-click to maximize,
  and **Aero Snap**); the top-right buttons are `HTMINBUTTON`,
  `HTMAXBUTTON` and `HTCLOSE` — `HTMAXBUTTON` also enables the Windows 11
  **Snap Layouts** flyout; everything else is `HTCLIENT`.
- `WM_GETMINMAXINFO` sets a minimum size of 1080×700 so the three-pane
  playground and builder header never collide, and DWM supplies the shadow,
  rounded corners and border styling. When maximized, the client area is
  clamped to the monitor work area so content cannot slide under the taskbar.

The window controls are drawn in the app's top bar but behave like native
caption buttons: minimize, maximize/restore and close sit at the right, with a
split-pane button on the main screen that collapses the navigation rail. Hover
surfaces and the red close treatment come from `WM_NCMOUSEMOVE`; the hit regions
are derived from the controls' actual laid-out positions, so they stay aligned
when Fyne's scale differs from the Windows DPI. Because GLFW suppresses the
default non-client click handling, `WM_NCLBUTTONDOWN` is handled directly for
`HTMINBUTTON`, `HTMAXBUTTON` and `HTCLOSE`, issuing the same native
minimize/maximize/restore/close commands. The small `◢` grip in the bottom-right
remains as a secondary resize affordance. Other platforms keep their native
window frame.

The geometry is stable: switching tabs, opening a dialog, dragging an internal
split or toggling the rail/theme never changes the window size. Dragging an
edge resizes it normally, Aero Snap keeps the system's exact snapped rect, and
maximize/restore returns to the same size.

## Requirements

- Go 1.27+ (the exact toolchain is recorded in `go.mod`)
- CGO and a C/C++ toolchain:
  - Windows: [zig](https://ziglang.org/) (`CC="zig cc" CXX="zig c++"`), or a mingw-w64 gcc
  - Linux: `gcc`/`g++`
  - macOS: Xcode command-line tools (`clang`)
- A running AgentFlow instance whose admin server is reachable, with
  `runtime.studio.enabled: true` (the default)

## Build and run

```bash
# Windows (hermetic, no system gcc)
CGO_ENABLED=1 CC="zig cc" CXX="zig c++" \
  go build -ldflags "-H=windowsgui -extldflags=-Wl,--subsystem,windows" \
  -o agentflow-studio.exe .
./agentflow-studio.exe

# Linux / macOS
CGO_ENABLED=1 go build -o agentflow-studio .
./agentflow-studio

# or
make build
make run
```

On Windows the subsystem matters: Go defaults to the **console subsystem**, so a
plain `go build` opens a CMD window next to the Fyne UI. With cgo the final PE
is produced by the external linker (zig), which ignores `-H=windowsgui` on its
own — `-extldflags=-Wl,--subsystem,windows` is what sets the GUI subsystem.
`make build` passes both automatically. Omit them only if you want stdout/stderr
on a console (the app does not log there).

Tests (including a headless Fyne view-construction test):

```bash
make test          # go test ./...

# Render every screen to PNGs for visual review (both themes, the loop editor
# with and without diagnostics, and the minimum window size):
STUDIO_SNAPSHOT_DIR=./snapshots go test ./internal/ui -run TestSnapshots
```

## Connecting

On first launch enter the instance URL and the admin token — `ADMIN_TOKEN`, the
per-boot token printed at startup, or a named operator token. The **Transport**
switch (HTTP, Unix socket, Named pipe) fills in a typical address, and the hint
under the URL says what it will reach before you connect. While connecting the
button becomes **Cancel** and shows each step. A failure points at the field to
fix: a rejected token marks the token, an address where nothing answers marks
the URL. The URL accepts a TCP origin, a Unix socket, or a Windows named pipe:

```
http://127.0.0.1:9090
unix:///run/agentflow/admin.sock
npipe://agentflow-admin
```

For a runtime on the same machine, enable the local transport and optionally
turn the TCP listener off:

```yaml
runtime:
  admin:
    tcp: false
    socket: /run/agentflow/admin.sock   # Windows: '\\.\pipe\agentflow-admin'
```

The socket is owner-only (0600 on Unix; the named pipe's DACL grants the owner,
Administrators and SYSTEM) and the bearer token is still required. The browser
console is served on the HTTP listener, so keep `tcp: true` if you use it; the
documentation is bundled in Studio and works on every transport. A relative
socket path is resolved against the config
directory. `-no-console` only drops the
console's static files: the admin and Studio APIs stay up, so Studio keeps
working with the console off.

The app verifies `GET /admin/api/state` and loads
`GET /admin/api/studio/catalog` before saving the connection. A wrong URL or
token is not persisted, so the next launch starts clean.

With **Remember the token on this computer** checked (the default) the token is
stored in plain text in the user's config directory; unchecked, only the URL is
kept and the token is asked for at each launch (`forget_token` in the file):

- Windows: `%APPDATA%\agentflow-studio\config.json`
- Linux: `~/.config/agentflow-studio/config.json`
- macOS: `~/Library/Application Support/agentflow-studio/config.json`

Set `AGENTFLOW_STUDIO_CONFIG=/path/to/config.json` to use another location.
The file is written with owner-only permissions. Treat the admin token like a
shell history secret: it can install agent logic and invoke their tools.

## Persistence and playground notes

Persistence is the runtime's: a config-directory deployment writes
`profiles/<name>.yaml`, `loops/<name>.lua` and `prompts/<name>.md`; a
single-file deployment writes the `agents.<name>` key plus a studio workspace
for loops/prompts. **Apply** is live immediately; the persisted definition loads
at the next boot. A profile the Studio did not create is refused unless forced.

Playground turns go through the runtime's synthetic `studio` channel, so safety,
journaling and normal egress still apply. Without **Act as user**, turns are
tenant-less: personal memory scopes, quotas and pool grants do not apply
(pool-bound stores will refuse). Chat history is the agent's memory, not the
desktop app; trace rings are in memory and reset with the runtime. In a fleet,
the playground is instance-local: a thread whose session is owned by another
instance will not capture replies here.

## Layout

```
main.go                 Fyne app bootstrap
internal/api            admin API client + SSE parsers (no UI dependency);
                        operate.go covers state, metrics, logs, config,
                        users, usage and tenancy
internal/appconfig      URL/token persistence
internal/docs           embedded documentation library (manifest, search, links)
internal/lua            Luau analyzer and in-process language server
internal/ui             the shell and every screen:
  nav.go                destinations, rail sections, shortcuts, enter()
  palette.go            Ctrl+K command palette
  screen.go             page scaffold, async helper, tables, tiles, sparkline
  overview.go users.go tenancy.go usage.go logs.go config.go
  agents.go builder.go playground.go channels.go models.go safety.go
  credentials.go docs.go settings.go
docs/                   bundled Markdown guides + OpenAPI and Luau assets
```

Adding a screen is one entry in `buildNav` (id, label, section, icon,
optional Ctrl+number, palette hint) plus its `show` function, which starts
with `a.enter("<id>")` and builds on `newScreen` or `newPane`.

## Packaging (optional)

Install the Fyne CLI and produce a packaged app:

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -os windows -name "AgentFlow Studio"
```

Add an icon with `-icon icon.png`. Packaging is not required to run the binary.
