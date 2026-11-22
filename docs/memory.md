# Memory

Memory is what an agent keeps between turns: the recent conversation, facts it
was told, documents it can search. This page explains how it fits together, how
to set it up in a config file or in Studio, and how a loop uses it.

There are three layers:

| Layer | Lives in | What it is |
| --- | --- | --- |
| **Backend** | `memory.backends.<name>` | A database: a SQLite file, a PostgreSQL or Redis server, a Qdrant cluster, or process memory. |
| **Profile** | `profiles.memory.<name>` | A named set of logical **stores**, each a table on a backend, plus how `memory.recall` searches them. |
| **Agent** | `agents.<name>.memory` | Which profile the agent uses: a profile name, the built-in `conversational`, or an inline profile. |

Several agents can share one profile. Each still gets its own tables (see
[Isolation](#isolation)), so sharing a profile shares the shape, not the data.

## The two-minute version

A config with no memory section at all already works:

```yaml
agents:
  assistant:
    loop: ./loops/chat.lua
    memory: conversational        # the built-in profile
```

`conversational` stores the conversation in a `dialogue` table (30 days, the
latest 1000 records) and facts in a searchable `facts` table (kept forever), on
a backend called `main_db`. When the config declares no backends, `main_db` is
the runtime store itself (`runtime.persistence`), so a minimal deployment needs
no second database.

In a loop:

```lua
memory.write({ type = "turn", id = msg.id, text = msg.text, reply = reply.text })
local recent = memory.recall({ kind = "all" })          -- records from dialogue
```

The agent needs the `memory` capability, which is on by default.

## Backends

```yaml
memory:
  backends:
    main_db:
      provider: sqlite
      config: { path: ./data/agentflow.db }
    vectors:
      provider: pgvector
      config: { url: "${PG_URL}", dim: 1536 }
```

| Provider | Good for | Settings it reads |
| --- | --- | --- |
| `sqlite` | One instance, no server. Key-value, full-text and brute-force vector search. | `path`, `vector_scan_limit` |
| `postgres` | A shared server. Key-value and full-text; vectors too when pgvector is installed. | `url`, `dim` |
| `pgvector` | PostgreSQL vector search (kept for compatibility; `postgres` covers it). | `url`, `dim` |
| `redis` | Fast shared key-value, no search. | `url` |
| `redisvector` | Redis Stack: key-value plus vector search. | `url`, `dim` |
| `qdrant` | Vector search at scale. | `url`, `api_key`, `collection`, `dim`, `distance`, `timeout` |
| `volatile` | Process memory: fast, lost on restart, shared by every session. | `max_entries` |

- **Secrets.** `url`, `password` and `api_key` accept `${VAR}`, `${VAR:-default}`
  or `cred:<service>`, so the secret stays out of the file. A backend whose
  secret cannot be resolved is skipped with a warning, and its stores move to a
  surviving backend.
- **`dim`** is fixed when the vector table is first created. It must match the
  embedding model (1536 for `text-embedding-3-small`, 768 for many open models).
- **`main_db`** is the built-in profile's backend. Once the config declares any
  backend, declare `main_db` too, or agents on `conversational` fall back to
  `./data/agentflow.db`. Studio's Memory screen writes it out for you.

The full provider table, locality rules and Qdrant transport details are in
[Configuration](configuration.md).

## Profiles

```yaml
profiles:
  memory:
    group_chat:
      stores:
        dialogue: { backend: main_db, table: group_dialogue, retention: 7d, window: 500, requires: [kv] }
        facts:    { backend: main_db, table: group_facts, retention: forever, requires: [kv, text_search] }
      write: [plugin:routing_table]
      recall: plugin:recency
```

Profile names are 1–64 letters, digits, `_`, `.` or `-`. `conversational` is
reserved for the built-in profile.

### Stores

A store is the name a loop uses (`dialogue`, `facts`, `semantic`, or your own)
bound to a physical table on a backend.

| Key | Meaning |
| --- | --- |
| `backend` | Required. A `memory.backends` name. |
| `table` | The physical table or collection. |
| `retention` | How long a record lives after it is written: `30d`, `12h`, `2d12h`, or `forever`. It is the default TTL of each write, not a sweep, so rows written before you set it never expire. |
| `window` | The most records kept per table, trimmed by a periodic sweep. Key-value backends only; the vector backends do not trim. |
| `requires` | Features the backend must have: `kv`, `text_search`, `vector`. Checked at boot, so a store on the wrong backend fails then rather than at the first recall. |
| `scope` | `user` (the default) keeps rows per user; `agent` shares one set across every user of the agent. |
| `pool` | Binds the store to a named shared pool from Tenancy instead. Grants decide who reads and writes it. A pool-bound store takes no `scope`. |
| `locality_ok` | In a regional cell, accept a durable store on an instance-local backend. |

The built-in plugins look for three store names:

- **`dialogue`** gets `turn` records and is what `memory.recall` reads by default.
- **`facts`** gets `fact` records. Search it with `kind = "text"` and `store = "facts"`.
- **`semantic`** is what semantic recall searches; without one it searches `dialogue`.

### Recall

`recall` picks the handler `memory.recall` dispatches to.

| Value | What `memory.recall` does |
| --- | --- |
| `plugin:recency` (default) | Reads a store directly: the latest records, a prefix scan, a full-text search or a time window. |
| `plugin:semantic` | Embeds the query text, runs a vector search, and optionally reranks. A query without `text` falls back to recency. |

Semantic recall needs three things on the profile:

```yaml
    knowledge:
      stores:
        semantic: { backend: vectors, table: passages, retention: forever, requires: [vector] }
        dialogue: { backend: main_db, table: kb_dialogue, retention: 30d, window: 200 }
      recall: plugin:semantic
      embed_model: embed          # a models: entry that can embed
      rerank_model: rerank        # optional: a models: entry with provider: rerank
      oversample: 4               # with a reranker: fetch k*4 hits, rerank down to k
```

- **`embed_model`** embeds the query at recall time. It also embeds each record
  `memory.write` sends to a store whose backend can hold vectors, so writes and
  queries use the same model.
- **`rerank_model`** is optional. When set, the vector search fetches
  `k × oversample` hits and a cross-encoder orders them; each result carries
  `meta.rerank_score`.

### Write

`write` lists the write chain. Today `memory.write` always runs the routing
table: the shipped `plugin:routing_table`, or `plugins/routing_table.lua` when
your deployment shadows it. The list itself is passed to loops as
`agent.info().memory.write`, so a loop can read it, but it does not switch the
chain. Leave it as `[plugin:routing_table]`.

## Giving an agent memory

```yaml
agents:
  helper:
    loop: ./loops/helper.lua
    memory: group_chat            # a named profile

  scratch:
    loop: ./loops/scratch.lua
    memory:                       # an inline profile, for one agent only
      stores:
        dialogue: { backend: main_db, table: scratch, retention: 1h }
```

- **Capability.** An agent with memory needs the `memory` capability. It is in
  the default set; if you list `capabilities:` yourself, include it.
- **Spawn templates.** `profiles.agent.<name>.memory` names a profile the same way.
- **No memory.** Leave `memory:` out. `memory.write` then has nowhere to route,
  and recall returns an empty list.

## Using memory from a loop

### Writing

```lua
-- A turn: routed to "dialogue", keyed turn:<id>.
memory.write({ type = "turn", id = msg.id, text = msg.text, reply = reply.text })

-- A fact: routed to "facts", keyed by key.
memory.write({ type = "fact", key = "name:" .. msg.from, text = "Their name is Ada." })

-- Explicit targets: any store, any key, an optional ttl in seconds.
memory.write({ targets = {
  { store = "notes", key = "todo:" .. msg.id, value = { text = msg.text }, ttl = 3600 },
} })
```

A record goes only to stores the profile declares: a `turn` written by an
agent whose profile has no `dialogue` store is dropped, not an error.

### Recalling

`memory.recall` returns a list of `{ key, value, meta }`.

```lua
-- plugin:recency
memory.recall({ kind = "all" })                                  -- dialogue
memory.recall({ kind = "text", store = "facts", text = "name", k = 5 })
memory.recall({ kind = "prefix", store = "notes", prefix = "todo:" })
memory.recall({ kind = "time", store = "dialogue",
                from = "2026-11-01T00:00:00Z", to = "2026-11-02T00:00:00Z" })

-- plugin:semantic
memory.recall({ text = msg.text, k = 5 })                        -- semantic, else dialogue
```

Without a `store`, recency reads `dialogue`, so name the store for a text
search on `facts`. Time bounds are RFC3339 strings.

### Raw tables

`store.put`, `store.get`, `store.query` and `store.delete` work on a store by
name with no routing or embedding. They are covered in [Lua loops](loops.md#memory).

## Isolation

- **Per agent.** Each agent binds its own physical tables (`<agent>.<table>`),
  so two agents on one profile never read each other's rows.
- **Per user.** Within an agent, rows are kept per user by default: one person's
  turns do not show up in another's recall. `scope: agent` turns that off for a
  store, for knowledge every user should see.
- **Shared on purpose.** `pool: <name>` binds a store to a pool created on the
  Tenancy screen. Grants decide which tenants may read or write it.

## Retention, window and bounds

- `retention` is a TTL stamped at write time; changing it later affects new rows only.
- `window` trims the oldest rows of a key-value table on a periodic sweep.
- A store on a `volatile` backend must be bounded: either the backend sets
  `max_entries` or the store sets a `retention`. Otherwise validation refuses
  it, because process memory shared by every session would grow until the
  process dies.
- In a regional cell, `dialogue`, `facts` and (with semantic recall) `semantic`
  must be on a shared backend, not SQLite or `volatile`, unless the store sets
  `locality_ok: true`.

## Setting it up in Studio

**Memory** (under Connect, in both apps) edits `memory.backends` and
`profiles.memory` without touching YAML.

- **Backends.** Pick a provider and the form shows only the settings that
  provider reads. URLs and API keys are write only: the screen shows a
  `${VAR}` or `cred:` reference, or the URL with its password masked, and
  leaving the field empty keeps what is stored.
- **Profiles.** Each store is one row: name, backend, table, retention, window,
  the features it requires, and its scope or pool. Below the stores are recall,
  write, the embed and rerank models, and oversample. A new profile starts as a
  copy of the built-in one.
- **Validate** checks the change against the whole config, the same way a
  restart would, without writing anything. **Save** validates again, keeps a
  `.bak`, and writes only those two sections.
- **Delete** is refused while something uses the entry; the screen names what.
- **The implicit `main_db`.** A config with no backends shows `main_db` as
  built-in. Saving your first backend writes `main_db` into the file as well, so
  agents on `conversational` keep their store.

In the agent builder, **Memory profile** (beside the safety profile) offers
*none*, *conversational (built-in)*, every named profile, and an inline profile
the agent already has, which is kept as written.

Agents open their stores when agentflow starts, so memory changes take effect
after a restart. A profile created on the Memory screen also appears in the
builder's picker after that restart.

The screen uses these admin routes:

| Route | Does |
| --- | --- |
| `GET /admin/api/studio/memory` | Backends (secrets as references or masked URLs), profiles, what uses each, and the providers, models and plugins an editor can offer. |
| `POST /admin/api/studio/memory/validate` | Dry run of a profile or backend draft. |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | Create, replace or remove one. Names are immutable. |

## Recipes

### Group chat: recent context plus remembered facts

```yaml
profiles:
  memory:
    group_chat:
      stores:
        dialogue: { backend: main_db, table: group_dialogue, retention: 7d, window: 500, requires: [kv], scope: agent }
        facts:    { backend: main_db, table: group_facts, retention: forever, requires: [kv, text_search], scope: agent }
      recall: plugin:recency
```

`scope: agent` lets everyone in the group share one history. The
`examples/qq-group-chat` scenario in `agentflow-studio` is a complete
version.

### Knowledge base on SQLite, no extra server

```yaml
models:
  embed: { provider: openai, model: text-embedding-3-small, api_key: "${OPENAI_API_KEY}" }

profiles:
  memory:
    knowledge:
      stores:
        semantic: { backend: main_db, table: passages, retention: forever, requires: [vector], scope: agent }
      recall: plugin:semantic
      embed_model: embed
```

SQLite searches vectors by brute force, capped by `vector_scan_limit` (50,000
rows by default). Past that, move the store to `pgvector` or `qdrant`; the loop
does not change. See `examples/telegram-rag`.

### Scratch space in process memory

```yaml
memory:
  backends:
    main_db: { provider: sqlite, config: { path: ./data/agentflow.db } }
    cache:   { provider: volatile, config: { max_entries: 10000 } }

profiles:
  memory:
    with_scratch:
      stores:
        dialogue: { backend: main_db, table: dialogue, retention: 30d }
        scratch:  { backend: cache, table: scratch, retention: 1h }
```

## When validation says no

| Message | What to change |
| --- | --- |
| `agent "x" uses memory but lacks memory capability` | Add `memory` to the agent's `capabilities`, or remove the list to use the defaults. |
| `agent "x" references unknown memory profile "p"` | Define `profiles.memory.p`, or fix the name. A profile created in Studio needs a restart before an agent can use it. |
| `store "s" references unknown backend "b"` | Define `memory.backends.b`. Once any backend is declared, `main_db` must be declared too. |
| `profiles.memory defines "conversational", which is reserved` | Pick another name. |
| `invalid retention "soon"` | Use `ms`, `s`, `m`, `h`, `d` units or `forever`. |
| `... volatile backend ... grows without bound` | Set `max_entries` on the backend or a `retention` on the store. |
| `regional topology: ... instance-local` | Move the store to a shared backend, or set `locality_ok: true` if losing it is acceptable. |
| `recall plugin:semantic needs an embed model` (Studio) | Choose an embed model on the profile. |
| `a postgres backend needs a URL` (Studio) | Fill in the URL, or a `${VAR}` / `cred:` reference to it. |
