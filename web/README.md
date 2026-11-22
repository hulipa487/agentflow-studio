# AgentFlow Studio (web)

AgentFlow Studio in a browser tab: build an agent, apply it live, chat with it
and watch every step of the turn. It talks to the runtime's `/admin/api/*`
surface. The [desktop app](../desktop) is this app in the system webview, so
a change here reaches both. The runtime stays
the source of truth for config, persistence, memory and traces; this app keeps
nothing on a server.

Built with React 19, TypeScript, Vite, Tailwind CSS 4 and CodeMirror 6.

## Run it

```bash
npm install
npm run dev          # http://127.0.0.1:5173, with hot reload
```

or as a built app:

```bash
npm run build
npm start            # http://127.0.0.1:4173 (PORT=… to change)
```

Then enter the runtime's address (`gateway.listen`, default
`http://127.0.0.1:8080`) and the admin token: `ADMIN_TOKEN`, a named operator
token, or the per-boot token AgentFlow prints at startup.

Both commands serve the app **and a small local proxy** from one loopback
port. That proxy is what makes the browser version work without touching the
runtime's config — see the next section.

## How it reaches the runtime

A browser tab is a different origin from the runtime, and it cannot open a
Unix socket. There are two ways through:

| | Local proxy (default) | Direct |
|---|---|---|
| Path | browser → this origin `/__af/…` → runtime | browser → runtime |
| Runtime config | none | `runtime.admin.cors_origins: ["http://127.0.0.1:5173"]` (the exact origin in your address bar) |
| Unix socket / named pipe | yes | no |
| Needs `npm run dev` / `npm start` | yes | no — any static host works |

The welcome screen picks the proxy when it is available and says which route a
connection will take; **Route through Studio's local proxy** switches it.
Addresses can be a TCP origin, a Unix socket or a Windows named pipe:

```
http://127.0.0.1:8080
unix:///run/agentflow/admin.sock
npipe://agentflow-admin
```

The proxy (`server/proxy.mjs`) is deliberately narrow:

- it forwards only paths under `/admin/`, to the target named per request in
  the `X-AF-Target` header;
- that header is required, and the proxy never answers a CORS preflight, so a
  page on another origin cannot drive it;
- requests with a non-loopback `Host` are refused (DNS rebinding), as are
  cross-site fetches;
- it stores nothing and logs nothing: the bearer token passes through.

It binds `127.0.0.1`. To serve other machines, put your own TLS and
authentication in front and set `HOST` and `ALLOW_HOSTS`:

```bash
HOST=0.0.0.0 ALLOW_HOSTS=studio.internal npm start
```

The admin token can install agent logic and invoke tools. Treat a Studio you
expose like the admin API itself.

### Where the token is kept

- **Remember the token in this browser** on: `localStorage`, in plain text —
  in the desktop app, the webview's. **Lock** forgets it.
- Off: `sessionStorage` only. A reload keeps you signed in; closing the tab
  forgets the token.

A connection is saved as a profile only after it has worked, so a wrong
address or token never becomes the next visit's default. Profiles and the
switcher live in the connection pill in the command bar.

## Screens

- **Overview** — instance, region, version, uptime and config epoch; live
  sessions; agents and channels; credential store; sparklines of the sampled
  counters; the instance registry. Refreshes every five seconds.
- **Agents** — one dense list: model, loop, capabilities, persisted /
  runtime-only, live sessions. Create from a template, edit, chat, delete.
- **Builder** — form (model, safety, capabilities, skills, history budget,
  daemon, `can_contact`, credentials, extras), a Lua 5.1 loop editor with
  diagnostics, completion and hover docs, an instructions editor and raw YAML.
  **Validate** runs the runtime's boot-equivalent check, **Apply** installs it
  live, **Apply & save** persists it. The inline Lua reference sits beside it.
- **Playground** — threaded chat, one session per thread, optional
  act-as-user, streamed replies and the live trace of turns, ops, model and
  tool calls, tokens, durations and errors.
- **Channels**, **Models**, **Credentials**, **Safety** — the same editors,
  with the same write-only secrets and the same `live` / `restart required`
  states.
- **Users**, **Tenancy**, **Usage** — profiles, quotas, handles, invites,
  export and purge; groups, pools and grants; the token ledger for a UTC day.
- **Logs** — the live log tail with a level floor, text filter, pause and copy.
- **Config** — the raw config with boot-equivalent validate and an
  mtime-guarded save.
- **Documentation** — the full guide set, bundled and searchable offline.
- **Settings** — language, light / dark / system, the Trans Pride and Obsidian
  palettes, connections.

### Loop editor

The Lua editor's diagnostics, completion and hover come from a TypeScript port
of the runtime's Lua 5.1 analyzer (`src/lua`): the
chunk contract (`-- af-prelude-version`, a global `loop()`), unknown prelude
names with "did you mean", capability checks against the agent's effective
capabilities, and AgentFlow-specific hazards. It knows the documented prelude;
the runtime's **Validate** is still the authoritative compile.

### Unsaved edits

A builder or config editor with unsaved changes marks its title with `•`. The
browser warns before the tab closes or reloads, and any way out inside the app
(the rail, the palette, a shortcut, a link, the browser's Back button) waits
for a "Discard changes?" answer.

## Keyboard

| Shortcut | Action |
|---|---|
| `Ctrl/⌘+K` (or `Ctrl/⌘+Shift+P`) | Go to: screens, agents and actions |
| `Alt+1` … `Alt+9` | Overview, Agents, Playground, Channels, Models, Credentials, Safety, Users, Logs |
| `Alt+N` | New agent |
| `Alt+R` | Refresh the current screen |
| `Ctrl/⌘+S` | Apply in the builder (its remembered choice); validate and save in Config |
| `Ctrl/⌘+Enter` | Send in the playground; in the builder, apply pending edits and run |
| `Ctrl+Space` | Completion in the loop editor |
| `F1` | Documentation; in the builder, the Lua reference dock |
| `Esc` | Back |

`Ctrl+N`, `Ctrl+1…9` and `F5` are the browser's, so navigation uses `Alt`; the
desktop app, being this page, uses the same keys.

## Theme

The palettes (`src/index.css`): the
trans flag's blue, pink and white as one continuous gradient hairline under
the command bar, a flag-gradient logo, a blue-to-pink marker on the active
rail row and tone-matched chips. Light and dark variants of both palettes; the
first visit follows the OS.

## Language

English, Simplified Chinese and Traditional Chinese, switched with the
**EN / 简体 / 繁體** control in the command bar (also on the welcome screen and
in Settings). The choice applies without a reload and is saved in
`localStorage`; a first visit follows the browser's languages and falls back
to English. The messages are the shared catalogues in `../i18n`, read through
`t()` from `src/i18n`; `../i18n/README.md` describes the syntax and the
glossary. Text the runtime returns is shown as it is. The guides follow the
same switch: `../docs/zh-Hans/` and `../docs/zh-Hant/` hold their translations,
and an untranslated guide is shown in English.

## Layout

```
server/proxy.mjs        the local proxy (shared by Vite and the static server)
server/serve.mjs        serves dist/ + the proxy: `npm start`
src/api                 typed admin API client, SSE reader, wire types
src/state               connections (saved profiles), session, appearance, UI
src/components/ui       the component kit, CodeMirror editor, Markdown
src/components/shell    command bar, rail, status bar, command palette, shortcuts
src/lua                 the Lua 5.1 analyzer and its CodeMirror extension
src/docs                the library over ../docs, the bundled guides
src/i18n                t(), the current language, and the catalogues from ../i18n
src/screens             one file per screen (+ a folder for its parts)
src/nav.ts              destinations: rail, breadcrumb, shortcuts and palette
```

Adding a screen is one entry in `src/nav.ts`, a route in `src/App.tsx` and a
component built on `Page` from the kit.

## Development

```bash
npm run typecheck    # tsc -b
npm test             # vitest: API client, proxy, Lua analyzer, screen logic
```

The documentation is not copied into this app: `src/docs/library.ts` bundles
the repository's [`docs/`](../docs) directory as it is. A new page there needs
an entry in the manifest in that file, and a
test fails if one is missing.

Two things worth knowing when changing the client (`src/api/client.ts`):

- Channels and safety stamp their files in **nanoseconds**, which a JavaScript
  number cannot hold exactly, and the runtime compares the stamp for equality.
  The client keeps those `mtime`s as digit strings end to end (`Mtime`); there
  is a regression test.
- Streams (the playground turn, the log tail) are read from `fetch`, not
  `EventSource`, because the bearer token travels in a header.
