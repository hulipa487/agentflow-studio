# NPC External API (`runtime: wasm`)

Pure-Go `wasip1/wasm` External API package running inside AgentFlow's embedded **wazero** sandbox, querying China's National Database of Laws and Regulations ([flk.npc.gov.cn](https://flk.npc.gov.cn)).

Includes the pure-Go document text extractor (`docx` / `doc` / `rtf` / `html` / `txt`) inside the Wasm module itself, using `host.http` with `response_encoding: "base64"` to fetch `.docx` downloads losslessly.

## Build the Wasm Binary

Requires Go 1.26+ (`CGO_ENABLED=0`, zero external dependencies):

```bash
cd src
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildmode=c-shared -o ../npc.wasm .
```

## Configuration

```yaml
apis:
  npc:
    dir: ./examples/apis/npc

agents:
  legal_assistant:
    loop: ./loops/chat.ts
    model: default
    capabilities: [llm.chat, tools, apis]
    skills: ["api:npc.search", "api:npc.read"]
```

## Actions

- `search`: Searches PRC laws and regulations (`POST /law-search/search/list`). Exposed as tool `api:npc.search` or callable from loops via `api.npc.search({ query, count, disable_fuzzy })`.
- `read`: Resolves a signed `.docx` download URL (`GET /law-search/download/pc`), fetches it as base64 via `af_host.http_request`, extracts plain text inside Wasm, and returns `start`/`max_chars`-paged text. Exposed as tool `api:npc.read` or callable via `api.npc.read({ path })`.
