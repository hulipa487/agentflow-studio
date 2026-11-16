# OpenAI-compatible API

AgentFlow can expose an agent as an OpenAI **chat completions** endpoint. A
client that speaks the Chat Completions API — an agent framework, a coding
assistant, a chat frontend, or plain SDK code — needs only a base URL and an API
key. There is no translation layer to install and no second port: the channel
mounts on the same `gateway.listen` as every other channel.

The endpoint is a **channel**, not a proxy. It authenticates, parses, correlates
the reply and speaks SSE. It makes no decision about meaning: the route handler
picks a session, and the loop decides what the request is. The default route
gives each request its own one-turn session, and the default loop
(`plugin:openai_passthrough`) forwards the client's request to the agent's model
unchanged. The result is that a client which is itself an agent — its own system
prompt, its own tools, its own conversation — gets exactly a model.

## Configuration

```yaml
gateway:
  channels:
    - name: api
      type: openai
      path: /v1/                  # serves <path>chat/completions and <path>models
      agents: [coder, helper]     # exposed as model ids (or `agent:` for one)
      auth: key                   # user_token | key | none
      keys: ["${COMPAT_API_KEY}"] # auth: key only; secret references work
      timeout: 600s               # whole-request limit
      max_request_bytes: 33554432 # POST body limit (32 MiB default)
      journal: metadata           # full | metadata | off; metadata is this type's default
      expose_reasoning: false     # send model reasoning as reasoning_content
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `path` | string | `/v1/` | Prefix the two endpoints hang off; must end in `/`. A request to any other path under it gets OpenAI's 404 envelope |
| `agents` | list | — | Agents exposed as model ids. Every name must be a configured agent. `agent:` is the shorthand for one; setting both is an error |
| `auth` | string | — | `user_token` (per-user bearer tokens; needs `runtime.identity.enabled`), `key` (a shared key from `keys`), or `none` (only on a loopback listener) |
| `keys` | list | — | Accepted shared keys for `auth: key`. `${VAR}` and `cred:<service>` references resolve at start; an unresolved one skips the channel with a warning |
| `timeout` | duration | `600s` | The whole request, including tool rounds |
| `max_request_bytes` | int | `33554432` | The POST body limit; larger requests get 413 `request_too_large` |
| `journal` | string | `metadata` | `full` keeps text and attachments, `metadata` keeps ids and sizes and **never the text**, `off` writes nothing. Other channels default to `full` |
| `expose_reasoning` | bool | `false` | Send the model's reasoning as `reasoning_content`. This leaks the model's thinking to the caller |

Identity for `auth: user_token` comes from the bearer token and is stamped onto
the message's core-owned provenance, not taken from the request body. The
request's `user` field and `metadata` are passed to the loop as **untrusted**
data. A data-URL image is ingested through the channel's `media:` policy; a
remote image URL is passed to providers that accept URLs.

The endpoint's errors use OpenAI's envelope `{"error": {"message", "type",
"code"}}`:

| Situation | Status | `code` |
| --- | --- | --- |
| Missing or bad credentials | 401 | `invalid_api_key` |
| Unknown model | 404 | `model_not_found` |
| Bad JSON, `n > 1`, unsupported content part | 400 | `invalid_request_error` |
| Request too large | 413 | `request_too_large` |
| User quota exhausted | 429 | `insufficient_quota` |
| Ephemeral or session cap reached | 429 | `rate_limit_exceeded` (with `Retry-After`) |
| Input blocked by safety | 400 | `content_filter` |
| Output blocked by safety | 200 | `finish_reason: "content_filter"` |
| Upstream model error | 502 | `upstream_error` (the upstream body is not leaked) |
| Turn timeout | 504 | `timeout` |
| Draining (readiness false) | 503 | `server_unavailable` |

## Sessions

The default route handler (`plugin:per_chat`) gives an OpenAI request a session
of its own: an **ephemeral** session that serves exactly one turn and is gone.
Its key is unique per request, so it takes no lease, writes no inbox row and is
never retired; its limit is separate from conversation sessions
(`runtime.limits.ephemeral_per_tenant`, default 16, `0` disables). When the
tenant's cap is reached the request gets 429 with `Retry-After`.

A client that wants a conversation opts in with `metadata.conversation_id` or
the `X-AgentFlow-Conversation` header. That routes to a normal keyed session,
keyed by the conversation, with the lease, inbox and failover behaviour of any
conversation.

`Idempotency-Key` is honoured: a retried request while the first is still
running joins that turn; a retry after it finished is a new turn. The request id
(and so the message id) is derived from the key, not sent verbatim.

A client disconnect cancels the request; for an ephemeral session the engine
also stops it, so no more tokens are spent on an answer nobody will read.

## What the loop receives

The body does not cross the Lua bridge. A large coding request stays in Go, and
the message carries metadata and a handle:

```lua
{
  api = "openai.chat",
  request_id = "…", request_ref = "…",
  model = "coder", stream = true,
  message_count = 12, has_tools = true, bytes = 48211,
  conversation_id = nil,          -- from header or metadata, untrusted
  user = "…", metadata = { … },   -- untrusted
}
```

Two ops read the rest: `openai.request()` returns the full request as a table
(refused above 4 MiB of JSON), and `llm.forward(opts?)` forwards it to a model
without materialising it.

## The default loop

`plugin:openai_passthrough` is four lines:

```lua
-- af-prelude-version: 1
function loop()
  while true do
    local msg = session.inbox()
    local reply = llm.forward()
    session.reply(reply)
  end
end
```

- Client `messages`, `tools`, `tool_choice` and sampling parameters are
  forwarded. Client tools are **never executed** by AgentFlow: the model's tool
  calls are returned unchanged, and the client sends the results back on its
  next request.
- Text deltas are relayed as they arrive when the client asked for a stream and
  the agent's safety profile cannot drop egress. If the profile can drop, the
  answer is buffered, checked and sent whole — safety wins over streaming, and
  the client still gets a normal stream.
- Tool calls are sent as one complete chunk at the end. The streaming protocol
  allows this: clients concatenate `arguments` fragments, and one fragment is
  valid.
- `usage` maps `input`→`prompt_tokens`, `output`→`completion_tokens`,
  `cached`→`prompt_tokens_details.cached_tokens`, and
  `reasoning`→`completion_tokens_details.reasoning_tokens`.

## Writing your own loop

The channel adds no policy; a loop can enrich the request or answer it as an
assistant instead of a model. A pass-through that prepends remembered facts:

```lua
function loop()
  while true do
    local msg = session.inbox()
    local facts = memory.recall({ store = "facts", kind = "text", text = msg.text or "", k = 5 })
    local opts = nil
    if facts and #facts > 0 then
      local lines = {}
      for _, hit in ipairs(facts) do lines[#lines + 1] = "- " .. tostring(hit.value.text or "") end
      opts = { prepend = { { role = "system", content = table.concat(lines, "\n") } } }
    end
    session.reply(llm.forward(opts))
  end
end
```

`examples/loops/openai_memory.lua` is a complete version of this pattern. A
loop that ignores the client's messages and uses the agent's own instructions
and server-side tools should advertise that clearly: the client is then talking
to an assistant, not to a model. The plan for that loop is `plugin:openai_agent`
(not part of the first release).

Useful options to `llm.forward`: `model` (another `models:` entry), `messages`
(replace the client's), `prepend`/`append`, `tools` (add definitions), `params`
(override sampling), `max_tokens_cap` (clamp the client's `max_tokens`).

## Compatibility

The channel implements the request fields that matter to Chat Completions
clients: `model`, `messages` (`system`, `developer`, `user`, `assistant`,
`tool`; text and image parts), `stream`, `stream_options.include_usage`,
`tools`, `tool_choice`, `parallel_tool_calls`, `temperature`, `top_p`,
`max_tokens`, `max_completion_tokens`, `stop`, `seed`, `response_format`, `n`
(1 only), `user` and `metadata`. Unknown fields are accepted and ignored. `GET
/v1/models` lists the exposed agents, and `GET /v1/models/{id}` describes one.

**No client is listed as verified yet.** The replay suite runs hand-written
fixtures derived from the OpenAI API reference. A client is added to this list
only when a captured request/response pair from that client is committed under
`cmd/agentflow/testdata/chatapi` and replayed by the tests. The recorder for
producing those captures is `internal/drivers/chatapi/cmd/record`; point the
client's base URL at it and drive the client through the scenarios above.
