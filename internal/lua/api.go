package lua

import (
	"sort"
	"strings"
)

// SymbolKind is the completion/hover class of a prelude symbol.
type SymbolKind int

const (
	SymbolFunction SymbolKind = iota
	SymbolTable
	SymbolVariable
	SymbolSnippet
)

// APIEntry describes one global or field of the AgentFlow Luau prelude.
type APIEntry struct {
	Path       string
	Signature  string
	Summary    string
	Capability string
	Kind       SymbolKind
	Insert     string
	Yields     bool
	// Maintenance marks calls the engine only permits for system/scheduler
	// turns.
	Maintenance bool
}

// preludeEntries is the documented surface of the prelude (docs/loops-reference.md).
// The runtime's internal/vm/prelude.go is the source of truth: keep this list in
// step with it and with PreludeVersion. The analyzer treats a version mismatch
// as an error, the same as the boot gate.
var preludeEntries = []APIEntry{
	// session
	{Path: "session", Signature: "session", Summary: "The current session: inbox, replies, pushes and durable state.", Kind: SymbolTable},
	{Path: "session.inbox", Signature: "session.inbox() -> Message", Summary: "Wait for the next inbound message. This parks the session.", Kind: SymbolFunction, Insert: "inbox()", Yields: true},
	{Path: "session.send", Signature: "session.send(text, opts?)", Summary: "Reply to the active inbound message. Needs an active message: timer, cron and boot turns have none.", Kind: SymbolFunction, Insert: "send(${1:text})", Yields: true},
	{Path: "session.push", Signature: "session.push(channel, reply_to, text, opts?)", Summary: "Send proactively to a channel. reply_to may be nil.", Capability: "channel.push", Kind: SymbolFunction, Insert: "push(${1:channel}, ${2:reply_to}, ${3:text})", Yields: true},
	{Path: "session.push_user", Signature: "session.push_user(uuid, text)", Summary: "Deliver a proactive message to a user's channel, resolved by identity.", Capability: "channel.push", Kind: SymbolFunction, Insert: "push_user(${1:uuid}, ${2:text})", Yields: true},
	{Path: "session.exit", Signature: "session.exit()", Summary: "End the session permanently. The loop never resumes.", Kind: SymbolFunction, Insert: "exit()"},
	{Path: "session.state", Signature: "session.state", Summary: "Durable per-session key/value store (64 KiB values, 256 keys).", Kind: SymbolTable},
	{Path: "session.state.set", Signature: "session.state.set(key, value)", Summary: "Store a JSON-encodable value for this session.", Capability: "session.state", Kind: SymbolFunction, Insert: "set(${1:key}, ${2:value})", Yields: true},
	{Path: "session.state.get", Signature: "session.state.get(key) -> value | nil", Summary: "Read a value; nil when the key was never set.", Capability: "session.state", Kind: SymbolFunction, Insert: "get(${1:key})", Yields: true},
	{Path: "session.state.delete", Signature: "session.state.delete(key)", Summary: "Remove a key from the session store.", Capability: "session.state", Kind: SymbolFunction, Insert: "delete(${1:key})", Yields: true},
	{Path: "session.state.list", Signature: "session.state.list() -> { key = value }", Summary: "Every stored key and value for this session.", Capability: "session.state", Kind: SymbolFunction, Insert: "list()", Yields: true},

	// log / time
	{Path: "log", Signature: "log", Summary: "Structured logging at the session's level.", Kind: SymbolTable},
	{Path: "log.debug", Signature: "log.debug(msg)", Summary: "Write a debug-level log line.", Kind: SymbolFunction, Insert: "debug(${1:msg})"},
	{Path: "log.info", Signature: "log.info(msg)", Summary: "Write an info-level log line.", Kind: SymbolFunction, Insert: "info(${1:msg})"},
	{Path: "log.warn", Signature: "log.warn(msg)", Summary: "Write a warning-level log line.", Kind: SymbolFunction, Insert: "warn(${1:msg})"},
	{Path: "log.error", Signature: "log.error(msg)", Summary: "Write an error-level log line.", Kind: SymbolFunction, Insert: "error(${1:msg})"},
	{Path: "time", Signature: "time", Summary: "Time helpers.", Kind: SymbolTable},
	{Path: "time.sleep", Signature: "time.sleep(seconds)", Summary: "Park the session for a duration; it is idle, not busy-waiting.", Kind: SymbolFunction, Insert: "sleep(${1:seconds})", Yields: true},

	// agent
	{Path: "agent", Signature: "agent", Summary: "Identity, profile and agent-to-agent sends.", Kind: SymbolTable},
	{Path: "agent.info", Signature: "agent.info() -> { name, session_id, address, model, instructions, ... }", Summary: "The running agent's identity and effective settings, including per-user overrides.", Kind: SymbolFunction, Insert: "info()"},
	{Path: "agent.config", Signature: "agent.config() -> { name, model, prompts, goal?, extras }", Summary: "The deployment profile as data. Secrets appear as {env=...} or {cred=...} markers.", Kind: SymbolFunction, Insert: "config()"},
	{Path: "agent.send", Signature: "agent.send(address, payload)", Summary: "Deliver a message to agent:<name> or session:<id>. Subject to can_contact.", Capability: "agent.send", Kind: SymbolFunction, Insert: "send(${1:address}, ${2:payload})", Yields: true},
	{Path: "agent.request", Signature: "agent.request(address, payload, timeout_s?) -> Message", Summary: "Send and wait for a reply; the timeout is in seconds.", Capability: "agent.request", Kind: SymbolFunction, Insert: "request(${1:address}, ${2:payload})", Yields: true},
	{Path: "agent.reply", Signature: "agent.reply(request_id, payload)", Summary: "Answer a request. Use msg.provenance.request_id, never a payload field.", Kind: SymbolFunction, Insert: "reply(${1:request_id}, ${2:payload})", Yields: true},
	{Path: "agent.list", Signature: "agent.list() -> [ { session_id, agent, parent_id } ]", Summary: "Visible sessions, scoped to the agent's contacts.", Kind: SymbolFunction, Insert: "list()"},
	{Path: "agent.spawn", Signature: "agent.spawn(profile, spec?) -> { address, session_id, agent }", Summary: "Start a child session from a spawn profile. spec is accepted but ignored.", Capability: "agent.spawn", Kind: SymbolFunction, Insert: "spawn(${1:profile})", Yields: true},

	// scheduler
	{Path: "scheduler", Signature: "scheduler", Summary: "Session-scoped timers delivered as timer messages.", Kind: SymbolTable},
	{Path: "scheduler.every", Signature: "scheduler.every(interval_s) -> { id }", Summary: "Fire every N seconds.", Kind: SymbolFunction, Insert: "every(${1:interval})", Yields: true},
	{Path: "scheduler.after", Signature: "scheduler.after(delay_s) -> { id }", Summary: "Fire once after a delay.", Kind: SymbolFunction, Insert: "after(${1:delay})", Yields: true},
	{Path: "scheduler.cron", Signature: "scheduler.cron(expr) -> { id }", Summary: "Cron subset: */N minutes or fixed minute/hour; the rest must be *.", Kind: SymbolFunction, Insert: "cron(${1:expr})", Yields: true},
	{Path: "scheduler.cancel", Signature: "scheduler.cancel(timer_id)", Summary: "Cancel a timer. Timers survive hot reload, so cancel before re-registering.", Kind: SymbolFunction, Insert: "cancel(${1:timer_id})", Yields: true},

	// llm
	{Path: "llm", Signature: "llm", Summary: "Model calls: chat, streaming, embeddings and reranking.", Kind: SymbolTable},
	{Path: "llm.chat", Signature: "llm.chat(messages, opts?) -> { text, thinking, usage, tool_calls }", Summary: "One model call. opts: model, temperature, max_tokens, thinking, tools, tool_choice.", Capability: "llm.chat", Kind: SymbolFunction, Insert: "chat(${1:messages}, ${2:opts})", Yields: true},
	{Path: "llm.stream", Signature: "llm.stream(messages, opts?) -> next_delta, close", Summary: "Token stream. Call next_delta() directly; a generic for fails across the bridge. Call close() on early exit.", Capability: "llm.chat", Kind: SymbolFunction, Insert: "stream(${1:messages})", Yields: true},
	{Path: "llm.embed", Signature: "llm.embed(inputs, opts?) -> { vectors, usage }", Summary: "Embed a string, an array of strings, or mixed part tables.", Capability: "llm.chat", Kind: SymbolFunction, Insert: "embed(${1:inputs})", Yields: true},
	{Path: "llm.rerank", Signature: "llm.rerank(query, documents, opts?) -> { results }", Summary: "Rerank documents; result indexes are zero-based.", Capability: "llm.chat", Kind: SymbolFunction, Insert: "rerank(${1:query}, ${2:documents})", Yields: true},

	// memory / store
	{Path: "memory", Signature: "memory", Summary: "Personal memory records, scoped by the turn's identity.", Kind: SymbolTable},
	{Path: "memory.write", Signature: "memory.write(record) -> true", Summary: "Write a memory record into the agent's memory profile.", Capability: "memory", Kind: SymbolFunction, Insert: "write(${1:record})", Yields: true},
	{Path: "memory.recall", Signature: "memory.recall(query, opts?) -> [ { key, value, meta } ]", Summary: "Recall memories. Returns a list, unlike store.query's { records }.", Capability: "memory", Kind: SymbolFunction, Insert: "recall(${1:query})", Yields: true},
	{Path: "store", Signature: "store", Summary: "Raw scoped stores, for when memory profiles are too narrow.", Kind: SymbolTable},
	{Path: "store.put", Signature: "store.put(table, key, value, opts?)", Summary: "Write a record. opts: ttl seconds, vector embedding.", Capability: "memory", Kind: SymbolFunction, Insert: "put(${1:table}, ${2:key}, ${3:value})", Yields: true},
	{Path: "store.get", Signature: "store.get(table, key) -> { value, found }", Summary: "Read one record.", Capability: "memory", Kind: SymbolFunction, Insert: "get(${1:table}, ${2:key})", Yields: true},
	{Path: "store.query", Signature: "store.query(table, query) -> { records = [ ... ] }", Summary: "Query a store; returns { records }, not a bare list.", Capability: "memory", Kind: SymbolFunction, Insert: "query(${1:table}, ${2:query})", Yields: true},
	{Path: "store.delete", Signature: "store.delete(table, key)", Summary: "Delete one record.", Capability: "memory", Kind: SymbolFunction, Insert: "delete(${1:table}, ${2:key})", Yields: true},
	{Path: "store.scopes", Signature: "store.scopes(table) -> [ scope ]", Summary: "Enumerate scopes. Maintenance provenance only (system or scheduler turns).", Capability: "memory", Kind: SymbolFunction, Insert: "scopes(${1:table})", Yields: true, Maintenance: true},

	// tools
	{Path: "tools", Signature: "tools", Summary: "Model-visible tools and their dispatcher.", Kind: SymbolTable},
	{Path: "tools.list", Signature: "tools.list() -> [ tool spec ]", Summary: "Tool specs visible to this agent after skills and policy.", Capability: "tools", Kind: SymbolFunction, Insert: "list()", Yields: true},
	{Path: "tools.run", Signature: "tools.run(name, args, opts?) -> result", Summary: "Run a tool. needs_confirm tools are intercepted and re-invoked with confirmed=true.", Capability: "tools", Kind: SymbolFunction, Insert: "run(${1:name}, ${2:args})", Yields: true},
	{Path: "tool", Signature: "tool", Summary: "Declaring Lua-implemented tools.", Kind: SymbolTable},
	{Path: "tool.def", Signature: "tool.def(spec)", Summary: "Declare a model-visible tool at chunk top level, before loop().", Capability: "tools", Kind: SymbolFunction, Insert: "def({\n\tname = ${1:\"lua:tool\"},\n\tdescription = ${2:\"...\"},\n\tparams = ${3:{ type = \"object\", properties = {} }},\n\thandler = function(args)\n\t\t$0\n\tend,\n})"},

	// shell
	{Path: "shell", Signature: "shell", Summary: "Persistent sandboxed shell handles.", Kind: SymbolTable},
	{Path: "shell.spawn", Signature: "shell.spawn(opts) -> { id, provider, state }", Summary: "Start a docker or ssh sandbox. opts: image, workdir, env, network, limits, project/ref.", Capability: "shell.exec", Kind: SymbolFunction, Insert: "spawn({ ${1:provider = \"docker\"} })", Yields: true},
	{Path: "shell.exec", Signature: "shell.exec(id, command) -> { stdout, stderr, exit_code, duration_ms }", Summary: "Run a command in a shell handle.", Capability: "shell.exec", Kind: SymbolFunction, Insert: "exec(${1:id}, ${2:command})", Yields: true},
	{Path: "shell.write", Signature: "shell.write(id, path, content)", Summary: "Write a file into the sandbox.", Capability: "shell.exec", Kind: SymbolFunction, Insert: "write(${1:id}, ${2:path}, ${3:content})", Yields: true},
	{Path: "shell.destroy", Signature: "shell.destroy(id)", Summary: "Tear down a shell handle.", Capability: "shell.exec", Kind: SymbolFunction, Insert: "destroy(${1:id})", Yields: true},

	// http / os
	{Path: "http", Signature: "http", Summary: "Outbound HTTP with a private-address guard.", Kind: SymbolTable},
	{Path: "http.request", Signature: "http.request(opts) -> { ok, status, headers, body, ... }", Summary: "Any HTTP status is a completed request; ok means it completed. Needs net.http.", Capability: "net.http", Kind: SymbolFunction, Insert: "request({ url = ${1:\"...\"} })", Yields: true},
	{Path: "http.get", Signature: "http.get(url, opts?)", Summary: "GET helper.", Capability: "net.http", Kind: SymbolFunction, Insert: "get(${1:url})", Yields: true},
	{Path: "http.post", Signature: "http.post(url, body, opts?)", Summary: "POST helper.", Capability: "net.http", Kind: SymbolFunction, Insert: "post(${1:url}, ${2:body})", Yields: true},
	{Path: "os", Signature: "os", Summary: "The AgentFlow os: env, time and date only, all UTC.", Kind: SymbolTable},
	{Path: "os.env", Signature: "os.env(name) -> string", Summary: "Read an engine environment variable; \"\" when unset.", Capability: "net.http", Kind: SymbolFunction, Insert: "env(${1:name})", Yields: true},
	{Path: "os.time", Signature: "os.time() -> Unix seconds", Summary: "Current UTC time. Takes no arguments.", Kind: SymbolFunction, Insert: "time()"},
	{Path: "os.date", Signature: "os.date([format,] t?) -> table | string", Summary: "UTC date table or strftime text. No local timezone.", Kind: SymbolFunction, Insert: "date(${1:format}, ${2:t})"},

	// mail
	{Path: "mail", Signature: "mail", Summary: "IMAP and SMTP; credentials resolve in Go, never in Lua.", Kind: SymbolTable},
	{Path: "mail.imap_fetch", Signature: "mail.imap_fetch(opts) -> { ok, mailbox, count, messages }", Summary: "Fetch messages. opts: host, port, user, mailbox, unseen, limit, auth.", Capability: "net.mail", Kind: SymbolFunction, Insert: "imap_fetch({ ${1:host = \"...\"} })", Yields: true},
	{Path: "mail.smtp_send", Signature: "mail.smtp_send(opts) -> { ok, sent }", Summary: "Send a message. opts: host, port, user, from, to, subject, text_body, auth.", Capability: "net.mail", Kind: SymbolFunction, Insert: "smtp_send({ ${1:to = {} } })", Yields: true},

	// files
	{Path: "files", Signature: "files", Summary: "User-scoped project files with engine-native commits and refs.", Kind: SymbolTable},
	{Path: "files.put", Signature: "files.put(project, path, text | part, opts?) -> { ok, entry }", Summary: "Write a file. Scope is resolved from the turn's identity.", Capability: "files", Kind: SymbolFunction, Insert: "put(${1:project}, ${2:path}, ${3:text})", Yields: true},
	{Path: "files.read", Signature: "files.read(project, path) -> { ok, entry }", Summary: "Read a file.", Capability: "files", Kind: SymbolFunction, Insert: "read(${1:project}, ${2:path})", Yields: true},
	{Path: "files.list", Signature: "files.list(project, prefix?) -> { ok, entries }", Summary: "List files under a prefix.", Capability: "files", Kind: SymbolFunction, Insert: "list(${1:project})", Yields: true},
	{Path: "files.delete", Signature: "files.delete(project, path) -> { ok = true }", Summary: "Delete a file.", Capability: "files", Kind: SymbolFunction, Insert: "delete(${1:project}, ${2:path})", Yields: true},
	{Path: "files.projects", Signature: "files.projects() -> { ok, projects }", Summary: "List the caller's projects.", Capability: "files", Kind: SymbolFunction, Insert: "projects()", Yields: true},
	{Path: "files.commit", Signature: "files.commit(project, opts?) -> { ok, commit }", Summary: "Snapshot the tree; opts: ref, message.", Capability: "files", Kind: SymbolFunction, Insert: "commit(${1:project})", Yields: true},
	{Path: "files.checkout", Signature: "files.checkout(project, ref) -> { ok, manifest }", Summary: "Restore a ref or commit id.", Capability: "files", Kind: SymbolFunction, Insert: "checkout(${1:project}, ${2:ref})", Yields: true},
	{Path: "files.scratch", Signature: "files.scratch", Summary: "Scratch files shared with HTTP save_to and media handles.", Kind: SymbolTable},
	{Path: "files.scratch.put", Signature: "files.scratch.put(name, text | part) -> { ok, entry }", Summary: "Write a scratch file.", Capability: "files", Kind: SymbolFunction, Insert: "put(${1:name}, ${2:text})", Yields: true},
	{Path: "files.scratch.read", Signature: "files.scratch.read(name) -> { ok, entry }", Summary: "Read a scratch file.", Capability: "files", Kind: SymbolFunction, Insert: "read(${1:name})", Yields: true},
	{Path: "files.scratch.list", Signature: "files.scratch.list() -> { ok, entries }", Summary: "List scratch files.", Capability: "files", Kind: SymbolFunction, Insert: "list()", Yields: true},

	// user / runtime / credential
	{Path: "user", Signature: "user", Summary: "Read-only facts about the person the turn serves.", Kind: SymbolTable},
	{Path: "user.current", Signature: "user.current() -> { ok, registered, id, display_name, identities }", Summary: "The turn's user, or registered=false.", Capability: "users", Kind: SymbolFunction, Insert: "current()", Yields: true},
	{Path: "user.usage", Signature: "user.usage() -> { input, output, used, limit, ... }", Summary: "Token and quota usage for the user.", Capability: "users", Kind: SymbolFunction, Insert: "usage()", Yields: true},
	{Path: "user.has_credential", Signature: "user.has_credential(service) -> { ok, present }", Summary: "Whether a stored credential exists; never returns its value.", Capability: "users", Kind: SymbolFunction, Insert: "has_credential(${1:service})", Yields: true},
	{Path: "user.get", Signature: "user.get(id) -> profile", Summary: "Read one profile. Maintenance provenance only.", Capability: "users", Kind: SymbolFunction, Insert: "get(${1:id})", Yields: true, Maintenance: true},
	{Path: "user.list", Signature: "user.list() -> [ profile ]", Summary: "List profiles. Maintenance provenance only.", Capability: "users", Kind: SymbolFunction, Insert: "list()", Yields: true, Maintenance: true},
	{Path: "runtime", Signature: "runtime", Summary: "Read-only deployment data.", Kind: SymbolTable},
	{Path: "runtime.triggers", Signature: "runtime.triggers() -> { ok, triggers }", Summary: "The merged trigger list snapshotted at boot.", Kind: SymbolFunction, Insert: "triggers()"},
	{Path: "credential", Signature: "credential", Summary: "Per-agent credential access.", Kind: SymbolTable},
	{Path: "credential.get", Signature: "credential.get(name) -> { ok, name, value }", Summary: "Resolve a credential the agent is allow-listed for. Every access is logged.", Kind: SymbolFunction, Insert: "get(${1:name})", Yields: true},

	// json / af
	{Path: "json", Signature: "json", Summary: "JSON encoding across the bridge.", Kind: SymbolTable},
	{Path: "json.encode", Signature: "json.encode(value) -> string", Summary: "Encode a value. Empty tables encode as {}.", Kind: SymbolFunction, Insert: "encode(${1:value})"},
	{Path: "json.decode", Signature: "json.decode(text) -> value", Summary: "Decode JSON; raises on malformed input.", Kind: SymbolFunction, Insert: "decode(${1:text})"},
	{Path: "af", Signature: "af", Summary: "Engine internals. Loop code should use the public namespaces instead.", Kind: SymbolTable},
	{Path: "af.op", Signature: "af.op(op) -> result", Summary: "Internal bridge call. Prefer session.*, llm.* and the other public namespaces.", Kind: SymbolFunction, Insert: "op({ type = ${1:\"...\"} })", Yields: true},
	{Path: "af.version", Signature: "af.version", Summary: "The prelude version the chunk is running against.", Kind: SymbolVariable},

	// Support chunks, evaluated before the loop in every session.
	{Path: "token_budget", Signature: "token_budget(messages, budget) -> messages", Summary: "Trim a prompt to the agent's history budget. Loaded from the token_budget support chunk.", Kind: SymbolFunction, Insert: "token_budget(${1:messages}, ${2:budget})"},
	{Path: "memory_routing_table", Signature: "memory_routing_table(record) -> record", Summary: "Rewrite a memory record before it is stored. Used by memory.write.", Kind: SymbolFunction, Insert: "memory_routing_table(${1:record})"},
	{Path: "memory_recall_handler", Signature: "memory_recall_handler(query, opts) -> hits", Summary: "Recall hook for recency-based memory. Used by memory.recall.", Kind: SymbolFunction, Insert: "memory_recall_handler(${1:query}, ${2:opts})"},
	{Path: "memory_semantic_recall_handler", Signature: "memory_semantic_recall_handler(query, opts) -> hits", Summary: "Recall hook for semantic memory. Used by memory.recall.", Kind: SymbolFunction, Insert: "memory_semantic_recall_handler(${1:query}, ${2:opts})"},
	{Path: "fact_extractor_turn", Signature: "fact_extractor_turn(turn) -> facts", Summary: "Extract facts after a turn. Called by loops that enable fact extraction.", Kind: SymbolFunction, Insert: "fact_extractor_turn(${1:turn})"},
}

// stdlibGlobals are the Luau/Lua names a loop may use without declaring them.
// The analyzer does not validate their fields (the surface is large); it only
// avoids reporting them as unknown globals.
var stdlibGlobals = map[string]bool{
	"print": true, "type": true, "typeof": true, "tostring": true, "tonumber": true,
	"pairs": true, "ipairs": true, "next": true, "select": true, "assert": true,
	"error": true, "pcall": true, "xpcall": true, "setmetatable": true,
	"getmetatable": true, "rawget": true, "rawset": true, "rawequal": true,
	"rawlen": true, "unpack": true, "collectgarbage": true,
	"string": true, "table": true, "math": true, "utf8": true, "bit32": true,
	"coroutine": true, "buffer": true, "vector": true, "_G": true,
	"_VERSION": true,
}

// unavailableGlobals are libraries the sandbox omits; a targeted message beats
// "unknown global".
var unavailableGlobals = map[string]string{
	"io":       "io is not available in the AgentFlow sandbox; use files.* for storage and log.* for output",
	"debug":    "debug is not available in the AgentFlow sandbox",
	"require":  "require is not available: every prelude name is already global",
	"dofile":   "dofile is not available in the AgentFlow sandbox",
	"loadfile": "loadfile is not available in the AgentFlow sandbox",
	"load":     "load or loadstring is not available in the AgentFlow sandbox",
	"package":  "package is not available in the AgentFlow sandbox",
	"os":       "os is the AgentFlow os: env, time and date only",
}

var (
	apiIndex    = map[string]*APIEntry{}
	apiChildren = map[string][]*APIEntry{}
	apiGlobals  = map[string]*APIEntry{}
)

func init() {
	for i := range preludeEntries {
		e := &preludeEntries[i]
		apiIndex[e.Path] = e
		if !strings.Contains(e.Path, ".") {
			apiGlobals[e.Path] = e
			apiChildren[""] = append(apiChildren[""], e)
			continue
		}
		parent := e.Path[:strings.LastIndex(e.Path, ".")]
		apiChildren[parent] = append(apiChildren[parent], e)
	}
	for _, list := range apiChildren {
		sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	}
}

// Lookup returns the entry for a dotted path, if it is part of the prelude.
func Lookup(path string) *APIEntry { return apiIndex[path] }

// Children returns the entries directly under a path ("" for globals).
func Children(path string) []*APIEntry { return apiChildren[path] }

// IsGlobalName reports whether name is a prelude namespace.
func IsGlobalName(name string) bool { return apiGlobals[name] != nil }

// IsKnownGlobal reports whether name resolves without a declaration.
func IsKnownGlobal(name string) bool {
	return stdlibGlobals[name] || apiGlobals[name] != nil
}

// Suggest returns the closest known name within two edits, or "".
func Suggest(name string, candidates []string) string {
	best, bestDist := "", 3
	for _, c := range candidates {
		if c == name {
			continue
		}
		if d := editDistance(name, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// FieldNames returns the known immediate field names under a path.
func FieldNames(path string) []string {
	out := make([]string, 0, len(apiChildren[path]))
	for _, e := range apiChildren[path] {
		out = append(out, e.Path[strings.LastIndex(e.Path, ".")+1:])
	}
	return out
}

// StdlibCompletion lists common stdlib globals for the root completion list.
func StdlibCompletion() []string {
	names := make([]string, 0, len(stdlibGlobals))
	for n := range stdlibGlobals {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
