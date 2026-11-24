# HKLII External API (`runtime: script`)

TypeScript External API package running inside AgentFlow's embedded Goja sandbox, querying the Hong Kong Legal Information Institute ([www.hklii.hk](https://www.hklii.hk)).

## Configuration

```yaml
apis:
  hklii:
    dir: ./examples/apis/hklii

agents:
  legal_assistant:
    loop: ./loops/chat.ts
    model: default
    capabilities: [llm.chat, tools, apis]
    skills: ["api:hklii.search", "api:hklii.read"]
```

## Actions

- `search`: Searches Hong Kong case law and legislation (`/api/simplesearch`). Exposed as tool `api:hklii.search` or callable from loops via `api.hklii.search({ query, count, disable_fuzzy })`.
- `read`: Fetches a case judgment's plain text (`/api/getjudgment`) by `path` (`/{lang}/cases/{abbr}/{year}/{num}`) or explicit `abbr`, `year`, `num`, `lang`, with optional `start`/`max_chars` paging. Exposed as tool `api:hklii.read` or callable via `api.hklii.read(...)`.
