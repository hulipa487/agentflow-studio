<div align="center">
<h1>
🎛️ AgentFlow Studio
<p>
</h1>
<i> Build it, apply it live, watch every step. <p></p></i>
</div>


## What is AgentFlow Studio

> TL;DR - The workbench for [AgentFlow](https://github.com/hulipa487/agentflow): build agents, run them, operate the deployment

AgentFlow Studio is the client for an AgentFlow runtime. Connect to any deployment and you get the whole thing in one window: write a loop, apply it to the running engine, chat with it, and read the trace of what it did - then manage the models, channels, users and credentials it runs on.

It comes two ways, with the same screens:

| | Where it runs | Built with |
|---|---|---|
| [**Desktop**](desktop) | A native app on Windows, macOS and Linux | Go + [Fyne](https://fyne.io/) |
| [**Web**](web) | A browser tab, served from your own machine | React + TypeScript |

The desktop app is the one this page describes: no web server to host, no CORS to configure, and on the same machine it can skip TCP entirely and talk over a Unix socket or a Windows named pipe. The web app reaches the same runtime through a small local proxy; [its README](web/README.md) covers how.

- [x] A Luau editor with completion, diagnostics and hover docs?
- [x] Apply a loop to a live engine without restarting it?
- [x] A trace of every op, model call and safety decision in a turn?
- [x] The full documentation, offline, inside the app?
- [x] Light and dark, in trans-pride colours?
- [ ] Writes your loops for you? (you have an agent for that now)

The runtime stays the source of truth. Studio holds no agent state of its own: config, persistence, memory and traces all live in the deployment it is connected to.

## What's in the box

**🛠️ Build**
- **Agents** - every configured agent with its model, capabilities and live sessions. Create from a template or from blank, edit, delete.
- **Builder** - a workbench: a form for the agent's model, safety profile, capabilities, skills and credentials, a Luau loop editor, an instructions editor and the raw YAML, with a dock beside them for a **Run** panel (chat and trace with this agent) or the Luau reference (`F1`). **Validate** runs the same checks boot does. **Apply** is a split button: *Apply* installs the agent live, *Apply & save* also writes it into the deployment's config, and the button remembers your pick. `Ctrl+Enter` applies pending edits and sends the Run panel's message (or repeats the last one), so you can edit, run and read the trace without leaving.
- **Playground** - threaded chat against one agent, one session per thread, with streamed replies and a live trace: turns, ops, model and tool calls, tokens, durations, safety decisions and errors. An optional *act as user* runs the turn as a real profile, so memory scopes and quotas apply.

**✍️ The code editor**
- One editor for every document: the loop (Luau), the instructions (Markdown, soft-wrapped), the agent YAML and the deployment config. The web app runs CodeMirror 6; the desktop app has its own editor with the same features, because Fyne has none and cannot host a web view.
- Highlighting per language, including multi-line Lua long strings and comments, YAML block scalars and Markdown code fences. Indent guides, bracket matching, folding, a status line (line, column, selections) and soft wrap.
- Multiple carets and selections, column selection, and IDE line commands. Find and replace with match case, whole word and regex, plus go to line.
- Completion for the whole Lua API with inline docs, and ghost text as you type.
- Diagnostics as wavy underlines with hover errors, before you ever hit Validate.
- Backed by a Luau analyzer and language server that live inside Studio. Nothing is added to the runtime and no external process is started.

| Keys (both apps) | Action |
|---|---|
| `Ctrl+F` / `Ctrl+H` | Find / replace; `Enter`, `F3` and `Shift+F3` move between matches |
| `Ctrl+G` (desktop) / `Ctrl+Alt+G` (web) | Go to line |
| `Ctrl+D` / `Ctrl+Shift+L` | Add the next occurrence / select every occurrence |
| `Ctrl+Alt+Up` / `Ctrl+Alt+Down` | Add a caret above / below |
| `Alt+click` (desktop) · `Alt+drag` | Add a caret · select a column (`Alt+Shift+drag` on desktop) |
| `Ctrl+/` | Toggle comment |
| `Alt+Up` / `Alt+Down` · `Shift+Alt+Up` / `Shift+Alt+Down` | Move lines · copy lines |
| `Ctrl+Shift+K` · `Ctrl+J` · `Ctrl+L` | Delete lines · join lines · select the line |
| `Ctrl+]` / `Ctrl+[` | Indent / outdent |
| `Ctrl+U` / `Ctrl+Shift+U` | Lower / upper case |
| `Ctrl+Shift+[` / `Ctrl+Shift+]` | Fold / unfold |
| `Ctrl+Shift+\` | Jump to the matching bracket |
| `Alt+Z` | Toggle word wrap |
| `Ctrl+=` / `Ctrl+-` / `Ctrl+0`, `Ctrl+wheel` (desktop) | Zoom the editor |

The desktop editor also has a right-click menu with fold all, unfold all and the word-wrap switch.

**🔌 Connect**
- **Channels** - webhook, Telegram, QQ Bot, GitHub webhook, bridge and OpenAI-compatible channels: add, edit, validate against the deployment and save. The screen marks which definitions are live and which need a restart.
- **Models** - add, edit and delete LLM endpoints in place, test one with a real one-token call, and save the live set back to the config.
- **Credentials** - the encrypted credential store by bucket, engine-wide or per user. Create, edit and revoke.

Secrets are write-only everywhere: a field left blank keeps the stored value, and the screens show a reference or a last-four fingerprint, never the secret.

**🛡️ Govern**
- **Safety** - the named filter chains and the classifier policies behind them, with dry runs that use boot's own validation. Something in use cannot be deleted.
- **Users** - profiles, today's quota against the limit, linked channel handles, recent metered calls and the audit trail. Set a daily token limit or a per-user model, issue invites, export everything held for a person, or purge a profile.
- **Tenancy** - groups and their members, and shared memory pools with per-agent grants.

**📊 Operate**
- **Overview** - instance, region, version, uptime, live sessions, and sparklines of the runtime's counters. Refreshes every five seconds.
- **Usage** - the token ledger for any day, grouped by user, agent or model.
- **Logs** - the live log tail with a level floor, a text filter and pause.
- **Config** - the raw config file for everything without its own screen, with validate, an atomic save that keeps a `.bak`, and a masked view of what the runtime parsed.

**🪟 The shell**
- **Command bar** - the breadcrumb, a search field that opens the go-to palette, the connection pill (saved profiles, switch, lock) and the theme toggle.
- **Rail** - flat and compact, so every screen fits the default window without scrolling. The logo collapses it.
- **Status bar** - which runtime this is, on every screen: address, instance, region, topology, version, uptime and busy or idle sessions.

**📚 Documentation**
- The complete AgentFlow guide set - getting started, architecture, the configuration reference, the Luau API reference, deployment and the OpenAI-compatible API - is embedded in the binary and rendered natively. Search and links work offline.

## Getting Started

You need a running AgentFlow instance (see [its README](https://github.com/hulipa487/agentflow#getting-started)) with `runtime.studio.enabled: true`, which is the default.

Build from source. You need Go 1.27+ and a C/C++ toolchain, because Fyne uses cgo: gcc on Linux, Xcode's clang on macOS, [zig](https://ziglang.org) on Windows.

```bash
make desktop
./desktop/agentflow-studio
```

`make -C desktop run` does both. On Windows the Makefile also sets the GUI subsystem, so no console window opens beside the app; a plain `go build` there would open one.

For the browser instead, you need Node.js:

```bash
cd web
npm install
npm run dev          # http://127.0.0.1:5173
```

## Connecting

On first launch, enter the instance URL and an admin token: `ADMIN_TOKEN`, a named operator token, or the per-boot token the runtime prints at startup.

```
http://127.0.0.1:8080
unix:///run/agentflow/admin.sock
npipe://agentflow-admin
```

The first is the runtime's one HTTP port. The other two are for a runtime on the same machine, which can serve its admin API on a local socket and turn the TCP side off:

```yaml
runtime:
  admin:
    tcp: false
    socket: /run/agentflow/admin.sock   # Windows: '\\.\pipe\agentflow-admin'
```

A few things worth knowing:

- **The per-boot token is local only.** A runtime with no `ADMIN_TOKEN` and no operators mints a token at each start and answers the admin API only from the same machine. To connect from another machine, set `ADMIN_TOKEN` or `runtime.admin.operators` on the runtime.
- **A bad connection is never saved.** Studio checks the URL and the token before it stores anything, and a failure points at the field to fix.
- **The token is stored in plain text** in your user config directory when *Remember the token on this computer* is checked (the default). Uncheck it to be asked at each launch. Treat it like a shell secret: it can install agent logic and run their tools.
- **Several deployments.** Saved connection profiles and a region switcher let you move between cells without retyping anything.

The config file is `agentflow-studio/config.json` under the OS config directory (`%APPDATA%`, `~/.config`, `~/Library/Application Support`). Set `AGENTFLOW_STUDIO_CONFIG` to use another path.

## Keyboard shortcuts

| Shortcut | Action |
|---|---|
| `Ctrl+K` (or `Ctrl+Shift+P`) | Go to any screen, agent or action |
| `Ctrl+N` | New agent |
| `Ctrl+1` … `Ctrl+9` | Overview, Agents, Playground, Channels, Models, Credentials, Safety, Users, Logs |
| `F1` | Documentation; in the builder, the Luau reference dock |
| `F5` | Refresh the current screen |
| `Ctrl+S` | Apply in the builder (its remembered choice); validate and save in Config |
| `Ctrl+Space` | API completion in the loop editor |
| `Ctrl+Enter` | Send in the playground; in the builder, apply pending edits and run |
| `Esc` | Back |

## Good to know

- **Apply is live, saving is for the next boot.** An applied agent runs at once. What *Apply & save* writes is what the runtime loads when it next starts.
- **Some screens change the next start, not the running engine.** Channels and safety profiles bind at boot; the screens say so and mark what is live.
- **Playground turns are real turns.** They go through the runtime's `studio` channel, so safety filters, the audit journal and normal delivery all apply.
- **In a multi-instance cell the playground is per instance.** A thread whose session another instance owns will not show its replies here.

## For developers

| Path | What's there |
|---|---|
| `desktop/` | The desktop app, a Go module of its own |
| `desktop/main.go` | App bootstrap, single-instance guard |
| `desktop/internal/ui` | The shell and every screen |
| `desktop/internal/api` | The admin API client and stream parsers, with no UI dependency |
| `desktop/internal/lua` | The Luau analyzer and the in-process language server |
| `desktop/internal/docs` | The documentation library: manifest, search, links |
| `desktop/internal/appconfig` | Saved connections and preferences |
| `web/` | The web app: `src/screens`, `src/api`, `src/lua`, and the local proxy in `server/` |
| `docs/` | The Markdown guides, the users OpenAPI spec and the Luau type stubs. One copy, shipped by both apps |

```bash
make test        # both apps: go test in desktop/, vitest in web/
make docs-sync   # refresh the OpenAPI spec and Luau stubs from the runtime repo
```

**The two apps are kept in step by hand.** They share no code, only facts: the admin API types (`desktop/internal/api` and `web/src/api`), the Lua API table behind editor completion (`desktop/internal/lua/api.go` and `web/src/lua/api.ts`), and the screens themselves. A change to one belongs in the same commit as the change to the other.

**The docs are shared for real.** The desktop app embeds `docs/` (it is a small Go module so the embed can reach it) and the web app bundles the same files at build time. A new page needs a manifest entry in both `desktop/internal/docs/docs.go` and `web/src/docs/library.ts`, and each app has a test that fails if its entry is missing. `docs/openapi/users.yaml` and `docs/luau/af.d.luau` are copies of files versioned with the runtime; `make docs-sync` expects that checkout next to this one, or at `AGENTFLOW_DIR`.

To see every desktop screen without clicking through them, render them to PNGs in both themes:

```bash
cd desktop
STUDIO_SNAPSHOT_DIR=./snapshots go test ./internal/ui -run TestSnapshots
```

Packaging the desktop app is optional; the binary runs as it is. For an installable app, from `desktop/`:

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -name "AgentFlow Studio"
```

## Desktop or web?
- **Desktop** when you want nothing to host: the token goes straight to the runtime with no CORS entry, a Unix socket or named pipe keeps the admin API off the network, and the editor, its language server and the docs are all in one binary.
- **Web** when installing an app is the obstacle: it runs from `npm start` on your own machine, and its local proxy reaches sockets and pipes too.
- Both work offline once built, in light and dark, on Windows, MacOS and Linux.

**LICENSE**: MIT

Made with Vibe and ❤️ by hulipa and her team.
