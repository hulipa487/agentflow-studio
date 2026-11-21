## Architecture

A request moves down through a short stack. A channel driver turns platform bytes into a `session.Message`. The optional identity layer resolves who sent it, and the router's Lua handler picks a session key. The supervisor finds or creates the session actor, which runs one Luau loop. The loop calls capability-backed ops through the async bridge, and drivers reach external systems. The core owns the safety filters, the audit journal and provenance. They sit on the path itself, so a loop cannot remove or forge them.

```
┌─ Channels (bytes in / replies out) ─────────────────────────────────────────┐
│  webhook (sync | async)   ghhook (GitHub HMAC)   bridge (HMAC adapter)       │
│  telegram: polling | webhook | auto     qq: websocket gateway | webhook       │
│  studio (synthetic, playground only)                                          │
│  media: per-channel allow-list → blob store (fs | s3)   ── one shared httpd ─ │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ session.Message{id, from, text, channel, reply_to, attachments}
            ▼
┌─ Identity sink (runtime.identity.enabled; optional) ────────────────────────┐
│  handle → user:<uuid> · /link codes consumed here · unregistered allow|reject │
└───────────┬──────────────────────────────────────────────────────────────────┘
            ▼
┌─ Router (one Luau service state per process) ───────────────────────────────┐
│  audit journal (inbound) ──► gateway.route handler (default plugin:per_chat)  │
│  computes the session key, may propose a group; router.state.* is durable     │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ deliver{agent, key, message, identity_group?, kind?}
            ▼
┌─ Supervisor (session authority) ────────────────────────────────────────────┐
│  one actor per (agent, key), spawned on first message · provenance stamped    │
│  group membership validated · per-tenant session cap · can_contact on hops    │
│  sessionhub: lease + shared inbox route to the owning instance in a fleet     │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ one mailbox per session
            ▼
┌─ Actor (1 goroutine + 1 Luau state per session) ────────────────────────────┐
│  session.inbox() ──► safety ingress ──► loop() continues                      │
│  llm.* / memory.* / tools.run / shell.* / http.* / files.* / agent.* / …      │
│  __af_op(json) yields ── inline op, or worker pool (fair-share per tenant)    │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ op handlers, gated per agent by `capabilities`
            ▼
┌─ Capabilities & Drivers ────────────────────────────────────────────────────┐
│  llm:    anthropic · openai · openai-responses · gemini · rerank (base_url)   │
│  memory: sqlite · redis · postgres · volatile · pgvector · qdrant · redisvector│
│  tools:  builtin:* · tools.http (declarative) · MCP (stdio | Streamable HTTP) │
│  shell:  sandbox (gVisor · microsandbox) · ssh   mail: imap+smtp   files: fs│
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ session.send / session.push / session.push_user
            ▼
┌─ Egress (actor-side, core-owned) ───────────────────────────────────────────┐
│  safety egress ──► audit journal (outbound) ──► gateway.Send ──► channel      │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

### Functional flow: one user turn

```
user ─► [channel driver]
          │ normalize → session.Message{from, text, attachments}
          ▼
       identity sink (optional) ─► router: journal "routed" ─► gateway.route (Lua)
          │                                          (per_chat → session key)
          ▼
       supervisor.Deliver ◄──────────────────────────┘
          │ stamp provenance (kind, principal, user_uuid), enqueue mailbox
          ▼
       actor mailbox ◄──── scheduler.every / after / cron ── (session timers)
                     ◄──── triggers: every: / cron: ──────── (engine-fired messages)
          ▼
       session.inbox() returns ◄── safety ingress ran here (drop | skip | rewrite)
          │  ├─ memory.recall(...)            ── async bridge ──► store backend
          │  ├─ llm.chat(messages, {tools})   ── async bridge ──► llm driver
          │  ├─ tools.run(name, args)         ── async bridge ──► builtin / http / mcp
          │  └─ memory.write(record)          ── async bridge ──► store backend
          ▼
       session.send(reply)
          ▼
       safety egress ─► journal "delivered" ─► gateway.Send ─► channel ─► user
```

A loop decides when and how to call tools. There is no `on_tool_call` hook: a loop reads `reply.tool_calls` from `llm.chat` and calls `tools.run` itself (see `examples/loops/tool_loop.lua`). A `tools.run` that answers `needs_confirm` parks the coroutine while the actor asks the user, then resumes it.

---

### Multi-agent orchestration

Static agents (each with its own model + instructions) collaborate via `agent.send`/`agent.request` under `can_contact` ACLs; ephemeral children are spawned from `profiles.agent` templates with capability attenuation. A reference orchestrator (main + expert + project manager + workers) is a common pattern built on these primitives — it lives outside this runtime, in a separate product repo:

```
chit-chat:      user ──telegram/webhook──► main ──llm──► user

expert consult: user ──► main ──[directive: consult]──► expert_code (agent.request)
                                    ◄── opinion ── (agent.reply)
                          main ──llm (fold opinion)──► user

project launch: user ──► main ──[directive: start_project]──► agent.spawn("project_manager")
                          main ─► PM: brief{goal, channel, reply_to, chat_key}
                          PM ── decompose → store tasks ── spawn N workers
                          PM ──► worker: task      worker ──► PM: report
                          PM ──► main: {kind: milestone|decision|notify, ...}
                          main ──session.push(channel, reply_to, text)──► user  (idle-time push)
                          PM ── shutdown workers ── session.exit()
```

---

`session.push` (cap `channel.push`) is proactive egress — a background agent reaches the user without an active inbound message. `session.exit` terminates a spawned session cleanly (the parent receives `agent.died`). The orchestrator wiring and role prompts ship with the reference product repo, not this runtime.

### Layer responsibilities

| Layer | Owns | Description |
| --- | --- | --- |
| channels | Transport | Normalize raw platform bytes into the shared `session.Message` and deliver replies back out. `studio` is a synthetic channel registered for the playground. |
| identity | Who | Optional. Resolves a channel handle to `user:<uuid>`, consumes `/link` codes, and refuses unregistered senders when `runtime.users.unregistered: reject`. |
| router | Routing | Journals the inbound, then runs the `gateway.route` Lua handler in one service state per process to compute a session key (and, optionally, propose a group identity). Hot-reloadable. |
| supervisor | Authority | Session lookup and spawn, provenance stamping, group-membership validation, per-tenant session caps, `can_contact` on agent hops, spawn/request/reply, fleet hand-off through the session hub. |
| actor | Execution | One goroutine + one Luau state per session. Drives the loop, parks on `__af_op`, resumes on results, runs safety ingress at `session.inbox()` and safety egress on every send. |
| capabilities | Ops | Lua-facing tables (`llm`, `memory`, `store`, `tools`, `shell`, `http`, `mail`, `files`, `agent`, `scheduler`, `session.state`, `user`, `credential`) over the single async-bridge primitive. Each agent gets only the handlers its `capabilities` grant. |
| drivers | I/O | Reach real systems: LLM providers, storage and vector backends, shell providers, MCP servers, search engines, the browser, mail, S3. |
| safety | Guard | Core-owned filter chain (`profiles.safety`, model-backed `safety.classifiers`) that runs inside the actor on ingress and egress regardless of the loop. Cannot be uninstalled from Lua. |
| admin plane | Operators | `/healthz`, `/readyz`, `/metrics`, the `/admin/api/*` JSON API used by Studio and the browser console, over TCP and/or a local socket or pipe. |

## Deployment: two tiers

The engine runs either as one process or as a regional cell. The tier is
explicit — `runtime.topology` — and it decides which backends are valid. Boot
refuses a setting the tier cannot honour rather than accepting it and silently
keeping a per-instance copy of the data. An unset key means `single`, so every
configuration written before the setting existed boots unchanged.

### `single` — one instance

`runtime.persistence: sqlite://./data/agentflow.db` (or nothing at all) gives
you the journal, the file store's metadata, the token ledger, the identity
registry, the credential store, the lease table, the session inbox and the shell
handle records, each in a local file beside the runtime database. Nothing to
install, nothing to reach over a network, and every guarantee the engine makes
about a single instance holds: the audit trail is complete, quotas are enforced
from a ledger that survives a restart, and scheduled work fires.

Two shapes are supported:

- **Single-database mode** — SQLite for everything (vectors included, from the
  SQLite vector support), or PostgreSQL with pgvector.
- **Split mode** — SQLite or PostgreSQL for conversation state, plus Qdrant or
  redisvector for vectors.

Blobs may be a local directory or S3. Redis and volatile memory backends are
instance-local by definition (the instance is the deployment). A log-plane
directory is allowed, and is exactly one process's log. With a self-hosted model
(Ollama, vLLM, llama.cpp behind an OpenAI-compatible endpoint) this tier needs no
third-party service at all.

A `single` deployment refuses a second live process on the same store. The
engine keeps an instance registry — one heartbeat row per process in the runtime
store — and the second process stops at boot with the peer's id, host and last
seen. That is the crashed-peer case included: wait out the 30-second liveness
window, or start once with `-force-single`.

### `regional` — several instances in one region

Point `runtime.persistence` at a PostgreSQL DSN and set the tier:

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require

media: { backend: s3, s3: { bucket: agentflow-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-files, region: eu-west-1 } }
```

The tier's requirements are enforced by `agentflow validate`, so every instance
refuses the same misconfiguration at the same point:

| Setting | Regional requirement | Why |
| --- | --- | --- |
| `runtime.persistence` | PostgreSQL | One store every instance can see. |
| `runtime.identity.persistence`, `runtime.credentials.path` | Unset (follow the runtime store), or PostgreSQL. SQLite is refused | A shared identity registry and credential store; two instances must resolve one handle to one user. |
| `runtime.cluster.persistence` | Deprecated. Unset, or the same target as `runtime.persistence` | Leases and the session inbox always live in the runtime store now; a different target in a regional cell is refused. |
| `memory.backends` | No `sqlite` provider | Every instance would keep its own conversation history. Use `postgres`, `pgvector`, `qdrant`, `redis` or `redisvector`. |
| `media.backend`, `files.backend` | `s3` | Every instance would keep its own blobs on a local directory. |
| `runtime.log_plane.persistence` | Unset | A log-plane directory belongs to one process; the cell's journal must be shared. |
| `ADMIN_TOKEN` or `runtime.admin.operators` | At least one set | A per-boot admin token is unknown to every peer, so the boot refuses to mint one. Named operators may replace the shared token entirely. |

Every instance registers in the same runtime store with a heartbeat every 10
seconds and is live while its last heartbeat is within 30 seconds.
`/admin/api/state` reports `topology`, `zone` and every live `instances` row,
flagging a peer whose `config_epoch` differs; Studio's Overview shows the same
list. The heartbeat is a liveness label, not a lock — the lease table remains
the authority for who owns work.

### Multi-region

There is no global tier. A multi-region deployment is one independent
`regional` cell per region, and cells share nothing at runtime. The application
in front pins each user to a home region and sends sign-up, invites, credential
setup and traffic there; each region is its own failure domain, has no
cross-region latency on the hot path, and keeps its data in its region. A shared
chat integration (a Telegram bot has one receiver) belongs to that front door,
which forwards to the user's region through the `bridge` channel. Admin and
billing fan out per region, and disaster recovery is a standby PostgreSQL
promoted to stand up a replacement cell. The full pattern — including the
bridge, per-region billing and the DR runbook — is in
[Multi-region](multi-region.md).

A target is a scheme and an address: `postgres://…` or `postgresql://…` for a
server, `sqlite://<path>` or a bare path for a local file, and for the log plane
a directory — `./data/log` or `file://./data/log`. Every target is parsed at
boot in one place, so a scheme the engine has no backend for is a boot error
naming the schemes it does have: a typo cannot quietly become a directory named
after it.

### The log plane

The engine's state has three shapes, and the shape is what decides where it
belongs:

- **Read on the hot path to make a decision** — the token rollups a quota check
  and a budget refresh read before every call, the file-store metadata, session
  state, route state, shell handles. These stay in the transactional store,
  where a sum is exact and immediately visible to every instance.
- **Written constantly, read rarely, expired by age** — the message journal and
  the per-call usage detail. That is the log plane, and it is the part a
  deployment can put somewhere else: `runtime.log_plane.persistence: ./data/log`
  writes both as newline-delimited JSON, one file per UTC day, and prunes by
  deleting whole days.

Nothing above the store changes: the audit trail an operator reads and the
per-call detail on a profile's page come from wherever the plane points, and
`usage_daily` — the rollup, which must not move — keeps every quota and budget
decision exact. Three limits are worth knowing before pointing it at a
directory. A machine crash can lose the last records written, because they are
not fsynced (the journal is best-effort at both call sites already, so this
weakens nothing that was promised). The directory belongs to one process. And a
journal on local files is *per instance*, which fragments exactly what an audit
trail is for — a regional cell keeps the journal in the shared store, and the
validator refuses a log-plane directory there. Retention stays per plane:
`audit.retention_days` prunes the journal wherever it lives and
`usage.retention_days` the detail, each reporting what it deleted.

### What multi-active guarantees

| Guarantee | Mechanism | Limit worth knowing |
| --- | --- | --- |
| One conversation runs in one place | A `session:<agent\|key>` lease; an instance that cannot take it queues the message instead of delivering it. A lease taken by a peer stops the old owner's actor, and timer ticks are routed through the hub like any other message | A lease outlives a hard-killed holder by its TTL (30s by default), and a graceful shutdown releases it immediately. Detection of a lost lease is within a third of the TTL while a session is idle, immediately when it has messages waiting |
| Every message reaches its session | The shared inbox, drained by the owner, ordered by enqueue time | At-least-once: a crash between delivery and acknowledgement redelivers. A repost of the same message id from the same instance is one row |
| A takeover carries the backlog | Undelivered rows stay claimable; the new owner drains them. Delivered rows are swept after an hour, so the inbox does not grow forever | An in-flight turn is lost and re-delivered, so a loop may see a message twice |
| A daemon survives its instance | A failover pass every half session TTL asks the lease table whether each persistent agent's `boot` session is owned; when nobody owns it, an instance claims and boots it | The boot turn runs once on the new owner. A daemon's conversation lives in the shared memory backend, so it resumes rather than starting empty |
| A session keeps its shell across a failover | There is nothing to keep: a shell command is a one-off sandbox job on a project in the shared files store, so the next command runs on whichever instance owns the session | A job running on an instance that dies is lost, with whatever it had not yet written back; a background job is then reported as failed rather than run again. An SSH handle does not move: the loop spawns a new one |
| A scheduled trigger fires once | A `trigger:<name>` lease, claimed for the trigger's own interval | A killed holder stalls its trigger until the claim expires (one interval plus a margin) |
| Housekeeping runs once per cell | `files-gc`, `files-scratch`, `journal-prune`, `usage-prune`, `rows-sweep` leases, each held for the job's own interval plus slack | A sweep that outlives its lease can still overlap another instance's |
| A graceful shutdown drains | Readiness turns false, then `runtime.shutdown_grace` (5s regional, 0 single) passes before listeners close; the hub releases its sessions as it exits | A crash skips the drain; peers recover at the lease TTL instead |
| Budgets hold in aggregate | A daily agent budget reads the ledger rather than its own counter (refreshed at most every 15s) | Overshoot is bounded by what the fleet spends in one refresh interval, not by the instance count. Rolling-window budgets stay per-instance |
| Per-user quotas hold | The shared ledger, plus in-flight holds that are shared in a regional cell and process-local in `single` | A regional hold costs one transaction per model call; the per-user advisory lock is what makes two instances' reservations one decision |

### Invariants a deployment has to keep

- **Route handlers keep their state in the store, not in Lua.** The router is
  one Luau state per process, and whichever instance receives an inbound runs it
  — so a handler that remembers anything in a Lua global gives each instance its
  own answer, and a restart forgets. `router.state.*` is the place for it
  instead: durable, deployment-wide, the same value on every instance, and it
  survives a failover.
- **`ADMIN_TOKEN` is set explicitly, or every instance is named as an
  operator.** In a regional cell a per-boot token is refused rather than minted,
  because each console would reject the others'.
- **Message ids are unique per instance, not per fleet.** Channel drivers often
  number their own messages (`wh-1`). The inbox qualifies them by the instance
  that accepted them, so nothing is lost — but an audit lookup on the journal by
  id alone can match two instances' rows. Treat the id as a per-instance key
  when querying.
- **Compare config epochs.** `/admin/api/state` reports `config_epoch`, a short
  hash of the configuration fragments as written, and flags live instances whose
  epoch differs. Two instances disagreeing means one is running a different
  deployment, and a rotated secret does not move it (it hashes what was written,
  not what was resolved).
- **A console config edit reaches one instance.** The Models tab hot-applies to
  the process serving the console; the reload watcher covers loop, instruction
  and prompt files, not the models registry. Persist to the config file, then
  roll the fleet.
- **Leases and the inbox live in the runtime store.** They share one target
  because they have to: a lease in one place with the inbox in another routes a
  session to an owner that never sees its messages. `runtime.cluster.persistence`
  is deprecated for that reason.
- **Every instance needs the sandbox to run shell jobs.** A job runs on the
  instance that owns the session, so each one needs the configured sandbox
  runtime (`runsc` and unprivileged user namespaces for `gvisor`, or `msb` with
  hypervisor access for `microsandbox`) and its environment images.
  Projects and background-job records are in the shared stores; the workspace
  directory on an instance is only a cache of them.
- **One job per project per instance, not per cell.** Two instances can run
  commands on the same project at once when two sessions of one user land on
  different instances. Neither overwrites the other: a path both changed keeps
  the version stored first, and the later job reports it as a conflict.
