## Lua Plugin Development

AgentFlow plugins are plain [Luau](https://luau-lang.org/) scripts. YAML defines which plugin runs for which agent; the plugin defines the behavior. The long-form guide in this documentation (Loops) covers the execution model, the complete API, runnable patterns and a gotchas list; this section is the reference summary.

There are three kinds of plugin:

- **Loop plugin** — defines a global `loop()` function that owns the session coroutine for its lifetime. The canonical pattern is `while true do local msg = session.inbox() ... end`.
- **Support chunk** — loaded into every session state *before* the loop plugin. Support chunks define globals and helper functions the loop reads. Builtins like `plugin:token_budget`, `plugin:routing_table`, `plugin:recency`, `plugin:semantic` and `plugin:fact_extractor` are support chunks.
- **Shadow** — a user plugin placed in `plugins/` with the same name as a builtin. It replaces the builtin instance-wide at load time.

### The async bridge

The only native primitive exposed to Luau is `__af_op(json_request)`. It yields the coroutine; Go executes the request (inline or on a worker pool); then the coroutine resumes with `(json_response, ok)`. The prelude wraps this raw primitive into the friendly tables documented below. Because every call yields, a busy instance stays cheap — no OS thread is blocked waiting for a model, database, or HTTP response.

```
-- What the prelude does for you (conceptually):
local function op(req)
  local resp, ok = __af_op(json.encode(req))
  if not ok then error(resp, 0) end
  return json.decode(resp)
end
```

Blocking ops include `send`, `llm.chat`, `tools.run`, all `store.*`, all `shell.*`, `http.request`, the `mail.*` ops, `agent.*`, and `scheduler.*`. Fast ops like `inbox`, `log`, and `agent.info` run inline on the actor goroutine.

### Message lifecycle

`session.inbox()` parks the coroutine until the session mailbox receives a message. A message is a table with at least:

```
{
  id = "msg-...",
  type = "user",           -- "user" | "system" | "timer" | "cron" | "agent" | ...
  from = "webhook:alice",  -- core-stamped sender
  text = "hello",
  channel = "webhook",
  reply_to = "...",
  ts = 1234567890,
  -- Core-assigned, never loop-controlled. user_uuid is the tenant this turn
  -- belongs to; it is absent when nothing stamped the message, and "" when the
  -- message belongs to no tenant (an unlinked handle). Read identity from here,
  -- never from payload -- a hop's payload is rebuilt from the caller's Lua.
  provenance = { kind = "agent", principal = "agent:planner", user_uuid = "u_xxxx" }
}
```

`session.send(text)` replies through the active channel (the channel and reply target of the current message). `session.push(channel, reply_to, text)` sends proactively and requires the `channel.push` capability. `session.push_user(uuid, text)` pushes to a user by identity UUID. `session.exit()` terminates the session cleanly.

### Writing a loop plugin

A minimal echo loop looks like this (every chunk starts with the prelude version):

```
-- af-prelude-version: 1
log.info("echo agent online")

function loop()
  while true do
    local msg = session.inbox()
    log.info("message from " .. (msg.from or "?"))
    time.sleep(0.3)
    session.send("echo: " .. (msg.text or ""))
  end
end
```

A ReAct-style loop calls an LLM and optionally tools: fetch history with `memory.recall`, pass tool definitions from `tools.list` to `llm.chat`, invoke any returned tool calls with `tools.run`, and reply with `session.send`. Write it as a plain loop file and point `loop:` at it.

### Built-in Lua API reference

All tables below are injected only when the agent's capabilities allow them. Attempting to call a disabled API raises an error.

#### `session` — core

```
session.inbox() -> Message            -- parks until the next mailbox message
session.send(text)                    -- reply to the active inbound message
session.send(text, { attachments = parts }) -- send with media (see below)
session.push(channel, reply_to, text) -- proactive egress; cap: channel.push
session.push(channel, reply_to, text, { attachments = parts })
session.push_user(uuid, text)         -- proactive egress to a user UUID; cap: channel.push
session.exit()                        -- terminate this session

session.state.set(key, value)         -- cap: session.state (declare it)
session.state.get(key)                -- nil when the key was never set
session.state.delete(key)
session.state.list()                  -- { key = value } for this session
```

Messages carry an `attachments` array of media part descriptors (`{type, mime, handle, name}`) when the channel ingested media. Forward them unchanged into `llm.chat` or `session.send`; the runtime resolves blob handles ("media:<sha256>") to bytes at the provider / channel boundary — raw media never enters Lua.

`session.state` is a small durable key/value store for this session alone: a cursor, a step count, the last thing said. It survives a restart, and in a fleet it follows the session to whichever instance owns it next, because the rows live in the shared store. Keys may contain letters, digits and `. _ - :` up to 128 characters; a value is any JSON-encodable Lua value up to 64 KiB, and a session may hold 256 keys. It is opt-in — declare `session.state` in the agent's `capabilities`, because it is a persistence surface rather than something to hand out by default.

#### `llm` — cap: `llm.chat`

```
llm.chat(messages, opts?) -> { text, thinking, thinking_blocks, usage, tool_calls }
llm.stream(messages, opts?) -> iterator
llm.embed(inputs, opts?) -> { vectors, usage }   -- strings and/or part tables
llm.rerank(query, documents, opts?) -> { results } -- results: [{ index, score }], 0-based

-- opts: { model = "default", temperature = 0.7, max_tokens = 4096,
--         thinking = "low"|"medium"|"high"|"xhigh"|"max"|"off",
--         tools = [...], tool_choice = "auto" | "none" }
```

Messages are arrays of `{ role, content, tool_calls?, tool_call_id?, tool_result?, thinking_blocks? }`. **Multimodal turns** use `{ role, parts = {...} }`; each part is a table:

```
{ type = "text",  text = "..." }
{ type = "image"|"audio"|"video"|"file", mime = "image/png", handle = "media:...",
  name = "photo.jpg" }   -- handle from msg.attachments (data base64 / url also valid)
```

Provider support matrix (unsupported part types error honestly; sources are base64 `data`, `url`, or a provider file reference):

| Provider | image | audio | video | PDF / files |
| --- | --- | --- | --- | --- |
| anthropic | ✅ base64/url | ❌ | ✅ base64/url (MiniMax video block) | ✅ document (base64) |
| openai (chat) | ✅ image\_url | ✅ input\_audio (wav/mp3) | ✅ video\_url — MiniMax/Kimi/GLM convention | ✅ file\_url (url) — GLM |
| openai-responses | ✅ input\_image | ✅ input\_audio (wav/mp3) | ✅ input\_video (url/file ref) | ✅ input\_file (data or file\_id) |
| gemini (interactions) | ✅ files api | ✅ files api | ✅ files api | ✅ files api |

Video sources follow the ecosystem conventions — MiniMax `mm_file://<file_id>`, Kimi `ms://<file_id>` — and are passed through verbatim as URL/file references. The shapes the SDKs' typed request unions do not model (`video_url`, GLM's `file_url`, the Anthropic video block, the Responses `input_video` and `input_audio` items) are built with `param.Override` — the SDKs' own raw-JSON escape hatch, which marshals verbatim when no typed variant is set. Blob-store `handle`s are resolved to base64 automatically.

For gemini, every media part is uploaded to the **Gemini Files API** (resumable upload) and referenced by URI — never inline base64. `data` (base64), `url`, and blob-store `handle` sources all work (urls are downloaded first); repeat uploads of identical bytes are deduped by content hash for the file's 48-hour lifetime, after which Google deletes it automatically. Anthropic/openai also accept `url` sources directly. `llm.stream` returns two values — `next_delta, close`. Call `next_delta()` directly (it returns nil at done), never in a generic `for ... in`: the iterator yields across the bridge and Luau refuses that from the generic-for C boundary. Call `close()` after an early exit; the iterator closes itself at done. It returns text deltas only — use `llm.chat` for usage and tool calls.

**Multimodal embeddings** follow the Jina convention: `llm.embed` accepts a mixed array of strings and part tables, and batches carrying media serialize each item as a typed doc (`{"text"}`, `{"image"|"video"|"audio"|"pdf": url|base64}`) — the shape used by `jina-embeddings-v5-omni-small` (text, image, video, audio, and PDF in one vector space) and `jina-clip-v2`. Text-only batches stay plain strings, so any OpenAI-compatible `/embeddings` endpoint (OpenAI, Ollama, vLLM, TEI, OpenRouter) sees the exact historical request. Opts: `task` (`retrieval.query` | `retrieval.passage` | `text-matching` | `clustering` | `classification`), `dimensions` (Matryoshka truncation), `merged = true` (fold all inputs into one embedding — a Jina content group; PDFs are single-input only). The Anthropic and Gemini providers have no embeddings API here and fail explicitly. `llm.rerank` requires a `provider: rerank` model entry speaking `POST {base_url}/rerank` (TEI, vLLM, Jina, Cohere). `llm.embed` is token-metered when the agent declares a budget. The `gemini` provider (Interactions API) is non-streaming: `llm.stream` over it yields the full reply as a single delta.

**Thinking content (response side).** Whatever reasoning content a provider sends is surfaced passively on the reply — `reply.thinking` is the human-readable text (empty string when none), `reply.thinking_blocks` the raw provider blocks (Anthropic `{type="thinking", thinking, signature}` / `redacted_thinking`, Responses reasoning items, Gemini `thought` parts; nil on Chat Completions, where reasoning is a bare string), and `usage.reasoning` counts thinking tokens where the provider reports them separately. Reasoning content presence is a per-provider, per-model-alias runtime property, not a contract: a serving config can bill reasoning tokens while withholding reasoning content from the stream entirely, so `usage.reasoning` is the reliable audit signal and an empty `reply.thinking` is not by itself a malfunction. The engine never asks for reasoning beyond what the thinking level already requests and never stores it — persistence is the loop's choice. For Anthropic multi-round tool use with thinking enabled, pass the blocks back verbatim on the assistant turn: `thinking_blocks = reply.thinking_blocks` (the Messages API rejects the continuation otherwise; other providers ignore the field).

```
-- multimodal embed (jina-embeddings-v5-omni-small via api.jina.ai/v1)
local r = llm.embed({
    "find me the cat photo",                    -- text
    { type = "image", handle = msg.attachments[1].handle },  -- blob-store media
  }, { model = "omni", task = "retrieval.query", dimensions = 512 })

-- one vector for a whole multimodal record (memory.write does this for
-- records carrying attachments when the profile names an omni embed_model):
local r = llm.embed({ "trip to the beach", img_part }, { model = "omni", merged = true })
```

#### `agent` — caps: `agent.send`, `agent.request`, `agent.spawn`

```
agent.info() -> { name, session_id, address, model, instructions,
                  history_budget, memory, shell, skills, capabilities, ... }
agent.config() -> { name, model, instructions_path, instructions_prompt?,
                    prompts = {  = "", ... },
                    goal = {type, success_signal, max_turns, on_goal_met}?,
                    extras = { ... } }
agent.send(address, payload)
agent.request(address, payload, timeout?) -> Message
agent.reply(request_id, payload)
agent.list() -> [{ session_id, agent, parent_id }]
agent.spawn(profile) -> { address, session_id, agent }
```

`agent.spawn` requires a `profiles.agent` template; the child’s capabilities are the intersection of the parent’s and the template’s. The second argument accepted by earlier builds is reserved and is not applied — configure the template instead.

`agent.config()` returns this agent's own profile as data: the config name, model, the instructions *path* (or `instructions_prompt` when the system prompt came from the `prompts` registry — `instructions_path` is then empty), the deployment's resolved `prompts` registry keyed by name, the profile's `extras.goal` hoisted to `goal`, and the remaining `extras` (a pm options block, a workflow name, …). Secret references inside render as opaque markers (`{env="VAR"}` / `{cred="service"}`) — a resolved secret never crosses the Lua bridge.

#### `user` — cap: `users`

```
user.current() -> { registered, id, display_name, identities }
user.usage() -> { registered, input, output, cached, reasoning, calls,
                  failed, billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { service, present }
user.get(id) -> { ... }    -- maintenance provenance only
user.list() -> [ { ... } ] -- maintenance provenance only
```

Read-only facts about the person this turn belongs to. `users` is a default capability, so a loop can call these without declaring anything. A loop learns a name and a channel, never an email and never a credential value — `has_credential` answers whether a key exists, not what it is. `get`/`list` are restricted to engine-fired maintenance provenance (a `system` or `scheduler` turn), which is what makes a per-user maintenance loop possible without exposing one user's profile to another user's turn.

#### `runtime` and `credential` — deployment config as data

```
runtime.triggers() -> { ok, triggers = [ { name, kind, run_on_boot,
                          target = { profile }, payload,
                          event = { channel, match }   -- kind = "event"
                          | cron                        -- kind = "cron"
                          | every } ] }                 -- kind = "every"

credential.get(name) -> { ok, name, value }              -- allow-listed only
```

`runtime.triggers()` returns the merged `triggers/*.yaml` (or top-level `triggers:`) list snapshotted at boot — read your CRON\_TASKS/ROUTES from the engine instead of baking tables into Lua source. It is answered the same way in **both** Lua surfaces: an agent loop and the gateway route handler (the router op switch serves the identical payload), so an event route can read the trigger list that drives it. Expressions pass through verbatim, and the shape is unchanged by engine scheduling: `every:`/`cron:` entries are also fired by the engine itself (see `triggers` in the config reference), so a loop only needs this call to *observe* the deployment's schedule. `credential.get(name)` fetches an engine-wide stored secret **only** when the agent's profile lists the name under `credentials: [...]` (empty or missing list = denied with an error saying so). Every access is logged (agent, name, granted/denied) and counted (`agentflow_credential_gets` / `agentflow_credential_gets_denied`); the value is never journaled and should never be echoed into a reply.

`router.state` is the route handler's own durable key/value store — where a handler keeps what it must not forget, such as which agent a chat was last routed to. It lives in the runtime store, so every instance reads the same value and nothing is lost to a restart or a failover: `router.state.get(key)` (nil when the key was never set), `router.state.set(key, value, {ttl_seconds = n})`, `router.state.delete(key)`, and `router.state.list(prefix)`. A value is any JSON-encodable Lua value up to 64 KiB; a key may contain letters, digits and `. _ - :` (128 characters). A key is kept for 30 days after its last write unless the handler asks for a different lifetime (`ttl_seconds = 0` keeps it until it is deleted), which is what bounds the store when a handler is keyed by something high-cardinality. The namespace is defined only in the router's state, so it is not reachable from an agent loop — and it is consistency, not serialization: two instances can interleave a read-modify-write on one key, while delivery itself is serialized by the session lease.

#### `scheduler` — not capability-gated

```
scheduler.every(interval) -> timer_id    -- seconds; dies with session
scheduler.after(delay) -> timer_id
scheduler.cron(expr) -> timer_id          -- cron subset (see below)
scheduler.cancel(timer_id)
```

Timers enqueue a `Message{ type = "timer" }` into the owning session's mailbox — carrying `msg.payload.timer_id` so one session can tell its timers apart. They never call Lua from a timer goroutine, so the VM stays single-threaded. `scheduler.cron` supports a **subset** of 5-field cron: `*/N * * * *` (every N minutes) and fixed minute/hour forms like `30 9 * * *`; the day-of-month, month, and weekday fields must be `*`, and a `*/N` minute cannot be paired with a fixed hour (a step cannot be anchored to one, so it would be silently dropped). Anything else is refused rather than reinterpreted — use `every`/`after` (or self-rescheduling) for richer schedules.

These are *session-scoped* timers: they die with the session. **There is no `scheduler` capability** — the ops are handled in the actor's own switch and never pass through the capability gate, so any loop may register a timer whatever it declares. The gate covers the ops reachable through a handler map (`llm.*`, `memory`/`store.*`, `tools.*`, `shell.*`, `http.request`, `mail.*`, the `agent.*` addresses); a timer is a message the engine delivers, not an authority. A deployment-wide schedule declared in YAML (`every:`/`cron:` triggers) is engine infrastructure instead — fired by the engine with no capability involved and delivered as `Message{ type = "cron" }`, so a loop does not have to run a timer loop to be scheduled. Use `os.time()`/`os.date()` (UTC) when a loop needs to reason about the clock itself. Note also that timer state is in memory: a restart forgets every timer until its loop re-registers it.

#### `memory` and `store` — cap: `memory`

```
memory.write(record) -> boolean          -- routed by plugin:routing_table
memory.recall(query, opts?) -> [ { key, value, meta } ] -- active recall handler

store.put(table, key, value, opts?)      -- opts: { ttl = seconds, vector = {...} }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records }
store.delete(table, key)
```

Store queries accept `{ kind = "all" | "prefix" | "text" | "vector" | "time_range", ... }`; vector queries take `{ kind = "vector", vector = {...}, k = n }`. Passing a `vector` to a store whose backend lacks the `vector` feature is an explicit error, never a silent drop. Prefer `memory.write`/`memory.recall` for agent memory; use `store.*` only when you need raw table access.

**Per-user scoping (private stores).** A channel-originated turn reads and writes only its own user scope, plus `service` rows (written by contexts with no tenant of their own: engine-fired work, unlinked handles, and agents that opted out with `inherit_user: false`) and `legacy` rows (written before this shipped — pre-upgrade history stays readable by every user of the agent and is never written again). Engine-fired contexts (`system`/`scheduler` provenance — nightly maintenance like the distiller) read every scope with scope-visible keys and write scope-explicitly; a maintenance session can enumerate a binding's user scopes via \`store.scopes\` (maintenance-only: channel and agent-hop sessions are denied). This is engine-enforced: no loop-code filtering. pool-bound stores (`pool: <name>`) are governed by tenancy grants instead, and a private store may opt back into one pool per agent with `scope: agent`.

#### `tools` — cap: `tools`

```
tools.list() -> [ToolSpec]
tools.run(name, args, opts?) -> ToolResult
-- opts: { confirmed = true, confirm_id = "..." }

tool.def({ name = "lua:shout",
           description = "Shout the text back",
           params = { type = "object",
                      properties = { text = { type = "string" } },
                      required = { "text" } },
           handler = function(args)
             return { ok = true, shouted = string.upper(args.text) }
           end })
```

If a tool declares `needs_confirm: true`, the first call returns a `needs_confirm` response. The runtime asks the user, then re-invokes the tool with `opts.confirmed = true`. Your loop sees only the final result. (The prompt wait is bounded: ten minutes, after which the call is denied — a confirm-gated tool on a scheduler turn has no channel to answer on, and an unbounded wait parked the session for good.)

**The tool loop is the loop's.** The engine executes a tool when asked and returns the result; it does not drive the conversation. Calling `llm.chat` with `tools`, running what came back through `tools.run`, and sending the results as a `role = "tool"` turn before asking again is the chunk's job — including going round a *second* time, which is the easy one to forget: a model that asks for another tool after seeing the first result will not get it unless the chunk checks `reply.tool_calls` again. The same applies to what a loop persists: a tool call and its result are history the next turn should see, and a chunk that stores only the assistant's text recalls a conversation in which the model refers to a tool result that is no longer there.

`tool.def` declares a model-visible tool in Lua — the engine-standard replacement for hand-assembled extra tool defs. Declared tools merge into `tools.list` (same `{ type = "function", function = {...} }` shape as Go tools, so they flow into `llm.chat` unchanged) and `tools.run` dispatches to the Lua handler. The handler runs synchronously on the loop coroutine and may itself call any op (`llm.chat`, `memory.*`, …). A declared name shadows a Go-registered tool of the same name in both list and run (the shadowing is applied after the `skills` filter, so a tool this agent cannot see never suppresses the Go tool it names). A handler error comes back as `{ ok = false, error = ... }` instead of killing the loop. Declared tools are the loop's own code: the Go tools policy (`forbidden`, `needs_confirm`, `permission: forbidden`) still does not gate them. Declare at chunk top level, before `loop()`.

**Visibility**: a declared tool is filtered by `skills` and `tools.policy.default` under exactly the rule that governs Go tools — listed when `skills` names it, hidden when `skills` is non-empty and omits it, all-or-nothing per `default` when `skills` is empty. That is what lets one shared loop directory carry tools only some agents get. It is a behavior change for a deployment with `default: none` and no `skills`: declared tools used to be listed unconditionally and are not now. `tools.run` applies the same rule — a declared name the agent cannot see falls through to the Go path and is refused as not-available, exactly as an invisible registered tool is. Registered tools were always enforced at dispatch, and `tools.run` is the model-facing path a hallucinated or injected tool call travels, so the two agree. A loop that wants its own handler calls the function directly.

**Overriding a declared tool's text**: `tools.policy.overrides` may name a declared tool, which moves its model-facing text out of the chunk and into config — the description (a literal, or `{prompt: <key>}` from the `prompts` registry) and `params.<param>.description`. The spec passed to `tool.def` stays the fallback when no override names the tool, and the override never touches the declared schema's types or `required`. Resolution happens when `tools.list()` is built, so a reloaded `file:`-backed prompt is reflected on the next call with no restart. An override never adds or removes a list entry: visibility comes from `skills` and the policy default, never from an override. A declared-but-hidden tool still counts as declared, so an override naming it is not reported as naming nothing.

#### `shell` — cap: `shell.exec`

```
shell.spawn(opts) -> { id, provider, state }
shell.exec(id, cmd) -> { stdout, stderr, exit_code, duration_ms }
shell.write(id, path, content)
shell.destroy(id)

-- spawn opts:
-- { provider = "docker", image = "alpine:3.20", workdir = "/work",
--   network = "none", mem_limit = "512m", cpu_limit = 1.0,
--   env = { KEY = "val" },
--   volumes = { "/host/dir:/work" },        -- docker -v bind mounts
--   project = "myapp", ref = "main",        -- check out a files snapshot into /work
--   scratch_mount = "/scratch",             -- copy this session's scratch there
--   -- SSH only:
--   host = "1.2.3.4:22", user = "root", password = "...", key_file = "/path" }
```

Docker handles are persistent: files written in one `shell.exec` are visible in the next. Handles are owned by the session and reaped automatically when the session dies. A spawn may carry a **checkout** (`project` + `ref`): the engine resolves the snapshot and writes every file into the container directory of the first `volumes` mount *before the handle reports ready* — a caller that gets a handle back can `exec` immediately and see the whole tree. A spawn may also request `scratch_mount`, which copies the calling session's scratch files into that container directory the same way. Both require the `files` store to be enabled at boot; a checkout without a volume mount fails with a clear error rather than silently doing nothing, and a failure mid-copy destroys the half-prepared container.

**A handle outlives the process that created it.** Every handle is recorded where the whole deployment can see it — the provider, the container it names, and the session that owns it — so a session that moves to another instance (a failover, a restart, a rolling deploy) finds the shell it was using rather than an empty one: the adopting instance reaches the container by id, and its files, environment and processes are all still there. That is Docker's shape — a container is an address. An SSH handle is a live connection instead, and a record never carries a password or a key, so a takeover dials a fresh connection from the session's shell profile. A handle whose session is alive nowhere, and which no live session has used for a day, is reclaimed: that sweep is the only thing that ever removes the containers of an instance that was killed. Handles are owned by the session and reaped when it dies, wherever they were created.

Agents with a bound shell profile (`agents.<name>.shell`) also get registry tools that resolve the profile implicitly — no `shell.spawn` glue in the loop: `builtin:shell.exec` (`{ command }` → `{ ok, stdout, stderr, exit_code, duration_ms, handle_id }`), `builtin:shell.write` (`{ path, content }`), and `builtin:shell.destroy`. The first call lazily spawns a handle from the profile; later calls reuse it — including on another instance, after a failover; destroy tears it down (a later call spawns a fresh one). An agent without a bound profile gets honest-unavailable, and agents without the `shell.exec` capability do not see these tools at all, even under `tools.policy.default: all`. All three default to `needs_confirm: true` — relax per deployment via `tools.policy.overrides`.

`builtin:fetch` is the general-purpose HTTP client for APIs and plain pages, and needs no account or credential — so unlike `web_search` and `browser` it is never honest-unavailable. Options: `url` (required), `method`, `headers`, `body` (raw) or `json` (marshalled; sets `Content-Type` unless a header already does), `query`, `timeout_ms`, `follow` / `max_redirects`, `user_agent`, `max_bytes`, `insecure`, and `save_to` (the response body also lands in this session's files scratch space, readable via `files.scratch.read`). It returns `{ok, status, status_text, headers, body, final_url, redirects?, content_type?, content_length?, size, truncated?, tls_verified, time_cost_ms}` — for *any* HTTP status, since a 404 is an answer the model should read rather than a tool failure; `ok` describes whether the request completed, `status` describes what the server said.

Bodies are capped (`max_bytes`, default 1 MiB) and the result always reports the true `size` and sets `truncated` when it cut. That bound is not a nicety: a tool result is handed to the model whole, nothing downstream truncates it, and the loop's token budgeter does not count tool results at all — so an oversized body would evict real conversation history rather than being trimmed. Destinations are subject to the `net` guard described above, and a refusal comes back as an error naming the address, counted as `agentflow_http_private_blocked` and logged against the calling agent.

`insecure: true` skips TLS certificate verification. It is always permitted — there is no config gate — but never silent: the skip increments `agentflow_http_insecure_tls`, logs a WARN naming the agent and URL, and reports `tls_verified: false` in the result. It skips only the certificate check; the address guard still applies, because who a peer claims to be and where the connection goes are different questions. The tool is `Autonomous: true` like `web_search`; a deployment that wants confirmation on it can set `needs_confirm` through `tools.policy.overrides`.

#### `http` and `os` — cap: `net.http`

```
http.request(opts) -> { ok, status, headers, body, truncated? }
http.get(url, opts?)
http.post(url, body, opts?)

-- opts: { method, url, headers, body, json, query, timeout, auth,
--         save_to = "name" }  -- body also lands in this session's files scratch
-- auth = { service = "..." } resolves a stored credential at request time.

os.env(name) -> string | nil
os.time() -> number                        -- Unix seconds (UTC)
os.date([format,] time?) -> table | string -- UTC; table unless a format is given
```

**Wall clock.** `os.time()` returns whole Unix seconds and takes no arguments (this is not Lua's `os.time(table)`). `os.date()` returns the date *table* — `{ year, month, day, hour, min, sec, wday, yday, isdst }` with `wday` 1 = Sunday … 7 = Saturday, `yday` 1–366, and `isdst` always `false` — and takes an optional Unix-seconds time (default: now). The table is JSON-encodable, so it can ride in a payload. Pass a strftime string to get text instead: `os.date("%Y-%m-%d %H:%M:%S", t)`. Supported directives are `%Y %y %m %d %e %H %I %M %S %p %A %a %B %b %j %w %u %Z %%`; an unknown one is left verbatim. **Everything is UTC** — the sandbox has no local timezone, so the fields are identical on every host, and a leading `!` (`os.date("!*t")`) is accepted as a no-op. Cron triggers, by contrast, are matched in `runtime.timezone_offset_hours`; converting between the two is your loop's business.

#### `files` — cap: `files`

```
files.put(project, path, content|{data=base64, handle=..., mime=...}, {mime}?) -> { ok, entry }
files.read(project, path) -> { ok, entry }        -- entry: {path, handle, size, mime, ts, revision}
files.list(project, prefix?) -> { ok, entries }
files.projects() -> { ok, projects }             -- the projects in this turn's scope
files.delete(project, path) -> { ok = true }
files.commit(project, {ref="main", message}?) -> { ok, commit }   -- snapshot; parent chain = history
files.checkout(project, ref|commit_id) -> { ok, manifest }        -- manifest: {commit, files=[{path,handle,size}]}
files.scratch.put(name, content|{data=base64, mime=...}) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

A **user-scoped file store**: named project trees whose scope is resolved by the engine from the turn's identity (`user:<uuid>` for channel-originated turns and for delegated turns that inherited a tenant, `agent:<name>` otherwise) — a loop can never name another scope. Bytes live in the content-addressed blob store (`files:` config; local filesystem or S3-compatible — unchanged files dedupe across snapshots for free); snapshot metadata (working tree, commits, refs, scratch) lives in the runtime store's `files_meta` table, engine-owned and inspectable with plain SQL. Versioning is **engine-native snapshots, not a git server**: a commit records the full tree (path → handle) with a parent pointer and message, and moves a named ref; checkout resolves a ref or a commit id. Raw bytes never cross the Lua bridge — `put` takes utf-8 content or base64 `data`, `read` returns the handle + metadata, and materialization is engine-side: shell spawn checkout, `scratch_mount`, or `save_to` from `http.request` / `builtin:fetch` / `builtin:browser`. Scratch is per-session (keyed by the calling session, not user), TTL'd by `files.scratch_ttl` (default 24h, lazy expiry on read + boot sweep). One `files` capability gates the whole family; it is not in the default set — declare it explicitly. With the store disabled at boot (e.g. unresolvable S3 credentials) every op fails honestly.

#### `mail` — cap: `net.mail`

```
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
-- opts: { host, port = 993, user, mailbox = "INBOX", unseen = false,
--          limit = 10, auth = { service = "..." } }
-- messages: [{ seq, uid, flags, subject, from, to, date, body }]
-- connects over implicit TLS; bodies capped at 256 KiB (truncated with a marker)

mail.smtp_send(opts) -> { ok, sent }
-- opts: { host, port = 465, user, from, to = {...}, subject, text_body, auth }
-- port 465 = implicit TLS; 587/25 = STARTTLS
```

Like `http.request`, `auth = { service = "..." }` names a stored credential resolved by Go at call time — the password never crosses the Lua bridge. Host, port, and username come from the loop (typically via `os.env`), so no secret sits in Lua source. `net.mail` is not in the default capability set; the agent must declare it.

#### Core utilities

```
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
json.encode(value) / json.decode(text)
time.sleep(seconds)        -- yields; never blocks a thread
```

### Writing support chunks

Support chunks are evaluated into the session state before the loop plugin. They can define globals that the loop or other support chunks read. For example, to replace the default recall strategy, shadow `plugin:recency` with a file in `plugins/recency.lua` that defines `memory_recall_handler(query, opts)`.

```
-- plugins/recency.lua (shadow example)
function memory_recall_handler(query, opts)
  local info = agent.info()
  local store_name = (info.memory and info.memory.stores.dialogue) or "dialogue"
  local res = store.query(store_name, { kind = "all" })
  return res.records or {}
end
```

### Error handling

Always wrap external calls in `pcall` so a sick provider or bad command does not crash-loop the session:

```
local ok, reply = pcall(llm.chat, messages, { model = info.model })
if not ok then
  log.warn("llm.chat failed: " .. tostring(reply))
  session.send("⚠️ I couldn't reach my model right now.")
  return
end
```

The bridge returns errors as JSON strings; the prelude unwraps them so stack traces point to your code, not the prelude.

### Confirmation and permissions

Tools and shell builtins can declare `needs_confirm`. The first `tools.run` returns a result with `needs_confirm = true` and a `confirm_id`; the actor asks the user, then re-invokes the tool with `opts.confirmed = true` and the same `confirm_id`. Your loop sees only the final result, so in most cases you do not need to handle confirmation manually.

### Restrictions and caveats

- `llm.stream` returns a closure — call it directly, never in a generic `for ... in`.
- The standard library includes base, coroutine, table, string, math, bit32, utf8, and buffer. `io` and `debug` are not available, and neither is the real `os` library: the prelude provides exactly `os.env`, `os.time`, and `os.date` (all UTC — there is no local timezone in the sandbox).
- Raw network access from Lua is only through `http.request` and the `mail.*` ops. There is no socket library. The HTTP paths — this op and the `builtin:fetch` tool — refuse private, loopback, link-local and reserved destinations in the dialer, so a URL is not enough to reach the host's own network; see the `net` block. `mail.*` is *not* covered by that guard and still dials whatever host the loop names.
- All values crossing the bridge are JSON-encoded. Tables must be serializable; functions and userdata cannot cross.
- Plugins are hot-reloadable: the reload watcher always runs (`runtime.reload.watch` has no effect). An edit to a loop file or an instructions file restarts the session coroutine from scratch on the next turn; an edit to the file behind `gateway.route` rebuilds the router's state between messages (routing runs there, not in a session), and an edit to a shadowed support chunk restarts every live session, which is where the new chunk is loaded. None of them needs a process restart, and none of them is applied if the new version does not compile — the version already running stays in place.

### Practical examples

Drop any of these into a loop file and point an agent's `loop:` at it:

#### Tool loop

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local info = agent.info()
      local tools = tools.list()
      local ok, reply = pcall(llm.chat,
        { { role = "user", content = msg.text } },
        { model = info.model, tools = tools, tool_choice = "auto" })
      if ok and reply.tool_calls then
        for _, tc in ipairs(reply.tool_calls) do
          local r = tools.run(tc.name, tc.args)
          log.info("tool result: " .. json.encode(r))
        end
        session.send(reply.text or "done")
      elseif ok then
        session.send(reply.text or "")
      else
        session.send("error: " .. tostring(reply))
      end
    end
  end
end
```

#### Shell loop

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local h = shell.spawn({ provider = "docker", image = "alpine:3.20" })
      local r = shell.exec(h.id, "echo hello from " .. msg.text)
      shell.destroy(h.id)
      session.send("exit=" .. r.exit_code .. " stdout=" .. r.stdout)
    end
  end
end
```

#### Multi-agent request

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local ok, answer = pcall(agent.request, "agent:expert", { question = msg.text }, 30)
      if ok then
        session.send("expert says: " .. (answer.payload and answer.payload.answer or "?"))
      else
        session.send("expert unavailable: " .. tostring(answer))
      end
    end
  end
end
```
