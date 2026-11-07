# Coding Luau loops

A practical guide to writing AgentFlow session loops in Luau: the execution
model, the whole Lua API, worked patterns, and the mistakes that are easy to
make.

Runnable versions of the patterns live in [`examples/loops/`](https://github.com/hulipa487/agentflow/tree/main/examples/loops).

The bundled Luau type definitions are at [luau/af.d.luau](luau/af.d.luau); open them from this page to inspect the API surface in an editor.

## Contents

1. [The smallest loop](#1-the-smallest-loop)
2. [Execution model](#2-execution-model)
3. [Messages](#3-messages)
4. [State and persistence](#4-state-and-persistence)
5. [Capabilities](#5-capabilities)
6. [API reference](#6-api-reference)
7. [Patterns](#7-patterns)
8. [Resilience](#8-resilience)
9. [Prompt assembly and token budgets](#9-prompt-assembly-and-token-budgets)
10. [Testing and debugging](#10-testing-and-debugging)
11. [Gotchas](#11-gotchas)
12. [Quick reference table](#12-quick-reference-table)
13. [Support chunks, shadows and route handlers](#13-support-chunks-shadows-and-route-handlers)

---

## 1. The smallest loop

A session loop is a Luau chunk that defines a global function `loop()`. The
runtime loads one fresh Lua state per session and calls `loop()`; the function
runs for the lifetime of that session.

```lua
-- af-prelude-version: 1
function loop()
  while true do
    local msg = session.inbox()      -- parks until the next message
    session.send("echo: " .. (msg.text or ""))
  end
end
```

Point an agent at the file:

```yaml
agents:
  echo:
    loop: ./examples/loops/echo.lua
```

Run the runtime and send it a message (the example config has a webhook
channel):

```bash
./agentflow -config agentflow.yaml
curl -X POST localhost:8080/webhook/local/ \
     -H 'Content-Type: application/json' \
     -d '{"from":"alice","text":"hello"}'
# echo: hello
```

### The one required line

Every chunk — loop, support chunk, or route handler — must declare the prelude
API version in its leading comment block:

```lua
-- af-prelude-version: 1
```

A chunk with no directive, or with another version, is refused **before it
runs**. There is no compatibility window: the prelude and the Lua that uses it
move together. The version is `vm.PreludeVersion` in the source and is shown by
`GET /admin/api/studio/catalog`; the current value is **1**.

### Validate before you run

The Studio app (`/admin/api/studio/*`) validates a loop by checking the
directive and compiling the chunk, then applies it live. `POST
/admin/api/studio/agents/validate` is the same check without side effects. A
plain compile check is also available from Go (`vm.CompileCheck`).

A compile check proves syntax, not behavior: a loop that references a missing
global, calls a disabled capability, or does the wrong thing at runtime fails
on the first turn and shows up in the Studio trace.

---

## 2. Execution model

Each session is an **actor**:

- one goroutine, one mailbox, one Luau state, one loop coroutine;
- messages are processed one at a time, in order;
- the loop never runs concurrently with itself.

The only native primitive exposed to Lua is `__af_op(json_request)`. It yields
the coroutine; Go executes the request (inline for cheap ops, on a worker pool
for blocking ones), then resumes with `(json_response, ok)`. The prelude wraps
this as `op(req)` and raises the response as a Lua error when `ok` is false.
Because every blocking call yields, a parked session costs no OS thread.

```lua
-- conceptually, what the prelude does for you:
local resp, ok = __af_op(json.encode(req))
if not ok then error(resp, 0) end
return json.decode(resp)
```

### Park points and restarts

`session.inbox()` is the park point. While the coroutine is parked the session
is idle and cheap. There is no need for `time.sleep` polling loops — never write
`while true do ... time.sleep(0.1) ... end` to look for work; the mailbox is the
work queue.

What happens around `loop()`:

| Event | Result |
|---|---|
| `session.inbox()` | parks until a message arrives |
| `session.send` / `push` / `llm.chat` / `tools.run` / `store.*` / `shell.*` / `http.*` / `mail.*` / `agent.*` | yields; resumes with the result |
| `log.*`, `agent.info`, `agent.config`, `json.*` | inline; no yield |
| `loop()` returns | the loop is **restarted** immediately (a finished loop is not an exited session) |
| a Lua error escapes `loop()` | the loop crashes and is restarted after ~1s, forever |
| `session.exit()` | the session ends; the coroutine is never resumed; timers/requests are reaped |

Two consequences worth internalizing:

- **To end a session, call `session.exit()`.** Returning from `loop()` restarts it.
- **Wrap fallible work in `pcall`.** An uncaught provider error is a crash loop,
  not a single failed turn.

### Hot reload

An edit to the loop file restarts the session's coroutine from a fresh Lua state
at its next park point. Instructions and file-backed prompts reload in place;
a route handler rebuilds between messages; a shadowed support chunk restarts
every live session. A new version that does not compile leaves the running
version in place. Each session is one conversation: a single edit can restart
every instance's copy of that session, and the restart loses all Lua state that
was not in `session.state`, memory, the store, or files.

### One op at a time

Keep op calls on the loop coroutine. Do not call `session.inbox`, `llm.chat`,
`store.*` or any other yielding op from a nested `coroutine.create` coroutine:
the actor drives the loop coroutine, and the bridge expects its yields to come
from there. Plain Lua coroutines are fine for pure computation; anything that
crosses the bridge belongs in the main loop.

---

## 3. Messages

`session.inbox()` returns a table like this:

```lua
{
  id = "msg-...",                 -- stable message id (audit joins on it)
  type = "user",                  -- user | agent | timer | cron | boot | system
  from = "webhook:alice",         -- core-stamped sender
  text = "hello",                 -- channel text; may be empty
  channel = "webhook",            -- origin channel (absent on timer/cron/boot)
  reply_to = "...",               -- channel-specific recipient for a reply
  ts = 1730000000,                -- Unix seconds
  payload = { ... },              -- op-specific data (see below)
  attachments = { ... },          -- media part descriptors, when present
  provenance = {                  -- CORE-STAMPED; never loop-controlled
    kind = "channel",             -- channel | agent | timer | scheduler | system | studio
                                  -- timer = a session.scheduler.* fire, scoped to the
                                  -- tenant that scheduled it; scheduler = an engine
                                  -- trigger and the only maintenance timer provenance
    principal = "webhook:alice",
    parent = "...",               -- parent session id (agent messages)
    request_id = "...",           -- correlation id (agent.request / agent.reply)
    user_uuid = "u_...",          -- tenant this turn acts for, or nil
    membership_uuid = "...",      -- group context, when one was validated
  },
}
```

### Message types

| `type` | Where it comes from | `payload` |
|---|---|---|
| `user` | a channel or the Studio playground | channel-specific (`chat_id`, `text`, ...) |
| `agent` | `agent.send` / `agent.request` / `agent.spawn` traffic | the sender's payload table |
| `timer` | `scheduler.every/after/cron` | `{ timer_id = "..." }` |
| `cron` | an engine `every:` / `cron:` trigger | the trigger's `payload:` copied verbatim |
| `boot` | the supervisor, for `persistent: true` agents | none |
| `system` | engine lifecycle messages (e.g. `agent.died`) | event-specific |

`timer`, `cron` and `boot` turns have **no channel**. `session.send` replies
only to an active inbound message and fails with `no active message to reply
to`; use `session.push(channel, reply_to, text)` (cap `channel.push`) or
`session.push_user(uuid, text)` when there is a real recipient, or record state
for the next user turn.

### Identity comes from provenance

Read identity from `msg.provenance`, never from `msg.payload`:

- `user_uuid` is the tenant the turn acts for. It is **absent** when nothing
  stamped the message, and **empty** when the message belongs to no tenant.
- `membership_uuid` is set when the router proposed a group and the engine
  validated the sender's membership.
- A delegation hop carries the sender's tenant forward; the engine stamps it,
  and `payload.user_uuid` is ignored no matter who wrote it.

A loop cannot widen a tenant by putting a UUID in a payload.

### Attachments

When a channel ingested media, the message carries small descriptors:

```lua
{
  type = "image", mime = "image/png",
  handle = "media:<sha256>", name = "photo.png",
}
```

Forward them unchanged into `llm.chat` (`parts = msg.attachments`) or
`session.send` (`{ attachments = msg.attachments }`). Raw bytes never enter Lua;
the runtime resolves handles at the provider/channel boundary.

### Sending

```lua
session.send(text)                              -- reply to the active message
session.send(text, { attachments = parts })     -- with media
session.push(channel, reply_to, text)           -- proactive egress; cap channel.push
session.push_user(uuid, text)                   -- by identity uuid; cap channel.push
```

`session.send` and `session.push` pass through the core-owned safety egress
chain and the audit journal; a loop cannot bypass either. `session.push_user`
requires the identity layer and a bare UUID (the prelude adds `user:`).

---

## 4. State and persistence

There are four places state can live. Pick the smallest one that survives the
thing you need to survive.

| Mechanism | Scope | Survives hot reload | Survives restart | Multi-instance | Use for |
|---|---|---|---|---|---|
| Lua locals | this coroutine | **no** | no | no | scratch within one turn |
| `session.state` | one session | yes | yes (runtime store) | yes, follows the session | cursors, counters, timer ids |
| memory profile (`memory.*`) | one agent (per user/key scope) | yes | yes | yes | conversational history, facts |
| `store.*` | a named table, with scopes | yes | yes | yes | raw key/value or vector data |
| `files.*` | one user/agent project tree | yes | yes | yes | documents and snapshots |

### `session.state`

A small durable key/value store for one session. It requires the
`session.state` capability (not in the default set).

```lua
session.state.set(key, value)   -- value is any JSON-encodable Lua value
session.state.get(key)          -- nil when never set
session.state.delete(key)
session.state.list()            -- { key = value }
```

Limits: keys up to 128 characters from letters, digits, `. _ - :`; values up to
64 KiB; 256 keys per session. It is the right place for a timer id, a cursor, or
a step count — not for conversation history.

### Memory

`memory.write` routes a record through the profile's write chain (by default
`plugin:routing_table`) into the declared stores. `memory.recall` dispatches to
the profile's recall handler (`plugin:recency` by default, or
`plugin:semantic`).

```lua
-- A turn record: routed to the "dialogue" store by the default chain.
memory.write({
  type = "turn",
  id = msg.id,
  text = msg.text,
  reply = reply.text,
})

-- A fact: routed to the "facts" store.
memory.write({ type = "fact", key = "name:" .. msg.from, value = { text = "..." } })

-- Explicit targets bypass the routing table.
memory.write({ targets = { { store = "dialogue", key = "custom:" .. msg.id, value = "..." } } })
```

`memory.recall` returns a **list** of records, each `{ key, value, meta }` — not
a `{ records }` wrapper. The query shape depends on the handler:

| Handler | Query |
|---|---|
| `plugin:recency` (default) | `{ store, kind = "all" \| "prefix" \| "text" \| "time_range", prefix? , text?, k?, from?, to? }` |
| `plugin:semantic` | `{ text, store?, k? }` (needs an `embed_model` on the profile) |

Time-range bounds are RFC3339 strings. Recall returns whatever the handler
returns; recency and semantic both return the record list.

### Raw stores

`store.*` is the lower-level table API. It is gated by the `memory` capability
and scoped by the engine (a private store keys rows per user, plus the service
and pre-upgrade rows; a pool-bound store is governed by tenancy grants).

```lua
store.put("table", "key", value, { ttl = 3600, vector = { ... } })
store.get("table", "key")                 -- { value, found }
store.query("table", { kind = "all" })    -- { records = { { key, value, meta }, ... } }
store.delete("table", "key")
```

Query kinds: `all`, `prefix` (`prefix = "..."`), `text` (`text = "...", k = n`),
`vector` (`vector = {...}, k = n`), `time_range` (`from`, `to` as RFC3339).
The backend must support the kind: a store that declares `requires: ["vector"]`
(or `["text_search"]`) fails at boot when its backend cannot. Passing a vector
to a backend without the `vector` feature is an explicit error, never a silent
empty result.

### Files

The user-scoped file store (`files` capability) holds project trees with
engine-native snapshot versioning. Bytes stay engine-side; `put` takes text or
base64, `read` returns metadata and a handle.

```lua
files.put("notes", "inbox/1.md", msg.text)
files.list("notes", "inbox/")
files.commit("notes", { ref = "main", message = "note " .. msg.id })
files.checkout("notes", "main")
```

Scratch is per-session and TTL'd:

```lua
files.scratch.put("page.html", body)
files.scratch.read("page.html")
files.scratch.list()
```

---

## 5. Capabilities

Capabilities gate the Lua surface at runtime. A refused op raises a clear error
naming the op, the agent and the missing capability — it does not disappear into
`attempt to call a nil value`.

| Capability | Gates |
|---|---|
| `llm.chat` | `llm.chat`, `llm.stream`, `llm.embed`, `llm.rerank` |
| `memory` | `memory.*`, `store.*` |
| `tools` | `tools.list`, `tools.run` (declared tools follow visibility rules) |
| `agent.send` | `agent.send` (`agent.reply` is authorized by request ownership, `agent.list` is contact-scoped and ungated) |
| `agent.request` | `agent.request` |
| `agent.spawn` | `agent.spawn` |
| `net.http` | `http.request`, `http.get`, `http.post`, `os.env` |
| `net.mail` | `mail.*` |
| `shell.exec` | `shell.*` |
| `files` | `files.*` |
| `session.state` | `session.state.*` |
| `users` | `user.current`, `user.usage`, `user.has_credential`, `user.get`, `user.list` |
| `channel.push` | `session.push`, `session.push_user` |

Defaults when an agent omits `capabilities`: `llm.chat`, `memory`, `tools`,
`agent.send`, `net.http`, `users`. Declare `net.mail`, `shell.exec`, `files`,
`session.state` and `channel.push` explicitly.

Not gated by this table:

- `scheduler.*` — timers are messages the engine delivers, not authority;
- `runtime.triggers()` — read-only deployment data;
- `credential.get(name)` — ungated but allow-listed per agent
  (`credentials: [...]`); every access is logged and counted.

`plugins.allow_capabilities` is the deployment ceiling: an agent cannot declare
more than it allows, a spawn child gets the intersection of parent and template,
and `plugins.enforce_capabilities: false` restores declare-only behavior for
old configurations. Do not rely on it.

---

## 6. API reference

All signatures below are the prelude's. Failures raise a Lua error; use
`pcall` where a failure should not crash the loop.

### 6.1 `session`

```lua
session.inbox() -> Message                  -- parks
session.send(text, opts?)                   -- reply to the active message
session.send(text, { attachments = parts })
session.push(channel, reply_to, text, opts?) -- cap: channel.push
session.push_user(uuid, text)                -- cap: channel.push; identity layer
session.exit()                               -- end the session (never resumes)
session.state.set(key, value)                -- cap: session.state
session.state.get(key) -> value | nil
session.state.delete(key)
session.state.list() -> { key = value }
```

### 6.2 Logging and time

```lua
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
time.sleep(seconds)       -- yields; the session is idle while parked
os.time() -> Unix seconds -- UTC; takes no arguments
os.date([format,] t?) -> date table | string
os.env(name) -> string    -- engine env var; "" when unset
```

`os.date` returns a table by default: `{ year, month, day, hour, min, sec, wday,
yday, isdst }`, `wday` 1 = Sunday, all UTC. Pass a strftime string for text:
`os.date("%Y-%m-%d %H:%M:%S", t)`. Supported directives: `%Y %y %m %d %e %H %I
%M %S %p %A %a %B %b %j %w %u %Z %%`. There is no local timezone; a leading `!`
is accepted as a no-op. Cron triggers, by contrast, match in
`runtime.timezone_offset_hours`.

### 6.3 `agent`

```lua
agent.info() -> {
  name, session_id, address, model, instructions,
  history_budget,
  memory = { stores = { [name] = { backend, table, features, locality } },
             recall, embed_model, rerank_model, oversample },
  shell, skills, capabilities,
  agent_model, instructions_append, ...          -- per-user overrides
}
agent.config() -> {
  name, model, instructions_path | instructions_prompt,
  prompts = { key = "text" },
  goal = { type, success_signal, max_turns, on_goal_met }?,
  extras = { ... }
}
agent.send(address, payload)                    -- cap: agent.send
agent.request(address, payload, timeout_s?) -> Message  -- cap: agent.request
agent.reply(request_id, payload)                -- authorized by the request owner
agent.list() -> [ { session_id, agent, parent_id } ]   -- contact-scoped
agent.spawn(profile, spec?) -> { address, session_id, agent } -- cap: agent.spawn
```

- `agent.info().instructions` is the system prompt after any per-user layer is
  applied; `agent_model` and `instructions_append` expose the override pieces
  if a loop wants to branch on them.
- `agent.info().memory.stores[name].locality` is `deployment`, `region` or
  `instance`: how far the store's data reaches. Durable conversation data (the
  `dialogue`/`facts` stores) belongs on a `deployment` or `region` store; an
  `instance` store dies with the process, which makes it a cache, not history.
  A single-instance deployment may use one deliberately, and a regional cell
  must set `locality_ok: true` on a durable store to do so.
- `agent.config()` is the deployment profile as data. Secret references render
  as opaque markers `{env="VAR"}` / `{cred="service"}`; resolved values never
  cross the bridge.
- Addresses: `agent:<name>` for a configured static agent, `session:<id>` for a
  live session (and for a spawned child — use `spawn_result.address`). `:new`
  is not accepted by `send`/`request`.
- `agent.request` cancels on timeout; the timeout is in **seconds**.
- `agent.reply` must use the request id from the core-stamped provenance:
  `msg.provenance.request_id`. Never build it from `msg.payload`.
- `agent.spawn` requires a `profiles.agent` template. The child's capabilities
  are parent ∩ template, and its contacts are the parent's contacts ∩ the
  template's. `spec` is accepted for compatibility but is **not applied** in
  this build; configure the template instead.

### 6.4 `scheduler`

```lua
scheduler.every(interval_s) -> { id }
scheduler.after(delay_s) -> { id }
scheduler.cron(expr) -> { id }
scheduler.cancel(id)
```

Timers deliver `Message{ type = "timer", payload = { timer_id = id } }` into the
same session. They are in memory and session-scoped: a restart forgets them, and
a hot reload restarts the loop **without** clearing timers, so re-registering
blindly duplicates them. Cancel the remembered id first (see
[`scheduled.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/scheduled.lua)).

`scheduler.cron` is a subset: `*/N * * * *` and fixed forms like `30 9 * * *`;
day-of-month, month and weekday must be `*`, and a `*/N` minute cannot be paired
with a fixed hour. Use `every`/`after` (or engine triggers) for richer schedules.

### 6.5 `llm`

```lua
llm.chat(messages, opts?) -> {
  text, thinking, thinking_blocks, usage, tool_calls
}
llm.stream(messages, opts?) -> next_delta, close
llm.embed(inputs, opts?) -> { vectors, usage }
llm.rerank(query, documents, opts?) -> { results = [ { index, score } ] }
```

`messages` is an array of `{ role, content }` turns, or `{ role, parts = {...} }`
for multimodal turns. Tool history uses `tool_calls`, `tool_call_id`, and
`tool_result`/`content` on a `role = "tool"` turn. Replay `thinking_blocks` on
the assistant turn when continuing an Anthropic tool loop with thinking on.

`opts` for `llm.chat`:

```lua
{
  model = "default",          -- a models: entry; "" = agent default
  temperature = 0.7,
  max_tokens = 4096,
  thinking = "off"|"low"|"medium"|"high"|"xhigh"|"max",
  tools = tools.list(),       -- model-visible tool specs
  tool_choice = "auto"|"none",
}
```

`usage` is `{ input, output, cached, cache_write, reasoning }` where the
provider reports them.

**Streaming.** Call `next_delta` directly; a generic `for` is refused because the
iterator yields across the bridge. The iterator closes itself at done; call
`close()` after an early exit. It returns text deltas only — use `llm.chat` for
usage and tool calls. See [`stream.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/stream.lua).

**Embeddings.** `llm.embed` takes a single string, an array of strings, or a
mixed array of strings and part tables (`{type="image", handle=..., url=...}`);
`merged = true` folds everything into one vector (a Jina content group).
`llm.rerank` returns 0-based indexes into `documents`.

### 6.6 `memory` and `store`

```lua
memory.write(record) -> true
memory.recall(query, opts?) -> [ { key, value, meta } ]

store.put(table, key, value, opts?)   -- opts: { ttl = seconds, vector = {...} }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records = [ { key, value, meta } ] }
store.delete(table, key)
store.scopes(table) -> [ "user:<uuid>", ... ]   -- maintenance provenance only
```

The memory record and query shapes are in [Memory](#memory). `store.scopes` is
only reachable from engine-fired maintenance provenance (`system` / `scheduler`
turns); a channel or agent-hop turn is denied. It is how a nightly distiller
enumerates user scopes without reading one user's turn.

### 6.7 `tools` and `tool.def`

```lua
tools.list() -> [ { type = "function", ["function"] = { name, description, parameters } } ]
tools.run(name, args, opts?) -> result
-- opts: { confirmed = true, confirm_id = "..." }
```

Declare a model-visible tool in Lua at chunk top level, before `loop()`:

```lua
tool.def({
  name = "lua:shout",
  description = "Uppercase the text and return it.",
  params = {
    type = "object",
    properties = { text = { type = "string" } },
    required = { "text" },
  },
  handler = function(args)
    return { ok = true, shouted = string.upper(args.text or "") }
  end,
})
```

Declared tools merge into `tools.list()` and dispatch in `tools.run`; a handler
that raises returns `{ ok = false, error = ... }` instead of killing the loop.
Visibility follows the agent's `skills` and `tools.policy.default` exactly as
Go-registered tools do — a hidden declared tool is not listed and not
dispatchable. The Go policy (`forbidden`, `needs_confirm`) does not gate the
loop's own declared handlers.

If a tool (Go or declared through an override) has `needs_confirm: true`, the
runtime intercepts the first `tools.run`, asks the user, and re-invokes with
`confirmed = true`; the loop sees only the final result. The prompt wait is
bounded (ten minutes).

### 6.8 `shell`

```lua
shell.spawn(opts) -> { id, provider, state }
shell.exec(id, command) -> { stdout, stderr, exit_code, duration_ms }
shell.write(id, path, content)
shell.destroy(id)

-- opts:
-- { provider = "docker"|"ssh", image, workdir, env, network,
--   mem_limit, cpu_limit, volumes = { "/host:/work" },
--   project = "notes", ref = "main", scratch_mount = "/scratch",
--   host, user, password, key_file }
```

A handle is persistent within the session and recoverable across a failover;
`project` + `ref` checks out a files snapshot into the container before the
handle reports ready. Needs `shell.exec`.

### 6.9 `http` and `os`

```lua
http.request(opts) -> {
  ok, status, headers, body,
  truncated?, saved?, save_error?
}
http.get(url, opts?)
http.post(url, body, opts?)

-- opts: {
--   method, url, headers, body, json, query, timeout,
--   auth = { service = "..." },      -- stored credential, resolved by Go
--   save_to = "name",                -- body also lands in files scratch
-- }
```

Any HTTP status is a successful request result (`ok` says the request
completed; `status` says what the server said). The body is capped at 10 MiB
(`truncated` is set when it was cut); `save_to` keeps a large body out of Lua
and returns `saved = { name, handle, size }`. Private, loopback, link-local and
reserved destinations are refused in the dialer. Needs `net.http` (which also
gates `os.env`).

The `builtin:fetch` **tool** is a different, richer surface: it additionally
supports `follow`/`max_redirects`, `user_agent`, `max_bytes`, and
`insecure` (TLS verification off, always logged and counted). Use the tool when
the model should call it; use `http.request` when the loop itself needs a
request.

### 6.10 `mail`

```lua
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
-- opts: { host, port = 993, user, mailbox = "INBOX", unseen = false,
--         limit = 10, auth = { service = "..." } }
-- messages: [ { seq, uid, flags, subject, from, to, date, body } ]

mail.smtp_send(opts) -> { ok, sent }
-- opts: { host, port = 465, user, from, to = { ... }, subject,
--         text_body, auth = { service = "..." } }
```

The password is resolved from the credential store at call time and never
crosses the bridge. `mail.*` is not covered by the HTTP private-address guard.
Needs `net.mail`.

### 6.11 `files`

```lua
files.put(project, path, text | { data = base64, handle = "media:...", mime = "..." }, opts?) -> { ok, entry }
files.read(project, path) -> { ok, entry }
files.list(project, prefix?) -> { ok, entries }
files.delete(project, path) -> { ok = true }
files.projects() -> { ok, projects }
files.commit(project, { ref = "main", message = "..." }?) -> { ok, commit }
files.checkout(project, ref | commit_id) -> { ok, manifest }

files.scratch.put(name, text | { data = base64, mime = "..." }) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

Scope is engine-resolved from the turn's identity; a loop cannot name another
user's project. A commit is a full-tree snapshot with a parent pointer and a
named ref — versioning is engine-native, not a git server. Needs `files`.

### 6.12 `user`

```lua
user.current() -> { ok, registered, id, display_name, identities }
user.usage() -> { ok, input, output, cached, reasoning, calls, failed,
                  billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { ok, service, present }
user.get(id) -> profile          -- maintenance provenance only
user.list() -> [ profile ]       -- maintenance provenance only
```

Read-only facts about the person the turn serves. `has_credential` answers
whether a key exists, never its value. `get`/`list` are limited to
engine-fired maintenance turns.

### 6.13 `runtime` and `credential`

```lua
runtime.triggers() -> {
  ok,
  triggers = [ {
    name, kind, run_on_boot,
    target = { profile }, payload,
    event = { channel, match } | cron | every,
  } ]
}

credential.get(name) -> { ok, name, value }
```

`runtime.triggers()` is the merged trigger list snapshotted at boot, available
to loops and route handlers. `credential.get` succeeds only for names in the
agent's `credentials:` allow-list; every access is logged and counted, and the
value should never be sent to a user.

### 6.14 `json`

```lua
json.encode(value) -> string
json.decode(text) -> value
```

Everything crossing the bridge is JSON-encoded. Consequences:

- an **empty table encodes as `{}`**, not `[]`; to send an empty array, omit the
  field or use a different signal;
- an array must be a dense 1..n sequence; mixed or non-numeric keys make it an
  object;
- functions, userdata and cyclic tables cannot cross;
- `nil` fields disappear; `NaN`/`±Inf` encode as `null`;
- `json.decode` requires a string and raises on malformed input.

---

## 7. Patterns

Each pattern below has a runnable file in `examples/loops/`.

### 7.1 Request/response

[`echo.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/echo.lua) — the smallest loop.

```lua
function loop()
  while true do
    local msg = session.inbox()
    session.send("echo: " .. (msg.text or ""))
  end
end
```

### 7.2 Conversational agent with memory

[`chat.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/chat.lua) recalls recent turns, builds the
prompt, trims it to the history budget, replies, and records the turn.

```lua
local info = agent.info()
local messages = {
  { role = "system", content = info.instructions or "You are a helpful assistant." },
}
local hits = memory.recall({ store = "dialogue", kind = "text", text = msg.text, k = 5 })
for _, hit in ipairs(hits) do
  local v = hit.value
  if type(v) == "table" then
    if v.text then table.insert(messages, { role = "user", content = v.text }) end
    if v.reply then table.insert(messages, { role = "assistant", content = v.reply }) end
  end
end
table.insert(messages, { role = "user", content = msg.text or "" })
messages = token_budget(messages, info.history_budget or 6000)
local reply = llm.chat(messages)
session.send(reply.text or "")
memory.write({ type = "turn", id = msg.id, text = msg.text, reply = reply.text })
```

`token_budget` is a support chunk loaded into every session; it keeps the system
message and the newest message and drops the oldest history until the estimate
fits.

### 7.3 Streaming

[`stream.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/stream.lua) — direct iterator, `close()` on an
early exit. Remember: text deltas only; a generic `for` does not work.

### 7.4 Multi-round tool loop

[`tool_loop.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/tool_loop.lua) — the full shape:

1. send the user turn with `tools = tools.list()`;
2. if the reply has `tool_calls`, append the **assistant** turn (including its
   tool calls and thinking blocks);
3. run each call, append a `role = "tool"` turn with `tool_call_id`;
4. ask again — and keep looping, because the model may request another tool;
5. stop at a final text reply or a round cap.

A loop that runs tools but never sends their results back, or never asks again,
produces a model that appears to ignore its tools.

### 7.5 Declaring a tool in Lua

Use `tool.def` at chunk top level (see the API reference). The handler runs on
the loop coroutine and may call any op; a raised error becomes
`{ ok = false, error = ... }`.

### 7.6 Timers and periodic work

[`scheduled.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/scheduled.lua) registers timers
idempotently and distinguishes them by `payload.timer_id`. Timer turns have no
channel; do not `session.send` from one.

### 7.7 Daemon agents

[`daemon.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/daemon.lua) handles the synthetic `boot`
message a `persistent: true` agent receives at startup, then serves user
messages and its own timer ticks. A daemon is one session; in a fleet exactly
one instance owns it.

### 7.8 Multi-agent delegation

[`delegate.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/delegate.lua) spawns a child from a
`profiles.agent` template and waits for a request/reply:

```lua
local child = agent.spawn("researcher")
local answer = agent.request(child.address, { question = msg.text }, 120)
```

On the worker side, answer a request with the core-stamped id:

```lua
if msg.type == "agent" then
  local rid = msg.provenance and msg.provenance.request_id
  if rid then agent.reply(rid, { answer = "..." }) end
elseif msg.type == "user" then
  session.send("...")
end
```

A static peer can be addressed as `agent:<name>` (subject to `can_contact`).
The tenant follows the hop; the child writes the same user's scopes unless it
opted out with `inherit_user: false`.

### 7.9 Files and snapshots

[`files.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/files.lua) writes a note, lists the project,
and commits a snapshot. Use `files.checkout` to restore a tree, or
`shell.spawn({ project = "...", ref = "main" })` to materialize it into a
container.

### 7.10 HTTP with a stored credential

```lua
local r = http.request({
  url = "https://api.example.com/v1/items",
  headers = { ["Accept"] = "application/json" },
  auth = { service = "example_api" },   -- resolved by Go; never in Lua
  query = { limit = 10 },
  timeout = 20,
})
if r.status == 200 then
  local data = json.decode(r.body)
  session.send("found " .. tostring(#(data.items or {})) .. " items")
else
  session.send("upstream said " .. tostring(r.status))
end
```

For a large body, add `save_to = "response.json"` and read it with
`files.scratch.read`. Destinations are address-guarded unless the deployment
opts out with `net.http.allow_private`.

### 7.11 Mail

```lua
local inbox = mail.imap_fetch({
  host = "imap.example.com", user = "bot@example.com",
  mailbox = "INBOX", unseen = true, limit = 5,
  auth = { service = "mail_password" },
})
```

### 7.12 Maintenance loops

A `system`/`scheduler`-provenance turn may enumerate scopes and profiles:

```lua
for _, scope in ipairs(store.scopes("dialogue")) do
  -- scope is "user:<uuid>"; read and write with scope-explicit keys
end
```

Engine-fired work (nightly compaction, a distiller) is the intended use.
Channel turns are denied, so one user can never enumerate another's scopes.

---

## 8. Resilience

### Catch failures per turn

```lua
local ok, err = pcall(handler, msg)
if not ok then
  log.error("turn failed: " .. tostring(err))
  session.send("Sorry — that turn failed.")
end
```

An uncaught error crashes the loop and it restarts every second; a user-facing
error is almost always better than a crash loop.

### Retry with backoff

```lua
local function with_retry(fn, attempts)
  for i = 1, attempts do
    local ok, res = pcall(fn)
    if ok then return true, res end
    if i < attempts then time.sleep(0.5 * i) end
  end
  return false, "retries exhausted"
end
```

`time.sleep` yields; it does not block a thread.

### Idempotency

A session can restart mid-turn (crash, hot reload, instance failover). If a
turn performs an external side effect, record a marker in `session.state`
before or after it and check it on the way in. There is no exactly-once
guarantee across a restart.

### Bound every external call

Set `timeout` on `http.request`; use the `agent.request` timeout; cap tool
rounds; cap history with `token_budget`. A loop that can wait forever will
eventually hold a session forever.

---

## 9. Prompt assembly and token budgets

A reliable prompt shape:

```lua
local info = agent.info()
local messages = {
  { role = "system", content = info.instructions or "" },
}
-- recalled history (oldest → newest)
-- current user turn
messages = token_budget(messages, info.history_budget or 6000)
local reply = llm.chat(messages, { model = info.model })
```

- `info.instructions` already includes any per-user instruction layer.
- `token_budget` estimates tokens as `ceil(chars / 4)`, keeps the system message
  and the newest message, and drops the oldest history until it fits.
- **Tool results are not counted by `token_budget`.** A tool that returns a huge
  body can evict real history; bound tool output (`builtin:fetch` has
  `max_bytes`, default 1 MiB) and summarize large results in the loop.
- Budgets are enforced before the call: a declared `budget.tokens_per_day` is a
  hard per-agent pool; per-user quotas are charged from the ledger.
- Thinking tokens are reported in `usage.reasoning`; they count toward the
  budget.

---

## 10. Testing and debugging

### In Studio

1. Start the runtime and connect Studio (`runtime.studio.enabled`, default on).
2. Edit the loop in the builder and press **Validate** — it checks the prelude
   directive, compiles the chunk, and validates the agent definition.
3. **Apply** and open the playground. Each turn streams a trace:
   `turn.start`, `op.start`/`op.end` (model, tool, duration, tokens), safety
   decisions, replies, and loop errors.
4. Set `runtime.studio.trace_payloads: true` to capture prompt and reply text
   in the trace (off by default).

### From a terminal

```bash
./agentflow -config agentflow.yaml -log-level debug -no-tui
```

`debug` logs every op dispatch, LLM request, memory operation and tool call.
`log.info` from a loop appears in the same stream. The web console's log tail
(SSE) is the same feed.

### Hot reload

Save the loop file; the running session restarts from a fresh state at its next
park point. If the new file does not compile or declares the wrong prelude
version, the running version stays in place. `session.state`, memory, stores and
files survive the restart; Lua locals do not.

### Example loops

Every file under `examples/loops/` is checked by
`TestExampleLoopsCompile` — it must declare the current prelude version and
compile. Use them as starting points, then point an agent's `loop:` at your
copy.

---

## 11. Gotchas

- **No directive, no run.** `-- af-prelude-version: N` in the leading comment
  block is mandatory. A mismatch is a hard refusal, not a warning.
- **Returning from `loop()` restarts it.** Call `session.exit()` to end a
  session.
- **An uncaught error is a crash loop.** Wrap turns in `pcall`.
- **`session.send` needs an active inbound message.** On `timer`, `cron` and
  `boot` turns it fails; use `push`/`push_user` or keep state.
- **Timers duplicate across hot reloads.** Cancel the remembered id before
  registering again; timers are in memory and lost on restart.
- **`llm.stream`: no generic `for`.** Call `next_delta()` directly and `close()`
  on an early exit. It yields text only.
- **The tool loop needs the second round.** Append the assistant tool-call turn
  and each tool result; then ask again. `token_budget` does not count tool
  results.
- **`agent.spawn(profile, spec)` ignores `spec`.** Configure the spawn template.
- **`agent.request` timeout is seconds**, and `agent.reply` needs
  `msg.provenance.request_id`, not a payload field.
- **Identity comes from provenance.** `payload.user_uuid` is ignored; a loop
  cannot widen a tenant.
- **`memory.recall` returns a list**, while `store.query` returns `{ records }`.
- **Scopes are enforced engine-side.** `store.scopes` and `user.get/list` are
  maintenance-only; a channel turn is denied.
- **`os` is not the real Lua os.** Only `os.env`, `os.time`, and `os.date`
  exist, all UTC. There is no `io`, no `debug`, no socket library.
- **Empty tables encode as `{}`.** Arrays must be dense; functions and cyclic
  tables cannot cross the bridge.
- **Capabilities are enforced, not advisory.** A withheld op raises a refusal
  naming the capability; declare `files`, `net.mail`, `shell.exec`,
  `session.state` and `channel.push` explicitly.
- **No hooks.** The engine dispatches only `safety.ingress`, `safety.egress`
  and `gateway.route`; everything else is a naming convention called by the
  prelude or your loop (`memory_routing_table`, `memory_recall_handler`,
  `token_budget`, ...).
- **`scheduler.cron` is a subset.** Use `every`/`after` or engine triggers for
  day/month/weekday schedules.
- **`http.request` refuses private destinations** in the dialer (redirect hops
  included) unless the deployment sets `net.http.allow_private`.

---

## 12. Quick reference table

| Op | Capability | Blocking | Notes |
|---|---|---|---|
| `session.inbox` | — | parks | the mail loop |
| `session.send` | — | yes | reply to the active message; safety + journal |
| `session.push` / `push_user` | `channel.push` | yes | proactive egress |
| `session.exit` | — | no | ends the session |
| `session.state.*` | `session.state` | yes | durable per-session KV |
| `log.*` | — | inline | log levels |
| `time.sleep` | — | yields | idle, not blocking |
| `agent.info` / `config` | — | inline | identity and profile |
| `agent.send` | `agent.send` | yes | `can_contact` |
| `agent.reply` / `agent.list` | — | yes/inline | request ownership / contact-scoped |
| `agent.request` | `agent.request` | yes | seconds |
| `agent.spawn` | `agent.spawn` | yes | spawn template |
| `scheduler.*` | — | yes | session timers |
| `llm.chat` / `stream` / `embed` / `rerank` | `llm.chat` | yes | metered/budgeted |
| `memory.*` / `store.*` | `memory` | yes | scoped |
| `tools.list` / `run` | `tools` | yes | confirm interception |
| `shell.*` | `shell.exec` | yes | persistent handle |
| `http.*` / `os.env` | `net.http` | yes | address guard |
| `mail.*` | `net.mail` | yes | no address guard |
| `files.*` | `files` | yes | user-scoped |
| `user.*` | `users` | yes | maintenance-only get/list |
| `runtime.triggers` | — | inline | deployment data |
| `credential.get` | allow-list | yes | logged; never echo |
| `json.*` | — | inline | bridge encoding |

---

## 13. Support chunks, shadows and route handlers

### Support chunks

Five builtins are evaluated into every session state **before** the loop:
`token_budget`, `routing_table`, `recency`, `semantic`, `fact_extractor`. They
define globals your loop may call:

| Chunk | Global | Used by |
|---|---|---|
| `plugin:token_budget` | `token_budget(messages, budget)` | your loop |
| `plugin:routing_table` | `memory_routing_table(record)` | `memory.write` |
| `plugin:recency` | `memory_recall_handler(query, opts)` | `memory.recall` |
| `plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | `memory.recall` |
| `plugin:fact_extractor` | `fact_extractor_turn(turn)` | your loop, after a turn |

To replace one, put a same-named file in `plugins.dir` (e.g.
`plugins/recency.lua`). It shadows the builtin for the whole deployment; the
shadow is watched, and it is loaded on every session start, so an edit takes
effect without a process restart. A shadow is still version-gated.

### Route handlers

`gateway.route` (default `plugin:per_chat`) is a different surface: one Luau
state per process, outside any session. Its `loop()` receives
`{ message = Message, agent = "..." }` items and decides where a message goes:

```lua
function loop()
  while true do
    local item = session.inbox()
    local msg = item.message
    af.op({
      type = "deliver",
      agent = item.agent,
      key = (msg.channel or "?") .. ":" .. (msg.from or "?"),
      message = msg,
    })
  end
end
```

A route handler may call only `inbox`, `deliver`, `router.state.*`,
`runtime.triggers` and `log`; everything else is refused by name. It cannot
send a channel reply or call a model — it routes, the session loop does the
work. `router.state` is durable, deployment-wide state (30-day default TTL per
key) for things like a chat's routing mode. The default `per_chat` handler also
implements the `/group <uuid>` / `/personal` toggle, with the engine validating
the proposed membership; replace the handler wholesale if your product wants a
different policy, but the engine-side validation is not optional.

---

## Further reading

- [Full config reference](configuration.md) —
  agents, memory, capabilities, triggers, MCP, channels.
- [Lua plugin development](loops-reference.md) — the
  embedded docs' API tables and multimodal matrix.
- [`examples/loops/`](https://github.com/hulipa487/agentflow/tree/main/examples/loops) — every runnable pattern above.
- [`agentflow.yaml`](https://github.com/hulipa487/agentflow/blob/main/agentflow.yaml) — the example deployment (the repo
  gitignores this file because it is the local instance config; the tracked
  example pieces live under `examples/`).
