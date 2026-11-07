## Architecture

A request moves down through a short stack. A channel driver turns platform bytes into a `session.Message`. The optional identity layer resolves who sent it, and the router's Lua handler picks a session key. The supervisor finds or creates the session actor, which runs one Luau loop. The loop calls capability-backed ops through the async bridge, and drivers reach external systems. The core owns the safety filters, the audit journal and provenance. They sit on the path itself, so a loop cannot remove or forge them.

```
┌─ Channels (bytes in / replies out) ─────────────────────────────────────────┐
│  webhook (sync | async)   ghhook (GitHub HMAC)   bridge (HMAC adapter)       │
│  telegram: polling | webhook | auto     studio (synthetic, playground only)  │
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
            │ deliver{agent, key, message, identity_group?}
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
│  shell:  docker · ssh    mail: imap + smtp    files: fs | s3    net: netguard │
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

## Deployment: one file to a fleet

The engine runs from a single SQLite file on one machine to several instances behind a load balancer, and the change between those is the persistence target — not a different binary, and not a different config shape. Everything below is one ladder: each rung adds a dependency and buys a guarantee, and the rung below keeps working.

### Tier 0 — one instance, one file

The default. `runtime.persistence: sqlite://./data/agentflow.db` (or nothing at all) gives you the journal, the file store's metadata, the token ledger, the identity registry, the credential store, the lease table, the session inbox and the shell handle records, each in a local file beside the runtime database. Nothing to install, nothing to reach over a network, and every guarantee the engine makes about a single instance holds: the audit trail is complete, quotas are enforced from a ledger that survives a restart, and scheduled work fires.

With a self-hosted model (Ollama, vLLM, llama.cpp behind an OpenAI-compatible endpoint) this tier needs no third-party service at all.

### Tier 1 — a fleet sharing one store

Point `runtime.persistence` at a PostgreSQL DSN and the stores follow it:

```
runtime:
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require
```

That one line shares the runtime store, the identity registry and the credential store. Each has an explicit override (`runtime.identity.persistence`, `runtime.credentials.path`) for the deliberate case — a shared identity registry with per-instance journals is legal, and the engine warns at boot when the two disagree, because it is usually a mistake. A multi-region deployment has one more: `runtime.cluster.persistence`, the store every region must share for the lease table and the session inbox ([Geo-clusters](#geo)).

A target is a scheme and an address: `postgres://…` or `postgresql://…` for a server, `sqlite://<path>` or a bare path for a local file, and for the log plane a directory — `./data/log` or `file://./data/log`. Every target is parsed at boot in one place, so a scheme the engine has no backend for is a boot error naming the schemes it does have: a typo cannot quietly become a directory named after it.

| State | Where it lives at Tier 1 | Why it has to be shared |
| --- | --- | --- |
| Message journal | `message_journal` | The audit trail is only an audit trail if it covers the whole deployment. |
| File store metadata | `files_meta` (blobs stay in media storage) | A commit made on one instance has to be readable on the next. |
| Token ledger | `usage_daily` / `usage_events` | Quotas and agent budgets are measured against it from every instance. |
| Profiles and identities | `profiles`, `identities`, `user_tokens` | One handle must resolve to one user scope everywhere, or a person's files and memory split in half. |
| Credentials | `credentials` | A key added through one console resolves on every instance. |
| Singleton leases | `leases` (the [cluster store](#geo)) | What makes the work below run once per deployment rather than once per instance. |
| Session inbox | `session_inbox` (the cluster store) | Messages for a session that runs elsewhere wait here instead of starting a second copy of the conversation. |
| Route state | rows under `route\|state\|` in the same table | A route handler's memory has to be the same everywhere, or a chat is routed by whichever instance happened to receive the message. |
| Shell handles | rows under `shell\|h\|` in the runtime store's key/value table | A session that moves has to find the container it was using, and a killed instance's containers have to be reclaimable by someone. |

Each store opens its own schema on boot, from the same statements, so a new table or index appears on the next start without a migration step. Stores on one DSN share a connection pool (ten connections per instance), which is the number that matters when a fleet multiplies it by its instance count.

### Tier 2 — shared memory for durable conversations

A session's conversation is not held in the actor: the default loop recalls it from the memory backend (`memory.recall`) and writes each turn back. That is what makes a conversation survive a failover — provided the backend is shared. Redis, PostgreSQL and Qdrant backends are all shared by construction; a per-instance SQLite or volatile backend gives each instance its own view of the conversation, and a takeover starts from an empty history.

So the fleet configuration pairs Tier 1 with a shared memory backend:

```
memory:
  backends:
    vec:
      provider: qdrant
      config: { url: http://qdrant.internal:6333, api_key: ${QDRANT_KEY} }
    hot:
      provider: redis
      config: { url: redis://redis.internal:6379 }
```

### The log plane

The engine's state has three shapes, and the shape is what decides where it belongs:

- **Read on the hot path to make a decision** — the token rollups a quota check and a budget refresh read before every call, the file-store metadata, session state, route state, shell handles. These stay in the transactional store, where a sum is exact and immediately visible to every instance.
- **Written constantly, read rarely, expired by age** — the message journal and the per-call usage detail. That is the log plane, and it is the part a deployment can put somewhere else: `runtime.log_plane.persistence: ./data/log` writes both as newline-delimited JSON, one file per UTC day, and prunes by deleting whole days.

Nothing above the store changes: the audit trail an operator reads and the per-call detail on a profile's page come from wherever the plane points, and `usage_daily` — the rollup, which must not move — keeps every quota and budget decision exact. Three limits are worth knowing before pointing it at a directory. A machine crash can lose the last records written, because they are not fsynced (the journal is best-effort at both call sites already, so this weakens nothing that was promised). The directory belongs to one process. And a journal on local files is *per instance*, which fragments exactly what an audit trail is for — a fleet keeps the journal in the shared store. Retention stays per plane: `audit.retention_days` prunes the journal wherever it lives and `usage.retention_days` the detail, each reporting what it deleted.

### What multi-active guarantees

| Guarantee | Mechanism | Limit worth knowing |
| --- | --- | --- |
| One conversation runs in one place | A `session:<agent\|key>` lease; an instance that cannot take it queues the message instead of delivering it | A lease outlives a hard-killed holder by its TTL (30s by default), and a graceful shutdown releases it immediately |
| Every message reaches its session | The shared inbox, drained by the owner, ordered by enqueue time | At-least-once: a crash between delivery and acknowledgement redelivers. A repost of the same message id from the same instance is one row |
| A takeover carries the backlog | Undelivered rows stay claimable; the new owner drains them | An in-flight turn is lost and re-delivered, so a loop may see a message twice |
| A cross-region arrival is arbitrated | The shared lease table and inbox: an instance that cannot take the session queues the message for the owner instead of running it | Requires `runtime.cluster.persistence`. With per-region leases the same arrival starts a second copy of the conversation, and nothing detects it |
| A session keeps its shell across a failover | Every handle is recorded with the container it names; the instance that takes the session over adopts it by id | Docker containers are adopted; an SSH handle is dialled again from the session's shell profile, because a record never carries a password or key. A handle whose session is alive nowhere, and unused for 24h, is reclaimed |
| A scheduled trigger fires once | A `trigger:<name>` lease, claimed for the trigger's own interval | A killed holder stalls its trigger until the claim expires (one interval plus a margin) |
| Housekeeping runs once | `files-gc`, `files-scratch`, `journal-prune`, `usage-prune`, `shell-reclaim`, `rows-sweep` leases | A sweep that outlives its lease can overlap another instance's |
| Budgets hold in aggregate | A daily agent budget reads the ledger rather than its own counter (refreshed at most every 15s) | Overshoot is bounded by what the fleet spends in one refresh interval, not by the instance count. Rolling-window budgets stay per-instance |
| Per-user quotas hold | The ledger, with in-flight holds, from the first version | None: it was never in-process |

### Invariants a deployment has to keep

- **Route handlers keep their state in the store, not in Lua.** The router is one Luau state per process, and whichever instance receives an inbound runs it — so a handler that remembers anything in a Lua global gives each instance its own answer, and a restart forgets. `router.state.*` is the place for it instead: durable, deployment-wide, the same value on every instance, and it survives a failover (see the route-handler ops below).
- **`ADMIN_TOKEN` is set explicitly.** Otherwise every instance mints its own console token at boot, and each console rejects the others'. The engine says so at boot.
- **Message ids are unique per instance, not per fleet.** Channel drivers often number their own messages (`wh-1`). The inbox qualifies them by the instance that accepted them, so nothing is lost — but an audit lookup on the journal by id alone can match two instances' rows. Treat the id as a per-instance key when querying.
- **Compare config epochs.** `/admin/api/state` reports `config_epoch`: a short hash of the configuration fragments as written. Two instances disagreeing means one is running a different deployment, and a rotated secret does not move it (it hashes what was written, not what was resolved).
- **A console config edit reaches one instance.** The Models tab hot-applies to the process serving the console; the reload watcher covers loop, instruction and prompt files, not the models registry. The epoch comparison above will not catch it either, because the file on disk is unchanged — a hot-applied edit is invisible to every other instance until it restarts. Persist to the config file, then roll the fleet.
- **Leases and the inbox are one store.** They share one target (`runtime.cluster.persistence`) because they have to: a lease in one place with the inbox in another routes a session to an owner that never sees its messages. Every instance of the deployment, in every region that can reach it, points at the same one.
- **Regions that share nothing get nothing.** The lease table is the only thing that tells an instance a session is already running elsewhere, so if two regions cannot reach one store the engine is blind across them: route by user at the edge, keep the regions' users disjoint, or share the cluster store.
- **Every instance runs the same shell providers.** A record names its provider (`docker`, `ssh`), and the adopting instance looks it up by that name: an instance that does not have it cannot reach the handle, and reports that rather than quietly spawning a second container for the same session.
- **Reclaiming another instance's containers needs the same Docker host.** A container id is addressable from anywhere that can reach the daemon. An instance that cannot reach it logs the failure and leaves the record for the next pass, rather than forgetting a container that is still running.

### Geo-clusters

The store is the sync point, so a geo-cluster is a question about *which* store, not about the engine. The rule is one sentence: **state that has to be singular for the whole deployment goes in one store every region can reach; everything else is region-local.**

- **The lease table and the session inbox are that singular pair.** `runtime.cluster.persistence` points them at the one store all regions can reach — a single PostgreSQL instance, or a primary with replicas. Two things follow, and both are why it is not optional. A session runs on exactly one instance *of the whole deployment*; and a message that arrives in the wrong region is *arbitrated* rather than acted on — the receiving instance cannot take the lease, so the message goes to the shared inbox and the owner delivers it. That is also what makes a cross-region takeover a real lease handover: the peer can see the session is running before it takes it.
- **With regional leases the engine cannot protect you.** If each region has its own lease table, an instance in one region cannot see that a session is already running in another — it takes the session and starts a second copy of the conversation, which is the one failure the whole lease design exists to prevent. The engine warns at boot when `runtime.region` is set and the lease store is a local file, because that combination is exactly this; nothing else can detect it, since the regions cannot see each other at all.
- **Everything else is region-local — and so are the singletons.** The journal, the file metadata, the session state, the ledger: point them at the region's own store, and the traffic that touches them never crosses a region. What that also means is that the work the engine elects is elected *per region*: triggers, the file GC, the retention prunes and the daemon boot all arbitrate through the lease table, so with regional leases every region fires the daily digest and runs the daemon. That is the right answer when each region serves its own users, and the wrong one when they serve the same people.
- **The append-heavy plane can move — and it stays in PostgreSQL.** The journal and the per-call usage events are append-mostly, keyed by time, and pruned by age, which is what lets them leave the transactional store while everything else stays put. What decides it is the rule above: written once, read rarely and expired by age can go to a log plane; anything read on the hot path to make a decision cannot. In a fleet that plane is the shared store like everything else, because a journal per instance fragments the audit trail it exists to be; on a single instance `runtime.log_plane.persistence` can point it at an append-log directory instead, which keeps the write volume out of the main database. The engine's stores are PostgreSQL and SQLite and there is no wide-column backend: if the journal's volume starts to hurt, the move that keeps everything on the database you already run is partitioning those two tables by day, so retention is a partition drop rather than a large delete.
