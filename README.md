<div align="center">
<h1>
🎛️ AgentFlow Studio
<p>
</h1>
<i> Build it, apply it live, watch every step. <p></p></i>
</div>


## What is AgentFlow Studio

> TL;DR - The desktop app for [AgentFlow](https://github.com/hulipa487/agentflow): build agents, run them, operate the deployment

AgentFlow Studio is a native desktop client for an AgentFlow runtime. Connect to any deployment and you get the whole thing in one window: write a loop, apply it to the running engine, chat with it, and read the trace of what it did - then manage the models, channels, users and credentials it runs on.

It is a real desktop app (Go + [Fyne](https://fyne.io/)), not a browser tab. No web server to host, no CORS to configure, and on the same machine it can skip TCP entirely and talk over a Unix socket or a Windows named pipe.

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
- **Builder** - a form for the agent's model, safety profile, capabilities, skills and credentials, next to a Luau loop editor, an instructions editor and the raw YAML. **Validate** runs the same checks boot does. **Apply** installs the agent live; **Apply & save** also writes it into the deployment's config.
- **Playground** - threaded chat against one agent, one session per thread, with streamed replies and a live trace: turns, ops, model and tool calls, tokens, durations, safety decisions and errors. An optional *act as user* runs the turn as a real profile, so memory scopes and quotas apply.

**✍️ The loop editor**
- Syntax highlighting, line numbers, auto-indent, bracket pairing, undo/redo and the keybindings you expect from an IDE.
- Completion for the whole Lua API with inline docs, and ghost text as you type.
- Diagnostics as wavy underlines with hover errors, before you ever hit Validate.
- Backed by a Luau analyzer and language server that live inside Studio. Nothing is added to the runtime and no external process is started.

**🔌 Connect**
- **Channels** - webhook, Telegram, GitHub webhook, bridge and OpenAI-compatible channels: add, edit, validate against the deployment and save. The screen marks which definitions are live and which need a restart.
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

**📚 Documentation**
- The complete AgentFlow guide set - getting started, architecture, the configuration reference, the Luau API reference, deployment and the OpenAI-compatible API - is embedded in the binary and rendered natively. Search and links work offline.

## Getting Started

You need a running AgentFlow instance (see [its README](https://github.com/hulipa487/agentflow#getting-started)) with `runtime.studio.enabled: true`, which is the default.

Build from source. You need Go 1.27+ and a C/C++ toolchain, because Fyne uses cgo: gcc on Linux, Xcode's clang on macOS, [zig](https://ziglang.org) on Windows.

```bash
make build
./agentflow-studio
```

`make run` does both. On Windows the Makefile also sets the GUI subsystem, so no console window opens beside the app; a plain `go build` there would open one.

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
| `F1` | Documentation |
| `F5` | Refresh the current screen |
| `Ctrl+S` | Apply in the builder; validate and save in Config |
| `Ctrl+Space` | API completion in the loop editor |
| `Ctrl+Enter` | Send in the playground |
| `Esc` | Back |

## Good to know

- **Apply is live, saving is for the next boot.** An applied agent runs at once. What *Apply & save* writes is what the runtime loads when it next starts.
- **Some screens change the next start, not the running engine.** Channels and safety profiles bind at boot; the screens say so and mark what is live.
- **Playground turns are real turns.** They go through the runtime's `studio` channel, so safety filters, the audit journal and normal delivery all apply.
- **In a multi-instance cell the playground is per instance.** A thread whose session another instance owns will not show its replies here.

## For developers

| Path | What's there |
|---|---|
| `main.go` | App bootstrap, single-instance guard |
| `internal/ui` | The shell and every screen |
| `internal/api` | The admin API client and stream parsers, with no UI dependency |
| `internal/lua` | The Luau analyzer and the in-process language server |
| `internal/docs` | The embedded documentation library: manifest, search, links |
| `internal/appconfig` | Saved connections and preferences |
| `docs/` | The Markdown guides, the users OpenAPI spec and the Luau type stubs |

```bash
make test        # go test ./..., including a headless build of every screen
make docs-sync   # refresh the OpenAPI spec and Luau stubs from the runtime repo
```

`docs/openapi/users.yaml` and `docs/luau/af.d.luau` are copies: the originals are versioned with the runtime. `make docs-sync` expects that checkout next to this one, or at `AGENTFLOW_DIR`. A new page under `docs/` also needs an entry in `internal/docs/docs.go`, and a test fails if it is missing.

To see every screen without clicking through them, render them to PNGs in both themes:

```bash
STUDIO_SNAPSHOT_DIR=./snapshots go test ./internal/ui -run TestSnapshots
```

Packaging is optional; the binary runs as it is. For an installable app:

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -name "AgentFlow Studio"
```

## Why a desktop app?
- **No Hosting**: Nothing to deploy next to the runtime, and nothing to expose.
- **No Browser Origin**: The token goes straight to the runtime; no CORS entry needed.
- **Local Transports**: A Unix socket or named pipe keeps the admin API off the network.
- **Works Offline**: The editor, its language server and the docs are all in the binary.
- **Cross Platform**: Windows, MacOS, Linux, same app.

**LICENSE**: MIT

Made with Vibe and ❤️ by hulipa and her team.
