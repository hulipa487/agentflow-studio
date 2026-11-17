## Configuration Guide

An empty deployment is valid: `agents:` may be omitted entirely, and the
runtime boots as an admin/Studio control plane so the first agent can be
created from Studio. Channels, triggers and other references must still name an
existing agent.

AgentFlow is configured through a single YAML file (`agentflow.yaml`). Every value is validated at boot — an unknown key is a boot error with file and line, never a runtime surprise. Environment interpolation (`${VAR}` and `${VAR:-default}`) is supported anywhere a string is accepted.

### Secrets: env references, the credential store, and honest degradation

Secret-bearing fields (`models.*.api_key`, `search.engines.*.api_key`, `gateway.channels[].token`/`secret`, `memory.backends.*.config.url`/`password`/`api_key`, `media.s3.access_key`/`secret_key`, `browser.api_token`, `profiles.shell.*.password`) accept three forms:

- a literal value,
- `${VAR}` (or `${VAR:-default}`) — resolved from the process environment, with the credential store (by the same name) as fallback,
- `cred:<service>` — resolved from the credential store only.

On the **configdir** path these stay as lazy references and resolve at consumer construction; on the single-file `-config` path the whole document is byte-expanded at load (legacy behavior, unchanged). Resolution order is env → credential store → unresolvable. The degradation policy is uniform: an unresolvable credential on an *optional* component (a search engine, a telegram/ghhook channel, a memory backend, the S3 media store) **skips that component with a warning naming the credential** — never a boot failure. A model `api_key` is *not* optional: the model stays configured and the first LLM call fails with a clear error naming the unresolved credential. `runtime.credentials.enabled=false` (or no store) makes `cred:` references unresolvable, feeding the same policy.

### File skeleton

```
version: "1"

runtime:      { ... }   # instance-wide: persistence, admin, cluster, log_plane,
                        #   identity, credentials, users, region, timezone_offset_hours
models:       { ... }   # named LLM providers
memory:       { ... }   # instance-wide backend instances
profiles:     { ... }   # reusable configs: memory/, safety/, shell/, agent/
gateway:      { ... }   # one shared listener + its channels
mcp:          { ... }   # MCP servers
tools:        { ... }   # global tool policy
search:       { ... }   # web-search engines
legal_search: { ... }   # HKLII / NPC
browser:      { ... }   # Cloudflare Browser Run
net:          { ... }   # outbound connection policy
media:        { ... }   # blob store backend: fs (default) | s3
files:        { ... }   # user-scoped file store: fs (default) | s3, scratch TTL
audit:        { ... }   # message journal: enabled, retention_days
usage:        { ... }   # per-user token ledger: quota, events, retention
triggers:     [ ... ]   # scheduled / event tasks the engine fires
prompts:      { ... }   # deployment-wide prompt registry
agents:       { ... }   # the agents themselves
plugins:      { ... }   # plugin dir, capability ceiling
```

### Philosophy

YAML defines *topology* — what exists and how it's wired. Behavior lives in Lua plugins. Parameters live in YAML. This separation means a working chatbot is a ~12-line YAML with zero code, while every point in the pipeline remains overridable.

> **Strict parsing**: `KnownFields(true)` means every unknown key is a boot error. There are no "ignored" typos.

### Config directories (`-config <dir>`)

As an alternative to the single file, pass a directory to `-config <dir>` to load the directory layout; a directory that holds only an `agentflow.yaml` (what `agentflow init DIR` writes) loads that file. Every fragment decodes under the same strict validation, and validation runs *after* the merge, so cross-file references check exactly as they do in the single file. The config source — the file's directory or the directory itself — is the resolution base for every relative path (loops, instructions, prompt `file:` paths, the router route, persistence, sqlite backend paths, shell `key_file`, `plugins.dir`, `media.dir`). This is true for a single file too: `agentflow run -config deploy/agentflow.yaml` resolves `./loops/x.lua` beside that file, regardless of the shell's working directory.

```
<dir>/
  system.yaml       # required: top-level config minus agents/channels
  channels.yaml     # optional: channels: [...]
  profiles/*.yaml   # optional: one agent per file (sorted glob)
  triggers/*.yaml   # optional: triggers: [...] (merged sorted, later file wins)
```

A `profiles/*.yaml` file is an agent config plus a `name` and a `spawn` flag: `spawn: false` (or absent) routes it to `agents`, `spawn: true` routes it to `profiles.agent` (a spawn template, including its `extras` and `credentials` allow-list — spawned children see both). Agent-only fields (`persistent`, `singleton`, `taps`, `channels`, `lifecycle`) on a spawn profile are boot errors. A trigger is `{name, event{channel, match} | cron | every, run_on_boot, target{profile}, payload}` — `every:` and `cron:` triggers are executed *by the engine* (see `triggers` below), and the merged list is served to loops as data via `runtime.triggers()`. A directory deployment re-reads `triggers/*.yaml` on a poll, so editing a schedule takes effect without a restart.

## Full Config Reference

### `runtime` — instance-wide tuning

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `vm.memory_limit` | string | — | Caps each Luau state’s total allocation (`b`/`kb`/`mb`/`gb`; empty = uncapped). Stamped onto every new session; over-allocation surfaces as a Luau out-of-memory error. With `limits.sessions_per_tenant` it bounds a tenant’s VM footprint |
| `vm.instruction_budget` | string | — | **Deprecated — no effect.** The per-resume budget is fixed at 5,000,000. Warns when set |
| `scheduler.workers` | int | 8 | Op-pool size. This is the only place it is set |
| `reload.watch` | bool | — | **Deprecated — no effect.** The reload watcher always runs. Warns when set |
| `persistence` | string | sqlite://./data/agentflow.db | Where the message journal, the token ledger and its per-call detail, and the engine’s key/value rows (file trees, session state, route state, shell handles) live. Scheduler timers are *not* here: the timer service is in memory, so a restart forgets every timer until its loop re-registers it |
| `topology` | string | single | The deployment tier: `single` (one process) or `regional` (several instances in one region sharing one PostgreSQL). Unset means `single`. The tier decides which other settings are valid; boot refuses one it cannot honour. See [Topology](#topology) |
| `zone` | string | "" | Availability-zone label inside the region (letters, digits and `. _ -`, at most 32 characters). Reported in metrics and `/admin/api/state`; it is a label, never a routing decision |
| `shutdown_grace` | duration | 5s regional · 0 single | How long a shutdown first reports unready and keeps serving before the listeners close, so a load balancer can drain and an in-flight turn can finish. `0` disables the wait |
| `sessions.idle_unload` | duration | 30m | Unload a conversation after this long without activity. Its Lua locals are gone; `session.state.*` survives, and the next message recreates the session. The session id is not retired and its lease is released. `0` disables unloading |
| `admin.cors_origins` | list | \[\] | Exact browser origins allowed to call `/admin/api/*` (a custom frontend or another browser client). The native Studio desktop app sends the token directly and does not need it. Empty keeps the API same-origin only; `"*"` is refused. Preflight is answered before the bearer check |
| `admin.socket` | string | "" | Adds a local-only admin listener: a unix socket path on Unix (relative paths resolve against the config directory) or a named pipe such as `\\.\pipe\agentflow-admin` on Windows. The bearer token is still required |
| `admin.tcp` | bool | true | Serve the admin plane on the HTTP listener (`gateway.listen`), under `/admin/` plus `/healthz`, `/readyz` and `/metrics`. `false` serves it only on `admin.socket`; at least one transport must remain. There is no `admin.listen`: the admin plane has no port of its own, and a config that sets one is refused |
| `admin.operators` | map | {} | Named admin tokens (`name: token`), each with the same authority as `ADMIN_TOKEN`. Requests are attributed to the name in logs. Values may be `${VAR}` or `cred:<service>` references, resolved at boot; an unresolvable one is skipped with a warning |
| `limits.sessions_per_tenant` | int | 0 | Caps live sessions per tenant at the supervisor's spawn boundary (routed first contact and `agent.spawn`). 0 disables |
| `limits.ephemeral_per_tenant` | int | 16 | Caps concurrent one-turn sessions per user (the OpenAI channel's default session kind). Separate from `sessions_per_tenant`, so stateless traffic cannot consume conversation slots. It needs a user to count against, so requests with no user (an `openai` channel with `auth: key` or `none`) are not limited by it. 0 disables |
| `limits.egress_per_minute` | int | 0 | Caps `http.request` and mail ops per tenant per minute (fixed window). 0 disables. Op-pool fairness between tenants is always on and not configurable |
| `studio.enabled` | bool | true | Expose `/admin/api/studio/*` (builder, playground, trace). The API is inert without the admin token; `false` answers 503 |
| `studio.workspace` | string | <config dir>/studio | Where a single-file deployment writes studio loops and prompts. A directory deployment writes its own `loops/`, `prompts/` and `profiles/` instead |
| `studio.trace_events` | int | 2000 | Events retained per session for the trace timeline (bounded ring, oldest dropped) |
| `studio.trace_sessions` | int | 200 | Session trace rings retained; the least recently active is evicted beyond this |
| `studio.trace_payloads` | bool | false | Include message, reply and result text in trace events. Metadata (op, model, duration, tokens, errors) is always kept |
| `studio.chat_timeout` | duration | 120s | How long one playground turn may wait before it returns with partial replies and `timed_out` |
| `identity.enabled` | bool | false | Mint a stable user UUID per inbound; rewrites `from` to `user:<uuid>`, enabling `session.push_user`. Turning it on re-scopes existing state — see the note below the table |
| `identity.persistence` | string | <persistence dir>/identity.db | sqlite path for the identity registry |
| `credentials.enabled` | bool | false | Encrypted per-tenant credential store backing `auth={service=...}` |
| `credentials.path` | string | <persistence dir>/credentials.db | sqlite path for the credential store |
| `credentials.master_key_env` | string | CREDENTIALS\_MASTER\_KEY | Env var holding the master key (read at boot, never from the file) |
| `credentials.engine_wide` | string | allow | Gates `credential.get()`, the op that returns a raw secret to Lua. `allow` (or empty) lets an agent read what its `credentials:` list names; `maintenance` restricts it to engine-fired turns (boot, scheduler, triggers), so a tenant turn never sees an engine-wide secret. The per-tenant path, `auth = {service = ...}` on `http.request` and `tools.http`, never returns the value |
| `users.enabled` | bool | false | Serve the public `/v1/users` API on the gateway listener (requires `identity.enabled`) |
| `users.registration` | string | open | `open` or `invite` (single-use codes from `POST /admin/api/users/invites`) |
| `users.require_registration` | bool | true | Keep an unlinked handle in the shared service stratum (no personal scope, no quota identity) until it is linked to a profile. `false` auto-claims a profile on first contact |
| `users.unregistered` | string | allow | What happens to a sender with no profile: `allow` keeps serving them in the shared stratum; `reject` drops the inbound and asks them to register or link |
| `users.link_ttl` | duration | 10m | How long a channel-link code stays valid |
| `users.push_same_tenant` | bool | false | Make `session.push_user` refuse targets other than the current turn's tenant. Set it for a multi-tenant chat product |
| `users.jit_provisioning` | bool | true | Create a profile on a verified first OIDC login; `false` refuses unknown subjects until an operator provisions them |
| `users.oidc.issuer` · `audience` · `jwks_url` | string | — | Identity provider verification (see *Identity provider* below) |
| `users.oidc.subject_claim` · `email_claim` · `name_claim` | string | sub · email · name | Token claims that carry the identity and the provisioned profile's fields |
| `cluster.persistence` | string | `persistence` | **Deprecated — warns when set.** Leases and the session inbox always live in `runtime.persistence` now. In a regional cell it is refused unless it names the same target |
| `cluster.session_ttl` | duration | 30s | How long a session claim survives without renewal |
| `cluster.poll_interval` | duration | 250ms | How often an instance looks for messages waiting for the sessions it owns — the added latency of a message that arrives on the wrong instance |
| `region` | string | "" | Names the region this instance serves: a metric label `region`, a field in `/admin/api/state`, and part of the instance registry. A region is one independent cell, not a shard of a global deployment |
| `log_plane.persistence` | string | "" | A directory for the append-only planes — the message journal and the per-call usage detail — written as one JSONL file per UTC day. Empty keeps both in the runtime store |
| `timezone_offset_hours` | float | 0 | UTC offset cron triggers are matched in (e.g. `8`, `-5.5`); `every:` is unaffected |

> **Turning the identity layer on re-scopes everything**: `identity.enabled` is not a cosmetic switch. With it on, every inbound `from` is rewritten from the channel-native sender to a stable `user:<identity>`. The personal scope comes later: once a handle is linked to a profile — or claims one on first contact, which happens only when `runtime.users.require_registration: false` — the engine stamps `user_uuid` on the message’s core-owned `provenance` (the field a loop reads; it is never written into the payload, which a delegation hop rebuilds from Lua), and that stamp is what the user-scoped subsystems resolve against. The file store's scope then moves from `agent:<agent>` to `user:<uuid>`, a turn's memory moves from the shared service stratum to the interactive one, and per-user quotas have an account to charge. Until a handle is linked it keeps no personal scope at all: only `from` has changed. Anything a loop keys on `from` or composes into a scope of its own moves with it.
>
>  **Nothing migrates, and the engine cannot tell what moved.** Rows written under the old head — files, memory, whatever a loop keyed itself — are not adopted, because there is no way to know that yesterday's `agent:bot` belonged to the person who is now `user:u_xxxx`. So turn the layer on before real traffic, or write the adoption yourself: a loop that copies the old head's rows to the new one, run once, at the switch. The same holds in reverse — turning it off, or unlinking a handle, leaves the personal scope behind.

The admin server exposes `GET /healthz`, `GET /readyz`, `GET /metrics` (Prometheus text format), `GET /admin/sessions`, and `/admin/credentials`. (The session listing was `/v1/sessions` before: that prefix belongs to the public user API, which this private listener does not serve.) Live counters include `agentflow_sessions_active`, `agentflow_children_spawned`/`died`, `agentflow_llm_calls`, `agentflow_llm_tokens`, `agentflow_budget_denied`, and `agentflow_trigger_fires`.

### Topology

`runtime.topology` picks the deployment shape, and the shape decides which
settings are valid. It is one setting rather than a dozen because the alternative
is a deployment that is half-configured and looks healthy: a regional fleet with
a local blob directory gives every instance its own copy of the data with no
error. Boot refuses those combinations, and `agentflow validate` refuses them
before boot.

#### `single`

The default, and every configuration written before the setting existed. One
process; anything valid before stays valid:

| Backend | `single` |
| --- | --- |
| Runtime, identity, credentials, leases, inbox | SQLite or PostgreSQL |
| Memory: `sqlite`, `postgres`, `pgvector`, `qdrant`, `redis`, `redisvector`, `volatile` | All allowed |
| Blobs (`media`, `files`) | Local directory or S3 |
| `runtime.log_plane.persistence` | Allowed |

A second live process on the same store is refused at boot with the peer's id
and last-seen time. `-force-single` starts anyway; use it only after a crash,
or wait out the 30-second liveness window.

A single instance installs no session hub: there is no second owner to hand a
conversation to, so no `session:` lease row and no shared-inbox row are written.
`-force-single` keeps the hub — an operator using it is saying two processes may
share the store, and then the lease is the only thing keeping one conversation
from running twice.

#### `regional`

Several instances across availability zones in one region, sharing one
PostgreSQL. Boot refuses:

| Setting | Refusal |
| --- | --- |
| `runtime.persistence` not PostgreSQL | Every instance would keep its own database |
| `runtime.identity.persistence` / `runtime.credentials.path` set to SQLite | Each instance would keep its own identities or credentials. Unset them to follow the runtime store, or point them at PostgreSQL |
| `runtime.cluster.persistence` naming a different target | Leases and the inbox always live in the runtime store |
| Any `memory.backends` entry with provider `sqlite` | Each instance would keep its own conversation history |
| `media.backend` or `files.backend` not `s3` | Each instance would keep its own blobs |
| `runtime.log_plane.persistence` set | A local directory belongs to one process |
| `ADMIN_TOKEN` unset and `runtime.admin.operators` empty | A per-boot token is unknown to every peer, so none is minted (checked at boot, since it is an environment setting) |

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require
  admin:
    operators: { alice: ${ALICE_ADMIN_TOKEN} }

media: { backend: s3, s3: { bucket: agentflow-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-files, region: eu-west-1 } }
```

Redis is one region-wide cluster (locality `region`); `volatile` stays
instance-local. Each instance writes a heartbeat row to the `instances` table
every 10 seconds and is live while its last heartbeat is within 30 seconds;
`/admin/api/state` reports `topology`, `zone` and every live `instances` entry,
flagging a peer whose `config_epoch` differs. Studio's Overview shows the same
list.

A multi-region deployment is one independent `regional` cell per region — there
is no global tier. See [Multi-region](multi-region.md).

`agentflow doctor -online` (the default for a regional config) checks the live
side: it opens the runtime store, confirms the `vector` extension on every
pgvector backend, puts/gets/deletes a probe object in each S3 bucket, and pings
Redis and Qdrant. Each line is PASS/FAIL with the fix.

### Command line

`agentflow help` lists the commands, and `agentflow help <command>` (or
`<command> -h`) prints a command's flags. That help is generated from the
binary's own flag definitions, so it is the reference that is always current.

`agentflow run [flags]`, or `agentflow [flags]` with no command, runs the engine:

| Flag | Default | Meaning |
|---|---|---|
| `-config PATH` | `agentflow.yaml`, then `config.yaml` | A file, a config directory, or a directory holding an `agentflow.yaml` |
| `-log-level` | `info` | `dev`, `debug`, `info`, `warn` or `error` |
| `-log-format` | `text` | `text` or `json` |
| `-log-file PATH` | | Also append logs to this file |
| `-no-tui` | off | No terminal dashboard; plain logs on stderr |
| `-no-console` | off | Serve the admin API without the web console |
| `-force-single` | off | Start even if another instance looks live on the store (`single` only; after a crash) |

Every command follows the same conventions:

- Flags may come after positional arguments (`agentflow cred set NAME -user u_1`),
  and `-config` may come before the command (`agentflow -config deploy/ validate`).
- The thing a command acts on is a positional argument: `init [DIR]`,
  `user export USER`, `user import BUNDLE.zip`, `user keygen [PREFIX]`,
  `migrate blobs DIR`, `cred set NAME`.
- `-out FILE` writes to a file; without it the output goes to stdout. The one
  exception is `support-bundle`, which writes a timestamped zip by default.
- `-json` prints machine-readable output on `doctor`, `config show`, `cred get`
  and `cred list`.
- A flag that does not apply to a command is an error, never silently ignored.
- `user export` and `user import` find the admin API from `-url`, then
  `$AGENTFLOW_URL`, then the config's `gateway.listen`, then
  `http://127.0.0.1:8080`; the token is `-token` or `$ADMIN_TOKEN`.

The worker pool size is `runtime.scheduler.workers` (there is no flag for it).
Removed flags (`-configdir`, `-workers`, `-log-json`, `-no-webui`, `init -dir`,
`user export -id`, `user keygen -out`, `migrate blobs -from`, `cred -format`)
are refused with a message that names the replacement.

### Credential CLI

`agentflow cred set|get|list|delete <name>` manages engine-wide credentials in the store named by the loaded config (`-config`). `set` reads the value from an interactive prompt or stdin — never argv (pipe it: `printf %s "$TOKEN" | agentflow cred set github_token`). The master key comes from the env var named by `credentials.master_key_env` (default `CREDENTIALS_MASTER_KEY`); an unset key is a clear error. `list` prints names only, never values. These are the same records `credential.get()` and `cred:<service>` config references resolve against.

### Migrating a deployment

The tier change from a single instance to a regional cell is a data move, and
`agentflow migrate` does it without a transform: both backends speak the same
SQL dialect and blobs are content-addressed.

```
agentflow migrate store -from sqlite://./data/agentflow.db -to postgres://...
agentflow migrate store -from <src> -to <dst> -tables files_meta -append
agentflow migrate blobs ./data/media -kind media -config agentflow.yaml
```

`migrate store` copies the runtime planes, the identity registry and the
credential store. It refuses a non-empty destination unless `-append` is
passed, and it reports which tables were copied or absent. Credentials are
copied as ciphertext: the destination must use the same master key, or every
secret fails to decrypt. `migrate blobs` walks a local media or files directory
and PUTs each blob into the S3 bucket named by `-kind`; content addressing
means a rerun HEADs what is already there and uploads only what is missing.
Both commands are safe to interrupt and rerun.

### User API

The engine ships **no user interface**. Instead it exposes a JSON-only profile API on the same public listener the HTTP channels use (`runtime.users.enabled`, requires `runtime.identity.enabled`), so a frontend can live in its own repository and deploy on its own cadence. The contract is published as OpenAPI at `openapi/users.yaml`.

- **Registration** — `POST /v1/users/register` creates a profile and returns its API token *once*. `runtime.users.registration: invite` makes it invite-only, with codes an operator issues from the admin API (`POST /admin/api/users/invites`).
- **Channel linking** — `POST /v1/users/me/links` returns a single-use code; the user sends `/link <code>` *from the handle being linked*, and the engine completes the link when that message arrives. Only the profile owner sees the code and only the handle's owner can send from the handle, so neither half alone links anything. A message carrying a code never reaches a loop. Only channels whose sender identity the platform authenticates can be linked — telegram today; a webhook's self-declared sender and ghhook's constant one cannot.
- **Self-service** — `/me` (profile and handles), `/me/usage` (today, a seven-day window, and the quota standing), `/me/projects` (the caller's file projects), `/me/links/{id}` to unlink, and `/me/token/rotate` to retire a leaked token.
- **Authentication is a bearer token, always** — either an access token from the deployment's identity provider, or an `afu_…` API token from `POST /register`. There is no cookie and no server-side session, so there is nothing ambient for another site to replay and no CSRF token to carry.
- **Cross-origin** — CORS is **off until an operator lists an origin** in `runtime.users.cors_origins`; only a listed origin is answered, and `Access-Control-Allow-Credentials` is deliberately never sent: requests carry a bearer token, never a cookie, so a browser has no ambient authority to lend to another origin. `GET /v1/users/config` is public and tells a frontend whether registration is open, which channels it can offer, and which region (`runtime.region`) this cell serves.

**Identity provider.** Set `runtime.users.oidc.issuer` (plus `audience`, and `jwks_url` when discovery is unavailable) and the engine verifies tokens minted there: signature against the issuer's JWKS, then `iss`/`aud`/`exp`, then the subject claim as the identity — keyed by `<issuer>|<sub>` so two issuers cannot collide. Login, passwords, MFA and session lifetime live in that provider; the engine only verifies. A verified first login provisions a profile from the token's `email`/`name` claims unless `jit_provisioning: false`, in which case an unknown subject is refused until an operator provisions them. Only asymmetric algorithms are accepted, and the issuer must be reachable at boot.

**Deployment note.** `POST /register` is rate limited per client address, so a frontend behind a reverse proxy must rate limit there too — every caller otherwise shares one address.

### Web console

The admin server also hosts an embedded **web console** (on by default; `-no-console` disables the static files) at `http://<gateway.listen>/admin/` — a single-page UI for the operator, no build step or separate port. It has four tabs:

- **Dashboard** — instance info, live sessions, agents/channels, an API-key manager over the credential store (per-user create/revoke, write-only secrets), and a streaming log tail.
- **Models** — list every named model (key masked to a last-4 fingerprint), edit/add/remove, and a *test* button that makes a 1-token call through the live manager and classifies the result (auth / not\_found / unreachable / …). Edits **hot-apply to the running process** immediately, then an explicit *Persist to config* writes them into the file (backup kept as `.bak`) and *Revert* reloads from the file. A drift badge shows whenever runtime and file disagree — on this instance: the comparison is between this process's model set and the file, so in a fleet each console sees its own drift. An edit here applies to **this instance** immediately; the others keep their current models until they restart, so the path to a fleet-wide change is *Persist to config* and a rolling restart.
- **Config** — a raw YAML editor with boot-equivalent *Validate* (strict fields, env expansion, cross-references — errors name the line), an atomic *Save* that backs up first and refuses if the file changed on disk underneath you, and a masked structured view. Saving prompts a restart; the runtime never silently live-reloads topology.
- **Metrics** — the counters as sparklines, sampled in-process (~2h at 5s) so no external TSDB is needed; `/metrics` stays the Prometheus integration point.

**One port.** The admin plane shares `gateway.listen` with the channels: `/admin/` (the console, `/admin/api/*`, `/admin/sessions`, `/admin/credentials`), `/healthz`, `/readyz` and `/metrics`. Channel paths under those are refused, and so are `/health` and `/v1/users/`.

**Auth:** every console API route (and `/metrics`, `/admin/sessions`, `/admin/credentials`) requires a bearer token — even from loopback. Set `ADMIN_TOKEN` (or name `runtime.admin.operators`) to pin it; otherwise a random per-boot token is generated and printed once to the log. A per-boot token is not a secret from anyone who can read that log, so while the token is minted the admin plane answers **only local clients**: a request from another machine, or one that carries `Forwarded`, `X-Forwarded-For` or `X-Real-IP` (so a reverse proxy on the same host does not count as local), gets `403`; `/healthz` and `/readyz` stay open for load balancers. A configured credential opens the admin plane to remote clients, guarded by the token alone, so put TLS in front of it. A regional cell must configure one: a per-boot token is unknown to the other instances. `-no-console` disables the static console files, *not* the admin or Studio APIs and not the authentication. The static SPA files are served open but are inert without the token. Secrets are write-only throughout: list endpoints return metadata and fingerprints, never values, and the config view masks sensitive keys while preserving `${VAR}` placeholders.

**Local transport:** `runtime.admin.socket` adds a second, local-only admin listener — a unix domain socket on Unix or a named pipe on Windows — and `runtime.admin.tcp: false` takes the admin plane off the HTTP listener. The socket is owner-only (0600; the pipe DACL grants the owner, Administrators and SYSTEM), the bearer token is still required, and the Studio desktop app accepts `unix:///path` or `npipe://name` URLs. The browser console is served on the HTTP listener, so keep TCP enabled if you use it. Documentation now lives in AgentFlow Studio and works over a local socket or pipe.

**Admin API.** Everything the console does is a JSON route under `/admin/api/`, so an operator can script it. All of them require the same bearer token.

| Route | Does |
| --- | --- |
| `GET /admin/api/state` | Instance identity, uptime, config epoch, live sessions, agents and channels. |
| `GET /admin/api/config` | The raw config text, a masked structured view, and the file's mtime. |
| `POST /admin/api/config/validate` | Boot-equivalent validation of posted text (strict fields, env expansion, cross-references). |
| `POST /admin/api/config/save` | Validate, back up to `.bak`, atomically replace. Refuses with 409 if the file changed under you. |
| `GET /admin/api/models` | Live ∪ file model set; keys masked to a last-4 fingerprint. |
| `PUT/DELETE /admin/api/models/{name}` | Hot-apply a model to *this process* (see the fleet limitation above). |
| `POST /admin/api/models/{name}/test` | A 1-token call through the live manager, classified (auth / not\_found / unreachable / …). |
| `POST /admin/api/models/persist` · `/revert` | Write the live set into the file, or reload the file's set into the live manager. |
| `GET /admin/api/metrics` | Sampled counter series (~2h at 5s) for the sparklines. |
| `GET /admin/api/logs` | SSE log tail: the ring replayed, then live. |
| `GET /admin/api/users` · `/{id}` | Profiles with today's usage and quota standing; one profile with its recent calls, audit tail and projects. |
| `POST /admin/api/users/{id}/limit` | Set a profile's daily token limit (0 inherits the deployment default). |
| `PUT /admin/api/users/{id}/settings` | A profile's model and instruction layer — the only way to set them, since which model a person runs on is a cost decision rather than a self-service one. |
| `POST /admin/api/users/{id}/unlink` | Detach one channel handle (the last one is refused). |
| `POST /admin/api/users/invites` | Mint a single-use registration invite. |
| `GET /admin/api/agents` · `PUT/DELETE /admin/api/agents/{name}` | Install, replace or remove an agent at runtime from a definition (no files written; Studio's agent routes are the persisting variant). |
| `GET/POST /admin/api/groups` · `GET/DELETE /{id}` | Tenancy groups: list, create, inspect with members, disband. |
| `POST /admin/api/groups/{id}/members` · `DELETE /{id}/members/{user}` · `PUT /{id}/master` | Add or remove a member, or hand the group to a new master. |
| `GET/POST /admin/api/pools` · `GET/PUT/DELETE /{id}/grants` | Named shared memory pools and the grants (tenant → read/write) that decide who may use them. Stores bind to a pool with `pool: <name>`. |
| `GET /admin/api/users/{id}/export` · `DELETE /admin/api/users/{id}` | Export everything held for a profile (privacy export), or purge it. |
| `GET /admin/api/users/{id}/bundle?public_key=…` | Full-fidelity migration bundle: profile, memory rows, file bytes, groups and credentials sealed to the caller's X25519 key. `POST /admin/api/users/import` takes it on the destination cell with the private key. See [Multi-region](multi-region.md). |
| `GET /admin/api/usage?day=YYYY-MM-DD` | The day's token rollup per tenant, with totals. |
| `GET /admin/api/credentials/users` | The user buckets that hold credentials. |
| `GET/POST /admin/credentials` · `DELETE /admin/credentials/{service}?user=` | The credential store: list metadata, provision a per-tenant key, revoke one. Write-only — a value is never returned. |

The browser console covers the four tabs above. Everything else in this table (users, invites, quotas, tenancy, usage, logs and the raw config) has a screen in AgentFlow Studio, or can be scripted with `curl`.

### Studio — agent builder, playground and trace

**AgentFlow Studio** is a native desktop app (Go + Fyne) for prototyping agents: write a loop and its instructions, validate them, apply the definition live, chat with it, watch every step of the turn, and persist the result into the deployment config. It lives in its own repository, `agentflow-studio` — not embedded in the binary — and talks to the admin API with the same bearer token as the console.

- **Agents** — every configured agent with model, capabilities, live session count, and whether it exists only in the runtime or is persisted. Create from a template (echo, LLM chat, tool loop, one-shot), edit, or delete.
- **Builder** — a form for the common fields (model, safety, capabilities, skills, history budget, daemon flag, `can_contact`, credentials, extras), a Luau editor, an instructions editor, and the raw YAML for everything else. *Validate* checks the prelude directive, compiles the chunk and runs the same agent validation boot uses; *Apply* installs it in the runtime; *Apply & save* also writes it to the config.
- **Playground** — threaded chat against one agent. Each thread is its own session (`<agent>|studio:<thread>`), so conversations do not mix. Replies arrive through the synthetic `studio` channel, which means they take the ordinary egress path: safety filters, the audit journal and delivery are unchanged. An optional *act as user* uuid stamps a tenant so personal memory scopes, quotas and pool grants apply; without it a turn is tenant-less.
- **Trace** — a live timeline of the turn: `turn.start`, each `op.start`/`op.end` (model, tool, duration, tokens, tool calls), safety drops, replies, reloads and loop errors. Text payloads are off by default (`studio.trace_payloads`); metadata is always captured.
- **Channels** — the gateway's persisted channel set: add, edit, validate and delete webhook, Telegram, GitHub-webhook and bridge entries; secrets stay write-only. Saves bind at the next start.
- **Models** — the live model registry: add, edit, test with a real call, delete, then persist the live set to the config or revert to the file.
- **Safety** — edit `profiles.safety` chains (ordered builtins and `classifier:<name>` references) and `safety.classifiers` policies. Validation is boot-equivalent; profiles and classifiers still referenced cannot be deleted. Dispatchers are built per agent, so saves bind at the next start.
- **Credentials** — browse the encrypted store by bucket (engine-wide or per-user), create, edit, revoke, and set the injection header/scheme. Values are write-only and shown only as a last-four fingerprint; edits are live because the store is read at request time.
- **Overview** — the landing screen: instance, region, version, uptime and config epoch, live sessions, agents, channels, and sparklines of the sampled counters (`/admin/api/state`, `/admin/api/metrics`).
- **Users** — profiles with today's quota, linked handles (unlink), projects, recent calls and the audit tail; set a daily limit and the per-user model and instruction layer; issue invites; export or purge a profile (`/admin/api/users/*`).
- **Tenancy** — groups (members, master hand-over, disband) and memory pools (create, grants per agent and tenant) (`/admin/api/groups`, `/admin/api/pools`).
- **Usage** — the daily token rollup grouped by tenant, agent or model (`/admin/api/usage`).
- **Logs** — the live log tail with level and text filters (`/admin/api/logs`).
- **Config** — the raw config file with boot-equivalent validation and an mtime-guarded, backed-up save (`/admin/api/config*`); a config-directory deployment is edited through the structured screens and the files instead.

`Ctrl+K` opens a palette over every screen, agent and action.

**Persistence is config-native.** With a directory config, Studio writes `profiles/<name>.yaml` (marked `# managed by agentflow studio`), `loops/<name>.lua` and `prompts/<name>.md`. With a single config file it replaces the `agents.<name>` key (comments and other sections preserved) and writes the loop/prompt under `studio.workspace`; the definition stores absolute paths so the persisted file does not depend on how the next process is started. Either way the change is applied live now and loaded from the config at the next boot. A profile Studio did not write is refused unless forced.

**Running it.** The client is a separate Go module in its own repository:

```
cd ../agentflow-studio
make build        # or: CGO_ENABLED=1 CC="zig cc" CXX="zig c++" go build -o agentflow-studio.exe .
./agentflow-studio
```

Enter the instance URL (`http://127.0.0.1:8080`, the `gateway.listen` address) and the admin token. As a native client it sends the token directly, so no `runtime.admin.cors_origins` entry is needed. To connect from another machine the runtime needs a configured `ADMIN_TOKEN` (a per-boot token answers local clients only); put TLS in front of it. That allow-list remains for *browser* clients of the admin API, and exact origins only (`http://localhost:5173` and `http://127.0.0.1:5173` are different origins); `"*"` is refused.

| Studio route | Does |
| --- | --- |
| `GET /admin/api/studio/catalog` | Models, tools, capabilities, safety/memory profiles, prompt keys, prelude version and templates. |
| `GET /admin/api/studio/agents` · `/{name}` | Agent list; one agent with its YAML, loop, instructions and persisted-file metadata. |
| `POST /admin/api/studio/agents/validate` | Side-effect-free validation of a posted definition and loop. |
| `PUT /admin/api/studio/agents/{name}` | Validate, install live (`apply`), optionally persist (`persist`); returns what applied and any persist error. |
| `DELETE /admin/api/studio/agents/{name}` | Stop its sessions, remove it, and remove only Studio-managed files. `force` overrides live sessions and hand-written profiles. |
| `POST /admin/api/studio/chat` | One playground turn. Streams SSE (`trace`, `reply`, then `done` or `error`) or returns JSON with `stream: false`. A turn ends with `done` (finished, timed out, or refused by safety); a message that could not be delivered or a loop that failed mid-turn ends with `error` instead, and `stream: false` then answers `ok: false` with the trace. A request that can never start a turn (unknown agent, an `act_as_user` that is not a profile) is refused with 4xx before any stream. |
| `GET /admin/api/studio/threads` · `POST /threads/{thread}/stop` | Observed threads and a stop request for one live session. |
| `GET /admin/api/studio/trace/{agent}/{thread}` | The buffered trace; add `?stream=true` for live SSE. |
| `GET/POST /admin/api/studio/channels` · `PUT/DELETE /{index}` | The persisted channel set; validate a draft, create, replace or remove one. Saves are restart-required. |
| `GET /admin/api/studio/safety` | Safety profiles and classifiers, their usage, and the builtin filter and model sets an editor can reference. |
| `POST /admin/api/studio/safety/validate` | Boot-equivalent dry run of a profile or classifier draft. |
| `POST/PUT/DELETE /admin/api/studio/safety/profiles` · `/classifiers` | Create, replace or remove a profile or classifier. An in-use entry is refused; writes validate, keep a `.bak`, and are restart-required. |
| `GET/POST/DELETE /admin/api/credentials` | List one bucket's metadata (`?user=`; empty is engine-wide), create or update a secret (empty secret keeps the stored value), revoke one (`?service=&user=`). Never returns a value. |

**Limits.** The playground is instance-local: the synthetic channel is registered in one process, so in a fleet a thread whose session is owned by another instance will not capture replies here. Studio does not store chat history — the agent’s memory is the source of truth — and the trace rings are in memory, reset by a restart. Spawn-profile authoring, attachments, replay and evaluation are not part of the first version.

### Terminal dashboard

When stdout is a TTY the runtime also attaches a **Bubbletea terminal dashboard** in the alternate screen (`-no-tui` to disable). It is the console's Metrics tab in the terminal: the same counters, grouped by subsystem, with in-process sparklines and the live session counts in the header. It is a **read-only metrics display** — no operator actions, so nothing it shows can change the runtime; `q`/`esc` quits and restores the screen. The metrics pane flows into one to three columns by width and the log footer scales with the terminal (3–8 lines); on a terminal too short for both the pane is clipped with a visible marker rather than dropping rows silently, and below a minimum height it is dropped entirely so the frame always fits. Piped output (systemd, `| tee`) keeps the plain stderr logger instead.

### Logging

Logs use five **additive** levels — selecting one prints it and everything above. Choose with `-log-level` (default `info`):

| Level | What goes here | Examples |
| --- | --- | --- |
| `ERR` | The runtime exits or cannot continue | config load failure, memory backend open failure, port bind failure |
| `WARN` | Degraded but continuing | upstream 429/502 + retry, channel delivery error, loop crash (restarting), reload compile failure (keeping old) |
| `INFO` | Lifecycle | runtime launch, channel registered, telegram webhook set, session spawned/exited, hot reload accepted |
| `DEBUG` | Every interaction | LLM API request, memory put/get/query (redis round-trip, pgvector lookup), telegram update received, webhook/ghhook inbound, op dispatch, imap fetch / smtp send |
| `DEV` | Temporary development logs | scratch instrumentation added while debugging — `llog.Dev(log, ...)`; never committed on purpose |

The same records feed the TUI log pane and the web console's log tail (short tags `ERR/WRN/INF/DBG/DEV`).

### `models` — named LLM providers

```
models:
  default:
    provider: anthropic        # anthropic | openai | openai-responses | gemini | rerank
    model: claude-sonnet-5
    api_key: ${ANTHROPIC_API_KEY}
    base_url: ~                # override endpoint host
    timeout: 30s
    retry: 2
    max_tokens: 4096
    thinking: medium         # off | low | medium | high | xhigh | max; empty = provider default
    server_tools: []           # provider-native server-side tools
```

`provider` selects the request/response shape. `base_url` selects the host. For OpenAI-compatible endpoints (Ollama, vLLM, LiteLLM), include `/v1` in `base_url`; `openai` appends `/chat/completions` and `openai-responses` appends `/responses`. For Anthropic-compatible endpoints, set `base_url` to the host root; the runtime appends `/v1/messages`. For `gemini`, point `base_url` at the version prefix (default `https://generativelanguage.googleapis.com/v1beta`); the runtime appends `/interactions` and sends the key in the `x-goog-api-key` header. The Gemini Interactions API is non-streaming and does not accept a token cap, so `llm.stream` yields one buffered delta and `max_tokens` is not sent. `provider: rerank` is a cross-encoder endpoint speaking `POST {base_url}/rerank` (TEI, vLLM, Jina, Cohere) and is used via `llm.rerank` / memory `rerank_model`, not as a chat model.

`server_tools` lists provider-native server-side tools to enable on every request (e.g. `google_search` on gemini, `web_search`/`x_search` on openai-responses). Unlike client-side function tools passed per-call via `opts.tools`, these execute inside the provider; the runtime only injects the native `{type: <name>}` entry, forwarded under the name you wrote. Values are provider-specific strings, keeping the runtime generic. On the OpenAI shapes that bare entry has no variant in the SDK's tool union (its tool type is pinned to `"function"`), so it is built with `param.Override`; on `gemini`, `google_search` maps to the native tool and any other name is passed through as a raw entry.

### `memory.backends` — backend instances

Every backend below is a real provider — no stubs. Storage backends carry KV/prefix/text/TTL; the vector backend carries semantic recall. SQLite intentionally rejects vector queries with an explicit error rather than returning silent empty results.

| Provider | Kind | Features |
| --- | --- | --- |
| `sqlite` | storage | kv, prefix\_scan, text\_search, ttl, transaction, vector (brute-force cosine over a BLOB column; no extension, capped by `vector_scan_limit`, default 50000 rows) |
| `redis` | storage | kv, prefix\_scan, ttl |
| `postgres` | storage | kv, prefix\_scan, text\_search, transaction — plus vector when the database has the pgvector extension (detected at boot; same table) |
| `volatile` | storage | kv, prefix\_scan, ttl — in-memory, non-persistent; bound by `max_entries` |
| `pgvector` | vector | vector (cosine k-NN; embedding dim configurable via `dim`, default 1536). Kept as an alias/compatibility provider; `postgres` now serves vectors when the extension is present |
| `qdrant` | vector | vector (cosine k-NN over Qdrant's gRPC API — `url`'s port is read as a REST one, so `:6333` becomes gRPC's `:6334`; `dim` default 1536, `collection` default `agentflow`, optional `api_key`) |
| `redisvector` | vector | kv, prefix\_scan, ttl, vector (needs the RediSearch module, e.g. Redis Stack; cosine k-NN, `dim` default 1536) |

Each backend has a **locality**, derived from its provider and `runtime.topology`
and exposed to loops as `agent.info().memory.stores[*].locality`:

| Provider | `single` | `regional` |
| --- | --- | --- |
| `postgres`, `pgvector`, `qdrant` | `deployment` | `deployment` |
| `sqlite` | `deployment` (the one instance is the deployment) | refused |
| `redis`, `redisvector` | `deployment` | `region` (one cluster per cell) |
| `volatile` | `instance` | `instance` |

In a regional cell a durable conversation store — `dialogue`, `facts`, or
`semantic` under `recall: plugin:semantic` — must be on a `deployment` or
`region` backend. Validation refuses a durable store on `volatile` (a session
that moves to another instance would lose its history) unless the store sets
`locality_ok: true`. Volatile memory is also bounded in every tier: either the
backend sets `config.max_entries` (oldest entries are evicted) or every store
bound to it sets a `retention` TTL.

```yaml
memory:
  backends:
    cache:
      provider: volatile
      config: { max_entries: 10000 }

profiles:
  memory:
    p:
      stores:
        dialogue: { backend: main_db, table: dialogue, retention: 30d }
        scratch:  { backend: cache, table: scratch, retention: 1h }
```

```
memory:
  backends:
    main_db:
      provider: sqlite
      config:
        path: ./data/agentflow.db
    vec:                                # any one vector store serves the semantic profile
      provider: pgvector
      config: { url: "postgres://user:pw@host/db", dim: 1536 }
    # provider: qdrant          # gRPC; the url keeps its REST shape, :6333 dials gRPC's :6334
    # config: { url: "http://localhost:6333", dim: 1536, collection: agentflow, api_key: "${QDRANT_KEY}" }
    # provider: redisvector      # Redis Stack (RediSearch), not plain Redis
    # config: { url: "redis://localhost:6379", dim: 1536 }
```

All three vector stores are vector-query-only, like `pgvector`: any other query kind is an explicit error, never a silent empty iterator. `dim` is fixed when the store is first created, so it must match the embedding model; changing it later needs a manual migration. `redisvector` stores the value beside the vector, so it also serves kv, prefix\_scan and ttl; `redis` is unchanged and remains the plain-KV option.

`qdrant` speaks gRPC, but its `url` keeps the shape every deployment already has — a REST one. The scheme picks the transport (`https` dials with TLS) and the port is translated: an explicit `6333`, Qdrant's REST default, becomes gRPC's `6334`; any other explicit port is dialled as given, so a gRPC proxy still works; a URL with no port gets `6334`. A deployment that reached Qdrant through a REST-only proxy has to point at the gRPC listener instead. The points themselves are addressed by the same deterministic ids as before, so an existing `agentflow` collection is read and written unchanged.

With no `memory.backends` block at all, the engine registers `main_db` on the runtime store: provider `sqlite` with `path` for a file, provider `postgres` with `url` for a DSN. That is what makes the built-in `memory: conversational` profile work in a minimal config, and it means a single-instance deployment needs no second database to have durable memory. A `postgres` runtime store therefore needs no `memory:` section either; its database serves vectors when the pgvector extension is installed.

A backend whose `config.url`/`config.password`/`config.api_key` secret reference cannot be resolved is **skipped with a warning** rather than failing the boot: stores bound to it rebind to a surviving backend (preferring one whose provider offers `text_search`, so a vector store degrades to full-text search; loops feature-detect from the binding's features) with `requires` lowered to what the fallback offers, or are dropped when no backend survives. A genuinely unknown backend name is still a boot error.

### `profiles` — reusable configs

Profile kinds are namespaced: `memory/<name>`, `safety/<name>`, `shell/<name>`, `agent/<name>`. Agents reference them by short name.

#### Memory profile

```
profiles:
  memory/conversational:
    stores:
      dialogue: { backend: main_db, table: dialogue, window: 1000, retention: 30d }
      facts: { backend: main_db, table: facts, requires: [kv, text_search] }
    write: [plugin:routing_table]
    recall: plugin:recency

  memory/semantic:                        # embedding + rerank recall
    stores:
      semantic: { backend: vec, table: semantic, requires: [vector] }
    write: [plugin:routing_table]
    recall: plugin:semantic
    embed_model: embed                    # models: entry; auto-embeds writes to vector stores
    rerank_model: rerank                  # optional provider:"rerank" entry
    oversample: 4                         # fetch k*4 vector hits, rerank down to k
```

With `recall: plugin:semantic`, `memory.recall({ text = "...", k = 5 })` embeds the query, runs cosine k-NN over the vector store, and reranks when `rerank_model` is set. Queries without text fall back to recency recall.

| Store key | Description |
| --- | --- |
| `backend` | Required. References a backend instance name. |
| `table` | The physical table (or collection) this store binds to. |
| `retention` | A duration in `ms`, `s`, `m`, `h`, `d` units (compound and fractional allowed, e.g. `2d12h`), or the literal `forever`. **It is a write TTL, not a sweep:** it becomes the default TTL of any `store.put` with no explicit TTL, so a record expires that long after it is *written* and rows written before the setting existed never age out. A value that does not parse is a boot error. |
| `window` | Max records kept per physical table, trimmed by the periodic GC. Applies to the KV backends; **the vector backends (pgvector, Qdrant, Redis) implement no trimming**, so a window or retention on one of those declares a bound nothing enforces — the engine warns at boot and the table grows without limit. |
| `requires` | Feature list validated against provider at boot. |
| `pool` | Binds the store to a named shared pool (created via the tenancy admin API). Grants decide which tenants read or write it; a pool-bound store takes no `scope`, and the pool must exist at boot. `shared: true` is a boot error — pools replaced it. |
| `scope` | `agent` opts a private store back into one pool per agent instead of the engine's per-user key scope. |
| `collection` / `policy` | **Deprecated — no effect.** A store is bound by backend and table, and no per-store policy knob is implemented. Parsed so an existing config still boots; the engine warns at boot when either is set. |

> **Isolation change**: memory stores are **private per agent by default** — two agents on the same profile (or `conversational`) no longer share physical tables; each binds `<agent>.<table>` (e.g. `writer.dialogue`). Loops keep using the profile's table names; the prefix is applied at bind time. **Deliberate sharing:** bind the store to a named pool (`pool: <name>`) and grant tenants access through the tenancy admin API; `shared: true` is now a boot error. Existing rows written before the per-agent change stay under the old unprefixed table and need an explicit migration.

#### Safety profile

```
profiles:
  safety/default:
    filters: [source-attribution, signal-gate, steady-directive, support-offer, affect-guard]
```

`safety: none` is an explicit opt-out. The dispatcher is core-owned and runs regardless of the loop handler.

#### Model-backed classifiers

`safety.classifiers` defines moderation or classification policies backed by a dedicated model. A profile opts in with `classifier:<name>`:

```
models:
  moderation:
    provider: openai
    model: gpt-4.1-mini
    api_key: ${MODERATION_API_KEY}
    timeout: 3s
    retry: 0
    max_tokens: 64

safety:
  classifiers:
    moderation:
      model: moderation
      prompt: safety/moderation          # prompts: registry key; default embedded
      mode: enforce                      # enforce | shadow
      directions: [ingress, egress]
      sources: [channel, agent]
      threshold: 0.7
      categories:
        safe: allow
        self_harm: block
        violence: block
        harassment: review
        prompt_injection: block
      timeout: 2s
      on_error: block                    # block | pass
      on_saturated: block
      on_budget_exhausted: block
      max_tokens: 64
      max_input_chars: 8000
      max_concurrency: 8
      cache: { size: 10000, ttl: 5m }
      budget: { tokens_per_day: 500000 }
      refusal: ""                        # optional ingress refusal text
      fallback: ""                       # optional egress fallback text

profiles:
  safety:
    strict:
      filters: [source-attribution, classifier:moderation, affect-guard]
```

The model returns only a label and a score; the runtime applies `categories` and `threshold`, so model output cannot choose its own action. `safe: allow` (or another `allow` label) is required. The classifier model must be engine-keyed (`key_source: config`, the default); a `key_source: user` model is rejected at boot because a user without a key could otherwise disable moderation. The model is called with no tools and no server tools.

Failures follow explicit policy instead of silently passing: `on_error` (provider error, invalid output, timeout), `on_saturated` (concurrency limit full), and `on_budget_exhausted` (dedicated moderation budget). The default is `block` for all three. Set `mode: shadow` to record verdicts without enforcing them; the model call still happens (and its latency still applies), only the action is suppressed. `sources` uses the normalized vocabulary `channel`, `agent`, `scheduler`, `system` (channel drivers stamp `user`, timers stamp `timer` internally).

An ingress block skips the turn: the loop never sees the message, and `refusal` is sent if configured. An egress block suppresses the reply and sends `fallback` if configured. Refusals and fallbacks are rate-limited to one per session per 30 seconds so blocked-message floods cannot become outbound floods. Classifier spend is recorded under a synthetic `__moderation_<name>` ledger bucket, so it does not drain an agent's `tokens_per_day` budget or a user's LLM quota. A spawn profile with no `safety:` inherits the spawning session's chain; set `safety:` on `profiles.agent.*` to give children a different profile.

#### Shell profile

A shell profile says what an agent's shell is. Bind one with `agents.<name>.shell` (the agent also needs the `shell.exec` capability).

```
profiles:
  shell:
    code:
      provider: sandbox          # sandbox (default) | ssh
      environment: python        # an entry of environments (required)
      project: workspace         # the files-store project jobs run in (default workspace)
      workdir: app               # where a command starts, relative to the project
      timeout: 90s               # a job's time limit, capped by runtime.sandbox.max_timeout
      mem_limit: 512MiB          # may lower runtime.sandbox.memory, never raise it
      cpu_limit: 1.0
      env: { CI: "1" }
      ignore: ["node_modules/", "__pycache__/"]   # created by jobs, never stored
      network: none              # the only value: sandbox jobs have no network
    box:
      provider: ssh              # a connection to a real remote machine
      host: build.example.com:22
      user: dev
      key_file: /etc/agentflow/id_ed25519      # or password: (a secret reference)
      workdir: /srv/work
```

**`provider: sandbox`** runs each command as a one-off [gVisor](https://gvisor.dev) job: the environment is the read-only root, the project is mounted read-write at `/workspace`, and what the command wrote into the project is stored when it ends. Nothing else survives a command, so there is nothing to keep track of and any instance can run the next one. `ignore` uses a small subset of `.gitignore`: `node_modules/` (a directory with that name anywhere), `*.pyc` (a name anywhere), `build/out` (a path from the project root); a trailing slash restricts a pattern to directories. Ignored paths are neither stored nor cleaned up — they stay on the instance as a best-effort cache.

A project can customise its agent's profile with a `.agentflow/shell.yaml` file in its working tree. It may set `environment` (one the deployment declares), `env`, `workdir`, `timeout` and `ignore`. It has no field for an image, a network policy or a resource limit, and an unknown field is an error rather than a silent no-op.

**`provider: ssh`** keeps a connection per session to a machine that already exists; nothing here provisions it. The connection lives in the instance that dialled it and closes with the session.

`provider: docker` was removed. A config that still uses it fails at load with a message naming the replacement.

#### `environments` — declared toolchains

An environment is the root filesystem and variables a sandbox job starts with. Environments are declared by the operator; a shell profile picks one, and a project may pick another.

```
environments:
  python:
    image: docker.io/library/python:3.12-slim@sha256:<64 hex digits>
    env: { PYTHONUNBUFFERED: "1" }
  tools:
    rootfs: /srv/agentflow/envs/tools
```

Set exactly one of:

- **`image`** — an OCI image **pinned by digest**. A tag alone is refused: a tag can be moved to different content, and then two instances would run different toolchains under one name. The image is pulled the first time the environment is used on an instance (that first job waits for the download) and kept under `<runtime.sandbox.data_dir>/images/<digest>`, so changing the pin is all it takes to move to a new image. Only public images, or ones a registry serves to anonymous pulls, are supported. Layers must be gzip-compressed. The image's own `Env` is applied, with `env` here on top.
- **`rootfs`** — a directory on the host that already holds a root filesystem (for example an exported container, or a `debootstrap` tree). It needs a `/bin/sh`.

Because jobs have no network, everything a command needs must already be in the environment: packages are part of the image, not something a command installs.

#### `runtime.sandbox` — how shell jobs run

Every setting has a default; a deployment that declares an environment and a shell profile needs none of this.

| Key | Default | Meaning |
|---|---|---|
| `runsc` | `runsc` on `PATH` | The gVisor runtime binary. Keep the release's `gvisor-bin/` directory beside it |
| `data_dir` | `<data dir>/sandbox` | Workspaces, unpacked images and runsc's own state. A cache: everything in it is rebuilt from the files store and the registry |
| `concurrency` | 4 | Jobs running at once on this instance |
| `per_user` | 2 | How many of those one user may hold |
| `queue_timeout` | 30s | How long a job waits for its turn before failing |
| `timeout` | 60s | A job's time limit when nothing else sets one |
| `max_timeout` | 10m | The most a call, a project or a profile may ask for |
| `background_max_timeout` | 2h | The limit for `shell.start` jobs |
| `output_limit` | 256KiB | Cap on each of stdout and stderr; the rest is dropped and `truncated` is set |
| `tmp_limit` | 256MiB | Size of a job's `/tmp` and home directory |
| `memory` | 1GiB | Memory limit for a whole sandbox. A job that exceeds it is killed and the call raises |
| `cpus` | none | CPU limit in cores |
| `pids` | 512 | Processes inside a sandbox |
| `write_back.max_files` | 10000 | Changed files one job may store |
| `write_back.max_bytes` | 1GiB | Total size of the files one job may store |

**Requirements.** gVisor runs on Linux only (x86_64 and ARM64, kernel 5.6 or newer). No root is needed: the sandbox runs in a user namespace, so unprivileged user namespaces must be enabled. On Windows, macOS, or a Linux host without `runsc`, the engine still starts and every shell call fails with one sentence saying what is missing.

**Memory and CPU limits** are enforced by the kernel through a systemd scope when the host has a systemd manager that will create one — the normal case for a service or a login session. Where it does not (a container without systemd, for example), jobs run without those two limits and should be bounded from outside; the process count, tmpfs sizes, time limit and output cap apply everywhere. `agentflow doctor` reports which case a host is in, whether `runsc` works, and whether each environment is usable.

**One host, one data directory.** Two engines sharing a `data_dir` would clear each other's job state at start.

#### Agent profile (spawnable)

```
profiles:
  agent/coder:
    model: default
    loop: ./loop.lua
    skills: [builtin:fs.read, builtin:fs.write]
    capabilities: [llm.chat, tools]
    can_contact: []
    credentials: [deploy_token]   # credential.get allow-list for children
    extras:                       # read-only via agent.config() in the child
      workflow: release-train
      goal: { type: autonomous, max_turns: 12 }
    ephemeral:
      max_lifetime: 2h
      idle_ttl: 15m
      max_turns: 200
    budget:
      tokens_per_day: 100000
      window: 168h            # optional rolling window; omit for daily reset
```

A spawn profile that declares `budget.tokens_per_day` gets a metered LLM pool **shared by every child spawned from it**, so a project type (manager + workers) draws from one budget. With `window` set, usage drains continuously as commits age past the window instead of resetting at the day boundary.

`extras` and `credentials` carry over to every child, so a spawned loop calling `agent.config()` sees its own profile options and its credential allow-list — the same shape a static agent sees.

### `gateway` — one shared HTTP listener

All HTTP channels (webhook, ghhook, bridge, telegram webhook/auto) mount paths on **one shared HTTP server** — there is no per-channel `listen`. The listener is set once at `gateway.listen` (default `:8080`), and the server also serves `GET /health` → `200 {"ok":true}` for probes and reverse-proxy health checks. `gateway.public_url` is the externally-reachable base (e.g. `https://bot.example.com`) used when the runtime should register webhooks for you.

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `gateway.listen` | string | :8080 | The single shared HTTP listener for every HTTP channel |
| `gateway.public_url` | string | — | External base URL; required for telegram `webhook`/`auto` |
| `gateway.channels` | list | — | Channel definitions (below) |

#### Channel types

| Type | Description |
| --- | --- |
| `webhook` | POST → router → session → reply. **Sync** (default): the response parks until the agent replies or `timeout` (default `55s`) → 504. **Async** (`async: true`): 202 + a job id immediately; the reply is collected via `GET <path>result/<id>` (202 while pending) or POSTed to a caller-supplied `callback_url` — async jobs are never cut off by the sync timeout and expire after 15 minutes. Default path `/webhook/` |
| `ghhook` | GitHub webhook intake: verifies the `X-Hub-Signature-256` HMAC with `secret`, acks 200 immediately, and forwards the event as an agent-typed message (payload `{type: project_event, source: github, event, data}`). No reply path. Default path `/hooks/github/` |
| `bridge` | Out-of-process adapter for any other chat platform (Slack, Discord, Teams, …). The adapter POSTs events to `path` with `X-Agentflow-Signature: sha256=<HMAC of the body>` keyed by `secret`; the runtime POSTs replies as `{to, text, attachments}` to `deliver_url`, authenticated only by `token` as a bearer (outbound deliveries are not HMAC-signed). `timeout` bounds each delivery (default 10s). Bridge senders are recorded as asserted identities, so they cannot be linked to a profile |
| `telegram` (`mode: polling`) | Default. Long-poll `getUpdates`, `allow_users` enforced. Clears any stale Telegram-side webhook first — getUpdates 409s while one is registered (e.g. reverting from webhook mode) |
| `telegram` (`mode: webhook`) | Receives POST updates at `path` (default `/webhook/telegram/`); requires `gateway.public_url`. Every delivery must carry the `secret_token` header (401/403 otherwise, checked before parsing); configured per channel, generated per boot when absent |
| `telegram` (`mode: auto`) | Probes `<public_url>/health` at startup: registers `setWebhook` when reachable, else `deleteWebhook` + long-poll. A `setWebhook` failure also clears the webhook before polling. Needs `gateway.public_url` |
| `openai` | Serves the OpenAI Chat Completions API at `<path>chat/completions` and the model list at `<path>models`. `model` names one of the exposed `agents`, each of which runs its own `loop:`; give an agent `loop: plugin:openai_passthrough` to forward the client's messages, tools and sampling parameters to its model unchanged. `auth` is `user_token` (per-user bearer, needs `runtime.identity.enabled`), `key` (a shared key from `keys`) or `none` (loopback listeners only). `journal` defaults to `metadata` for this type, so conversation text never enters the audit journal. See [the OpenAI-compatible API](openai-api.md) |

Every HTTP channel `path` must end in `/` (a subtree mount). A UUID-style path segment is a recommended unguessable secret prefix. Two channels mounting the same path are a boot error, and so is an `openai` channel whose endpoints overlap another channel's subtree. Telegram's `token` and `secret_token`, ghhook's `secret`, bridge's `secret` and `token`, and an `openai` channel's `keys` are secrets (see the registry): env-interpolated on the single-file path, lazy references on the configdir path — and an unresolvable one skips the channel with a warning, never a boot failure.

**Audit mode** is per channel: `journal: full` (the default for every type except `openai`) records message text and attachment descriptors, `journal: metadata` (the `openai` default) records one row per message with ids, sizes, user and timings and **never the text**, and `journal: off` records nothing for that channel. See [`audit`](#audit--message-journal).

**Inbound media** is opt-in per channel via a `media:` block — an allow-list of MIME patterns (`image/*`, `application/pdf`, `audio/*`, …) plus `max_bytes` (default 8 MiB). Telegram downloads the file (getFile), webhook accepts base64 `attachments`; bytes land in the content-addressed blob store (local filesystem by default, or S3/MinIO via the top-level `media:` config) and the message carries tiny part descriptors. With no media block, attachments are dropped and only the text/caption flows.

```
gateway:
  listen: ":8080"               # the single shared listener (default :8080)
  public_url: ""                # external base, e.g. https://bot.example.com
  channels:
    - type: webhook
      path: /webhook/dev/9f2c.../   # subtree; UUID segment = secret prefix
      agent: bot
#     timeout: 120s                 # sync reply wait (default 55s)
#     async: true                   # or fire-and-poll: 202 + job id, then
                                    # GET result/ or callback_url
#     media:                        # optional — media ingestion off by default
#       max_bytes: 8388608
#       allow: [image/*, application/pdf]
    - type: ghhook
      path: /webhook/github/9f2c.../
      secret: ${GITHUB_WEBHOOK_SECRET}   # HMAC key; empty skips verify (dev only)
      agent: bot
    - type: telegram
      token: ${TG_TOKEN}
#     secret_token: ${TG_WEBHOOK_SECRET}  # webhook auth; empty = generated per boot
      mode: auto                 # polling (default) | webhook | auto
      allow_users: [123456789]
      agent: bot
      media:
        max_bytes: 8388608
        allow: [image/*, application/pdf]   # photos + docs feed the vision model
    - type: bridge
      name: teams
      path: /bridge/teams/
      agent: bot
      deliver_url: https://bridge.example.com/deliver
      secret: ${BRIDGE_SECRET}       # HMAC key for inbound events (required)
      token: ${BRIDGE_TOKEN}         # optional bearer on outbound deliveries
#     timeout: 10s                   # per delivery (default 10s)
    - type: openai
      name: api
      path: /v1/                     # serves <path>chat/completions and <path>models
      agents: [bot]                  # exposed as model ids
      auth: key                      # user_token | key | none (loopback only)
      keys: ["${COMPAT_API_KEY}"]    # auth: key
#     timeout: 600s                  # whole-request limit (default 600s)
#     max_request_bytes: 33554432    # POST body limit (default 32 MiB)
#     journal: metadata              # full | metadata | off; metadata is this type's default
#     expose_reasoning: false        # send model reasoning as reasoning_content
```

> **Breaking change**: the per-channel `listen:` field was removed. Configs that still carry it fail to load with `field listen not found in type config.Channel`. Move each channel's port to the single `gateway.listen`, give every HTTP channel a `path` ending in `/`, and set `gateway.public_url` for telegram webhook/auto. See the README migration checklist.

### `mcp.servers`

```
mcp:
  servers:
    filesystem:                  # stdio: the runtime spawns the server
      command: npx
      args: ["-y", "@mcp/server-filesystem", "/data"]
    affiliate:                   # Streamable HTTP: the runtime dials a running server
      url: https://mcp.internal.example/mcp
      token: ${MCP_AFFILIATE_TOKEN}   # sent as Authorization: Bearer
```

A server declares **either** `command` (spawned as a child process, spoken to over stdio) or `url` (a Streamable HTTP endpoint, dialled). Both are connected once at boot, and a server that fails to connect is skipped with a warning rather than stopping the run. The HTTP transport is what lets an integration live outside this repository entirely — a separately deployed, separately versioned service, which is how the runtime supports proprietary or licensed connectors without shipping them. `token` applies to the HTTP transport only and is sent as a bearer credential on every request; leave it unset for an endpoint that needs none.

Tools discovered at connect enter the registry as `mcp:<server>/<tool>`. If the server disconnects, invocations return structured `unavailable`.

### `search`

```
search:
  default: ollama               # engine used when a call omits `engine`
  engines:
    doubao:
      api_key: ${DOUBAO_API_KEY}   # Volcano Engine web-search Custom API
    ollama:
      api_key: ${OLLAMA_API_KEY}   # ollama.com hosted web search
    stackoverflow:
      site: stackoverflow          # optional; any StackExchange site (default stackoverflow)
      # api_key optional — anonymous quota is 300 req/day; a free app key lifts it to 10k
    github:
      # repository search — api_key optional (a token lifts 10 req/min to 30/min)
    youtube:
      api_key: ${YOUTUBE_API_KEY}  # YouTube Data API v3 — required; ~100 searches/day quota
```

Configures the engines behind the `builtin:web_search` tool, selectable per call via the tool's `engine` parameter (`tools.run("builtin:web_search", {query=..., engine="doubao"})`); omitting `engine` uses `search.default`. With no engines configured the tool reports honest-unavailable (`{ok:false, unavailable:true}`). Per-engine `base_url` and `timeout` are optional overrides. Results normalize to `{title, url, site, summary, snippet, content, published, score}`; `score` is engine-native (doubao is a 0..1 relevance, stackoverflow is the raw upvote count, github is the stargazer count; youtube has none). doubao, ollama, and youtube (and the grounded engines google\_search/x\_search) need an `api_key`; stackoverflow and github need none (the `site` field picks any StackExchange network site, e.g. `superuser` or `serverfault`). A missing or unresolvable key **skips the engine with a boot warning** instead of failing the boot; if the skipped engine was `search.default`, the default falls back to the sole surviving engine (or none). github searches repositories and honors GitHub's full qualifier syntax in the query (`language:go stars:>100 pushed:>2024-01-01`). youtube searches videos via the YouTube Data API v3 (site = channel title, content = description, a `LIVE:` snippet prefix marks live broadcasts); the shared `time_range` maps to `publishedAfter`/`publishedBefore` — note search.list costs 100 quota units per call (~100 searches/day on the default project quota), and a spent quota surfaces an honest `quotaExceeded` error.

### `legal_search`

```
legal_search:
  default: hklii                # engine used when a call omits `engine`
  engines:
    hklii: {}                   # Hong Kong Legal Information Institute — free, no key
    npc: {}                     # China National Database of Laws and Regulations — free, no key
                                # (optional base_url / timeout overrides)
```

Configures the engines behind two tools, selectable per call via the `engine` param (default `legal_search.default`); both report honest-unavailable when no engine is configured. The two engines answer differently, so the tool descriptions are assembled from whichever engines are configured — an `abbr`/`year`/`num`/`lang` parameter, for instance, is only offered when hklii is present, because npc reads none of them. The same reasoning applies to the result: a field the selected engine does not fill is left out rather than sent as an empty string, which would read as "unknown" instead of "not applicable".

`builtin:legal_search` returns hits as `{title, path, url, court, date, type?, effective?, status?, neutral?, parallel?, action?, parties?, coram?}`, with `total` the full match count. Its `disable_fuzzy` param (default false) means something different per engine — keyword-vs-concept on hklii, title-only-vs-full-text on npc. `builtin:legal_read` retrieves one document's full plain text, keyed by the hit's `path`, with `start`/`max_chars` paging for long bodies. With more than one engine configured, the `path` is routed to the engine that issued it, so `engine` need not be repeated from the search call: a hit's path is an opaque token that only its own engine can read, and requiring the caller to name that engine again is a way to fail at something the runtime already knows. Naming `engine` explicitly still wins and is never second-guessed.

**hklii** covers Hong Kong case law and legislation. Search hits carry a neutral and parallel citation, court, date, parties and coram. Reading is *case judgments only*: `path` is `/en/cases/hkcfi/2020/1808`, or give explicit `abbr`/`year`/`num`, with an optional `lang` (`en`|`tc`|`sc`). A legislation hit — one whose `court` reads `Hong Kong Ordinances`, with a path like `/en/legis/ord/201/` — has no readable full text here; its path is rejected with a clear error rather than returning something wrong.

**npc** covers PRC national laws, administrative regulations, and judicial interpretations. Hits carry the enacting organ as `court`, the promulgation date as `date`, and add `type` (法律/行政法规/…), `effective` (施行日期) and `status` (有效/已修改/已废止/尚未生效); npc has no citations and no parties, so those fields are absent. Reading takes the hit's `path` — the opaque `bbbs` document id — and nothing else: `abbr`, `year`, `num` and `lang` have no meaning here. The returned `neutral` is that same id, not a citation. Text is extracted from the returned Word document via the built-in doc/docx parser.

### `browser`

```
browser:
  account_id: ${CLOUDFLARE_ACCOUNT_ID}   # Cloudflare account id (not a secret)
  api_token: ${CLOUDFLARE_API_TOKEN}     # needs the "Browser Rendering - Edit" permission
  base_url: https://api.cloudflare.com/client/v4   # optional endpoint override
  timeout: 60s                           # per-request bound (default 60s)
```

Configures `builtin:browser`, which reads a page through Cloudflare **Browser Run** (formerly Browser Rendering) Quick Actions — a real headless browser, so JavaScript-rendered content is present where a raw HTTP fetch returns an empty shell. One tool with a per-call `action`: `markdown` returns the page as Markdown, `content` the fully rendered HTML, `links` every link, `scrape` the elements matching `selectors` (CSS), `json` the structured data described by `prompt`/`schema`, and `accessibility_tree` the accessibility tree. Either `url` or `html` selects the page; `wait_until` (`load`|`domcontentloaded`|`networkidle0`|`networkidle2`) and `wait_for_selector` control when a page counts as loaded — the `networkidle` values are what make single-page apps work. The `json` action runs an AI model on Cloudflare's side and is billed as Workers AI usage.

Long results are bounded: `max_chars` (default 20000, `0` for no cap) caps the text an action returns, and the result always reports the true length as `chars` and sets `truncated` when it cut, so a model can tell a short page from a clipped one; `links` is likewise capped at 500 entries. With no account configured the tool reports honest-unavailable (`{ok:false, unavailable:true}`), and an `api_token` that will not resolve **skips the tool with a boot warning** rather than failing the boot — the same policy every optional component follows. Setting `account_id` and `api_token` one without the other is a boot error, since `account_id` is not a secret and a half-configured block is a mistake worth catching immediately.

Three Quick Actions are deliberately not exposed: `screenshot` and `pdf` return raw bytes, which have no correct destination yet (a tool result is JSON that goes into the model's context whole, and nothing downstream truncates it), and `crawl` is asynchronous, returning a job id that must be polled.

### `net` — outbound connection policy

```
net:
  http:
    allow_private: false     # default; true disables the address guard
```

Governs **both** outbound HTTP paths — the `builtin:fetch` tool and the Lua `http.request` op — from one setting, so a deployment cannot end up with one guarded and the other not.

The guard refuses connections to loopback, RFC 1918, carrier-grade NAT, link-local (which includes the cloud metadata endpoint at `169.254.169.254`), unique-local, multicast, and the reserved / special-purpose ranges. It is installed in the **dialer**, not checked when the URL is parsed, and that placement is the whole point: the check runs after DNS resolution and immediately before connect, so it sees the address actually being dialed. A check at parse time is defeated by DNS rebinding — the name is judged once and resolves somewhere else by the time the connection is made — and every redirect hop opens its own connection, so every hop is checked by construction.

Two details worth knowing. A *zoned* address (`http://[fc00::1%25en0]/`) is refused outright and before anything else: Go's prefix matching will not match an address carrying a zone, so without an explicit denial such an address falls through every range rule and reaches a unique-local host with the guard none the wiser. And a v4-mapped v6 address (`::ffff:10.0.0.1`) is unmapped before the check, since it names the same host as its v4 form.

**Trade-off:** the guarded transports deliberately ignore `HTTP_PROXY` / `HTTPS_PROXY`. Behind an egress proxy the dialer connects to the *proxy*, so the guard would judge the proxy's address and the target would never be checked at all. A deployment that egresses only through a proxy will find these paths do not use it. Also note `198.18.0.0/15` is blocked as a benchmarking range, which is the default fake-IP pool for Clash/mihomo and sing-box in TUN mode — on a developer machine running either, every proxied domain resolves into it and will be refused.

`allow_private: true` disables the guard entirely and logs a WARN at boot naming the risk, so it cannot be switched on silently. It exists for deployments that legitimately call an internal service; it is off by default because the guard is the only thing between a prompt-injected agent and the host's own network. The guard scopes to these two agent-controlled paths only — model, search, legal and browser drivers use admin-configured `base_url`s and are deliberately not covered, so an internal mirror keeps working.

### `media` — blob store backend

```
media:
  backend: fs                 # fs (default) | s3
  dir: ./data/media           # fs root (default: beside the persistence db)
  s3:                         # required when backend: s3 — works with AWS S3 and MinIO
    bucket: my-media
    region: us-east-1
    endpoint: ~               # set for MinIO/S3-compatible (path-style); empty = AWS (virtual-hosted)
    prefix: agentflow         # object key prefix
    access_key: ${S3_ACCESS_KEY}
    secret_key: ${S3_SECRET_KEY}
```

Selects where channel-ingested media bytes live. Objects are content-addressed (`media:<sha256>` handles) regardless of backend, so loops and the audit journal never change shape when you switch. The `s3` backend speaks plain SigV4 (no SDK): repeated writes of the same bytes are deduped by a HEAD before PUT, and the secret key never appears in any error. S3 credentials are secrets (see the registry): an unresolvable pair skips the store with a boot warning (channels then run with media disabled) instead of failing the boot. Only handles cross the Lua bridge — bytes stay in the store until a provider or channel resolves them.

### `files` — user-scoped file store

```
files:
  backend: fs                 # fs (default) | s3 (AWS S3, MinIO, Cloudflare R2)
  dir: ./data/files           # fs root (default: beside the persistence db)
  s3:                         # same fields and resolution as media.s3
    bucket: my-files
    region: auto
    endpoint: https://<accountid>.r2.cloudflarestorage.com
    access_key: ${R2_ACCESS_KEY_ID}
    secret_key: ${R2_SECRET_ACCESS_KEY}
  max_file_bytes: 33554432    # per-file ceiling (default 32 MiB)
  scratch_ttl: 24h            # per-session scratch lifetime; 0 = no expiry
  gc_grace: 24h               # blob GC grace window; 0 = GC disabled
```

Backs the `files.*` Lua ops (cap `files`): user-scoped project trees with engine-native snapshot versioning, plus per-session scratch. A boot-time mark-sweep reclaims orphaned blobs (deleted files, superseded contents) past `gc_grace`: the live set is every handle referenced by tree records, commit trees (rollback stays possible), and scratch — dedupe is untouched, and the grace window (vs the put blob-then-metadata write order) makes sweeping safe without locking.

File bytes are content-addressed in the blob store exactly like media; snapshot metadata lives in the runtime store's `files_meta` table (plain sqlite, inspectable). S3-compatible endpoints need a real SigV4 key pair — a bearer token cannot speak S3. An unresolvable credential pair disables the store with a boot warning (the ops then fail honestly), never a boot failure.

### `audit` — message journal

```
audit:
  enabled: true               # default on
  retention_days: 90          # prune horizon; 0 = keep forever
```

The core journals every message to a `message_journal` table in the runtime SQLite store: inbound at the router (stamped with a stable message id and a `routed`/`dropped_queue` status), outbound at each session's egress (`delivered` / `failed` / `blocked_safety`, correlated to the inbound id it replies to). Rows carry channel, chat, sender, agent, session, text, attachment descriptors, text and attachment sizes, and provenance. A channel's `journal:` setting narrows that: `metadata` keeps the row and its sizes without the text or attachments, and `off` writes no row for that channel. This is a trust-boundary feature: loops and channels can neither bypass nor forge it — media is recorded by handle only, keeping the audit trail small. Pruning runs at boot and daily against `retention_days`.

### `usage` — token ledger

```
usage:
  events: false               # per-call detail rows (default off; the daily rollup is always written)
  retention_days: 30          # prune horizon for the ledger; 0 = keep forever
  default_tokens_per_day: 0   # per-user daily limit; 0 = unlimited. A profile's own limit overrides it
```

The daily rollup (`usage_daily`) feeds every quota and agent budget decision and stays in the runtime store. Per-call events go to the log plane when one is configured. `GET /admin/api/usage` and Studio's Usage screen read the rollup.

### `tools.policy`

```
tools:
  policy:
    default: all          # all | none
    forbidden: [social_post, auto_purchase]
    overrides:
      "builtin:fs.write": { needs_confirm: true }
      "mcp:github/create_issue": { needs_confirm: true }
      "builtin:web_search":
        description: "Search the deployment's runbooks."   # verbatim replacement,
                                                           # or {prompt: } from prompts:
        params:                            # shallow-merged into parameters.properties
          query: { description: "What to look up, in the user's words" }
        needs_confirm: false               # policy fields: needs_confirm, permission
        autonomous: true                   #   (cost_level / user_visible / autonomous are
                                           #    parsed and read by nothing — boot warns)
```

Overrides are baked into the registry once at boot, after every tool is registered (builtins, legal, shell, MCP) and before any agent set is exposed. A `params` entry naming a param the schema does not declare is skipped with a warning, never an error. Policy fields apply per tool; `permission: forbidden` removes the tool from every exposed set — and is the only one of them with an effect. `cost_level`, `user_visible` and `autonomous` are accepted, copied onto the tool, and consulted by nothing; `tools.policy.write` is likewise parsed and unused. The engine warns at boot when one is set.

**Reaching Lua-declared tools**: an override may also name a tool a loop declares with `tool.def` — see below. Those entries are not baked at boot (the tool does not exist until a chunk loads); they are resolved when the loop builds its list, against the live prompt registry, so editing a `file:`-backed prompt shows up on the next `tools.list()`. A misspelled *prompt key* is still a boot error for either kind of tool. A tool name that is neither registered nor declared by any loaded loop cannot be caught at boot, so it is logged once at boot as held for a Lua declaration (one informational line naming them, so a typo is visible before any agent takes a turn) and then warned about once per name the first time a loop declares its tools without claiming it.

### `agents`

```
agents:
  bot:
    loop: ./loop.lua
    model: default
    instructions: ./prompts/bot.md     # or {prompt: assistant_system} — see prompts:
    capabilities: [llm.chat, memory, tools, agent.send, shell.exec, net.mail]
    skills: [builtin:web_search, builtin:fs.read]
    memory: conversational     # preset, or inline
    safety: default
    shell: sandboxed                    # optional
    can_contact: [worker]               # ACL
    budget:
      tokens_per_day: 500000
      window: 24h                       # optional rolling window; omit for daily reset
    channels: [webhook]
    persistent: true                    # optional — daemon agent (see below)
    credentials: [deploy_token]         # optional — credential.get allow-list
    inherit_user: false                 # optional — run tenant-less even when delegated to
    extras:                             # optional — read-only via agent.config()
      goal: { type: autonomous, success_signal: "PR merged", max_turns: 40 }
      workflow: release-train
```

**Capability ceiling**: `plugins.allow_capabilities` (global) ⊇ `agents.<name>.capabilities` (per-agent) ⊇ spawn-spec grants (child). You cannot delegate what you lack. If an agent omits `capabilities` it gets the defaults `[llm.chat, memory, tools, agent.send, net.http, users]` — note `net.mail`, `shell.exec` and `session.state` are *not* defaults and must be declared explicitly.

**Capabilities are enforced at runtime**, not merely validated at boot. Each capability gates the Lua ops behind it: `llm.chat` → `llm.*`, `memory` → `store.*`, `tools` → `tools.*`, `shell.exec` → `shell.*`, `net.http` → `http.request` and `os.env`, `net.mail` → `mail.*`, `files` → `files.*`. An agent that omits one is refused with a message naming the op, the agent and the missing capability — the op stays addressable and fails clearly rather than vanishing into *"attempt to call a nil value"*. `agent.send`, `agent.request` and `agent.spawn` were always enforced this way; the rest now match. `runtime.*` and `credential.get` are deliberately ungated — the first is loop machinery every agent needs, the second is already scoped by the agent's own credential list. A boot warning lists exactly what was withheld from each agent and profile. Set `plugins.enforce_capabilities: false` (default `true`) only to keep a configuration running that depended on every agent having every op.

**`builtin:` is the tool namespace.** Four vocabularies used to spell it — loop/plugin names, tool names, memory provider names, and the built-in memory profile — and nothing in the string said which one a name belonged to. Each now has its own spelling, so the prefix means exactly one thing:

- **tools** keep it: `builtin:web_search`, `builtin:fetch`, `builtin:browser`, `builtin:legal_read`;
- **loops and plugins** are `plugin:<name>`: `plugin:per_chat`, `plugin:recency`, `plugin:semantic`. This also settles the loop-versus-path question outright, since a path never begins with `plugin:` — where before a file named `per_chat` was unreachable;
- **memory providers** are bare: `sqlite`, `qdrant`, `redisvector` — the `provider:` key already says what kind of name it is;
- the **built-in memory profile** is `conversational`, and that name is now reserved: a `profiles.memory` entry of the same name is a boot error rather than a profile that could never run.

The old spellings are recognized by name rather than reported as unknown, so the error says what to write: a `loop:` of `builtin:per_chat` is answered with `loops are spelled plugin: now`, and a provider of `builtin:sqlite` with `providers are unprefixed now`. Two other mistakes are reported too — a `loop:` holding a *tool* name fails the boot with a hint naming the field it belongs in, and a `skills:` entry holding a loop name warns, because that one is otherwise silent (it matches no tool, is dropped, and the agent simply looks toolless). A `skills:` entry matching no registered tool at all is *not* reported, since a loop may legitimately list tools it declares itself with `tool.def`.

**Profile extras & credentials**: `extras` is deployment-specific, read-only profile data surfaced to the loop by `agent.config()` (a `goal` block is hoisted to a top-level `goal` key; everything else lands under `extras`). Values that are *exactly* a secret reference — `${VAR}`, `${VAR:-default}`, or `cred:<service>` — are never resolved for this surface: on the configdir path they stay raw and render as opaque markers (`{env="VAR"}` / `{cred="service"}`), so no resolved secret reaches a loop (or a spawned child); a string that merely embeds a reference (`prefix-${VAR}`) is an ordinary literal and expands as usual. `credentials` is the allow-list for `credential.get()`: an agent can only fetch a stored secret it names here; empty or missing = no access. Both fields apply to spawn profiles too and reach every child.

**Daemon agents**: sessions normally spawn lazily on first message. An agent with `persistent: true` receives a synthetic `{ type = "boot", from = "system:supervisor" }` message once the runtime is up (after channels register), so cron schedulers and queue consumers start without external traffic.

**Tenant scope follows a delegation**: an agent hop (`agent.send` / `agent.request` / `agent.reply`) carries the sending turn's tenant on the message's core-stamped provenance, and the receiving agent acts as that tenant — so a fleet working for one person writes that person's files, memory and shell scope at every hop, not one fleet-wide scope. The stamp is taken from the sender’s engine-assigned identity and never from the payload, so a loop cannot name another tenant (`payload.user_uuid` is ignored, whoever wrote it). The chain can only attenuate: an agent has the tenant it was given or none. `inherit_user: false` opts an agent out of inheriting across a hop — it then runs tenant-less in the shared `service` stratum — while a person talking to it directly still reaches it under their own scope. Available on spawn profiles too.

**Budget**: `tokens_per_day` wraps LLM calls in reserve/commit/release against a token pool. By default the pool resets daily; set `window` (e.g. `24h`, `168h`) for rolling-window accounting where usage drains continuously as commits age out — no midnight cliff. A call that would exceed the budget is denied (counted in `agentflow_budget_denied`).

### `prompts` — a deployment-wide prompt registry

```
prompts:
  assistant_system:
    file: ./prompts/assistant.md       # resolved relative to the configdir
  distiller:
    inline: "You are a memory distiller..."   # literal (text: is an alias)
  kb_header:
    inline: "[Knowledge base — recalled facts with provenance]"
```

One place for system-prompt, tool-prompt, injection-prompt and distillation text, so it is not hardcoded in Lua or duplicated across files. Each entry names *exactly one* of `file:`, `inline:`, or `text:` (an alias for `inline:`); a `file:` that cannot be read fails the boot, and a profile or tool override naming a key that is not defined fails the boot — a missing prompt never degrades into a silently empty string. Prompt files are read at load time (and re-read when they change) and resolved relative to the configdir (single-file configs resolve them relative to the config file itself). The resolved text is surfaced read-only to every loop as `agent.config().prompts` (an empty table when no prompts are declared), keyed by the same names.

**Referencing a prompt**: `instructions` on an agent or spawn profile accepts either a file path (as before) or `{prompt: <key>}`, which makes the registry text that agent's system prompt. Tool overrides accept the same form for a description: `description: {prompt: <key>}` alongside the existing literal string. A `file:`-backed entry is *live*: editing the file republishes its text to every loop's `agent.config().prompts` and refreshes in place the system prompt of any agent sourced from that key, with no session restart (an unreadable file warns and keeps the text already published). The `prompts:` block itself is not live — adding a key, re-pointing one at a different file, or switching an entry between `inline:` and `file:` still needs a restart.

### `plugins`

```
plugins:
  dir: ./plugins
  allow_capabilities:           # global ceiling — nothing exceeds this
    - llm.chat
    - memory
    - tools
    - scheduler
    - shell.exec
    - net.http
    - net.mail
    - channel.push
    - agent.spawn
    - agent.send
    - agent.request
  enforce_capabilities: true    # default; false restores pre-enforcement behavior
```

`plugins.dir` also **shadows builtins by name**: a file `<dir>/<name>.lua` (e.g. `./plugins/recency.lua`) replaces the embedded builtin of that name everywhere — loop/route refs (`loop: plugin:recency`, including the hot-reload watch path) and the support chunks loaded into every session. The shadow files are watched: a shadowed support chunk is re-read at every session start (the first one and each reload restart), and an edit restarts every live session, so it takes effect without a process restart. An unshadowed chunk is embedded and has no file to edit.

### `triggers` — scheduled and event tasks

```
triggers:                       # top-level, or triggers/*.yaml in a config directory
  - name: morning-digest
    cron: "30 9 * * 1-5"        # kind = cron  (5 fields)
    every: 15m                  # kind = every (ms, s, m, h, d)
    event: { channel: webhook, match: urgent }   # kind = event
    run_on_boot: false          # fire once at startup, then follow the schedule
    target: { profile: pm }     # a configured agent, or a spawn profile
    payload: { topic: news }    # copied verbatim into the fired message
```

**`every:` and `cron:` triggers are executed by the engine**, not by an agent: when one is due the runtime enqueues an ordinary message into the target's mailbox —

```
{ id = "cron:<trigger-name>:<unix>", type = "cron", from = "system:scheduler",
  payload = { ... }, ts = ..., provenance = { kind = "scheduler", principal = "trigger:<name>" } }
```

— into session `<target>|<trigger-name>`. No agent holds a capability for this and no loop needs a timer: scheduling is engine infrastructure, so a deployment can drop the user-space scheduler agent it used to run. Deliveries always journal and count (`agentflow_trigger_fires`), and fire at `info` (`debug` carries the next computed occurrence).

- `every:` takes a duration in `ms`, `s`, `m`, `h`, or `d`, compound and fractional allowed (`30s`, `15m`, `6h`, `1d`, `1.5h`, `2d12h`). The first fire is one interval after startup (or immediately with `run_on_boot`); subsequent fires are anchored to that first tick, so they do not drift. The shortest interval is 100ms.
- `cron:` takes the classic 5 fields (`minute hour day-of-month month day-of-week`) with `*`, `N`, `N-M`, `*/N`, `N-M/N`, and comma-separated lists. Day-of-week accepts `0`–`7` (both `0` and `7` are Sunday). When day-of-month *and* day-of-week are both restricted, a day matching *either* fires (the classic rule). Names (`mon`, `jan`) are not supported, and there is no seconds field.
- `cron:` fields are matched in `runtime.timezone_offset_hours` (a fixed UTC offset, default 0 = UTC). It is an offset, not a named zone, so a schedule never shifts under another region's daylight saving — `30 9 * * 1-5` with `8` means 09:30 UTC+8, every day of the year.
- `target.profile` names a configured agent or a spawn profile. A spawn profile resolves to the `spawn:<profile>` address — a top-level session running that profile's loop, model, and grants (no parent, no capability attenuation to a parent).
- `event:` triggers are **not** scheduled: the router and loops match those themselves. `runtime.triggers()` still returns the whole merged list, unchanged in shape, so loops that read it keep working.
- A trigger that cannot be scheduled — a malformed expression, an interval under the floor, or a `target.profile` that is neither an agent nor a spawn profile — is **logged and skipped**; the rest still run. A config typo costs you one trigger and one warning line, never a boot.
