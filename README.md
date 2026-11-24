<div align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/brand/logo-banner-darkmode.svg">
  <img src="assets/brand/logo-banner-lightmode.svg" alt="AgentFlow" height="96">
</picture>
<h1>
AgentFlow Studio
<p>
</h1>
<i> Build it, apply it live, watch every step. <p></p></i>
</div>


## What is AgentFlow Studio

> TL;DR - The workbench for [AgentFlow](https://github.com/hulipa487/agentflow), a programmable runtime for custom LLM bots and workflows

AgentFlow Studio is the visual control plane and IDE for an AgentFlow runtime. Start by picking a supported starter — **QQ Group Chatbot (FTS5)**, **QQ Group Chatbot (Embedding)**, **Telegram Group Chatbot (FTS5)**, or **Telegram Group Chatbot (Embedding)** (configurable with zero TypeScript, inline FTS5/vector memory, and live setup with credential checks) — verify what every group message did in **Event Center**, and edit the underlying TypeScript loop with live reload whenever you need custom routing, memory rules, or workflows.

It is one app, two ways to open it:

| | Where it runs | Built with |
|---|---|---|
| [**Desktop**](desktop) | A window on Windows, macOS and Linux | The web app in the system webview, via Go + [Wails](https://wails.io/) |
| [**Web**](web) | A browser tab, served from your own machine | React + TypeScript |

The screens are the same because they are the same code: the desktop app embeds the web app and adds what a browser tab cannot do (a local proxy to Unix sockets and Windows named pipes, native save dialogs, its own title bar on Windows). Neither needs a web server to host or CORS to configure; [the web README](web/README.md) and [the desktop README](desktop/README.md) cover how each reaches the runtime.

- [x] A TypeScript/JS editor with completion, diagnostics and hover docs?
- [x] Apply a loop to a live engine without restarting it?
- [x] A trace of every op, model call and safety decision in a turn?
- [x] The full documentation, offline, inside the app?
- [x] Light and dark, in trans-pride colours?
- [ ] Writes your loops for you? (you have an agent for that now)

The runtime stays the source of truth. Studio holds no agent state of its own: config, persistence, memory and traces all live in the deployment it is connected to.

## What's in the box

**🛠️ Build**
- **Agents** - every configured agent with its model, capabilities and live sessions. Create from a starter template (**QQ Group Chatbot (FTS5)**, **QQ Group Chatbot (Embedding)**, **Telegram Group Chatbot (FTS5)**, **Telegram Group Chatbot (Embedding)**) or from blank, edit, delete.
- **Builder** - a workbench: schema-driven **Starter settings** (no JSON/TS needed for common rules), a form for the agent's model, safety and memory profiles, capabilities, skills and credentials, a TypeScript loop editor, an instructions editor and the raw YAML, with a dock beside them for a **Run** panel (chat and trace with this agent) or the Script reference (`F1`). **Validate** runs the same checks boot does. **Apply** is a split button: *Apply* installs the agent live, *Apply & save* also writes it into the deployment's config, and the button remembers your pick. `Ctrl+Enter` applies pending edits and sends the Run panel's message (or repeats the last one), so you can edit, run and read the trace without leaving.
- **Playground** - threaded chat against one agent, one session per thread, with streamed replies and a live trace: turns, ops, model and tool calls, tokens, durations, safety decisions and errors. An optional *act as user* runs the turn as a real profile, so memory scopes and quotas apply.

**✍️ The code editor**
- One editor for every document: the loop (TypeScript / JavaScript), the instructions (Markdown, soft-wrapped), the agent YAML and the deployment config, on CodeMirror 6.
- Highlighting per language, including TypeScript/JS expressions, regex literals and comments, YAML block scalars and Markdown code fences. Indent guides, bracket matching, folding, a status line (line, column, selections) and soft wrap.
- Multiple carets and selections, column selection, and IDE line commands. Find and replace with match case, whole word and regex, plus go to line.
- Completion for the whole Prelude v2 API with inline docs, and ghost text as you type.
- Diagnostics as wavy underlines with hover errors, before you ever hit Validate.
- Backed by a TypeScript/JS analyzer and language server that live inside Studio. Nothing is added to the runtime and no external process is started.

| Keys | Action |
|---|---|
| `Ctrl+F` / `Ctrl+H` | Find / replace; `Enter`, `F3` and `Shift+F3` move between matches |
| `Ctrl+Alt+G` | Go to line |
| `Ctrl+D` / `Ctrl+Shift+L` | Add the next occurrence / select every occurrence |
| `Ctrl+Alt+Up` / `Ctrl+Alt+Down` | Add a caret above / below |
| `Ctrl+click` · `Alt+drag` | Add a caret · select a column |
| `Ctrl+/` | Toggle comment |
| `Alt+Up` / `Alt+Down` · `Shift+Alt+Up` / `Shift+Alt+Down` | Move lines · copy lines |
| `Ctrl+Shift+K` · `Ctrl+J` · `Ctrl+L` | Delete lines · join lines · select the line |
| `Ctrl+]` / `Ctrl+[` | Indent / outdent |
| `Ctrl+U` / `Ctrl+Shift+U` | Lower / upper case |
| `Ctrl+Shift+[` / `Ctrl+Shift+]` | Fold / unfold |
| `Ctrl+Shift+\` | Jump to the matching bracket |
| `Alt+Z` | Toggle word wrap |

**🔌 Connect**
- **Channels** - webhook, Telegram, QQ Bot, GitHub webhook, bridge and OpenAI-compatible channels: add, edit, validate against the deployment and save. The screen marks which definitions are live and which need a restart.
- **Models** - add, edit and delete LLM endpoints in place, test one with a real one-token call, and save the live set back to the config.
- **Memory** - the databases memory lives in and the profiles agents pick: stores, retention, recall, embedding and reranking, validated the way boot does. Something in use cannot be deleted.
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
- **Command bar** - the breadcrumb, a search field that opens the go-to palette, the connection pill (saved profiles, switch, lock), the language switch and the theme toggle. In the desktop app on Windows it is also the window's title bar, with the minimise, maximise and close buttons.
- **Language** - English, 简体中文 and 繁體中文, for the interface and the bundled guides alike. The **EN / 简体 / 繁體** switch is in the command bar, on the sign-in screen and in Settings; it applies at once and is remembered. The first launch follows the language of the system (desktop) or the browser (web).
- **Rail** - flat and compact, so every screen fits the default window without scrolling. The logo collapses it.
- **Status bar** - which runtime this is, on every screen: address, instance, region, topology, version, uptime and busy or idle sessions.

**📚 Documentation**
- The complete AgentFlow guide set - getting started, architecture, the configuration reference, the Script API reference, deployment and the OpenAI-compatible API - in English, 简体中文 and 繁體中文, is built into the app. Search and links work offline.

## Getting Started

You need a running AgentFlow instance (see [its README](https://github.com/hulipa487/agentflow#getting-started)) with `runtime.studio.enabled: true`, which is the default.

Build from source. The desktop app needs Go 1.27+ and Node.js (it embeds the web build); on Linux also a C toolchain and the WebKitGTK 4.1 development files (`libwebkit2gtk-4.1-dev`), on macOS Xcode's clang. Windows needs neither, and `make -C desktop windows` cross-compiles the Windows .exe from any OS. [desktop/README.md](desktop/README.md) has the details.

```bash
make desktop
./desktop/agentflow-studio
```

For the browser instead, Node.js is enough:

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
- **The token is stored in plain text** in the app's (or the browser's) local storage when *Remember the token* is checked (the default). Uncheck it to be asked at each launch. Treat it like a shell secret: it can install agent logic and run their tools.
- **Several deployments.** Saved connection profiles and a region switcher let you move between cells without retyping anything.

The desktop app keeps that storage in the webview's profile: `$XDG_DATA_HOME/agentflow-studio` on Linux, the app data folder on Windows. On its first launch it imports the connections, language and theme saved by the earlier Fyne desktop app (`agentflow-studio/config.json` under the OS config directory, or `AGENTFLOW_STUDIO_CONFIG`).

## Keyboard shortcuts

| Shortcut | Action |
|---|---|
| `Ctrl+K` (or `Ctrl+Shift+P`) | Go to any screen, agent or action |
| `Alt+N` | New agent |
| `Alt+1` … `Alt+9` | Overview, Agents, Playground, Channels, Models, Credentials, Safety, Users, Logs |
| `F1` | Documentation; in the builder, the Script reference dock |
| `Alt+R` | Refresh the current screen |
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
| `web/` | The app: `src/screens`, `src/api`, `src/script` (the TypeScript/JS analyzer and language server), `src/docs`, and the local proxy in `server/` |
| `desktop/` | The desktop app: the web build in the system webview, a Go module of its own (proxy, save dialog, window, title bar) |
| `docs/` | The Markdown guides, their translations, the users OpenAPI spec and the TypeScript API definitions |
| `examples/` | Runnable starters ([QQ Group Chatbot (FTS5)](examples/qq-group-bot-fts), [QQ Group Chatbot (Embedding)](examples/qq-group-bot-vec), [Telegram Group Chatbot (FTS5)](examples/telegram-group-bot-fts), [Telegram Group Chatbot (Embedding)](examples/telegram-group-bot-vec)) |
| `i18n/` | The message catalogues in English, 简体中文 and 繁體中文 |

```bash
make test        # vitest in web/, go test in desktop/
make docs-sync   # refresh the OpenAPI spec and TypeScript API definitions from the runtime repo
```

**New screens and features go in `web/`.** The desktop app shows the web app, so a change there reaches both. The desktop side changes only for what a browser cannot do; `web/src/lib/desktop.ts` is the page's side of it and falls back to the browser's behaviour everywhere else.

**The message catalogues.** Every string the app shows lives in `i18n/<language>/<namespace>.json` (`en`, `zh-Hans`, `zh-Hant`). English is the source and the fallback. `i18n/README.md` has the message syntax, the style rules and the glossary; a test fails when the three languages do not hold the same keys and parameters, or when code asks for a key that does not exist.

**The guides are translated too.** `docs/zh-Hans/` and `docs/zh-Hant/` hold a translation of every guide under the same file name; the app shows a guide in the current language and falls back to English where a translation is missing. A translation keeps the English headings one for one, its code blocks (only prose comments are translated), its link targets and its table shapes, and each translated heading takes the English heading's anchor, so `configuration.md#memory` resolves in every language. `web/src/docs/translations.test.ts` fails when a translation is missing or breaks that structure. Change an English guide, then its two translations, in the same commit.

**The docs are bundled at build time.** A new page needs a manifest entry in `web/src/docs/library.ts`, and a test fails if it is missing. `docs/openapi/users.yaml` and `docs/ts/af.d.ts` are copies of files versioned with the runtime; `make docs-sync` expects that checkout next to this one, or at `AGENTFLOW_DIR`.

**The Fyne desktop app** that came before the Wails one is at the git tag `fyne-desktop-final`.

## Desktop or web?
- **Desktop** when you want an app in its own window: one binary with the editor, its language server and the docs, native save dialogs, and a built-in proxy to Unix sockets and named pipes that keeps the admin API off the network.
- **Web** when installing an app is the obstacle: it runs from `npm start` on your own machine, and its local proxy reaches sockets and pipes too.
- Both work offline once built, in light and dark, on Windows, MacOS and Linux.

**LICENSE**: AGPLv3

Made with Vibe and ❤️ by hulipa and her team.
