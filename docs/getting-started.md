# AgentFlow

A pure-Go, single-binary, self-hosted LLM agent runtime with embedded TypeScript/JS loops and Wasm External APIs. Configure the ordinary; program the exceptional.

## What it is

AgentFlow runs LLM-powered agents that can remember, use tools, execute shell commands, schedule background work, talk to other agents, and serve users through webhooks, Telegram, QQ, and OpenAI-compatible clients. Every agent is defined declaratively in YAML; all behavior beyond the defaults comes from hot-reloadable TypeScript/JS plugins that hook into versioned interfaces exposed by the core.

## Design principles

- **Core provides capabilities; plugins provide behavior; YAML provides wiring.**
- **One VM state per session** — actor model, message-passing only.
- **Failure defaults to truthful** — unavailable tools return structured errors, never fabricated results.
- **Builtins are plugins** — shipped in the binary, shadowable by name, same contract as user plugins.
- **The capability boundary is a message protocol** — not a language API.

## Stack

| Decision | Choice | Rationale |
| --- | --- | --- |
| Runtime | Go (`CGO_ENABLED=0`) | Pure-Go single binary; goroutine-per-actor; trivial cross-compilation |
| Script & External APIs | TypeScript/JS (`goja`) + Wasm (`wazero`) | Pure Go (no cgo); TypeScript loops & extensions; isolated Wasm External API modules |
| Concurrency | Actors + worker pool | Script VM states park on async ops; no OS threads burned |
| UI & Docs | AgentFlow Studio | Headless runtime serves `/admin/api/`; Studio provides the desktop/web workbench and offline docs |

> **Current status**: The core runtime (actors, memory, tools, shell, multi-agent, scheduler, budgets, safety, metrics) is implemented and tested. Storage backends: SQLite, Redis and PostgreSQL for key/value and document rows, an in-memory volatile store, and vector search through pgvector, Qdrant, Redis Stack/RediSearch, PostgreSQL with the vector extension, or SQLite's capped brute-force scan — never a silent empty result. LLM providers cover four chat API shapes (Anthropic Messages, OpenAI Chat Completions, OpenAI Responses, Gemini Interactions) plus a `rerank` provider, each pointed at an official host or any compatible endpoint via `base_url`; per-model `server_tools` enable provider-native tools such as Google Search grounding. Channels are webhook (sync or async), GitHub webhook (`ghhook`), Telegram (polling, webhook, or self-configuring `auto`), QQ Bot, `openai` (Chat Completions), and `bridge`, an HMAC-signed adapter protocol that lets an out-of-process service connect any other chat platform. They all mount on one shared HTTP listener with a built-in `/health` probe. Beyond that the runtime ships a unified media/files store, a core-owned audit journal, embeddings and reranking for semantic memory, the identity and user layer (profiles, channel linking, OIDC, per-user quotas), tenancy (groups, memberships, shared memory pools and grants), a user-scoped file store with snapshot versioning, an encrypted credential store, engine-fired scheduled triggers, config directories, the append-only log plane, single and regional deployment tiers, MCP servers over stdio or Streamable HTTP, declarative HTTP tools, and the headless admin API (`/admin/api/`) that AgentFlow Studio connects to.

## Install Guide

### Prerequisites

- **Go 1.26+** — AgentFlow is 100% pure Go (`CGO_ENABLED=0`). No C/C++ compiler, no git submodules, and no `zig` are required on any OS.
- **GNU make** (optional) — convenience wrapper around `go build`, `go test`, and `go vet`.

### Windows / Linux / macOS

1. Install [Go](https://go.dev/dl/) 1.26 or newer.
2. Clone and build:

```
git clone https://github.com/hulipa487/agentflow.git
cd agentflow
go build -o agentflow ./cmd/agentflow   # or agentflow.exe on Windows, or simply `make`
go test ./...
go vet ./...
```

Cross-compiling works natively with Go's standard environment variables (for example `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/agentflow`, or `make TARGET_OS=linux`).

### Or start from a template

`agentflow init` writes a runnable starter deployment (`-layout dir` for a config directory), and `agentflow validate` and `agentflow doctor` check a config without starting the engine:

```
./agentflow init                 # writes agentflow.yaml
./agentflow validate             # load the config and compile every loop
./agentflow doctor               # offline checks: loops, credentials, models, channels
./agentflow doctor -online      # also reach the stores and backends (default for regional)
./agentflow                      # run; discovers agentflow.yaml, then config.yaml
```

### Verify the install

Save a minimal echo loop as `echo.ts` (the first line is the prelude API contract — a chunk without it is refused):

```
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    session.send("echo: " + (msg.text || ""));
  }
}
```

and a no-model config that wires it to a webhook as `agentflow.yaml`:

```
version: "1"
agents:
  echo:
    loop: ./echo.ts
gateway:
  listen: ":8080"
  channels:
    - type: webhook
      path: /webhook/
      agent: echo
```

Run it and hit the webhook with curl:

```
./agentflow run -config agentflow.yaml
# in another terminal:
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'
```

You should see `echo: hello` in the response. Note the trailing slash — the webhook mounts the `/webhook/` subtree, and a POST to bare `/webhook` returns a `307` redirect that `curl -d` will not follow. Everything is on that one port, including the admin plane's probes:

```
curl http://127.0.0.1:8080/healthz   # liveness
curl http://127.0.0.1:8080/readyz    # readiness (503 while starting or draining)
curl http://127.0.0.1:8080/health    # the channel listener's own probe -> {"ok":true}
```

### Troubleshooting

| Symptom | Fix |
| --- | --- |
| `Nothing to be done for 'all'` after switching branches | The Makefile tracks Go sources and embedded script files, so this is rare. If the binary still looks stale, run `make clean && make` or `go build ./cmd/agentflow`. |
| Tests fail with SQLite / database locked | Run tests serially (`go test -p 1 ./...`) or remove stale `data/` directory between runs. |

### Updating

When a new version lands, pull and rebuild cleanly:

```
git pull
go build -o agentflow ./cmd/agentflow   # or `make clean && make`
```

## Quick Start

### Build

AgentFlow is pure Go (`CGO_ENABLED=0`) — build directly with `go` or `make`:

```
go build -o agentflow ./cmd/agentflow   # or `make`
go test ./...                           # or `make test`
```

### Run

The fastest path needs no model — a file-based echo loop replies out of the box. Point the binary at your config:

```
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'   # note trailing /
```

For an LLM-backed bot, point a real model at it (next block) and set an agent's `model` to that entry.

### Or start from a scenario

AgentFlow Studio hosts complete sample deployments under [`examples/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples): a config, its TypeScript loop, persona prompt, `.env.example` and a README each. Copy the one closest to what you want, or point the runtime binary at it:

| Scenario | What it shows |
| --- | --- |
| [`qq-group-bot-fts`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/qq-group-bot-fts) | QQ Group Chatbot (FTS5): SQLite FTS5 keyword recall (`plugin:recency`), shared per-group context, proactive `[NO_REPLY]` participation, Doubao search, and `fetch` |
| [`qq-group-bot-vec`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/qq-group-bot-vec) | QQ Group Chatbot (Embedding): `bge-m3` SQLite vector semantic recall (`plugin:semantic`), shared per-group context, proactive `[NO_REPLY]` participation, Doubao search, and `fetch` |
| [`telegram-group-bot-fts`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/telegram-group-bot-fts) | Telegram Group Chatbot (FTS5): SQLite FTS5 keyword recall (`plugin:recency`), shared per-group context, proactive `[NO_REPLY]` participation, Google/X search, `fetch`, and `browser` |
| [`telegram-group-bot-vec`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/telegram-group-bot-vec) | Telegram Group Chatbot (Embedding): `bge-m3` SQLite vector semantic recall (`plugin:semantic`), shared per-group context, proactive `[NO_REPLY]` participation, Google/X search, `fetch`, and `browser` |

```
./agentflow validate -config examples/qq-group-bot-fts/agentflow.yaml
./agentflow run -config examples/qq-group-bot-fts/agentflow.yaml
```

Studio's **New agent** dialog offers the same four templates (**QQ Group Chatbot (FTS5)**, **QQ Group Chatbot (Embedding)**, **Telegram Group Chatbot (FTS5)**, and **Telegram Group Chatbot (Embedding)**) with zero-TS **Starter settings** when creating an agent in the builder.

### Connect a real model

`provider` picks the wire format. Leave `base_url` empty to use that provider's official host, or set it to any compatible endpoint (Ollama, vLLM, LiteLLM, OpenRouter, a corporate gateway):

```
models:
  default:
    provider: openai          # API shape: anthropic | openai | openai-responses | gemini | rerank
    model: gpt-4o
    base_url: http://127.0.0.1:11434/v1   # Ollama / vLLM / LiteLLM / OpenRouter
    api_key: ${OPENAI_API_KEY}
    thinking: medium        # off | low | medium | high | xhigh | max; empty = provider default
    server_tools: []          # provider-native tools (gemini: google_search; openai-responses: web_search)
```

`provider` selects the request/response format. `base_url` selects the host. `api_key` may be empty for keyless local endpoints (Ollama) — except on `gemini`, whose client refuses to build without one. `thinking` sets the model's default thinking level — one provider-neutral vocabulary (`off` | `low` | `medium` | `high` | `xhigh` | `max`) that the runtime maps to each provider's native knob: an Anthropic `budget_tokens`, an OpenAI `reasoning_effort`, or a Gemini `thinking_level` (plus `thinking_summaries`, which is what asks for the summaries back). Empty sends nothing and the provider uses its own default; overridable per call via `llm.chat` `opts.thinking`. Two Gemini limits follow from its SDK: the request has no rung below `minimal`, so `off` maps to `minimal` rather than disabling thinking, and its generation config has no temperature field, so `llm.chat{temperature=...}` is refused by name there. `server_tools` names provider-native server-side tools that run inside the provider's completion — the runtime injects only the native tool entry, no client-side schema.
