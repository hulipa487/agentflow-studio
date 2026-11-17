/** The completion/hover class of a prelude symbol. */
export type SymbolKind = 'function' | 'table' | 'variable' | 'snippet'

/** One global or field of the AgentFlow Luau prelude. */
export interface ApiEntry {
  path: string
  signature: string
  summary: string
  /** The capability a call needs, when it is gated. */
  capability?: string
  kind: SymbolKind
  /** Snippet text inserted by completion (LSP snippet syntax). */
  insert?: string
  /** The call parks the session, so it cannot run at chunk top level. */
  yields?: boolean
  /** Calls the engine only permits for system/scheduler turns. */
  maintenance?: boolean
}

// The documented surface of the prelude (docs/loops-reference.md). The
// runtime's internal/vm/prelude.go is the source of truth, and the desktop
// Studio's internal/lua/api.go is the list this one mirrors entry for entry:
// keep the three in step, together with the prelude version. The analyzer
// treats a version mismatch as an error, the same as the boot gate.
// prettier-ignore
export const preludeEntries: readonly ApiEntry[] = [
  // session
  { path: "session", signature: "session", summary: "The current session: inbox, replies, pushes and durable state.", kind: 'table' },
  { path: "session.inbox", signature: "session.inbox() -> Message", summary: "Wait for the next inbound message. This parks the session.", kind: 'function', insert: "inbox()", yields: true },
  { path: "session.send", signature: "session.send(text, opts?)", summary: "Reply to the active inbound message. Needs an active message: timer, cron and boot turns have none.", kind: 'function', insert: "send(${1:text})", yields: true },
  { path: "session.reply", signature: "session.reply({ text, tool_calls, finish_reason, usage, reasoning })", summary: "Finish the turn with a structured assistant message; an llm.forward result can be passed as it is. At most one per turn; a channel that cannot express tool calls refuses them rather than dropping them.", kind: 'function', insert: "reply({ text = ${1:text} })", yields: true },
  { path: "session.send_delta", signature: "session.send_delta(text)", summary: "One fragment of a streamed answer. Held until end of turn when the agent's safety profile has filters.", kind: 'function', insert: "send_delta(${1:text})", yields: true },
  { path: "session.push", signature: "session.push(channel, reply_to, text, opts?)", summary: "Send proactively to a channel. reply_to may be nil.", capability: "channel.push", kind: 'function', insert: "push(${1:channel}, ${2:reply_to}, ${3:text})", yields: true },
  { path: "session.push_user", signature: "session.push_user(uuid, text)", summary: "Deliver a proactive message to a user's channel, resolved by identity.", capability: "channel.push", kind: 'function', insert: "push_user(${1:uuid}, ${2:text})", yields: true },
  { path: "session.exit", signature: "session.exit()", summary: "End the session permanently. The loop never resumes.", kind: 'function', insert: "exit()" },
  { path: "session.state", signature: "session.state", summary: "Durable per-session key/value store (64 KiB values, 256 keys).", kind: 'table' },
  { path: "session.state.set", signature: "session.state.set(key, value)", summary: "Store a JSON-encodable value for this session.", capability: "session.state", kind: 'function', insert: "set(${1:key}, ${2:value})", yields: true },
  { path: "session.state.get", signature: "session.state.get(key) -> value | nil", summary: "Read a value; nil when the key was never set.", capability: "session.state", kind: 'function', insert: "get(${1:key})", yields: true },
  { path: "session.state.delete", signature: "session.state.delete(key)", summary: "Remove a key from the session store.", capability: "session.state", kind: 'function', insert: "delete(${1:key})", yields: true },
  { path: "session.state.list", signature: "session.state.list() -> { key = value }", summary: "Every stored key and value for this session.", capability: "session.state", kind: 'function', insert: "list()", yields: true },

  // log / time
  { path: "log", signature: "log", summary: "Structured logging at the session's level.", kind: 'table' },
  { path: "log.debug", signature: "log.debug(msg)", summary: "Write a debug-level log line.", kind: 'function', insert: "debug(${1:msg})" },
  { path: "log.info", signature: "log.info(msg)", summary: "Write an info-level log line.", kind: 'function', insert: "info(${1:msg})" },
  { path: "log.warn", signature: "log.warn(msg)", summary: "Write a warning-level log line.", kind: 'function', insert: "warn(${1:msg})" },
  { path: "log.error", signature: "log.error(msg)", summary: "Write an error-level log line.", kind: 'function', insert: "error(${1:msg})" },
  { path: "time", signature: "time", summary: "Time helpers.", kind: 'table' },
  { path: "time.sleep", signature: "time.sleep(seconds)", summary: "Park the session for a duration; it is idle, not busy-waiting.", kind: 'function', insert: "sleep(${1:seconds})", yields: true },

  // agent
  { path: "agent", signature: "agent", summary: "Identity, profile and agent-to-agent sends.", kind: 'table' },
  { path: "agent.info", signature: "agent.info() -> { name, session_id, address, model, instructions, memory = { stores = { [name] = { backend, table, features, locality } } }, ... }", summary: "The running agent's identity and effective settings, including per-user overrides and each memory store's locality (deployment, region or instance).", kind: 'function', insert: "info()" },
  { path: "agent.config", signature: "agent.config() -> { name, model, prompts, goal?, extras }", summary: "The deployment profile as data. Secrets appear as {env=...} or {cred=...} markers.", kind: 'function', insert: "config()" },
  { path: "agent.send", signature: "agent.send(address, payload)", summary: "Deliver a message to agent:<name> or session:<id>. Subject to can_contact.", capability: "agent.send", kind: 'function', insert: "send(${1:address}, ${2:payload})", yields: true },
  { path: "agent.request", signature: "agent.request(address, payload, timeout_s?) -> Message", summary: "Send and wait for a reply; the timeout is in seconds.", capability: "agent.request", kind: 'function', insert: "request(${1:address}, ${2:payload})", yields: true },
  { path: "agent.reply", signature: "agent.reply(request_id, payload)", summary: "Answer a request. Use msg.provenance.request_id, never a payload field.", kind: 'function', insert: "reply(${1:request_id}, ${2:payload})", yields: true },
  { path: "agent.list", signature: "agent.list() -> [ { session_id, agent, parent_id } ]", summary: "Visible sessions, scoped to the agent's contacts.", kind: 'function', insert: "list()" },
  { path: "agent.spawn", signature: "agent.spawn(profile, spec?) -> { address, session_id, agent }", summary: "Start a child session from a spawn profile. spec is accepted but ignored.", capability: "agent.spawn", kind: 'function', insert: "spawn(${1:profile})", yields: true },

  // scheduler
  { path: "scheduler", signature: "scheduler", summary: "Session-scoped timers delivered as timer messages.", kind: 'table' },
  { path: "scheduler.every", signature: "scheduler.every(interval_s) -> { id }", summary: "Fire every N seconds.", kind: 'function', insert: "every(${1:interval})", yields: true },
  { path: "scheduler.after", signature: "scheduler.after(delay_s) -> { id }", summary: "Fire once after a delay.", kind: 'function', insert: "after(${1:delay})", yields: true },
  { path: "scheduler.cron", signature: "scheduler.cron(expr) -> { id }", summary: "Cron subset: */N minutes or fixed minute/hour; the rest must be *.", kind: 'function', insert: "cron(${1:expr})", yields: true },
  { path: "scheduler.cancel", signature: "scheduler.cancel(timer_id)", summary: "Cancel a timer. Timers survive hot reload, so cancel before re-registering.", kind: 'function', insert: "cancel(${1:timer_id})", yields: true },

  // llm
  { path: "llm", signature: "llm", summary: "Model calls: chat, streaming, embeddings and reranking.", kind: 'table' },
  { path: "llm.chat", signature: "llm.chat(messages, opts?) -> { text, thinking, usage, tool_calls }", summary: "One model call. opts: model, temperature, max_tokens, thinking, tools, tool_choice.", capability: "llm.chat", kind: 'function', insert: "chat(${1:messages}, ${2:opts})", yields: true },
  { path: "llm.stream", signature: "llm.stream(messages, opts?) -> next_delta, close", summary: "Token stream. Call next_delta() directly; a generic for fails across the bridge. Call close() on early exit.", capability: "llm.chat", kind: 'function', insert: "stream(${1:messages})", yields: true },
  { path: "llm.forward", signature: "llm.forward(opts?) -> { text, tool_calls, finish_reason, usage, thinking, thinking_blocks }", summary: "Forward the current OpenAI-channel request to a model on the metered path. opts: model, messages, prepend, append, tools, params, max_tokens_cap. Does not finish the HTTP response; session.reply does.", capability: "llm.chat", kind: 'function', insert: "forward()", yields: true },
  { path: "llm.embed", signature: "llm.embed(inputs, opts?) -> { vectors, usage }", summary: "Embed a string, an array of strings, or mixed part tables.", capability: "llm.chat", kind: 'function', insert: "embed(${1:inputs})", yields: true },
  { path: "llm.rerank", signature: "llm.rerank(query, documents, opts?) -> { results }", summary: "Rerank documents; result indexes are zero-based.", capability: "llm.chat", kind: 'function', insert: "rerank(${1:query}, ${2:documents})", yields: true },

  // openai
  { path: "openai", signature: "openai", summary: "The request behind the current OpenAI-compatible turn (read-only).", kind: 'table' },
  { path: "openai.request", signature: "openai.request() -> { model, messages, tools, tool_choice, params, metadata, user, stream }", summary: "The full request as data. Refused above 4 MiB of JSON — forward it with llm.forward instead.", capability: "llm.chat", kind: 'function', insert: "request()", yields: true },

  // memory / store
  { path: "memory", signature: "memory", summary: "Personal memory records, scoped by the turn's identity.", kind: 'table' },
  { path: "memory.write", signature: "memory.write(record) -> true", summary: "Write a memory record into the agent's memory profile.", capability: "memory", kind: 'function', insert: "write(${1:record})", yields: true },
  { path: "memory.recall", signature: "memory.recall(query, opts?) -> [ { key, value, meta } ]", summary: "Recall memories. Returns a list, unlike store.query's { records }.", capability: "memory", kind: 'function', insert: "recall(${1:query})", yields: true },
  { path: "store", signature: "store", summary: "Raw scoped stores, for when memory profiles are too narrow.", kind: 'table' },
  { path: "store.put", signature: "store.put(table, key, value, opts?)", summary: "Write a record. opts: ttl seconds, vector embedding.", capability: "memory", kind: 'function', insert: "put(${1:table}, ${2:key}, ${3:value})", yields: true },
  { path: "store.get", signature: "store.get(table, key) -> { value, found }", summary: "Read one record.", capability: "memory", kind: 'function', insert: "get(${1:table}, ${2:key})", yields: true },
  { path: "store.query", signature: "store.query(table, query) -> { records = [ ... ] }", summary: "Query a store; returns { records }, not a bare list.", capability: "memory", kind: 'function', insert: "query(${1:table}, ${2:query})", yields: true },
  { path: "store.delete", signature: "store.delete(table, key)", summary: "Delete one record.", capability: "memory", kind: 'function', insert: "delete(${1:table}, ${2:key})", yields: true },
  { path: "store.scopes", signature: "store.scopes(table) -> [ scope ]", summary: "Enumerate scopes. Maintenance provenance only (system or scheduler turns).", capability: "memory", kind: 'function', insert: "scopes(${1:table})", yields: true, maintenance: true },

  // tools
  { path: "tools", signature: "tools", summary: "Model-visible tools and their dispatcher.", kind: 'table' },
  { path: "tools.list", signature: "tools.list() -> [ tool spec ]", summary: "Tool specs visible to this agent after skills and policy.", capability: "tools", kind: 'function', insert: "list()", yields: true },
  { path: "tools.run", signature: "tools.run(name, args, opts?) -> result", summary: "Run a tool. needs_confirm tools are intercepted and re-invoked with confirmed=true.", capability: "tools", kind: 'function', insert: "run(${1:name}, ${2:args})", yields: true },
  { path: "tool", signature: "tool", summary: "Declaring Lua-implemented tools.", kind: 'table' },
  { path: "tool.def", signature: "tool.def(spec)", summary: "Declare a model-visible tool at chunk top level, before loop().", capability: "tools", kind: 'function', insert: "def({\n\tname = ${1:\"lua:tool\"},\n\tdescription = ${2:\"...\"},\n\tparams = ${3:{ type = \"object\", properties = {} }},\n\thandler = function(args)\n\t\t$0\n\tend,\n})" },

  // shell
  { path: "shell", signature: "shell", summary: "Persistent sandboxed shell handles.", kind: 'table' },
  { path: "shell.spawn", signature: "shell.spawn(opts) -> { id, provider, state }", summary: "Start a docker or ssh sandbox. opts: image, workdir, env, network, limits, project/ref.", capability: "shell.exec", kind: 'function', insert: "spawn({ ${1:provider = \"docker\"} })", yields: true },
  { path: "shell.exec", signature: "shell.exec(id, command) -> { stdout, stderr, exit_code, duration_ms }", summary: "Run a command in a shell handle.", capability: "shell.exec", kind: 'function', insert: "exec(${1:id}, ${2:command})", yields: true },
  { path: "shell.write", signature: "shell.write(id, path, content)", summary: "Write a file into the sandbox.", capability: "shell.exec", kind: 'function', insert: "write(${1:id}, ${2:path}, ${3:content})", yields: true },
  { path: "shell.destroy", signature: "shell.destroy(id)", summary: "Tear down a shell handle.", capability: "shell.exec", kind: 'function', insert: "destroy(${1:id})", yields: true },

  // http / os
  { path: "http", signature: "http", summary: "Outbound HTTP with a private-address guard.", kind: 'table' },
  { path: "http.request", signature: "http.request(opts) -> { ok, status, headers, body, ... }", summary: "Any HTTP status is a completed request; ok means it completed. Needs net.http.", capability: "net.http", kind: 'function', insert: "request({ url = ${1:\"...\"} })", yields: true },
  { path: "http.get", signature: "http.get(url, opts?)", summary: "GET helper.", capability: "net.http", kind: 'function', insert: "get(${1:url})", yields: true },
  { path: "http.post", signature: "http.post(url, body, opts?)", summary: "POST helper.", capability: "net.http", kind: 'function', insert: "post(${1:url}, ${2:body})", yields: true },
  { path: "os", signature: "os", summary: "The AgentFlow os: env, time and date only, all UTC.", kind: 'table' },
  { path: "os.env", signature: "os.env(name) -> string", summary: "Read an engine environment variable; \"\" when unset.", capability: "net.http", kind: 'function', insert: "env(${1:name})", yields: true },
  { path: "os.time", signature: "os.time() -> Unix seconds", summary: "Current UTC time. Takes no arguments.", kind: 'function', insert: "time()" },
  { path: "os.date", signature: "os.date([format,] t?) -> table | string", summary: "UTC date table or strftime text. No local timezone.", kind: 'function', insert: "date(${1:format}, ${2:t})" },

  // mail
  { path: "mail", signature: "mail", summary: "IMAP and SMTP; credentials resolve in Go, never in Lua.", kind: 'table' },
  { path: "mail.imap_fetch", signature: "mail.imap_fetch(opts) -> { ok, mailbox, count, messages }", summary: "Fetch messages. opts: host, port, user, mailbox, unseen, limit, auth.", capability: "net.mail", kind: 'function', insert: "imap_fetch({ ${1:host = \"...\"} })", yields: true },
  { path: "mail.smtp_send", signature: "mail.smtp_send(opts) -> { ok, sent }", summary: "Send a message. opts: host, port, user, from, to, subject, text_body, auth.", capability: "net.mail", kind: 'function', insert: "smtp_send({ ${1:to = {} } })", yields: true },

  // files
  { path: "files", signature: "files", summary: "User-scoped project files with engine-native commits and refs.", kind: 'table' },
  { path: "files.put", signature: "files.put(project, path, text | part, opts?) -> { ok, entry }", summary: "Write a file. Scope is resolved from the turn's identity.", capability: "files", kind: 'function', insert: "put(${1:project}, ${2:path}, ${3:text})", yields: true },
  { path: "files.read", signature: "files.read(project, path) -> { ok, entry }", summary: "Read a file.", capability: "files", kind: 'function', insert: "read(${1:project}, ${2:path})", yields: true },
  { path: "files.list", signature: "files.list(project, prefix?) -> { ok, entries }", summary: "List files under a prefix.", capability: "files", kind: 'function', insert: "list(${1:project})", yields: true },
  { path: "files.delete", signature: "files.delete(project, path) -> { ok = true }", summary: "Delete a file.", capability: "files", kind: 'function', insert: "delete(${1:project}, ${2:path})", yields: true },
  { path: "files.projects", signature: "files.projects() -> { ok, projects }", summary: "List the caller's projects.", capability: "files", kind: 'function', insert: "projects()", yields: true },
  { path: "files.commit", signature: "files.commit(project, opts?) -> { ok, commit }", summary: "Snapshot the tree; opts: ref, message.", capability: "files", kind: 'function', insert: "commit(${1:project})", yields: true },
  { path: "files.checkout", signature: "files.checkout(project, ref) -> { ok, manifest }", summary: "Restore a ref or commit id.", capability: "files", kind: 'function', insert: "checkout(${1:project}, ${2:ref})", yields: true },
  { path: "files.scratch", signature: "files.scratch", summary: "Scratch files shared with HTTP save_to and media handles.", kind: 'table' },
  { path: "files.scratch.put", signature: "files.scratch.put(name, text | part) -> { ok, entry }", summary: "Write a scratch file.", capability: "files", kind: 'function', insert: "put(${1:name}, ${2:text})", yields: true },
  { path: "files.scratch.read", signature: "files.scratch.read(name) -> { ok, entry }", summary: "Read a scratch file.", capability: "files", kind: 'function', insert: "read(${1:name})", yields: true },
  { path: "files.scratch.list", signature: "files.scratch.list() -> { ok, entries }", summary: "List scratch files.", capability: "files", kind: 'function', insert: "list()", yields: true },

  // user / runtime / credential
  { path: "user", signature: "user", summary: "Read-only facts about the person the turn serves.", kind: 'table' },
  { path: "user.current", signature: "user.current() -> { ok, registered, id, display_name, identities }", summary: "The turn's user, or registered=false.", capability: "users", kind: 'function', insert: "current()", yields: true },
  { path: "user.usage", signature: "user.usage() -> { input, output, used, limit, ... }", summary: "Token and quota usage for the user.", capability: "users", kind: 'function', insert: "usage()", yields: true },
  { path: "user.has_credential", signature: "user.has_credential(service) -> { ok, present }", summary: "Whether a stored credential exists; never returns its value.", capability: "users", kind: 'function', insert: "has_credential(${1:service})", yields: true },
  { path: "user.get", signature: "user.get(id) -> profile", summary: "Read one profile. Maintenance provenance only.", capability: "users", kind: 'function', insert: "get(${1:id})", yields: true, maintenance: true },
  { path: "user.list", signature: "user.list() -> [ profile ]", summary: "List profiles. Maintenance provenance only.", capability: "users", kind: 'function', insert: "list()", yields: true, maintenance: true },
  { path: "runtime", signature: "runtime", summary: "Read-only deployment data.", kind: 'table' },
  { path: "runtime.triggers", signature: "runtime.triggers() -> { ok, triggers }", summary: "The merged trigger list snapshotted at boot.", kind: 'function', insert: "triggers()" },
  { path: "credential", signature: "credential", summary: "Per-agent credential access.", kind: 'table' },
  { path: "credential.get", signature: "credential.get(name) -> { ok, name, value }", summary: "Resolve a credential the agent is allow-listed for. Every access is logged.", kind: 'function', insert: "get(${1:name})", yields: true },

  // json / af
  { path: "json", signature: "json", summary: "JSON encoding across the bridge.", kind: 'table' },
  { path: "json.encode", signature: "json.encode(value) -> string", summary: "Encode a value. Empty tables encode as {}.", kind: 'function', insert: "encode(${1:value})" },
  { path: "json.decode", signature: "json.decode(text) -> value", summary: "Decode JSON; raises on malformed input.", kind: 'function', insert: "decode(${1:text})" },
  { path: "af", signature: "af", summary: "Engine internals. Loop code should use the public namespaces instead.", kind: 'table' },
  { path: "af.op", signature: "af.op(op) -> result", summary: "Internal bridge call. Prefer session.*, llm.* and the other public namespaces.", kind: 'function', insert: "op({ type = ${1:\"...\"} })", yields: true },
  { path: "af.version", signature: "af.version", summary: "The prelude version the chunk is running against.", kind: 'variable' },

  // Support chunks, evaluated before the loop in every session.
  { path: "token_budget", signature: "token_budget(messages, budget) -> messages", summary: "Trim a prompt to the agent's history budget. Loaded from the token_budget support chunk.", kind: 'function', insert: "token_budget(${1:messages}, ${2:budget})" },
  { path: "memory_routing_table", signature: "memory_routing_table(record) -> record", summary: "Rewrite a memory record before it is stored. Used by memory.write.", kind: 'function', insert: "memory_routing_table(${1:record})" },
  { path: "memory_recall_handler", signature: "memory_recall_handler(query, opts) -> hits", summary: "Recall hook for recency-based memory. Used by memory.recall.", kind: 'function', insert: "memory_recall_handler(${1:query}, ${2:opts})" },
  { path: "memory_semantic_recall_handler", signature: "memory_semantic_recall_handler(query, opts) -> hits", summary: "Recall hook for semantic memory. Used by memory.recall.", kind: 'function', insert: "memory_semantic_recall_handler(${1:query}, ${2:opts})" },
  { path: "fact_extractor_turn", signature: "fact_extractor_turn(turn) -> facts", summary: "Extract facts after a turn. Called by loops that enable fact extraction.", kind: 'function', insert: "fact_extractor_turn(${1:turn})" },
]

/**
 * The Luau/Lua names a loop may use without declaring them. The analyzer does
 * not validate their fields (the surface is large); it only avoids reporting
 * them as unknown globals.
 */
export const stdlibGlobals: ReadonlySet<string> = new Set([
  'print', 'type', 'typeof', 'tostring', 'tonumber',
  'pairs', 'ipairs', 'next', 'select', 'assert',
  'error', 'pcall', 'xpcall', 'setmetatable',
  'getmetatable', 'rawget', 'rawset', 'rawequal',
  'rawlen', 'unpack', 'collectgarbage',
  'string', 'table', 'math', 'utf8', 'bit32',
  'coroutine', 'buffer', 'vector', '_G',
  '_VERSION',
])

/** Standard library names that are tables rather than functions. */
export const stdlibModules: ReadonlySet<string> = new Set([
  'string', 'table', 'math', 'utf8', 'bit32', 'coroutine', 'buffer', 'vector',
])

/**
 * Libraries the sandbox omits; a targeted message beats "unknown global".
 */
export const unavailableGlobals: ReadonlyMap<string, string> = new Map([
  ['io', 'io is not available in the AgentFlow sandbox; use files.* for storage and log.* for output'],
  ['debug', 'debug is not available in the AgentFlow sandbox'],
  ['require', 'require is not available: every prelude name is already global'],
  ['dofile', 'dofile is not available in the AgentFlow sandbox'],
  ['loadfile', 'loadfile is not available in the AgentFlow sandbox'],
  ['load', 'load or loadstring is not available in the AgentFlow sandbox'],
  ['package', 'package is not available in the AgentFlow sandbox'],
  ['os', 'os is the AgentFlow os: env, time and date only'],
])

const apiIndex = new Map<string, ApiEntry>()
const apiChildren = new Map<string, ApiEntry[]>()
const apiGlobals = new Map<string, ApiEntry>()

for (const e of preludeEntries) {
  apiIndex.set(e.path, e)
  const dot = e.path.lastIndexOf('.')
  const parent = dot < 0 ? '' : e.path.slice(0, dot)
  if (dot < 0) apiGlobals.set(e.path, e)
  const list = apiChildren.get(parent)
  if (list) list.push(e)
  else apiChildren.set(parent, [e])
}
for (const list of apiChildren.values()) {
  list.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0))
}

const noEntries: readonly ApiEntry[] = []

/** Returns the entry for a dotted path, if it is part of the prelude. */
export function lookup(path: string): ApiEntry | undefined {
  return apiIndex.get(path)
}

/** Returns the entries directly under a path ("" for globals), sorted by path. */
export function children(path: string): readonly ApiEntry[] {
  return apiChildren.get(path) ?? noEntries
}

/** Reports whether name is a prelude global. */
export function isGlobalName(name: string): boolean {
  return apiGlobals.has(name)
}

/** The names of the prelude globals. */
export function globalNames(): string[] {
  return [...apiGlobals.keys()]
}

/** Reports whether name resolves without a declaration. */
export function isKnownGlobal(name: string): boolean {
  return stdlibGlobals.has(name) || apiGlobals.has(name)
}

/** The last segment of a dotted path. */
export function leafName(path: string): string {
  return path.slice(path.lastIndexOf('.') + 1)
}

/** Returns the closest known name within two edits, or "". */
export function suggest(name: string, candidates: Iterable<string>): string {
  let best = ''
  let bestDist = 3
  for (const c of candidates) {
    if (c === name) continue
    const d = editDistance(name, c)
    if (d < bestDist) {
      best = c
      bestDist = d
    }
  }
  return best
}

export function editDistance(a: string, b: string): number {
  const ar = Array.from(a)
  const br = Array.from(b)
  if (ar.length === 0) return br.length
  if (br.length === 0) return ar.length
  let prev = new Array<number>(br.length + 1)
  let cur = new Array<number>(br.length + 1)
  for (let j = 0; j <= br.length; j++) prev[j] = j
  for (let i = 1; i <= ar.length; i++) {
    cur[0] = i
    for (let j = 1; j <= br.length; j++) {
      const cost = ar[i - 1] === br[j - 1] ? 0 : 1
      cur[j] = Math.min(cur[j - 1] + 1, prev[j] + 1, prev[j - 1] + cost)
    }
    const swap = prev
    prev = cur
    cur = swap
  }
  return prev[br.length]
}

/** Returns the known immediate field names under a path. */
export function fieldNames(path: string): string[] {
  return children(path).map((e) => leafName(e.path))
}

/** Lists the stdlib globals for the root completion list, sorted. */
export function stdlibCompletion(): string[] {
  return [...stdlibGlobals].sort()
}
