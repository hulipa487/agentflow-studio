# AgentFlow

A single-binary, self-hosted LLM agent runtime built in Go and Luau. Configure the ordinary; program the exceptional.

## What it is

AgentFlow runs LLM-powered agents that can remember, use tools, execute shell commands, schedule background work, talk to other agents, and serve users through webhooks and Telegram. Every agent is defined declaratively in YAML; all behavior beyond the defaults comes from hot-reloadable Lua plugins that hook into versioned interfaces exposed by the core.

## Design principles

- **Core provides capabilities; plugins provide behavior; YAML provides wiring.**
- **One Luau state per session** — actor model, message-passing only.
- **Failure defaults to truthful** — unavailable tools return structured errors, never fabricated results.
- **Builtins are plugins** — shipped in the binary, shadowable by name, same contract as user plugins.
- **The capability boundary is a message protocol** — not a language API.

## Stack

| Decision | Choice | Rationale |
| --- | --- | --- |
| Runtime | Go | Orchestration-shaped; goroutine-per-actor; driver ecosystem |
| Script | Luau (cgo) | Untrusted scripts; native coroutines; strong sandboxing |
| cgo toolchain | zig cc | Hermetic C compiler; cross-compile; sane Windows dev |
| Concurrency | Actors + worker pool | Lua coroutines park on async ops; no OS threads burned |

> **Current status**: The core runtime (actors, memory, tools, shell, multi-agent, scheduler, budgets, safety, metrics) is implemented and tested. Storage backends: SQLite, Redis and PostgreSQL for key/value and document rows, an in-memory volatile store, and vector search through pgvector, Qdrant, Redis Stack/RediSearch, PostgreSQL with the vector extension, or SQLite's capped brute-force scan — never a silent empty result. LLM providers cover four chat API shapes (Anthropic Messages, OpenAI Chat Completions, OpenAI Responses, Gemini Interactions) plus a `rerank` provider, each pointed at an official host or any compatible endpoint via `base_url`; per-model `server_tools` enable provider-native tools such as Google Search grounding. Channels are webhook (sync or async), GitHub webhook (`ghhook`), Telegram (polling, webhook, or self-configuring `auto`) and `bridge`, an HMAC-signed adapter protocol that lets an out-of-process service connect any other chat platform. They all mount on one shared HTTP listener with a built-in `/health` probe. Beyond that the runtime ships multimodal media ingestion, a core-owned audit journal, embeddings and reranking for semantic memory, the identity and user layer (profiles, channel linking, OIDC, per-user quotas), tenancy (groups, memberships, shared memory pools and grants), a user-scoped file store with snapshot versioning, an encrypted credential store, engine-fired scheduled triggers, config directories, the append-only log plane, single and regional deployment tiers, MCP servers over stdio or Streamable HTTP, declarative HTTP tools, a terminal dashboard, a browser console and the admin API that AgentFlow Studio uses.

## Install Guide

### Prerequisites

- **Go 1.26+** — the runtime is built with Go modules.
- **GNU make** — drives the Luau static library + Go binary build.
- **A C/C++ toolchain** for the Luau cgo bridge. The Makefile auto-selects by host OS:
  - **Windows**: [zig](https://ziglang.org/) (hermetic; no system gcc required).
  - **Linux**: system gcc (`cc`/`c++`/`ar`), usually from `build-essential`.
  - **macOS**: Xcode / Apple Command Line Tools (`clang`/`clang++`/`ar`).

Luau itself is a git submodule under `third_party/luau/`, pinned to upstream release 0.731 (MIT) — clone with `--recurse-submodules` or run `git submodule update --init` first.

### Windows

1. Install [Go](https://go.dev/dl/) and make sure `go version` reports 1.26 or newer.
2. Install [Git for Windows](https://git-scm.com/download/win).
3. Install [zig](https://ziglang.org/download/) and add it to your PATH.
4. Install GNU make, for example `choco install make` or `winget install GnuWin32.Make`, or use the make bundled with Git Bash/MSYS2.
5. Clone and build. The Makefile works from PowerShell, cmd.exe and Git Bash alike: it detects the recipe shell and uses the matching commands.

```
git clone --recurse-submodules https://github.com/<you>/agentflow.git
cd agentflow
make          # compiles the pinned Luau submodule -> internal/vm/lib/libluau.a, then agentflow.exe
make test     # go test ./... with cgo enabled
make vet
```

On Windows the binary is `agentflow.exe`. `make TARGET_OS=linux` cross-compiles a Linux binary with zig.

### Linux

1. Install Go and a C++ toolchain:

```
# Debian/Ubuntu
sudo apt update && sudo apt install -y golang-go build-essential
# Fedora
sudo dnf install -y golang gcc gcc-c++ make
```

2. Clone and build:

```
git clone --recurse-submodules https://github.com/<you>/agentflow.git
cd agentflow
make && make test
```

### macOS

1. Install Go from [go.dev](https://go.dev/dl/) or with Homebrew: `brew install go`.
2. Install the Xcode Command Line Tools: `xcode-select --install`.
3. Clone and build:

```
git clone --recurse-submodules https://github.com/<you>/agentflow.git
cd agentflow
make && make test
```

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

Save a minimal echo loop as `echo.lua` (the first line is the prelude API contract — a chunk without it is refused):

```
-- af-prelude-version: 1
function loop()
  while true do
    local msg = session.inbox()
    session.send("echo: " .. (msg.text or ""))
  end
end
```

and a no-model config that wires it to a webhook as `agentflow.yaml`:

```
version: "1"
agents:
  echo:
    loop: ./echo.lua
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

You should see `echo: hello` in the response. Note the trailing slash — the webhook mounts the `/webhook/` subtree, and a POST to bare `/webhook` returns a `307` redirect that `curl -d` will not follow. Also check the admin endpoints and the shared listener's health probe:

```
curl http://127.0.0.1:9090/healthz
curl http://127.0.0.1:9090/readyz
curl http://127.0.0.1:8080/health    # shared channel listener -> {"ok":true}
```

### Troubleshooting

| Symptom | Fix |
| --- | --- |
| `undefined: State` or cgo errors from `internal/vm` | Run commands through `make`; it sets `CGO_ENABLED=1` and the correct `CC`/`CXX`. Do not run plain `go build` before the Luau library exists. |
| `Nothing to be done for 'all'` after switching branches | The Makefile tracks Go sources, embedded Lua and the console files, so this is rare. If the binary still looks stale, run `make clean && make`. |
| Windows: `'CGO_ENABLED' is not recognized` or `rm -rf` errors | An old Makefile. Pull the current one: it exports its variables and picks cmd.exe or sh recipes automatically. |
| `zig: command not found` on Windows | Add zig to PATH or install it. Verify with `zig version`. |
| Tests fail with SQLite / database locked | Run tests serially or remove stale `data/` directory between runs. |

### Updating

When a new version lands, pull and rebuild cleanly:

```
git pull
make clean && make
```

## Quick Start

### Build

The Makefile compiles the pinned Luau submodule's VM bridge via cgo (zig as the hermetic C compiler), then the binary:

```
make        # luau static lib -> agentflow binary
make test   # go test with the cgo/zig toolchain
```

### Run

The fastest path needs no model — a file-based echo loop replies out of the box. Point the binary at your config:

```
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'   # note trailing /
```

For an LLM-backed bot, point a real model at it (next block) and set an agent's `model` to that entry.

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
