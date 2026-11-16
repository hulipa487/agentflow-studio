# OpenAI-compatible API

AgentFlow can expose an agent as an OpenAI **chat completions** endpoint. A
client that speaks the Chat Completions API — an agent framework, a coding
assistant, a chat frontend, or plain SDK code — needs only a base URL and an API
key. There is no translation layer to install and no second port: the channel
mounts on the same `gateway.listen` as every other channel.

The endpoint is a **channel**, not a proxy. It authenticates, parses, correlates
the reply and speaks SSE. It makes no decision about meaning: the route handler
picks a session, and the agent's loop decides what the request is. With the
built-in route and the `plugin:openai_passthrough` loop, each request gets its
own one-turn session and is forwarded to the agent's model unchanged, so a
client that is itself an agent — its own system prompt, its own tools, its own
conversation — gets exactly a model.

## Quick start

An agent answers as its model when its loop is `plugin:openai_passthrough`. The
channel does not choose the loop for you: an agent listed under `agents` runs
whatever its own `loop:` says.

```yaml
agents:
  coder:
    loop: plugin:openai_passthrough
    model: default
    capabilities: [llm.chat]      # llm.forward and openai.request are gated by it

gateway:
  listen: ":8080"
  channels:
    - name: api
      type: openai
      agents: [coder]
      auth: key
      keys: ["${COMPAT_API_KEY}"]
```

Point the client at `http://<host>:8080/v1` with the key, and use `coder` as the
model name.

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
      media:                      # needed for inline images and files
        allow: ["image/*"]
```

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `path` | string | `/v1/` | Prefix the endpoints hang off; must end in `/`. A request to any other path under it gets OpenAI's 404 envelope |
| `agents` | list | — | Agents exposed as model ids. Every name must be a configured agent. `agent:` is the shorthand for one; setting both is an error |
| `auth` | string | — | Required. `user_token` (per-user bearer tokens; needs `runtime.identity.enabled`), `key` (a shared key from `keys`, sent as a bearer token or in `x-api-key`), or `none` (only when `gateway.listen` is a loopback address) |
| `keys` | list | — | Accepted shared keys for `auth: key`. `${VAR}` and `cred:<service>` references resolve at start; an unresolved one skips the channel with a warning |
| `timeout` | duration | `600s` | The whole request, including tool rounds |
| `max_request_bytes` | int | `33554432` | The POST body limit; larger requests get 413 `request_too_large` |
| `journal` | string | `metadata` | `full` keeps text and attachments, `metadata` keeps ids and sizes and **never the text**, `off` writes nothing. Other channel types default to `full` |
| `expose_reasoning` | bool | `false` | Send the model's reasoning as `reasoning_content`. This shows the model's thinking to the caller |
| `media` | map | off | The channel's media policy. An inline image or file (a data URL) is refused with 400 unless its type is in `media.allow` |

Identity for `auth: user_token` comes from the bearer token and is stamped onto
the message's core-owned provenance, not taken from the request body. With
`auth: key` or `none` a request has no user: memory scopes, quotas and the
per-user limits below do not apply to it. The request's `user` field and
`metadata` are passed to the loop as **untrusted** data.

## Requests

| Field | Handling |
| --- | --- |
| `model` | An agent name from `agents`. Unknown: 404 `model_not_found` |
| `messages` | Roles `system`, `developer`, `user`, `assistant`, `tool`. Content is a string or parts: `text`, `image_url`, `input_audio`, `file`, `video_url` |
| `stream`, `stream_options.include_usage` | Supported |
| `tools`, `tool_choice`, `parallel_tool_calls` | Forwarded to the model. **Never executed** by AgentFlow: the model's tool calls are returned, and the client sends results back on its next request |
| `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stop`, `seed`, `response_format` | Forwarded where the agent's provider supports them (see [Providers](#providers)) |
| `n` | Only 1; anything larger is 400 |
| `user`, `metadata` | Passed to the loop as untrusted data. `metadata.conversation_id`, or the `X-AgentFlow-Conversation` header, asks for a conversation session |
| `Idempotency-Key` header | See [Sessions](#sessions) |
| anything else | Accepted and ignored |

Inline media (a `data:` URL) is stored through the channel's `media:` policy and
reaches the loop and the model as a handle. A remote URL is passed to a provider
that fetches URLs itself; the runtime never fetches a URL a client supplied.

`GET <path>models` lists the exposed agents and `GET <path>models/{id}`
describes one.

## Responses and errors

Responses are the standard `chat.completion` and `chat.completion.chunk`
objects. `finish_reason` is the model's own (`stop`, `length`, `tool_calls`,
`content_filter`). A tool call's `arguments` is the model's argument text,
unchanged. `usage` maps `input`→`prompt_tokens`, `output`→`completion_tokens`,
`cached`→`prompt_tokens_details.cached_tokens` and
`reasoning`→`completion_tokens_details.reasoning_tokens`.

In a stream, text arrives as it is produced (see [Streaming](#streaming)), tool
calls arrive as one complete chunk at the end — the protocol allows a single
`arguments` fragment — then the `finish_reason` chunk, a usage chunk when
`stream_options.include_usage` was set, and `data: [DONE]`. An idle stream
carries an SSE comment every 15 seconds so proxies keep it open.

Errors use OpenAI's envelope `{"error": {"message", "type", "code"}}`:

| Situation | Status | `code` |
| --- | --- | --- |
| Missing or bad credentials | 401 | `invalid_api_key` |
| Unknown model | 404 | `model_not_found` |
| Bad JSON, `n > 1`, an unsupported content part, media the channel does not accept | 400 | `invalid_request_error` |
| An option the agent's model cannot honour (see [Providers](#providers)) | 400 | `invalid_request_error` |
| Wrong HTTP method | 405 | `method_not_allowed` |
| Request too large | 413 | `request_too_large` |
| User quota or agent budget exhausted | 429 | `insufficient_quota` |
| Session limit reached | 429 | `rate_limit_exceeded` (with `Retry-After`) |
| Input blocked by safety | 400 | `content_filter`; the profile's refusal text, if it has one, is the message |
| Output blocked by safety | 200 | `finish_reason: "content_filter"`; the content is the profile's fallback text, or empty |
| The model call failed | 502 | `upstream_error` (the upstream's own message is logged, not returned) |
| The agent's loop failed | 500 | `server_error` (logged, not returned) |
| The route handler did not deliver the request | 503 | `server_unavailable` |
| Draining (readiness false) | 503 | `server_unavailable` |
| Request timeout | 504 | `timeout` |

**Streaming requests report late errors in the stream.** A streaming response
begins (HTTP 200 and the role chunk) before the turn runs, so only what is known
before then has an HTTP status: 401, 404, 400 for a malformed request, 405, 413
and the draining 503. Everything later — a limit, a blocked input, a model
failure, a timeout — arrives as one `data: {"error": {...}}` event with the same
`type` and `code`, followed by `data: [DONE]`. If text had already been sent,
the stream is finished normally with `finish_reason: "stop"` and the error is
logged.

## Sessions

The built-in route handler (`plugin:per_chat`) gives an OpenAI request a session
of its own: an **ephemeral** session that serves exactly one turn and is gone.
Its key is unique per request, so it takes no lease, writes no inbox row and is
never retired.

Concurrent ephemeral sessions are limited per user
(`runtime.limits.ephemeral_per_tenant`, default 16, `0` disables), separately
from conversation sessions. The limit needs a user, so it applies with
`auth: user_token`; with `auth: key` or `none` there is no per-caller limit, and
a shared key should sit behind a rate limiter of your own.

A client that wants a conversation opts in with `metadata.conversation_id` or
the `X-AgentFlow-Conversation` header. That routes to a normal keyed session,
keyed by the channel and the conversation id. The id is client-chosen and not
scoped to the caller: with a shared key, two callers who send the same id share
the session.

`Idempotency-Key` is honoured for a request that is still running: a retry with
the same key, from the same caller, with the same body, joins the running turn
and receives the whole answer (a stream is replayed from its first chunk). A
retry after the turn finished is a new turn, as is the same key with a different
body or from a different caller. The message id is derived from the key (a hash,
never the header itself); the request id is new for every turn.

A client disconnect ends the request; when nobody is left listening to an
ephemeral session the engine stops it, so no more tokens are spent on an answer
nobody will read. A conversation's turn is left to finish.

**Regional cells.** One-turn sessions work on any instance. A conversation
session is answered only when the request reaches the instance that owns that
conversation: a request for it that lands elsewhere is queued for the owner,
which has no connection to answer on, and `llm.forward` on the owner cannot read
a request body that arrived on another instance. `agentflow validate` and
`agentflow doctor` warn about this combination. Until it is lifted, pin a
conversation to one instance at the load balancer, or do not use conversation
ids in a regional cell.

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

`msg.text` is the text of the last user message (empty when the last message is
a tool result); it is what the safety profile screens on the way in.

| Call | Purpose |
| --- | --- |
| `openai.request()` | The full request as a table: `{ model, stream, messages, tools, tool_choice, params, user, metadata }`. Refused above 4 MiB of JSON |
| `llm.forward(opts?)` | Forward the turn's request to a model without materialising it. Returns `{ text, tool_calls, finish_reason, usage, thinking, thinking_blocks }` |
| `session.reply(t)` | Finish the turn with a structured answer: `{ text, tool_calls, finish_reason, usage, reasoning }` |
| `session.send_delta(text)` | Send one text fragment of the answer |
| `session.send(text)` | Append text to the answer; the turn ends when the loop asks for the next message |

`llm.forward`, `openai.request` and the other `llm.*` calls need the `llm.chat`
capability.

## The pass-through loop

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

`llm.forward()` with no arguments sends the client's own messages, tools, tool
choice and sampling parameters to the model the turn runs on: the agent's
`model`, or the user's profile model when they have one. It runs on the same
metered path as every model call: the user's quota, the agent's budget and the
usage ledger apply, and the ledger row has kind `forward`.

### Streaming

Text deltas are relayed as they arrive when the client asked for a stream and
the agent's safety profile has no filters. If the profile has any, the answer is
buffered, checked and then sent whole — safety wins over streaming, and the
client still receives a valid stream.

### Reasoning

With `expose_reasoning: true`, the reasoning the model reported is sent as
`reasoning_content`, on the message or in one chunk before the tool calls. A
reply whose text the safety profile refused carries no reasoning.

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

`examples/loops/openai_memory.lua` in the runtime repository is a complete
version of this pattern.

Options to `llm.forward`: `model` (another `models:` entry), `messages` (replace
the client's), `prepend` / `append` (add turns around them), `tools` (add
definitions to the client's), `params` (override sampling: `temperature`,
`max_tokens`, `top_p`, `stop`, `seed`, `response_format`,
`parallel_tool_calls`, `tool_choice`, `thinking`) and `max_tokens_cap` (clamp
the client's `max_tokens`).

Three things to know when a loop handles the reply itself:

- A tool call in the reply has `args` (a table, for reading) and `arguments`
  (the model's own JSON text). `session.reply` sends `arguments` when it is
  present, because a Lua table cannot tell `[]` from `{}` and would return a
  different call. If you change a call's `args`, set its `arguments` to `nil`.
- A loop that sends fragments with `session.send_delta` and then replies with
  the full text has it sent once: the reply adds only what the fragments did not
  already carry.
- A loop that ends its turn without `session.reply` still answers: whatever it
  sent with `session.send` or `session.send_delta` is the content, with
  `finish_reason: "stop"`. A turn that produced nothing is an empty assistant
  message.

Keep durable state out of Lua locals. A one-turn session has none to keep, and
a conversation session is unloaded when idle (see
[`runtime.sessions.idle_unload`](configuration.md)); use `session.state.*`,
memory or the store.

## Providers

What the client sends is forwarded to the agent's model through that model's
provider, and providers differ.

| Provider | Notes |
| --- | --- |
| `openai` | Everything above is sent as given, including `developer` messages. `max_completion_tokens` is sent as `max_tokens` |
| `openai-responses` | `stop` and `seed` have no equivalent and are dropped |
| `anthropic` | `system` and `developer` messages are joined into the system prompt. `seed` is dropped. `response_format` other than `{"type": "text"}` is refused with 400 |
| `gemini` | Client `tools` are refused with 400, so a coding client that sends tools cannot use a Gemini agent. `temperature`, `top_p` and `parallel_tool_calls` are dropped. `response_format` other than `text` and media given as a remote URL are refused with 400 |

**Output limit.** When the client sends neither `max_tokens` nor
`max_completion_tokens`, the model entry's `max_tokens` applies, and that
defaults to 4096. Coding clients often leave the limit out and expect the
model's own maximum, so set `max_tokens` on the model an exposed agent uses to
what you want such requests to get. An answer the limit cut short is reported
with `finish_reason: "length"`.

A request field is dropped only where leaving it out cannot change what the
client gets back in kind; `response_format` and a named `tool_choice` are
refused instead, because a caller that asked for JSON and silently received
prose could not tell.

## Operations

- **Audit.** With `journal: metadata` (the default here) the journal keeps one
  row per request and per answer with ids, sizes, user and timings, and no
  text.
- **Metrics.** `agentflow_ephemeral_sessions` (live one-turn sessions),
  `agentflow_ephemeral_cap_drops` (requests refused by the limit) and
  `agentflow_sessions_unloaded` (idle conversations unloaded), next to the
  usual `agentflow_llm_*` and `agentflow_egress_*` counters.
- **Doctor.** `agentflow doctor` reports each `openai` channel's auth mode and
  exposed agents, and warns about `auth: none`.

## Compatibility

**No client is listed as verified yet.** The replay suite runs hand-written
fixtures derived from the OpenAI API reference. A client is added to this list
only when a captured request/response pair from that client is committed under
`cmd/agentflow/testdata/chatapi` and replayed by the tests. The recorder for
producing those captures is `internal/drivers/chatapi/cmd/record` in the runtime
repository; point the client's base URL at it and drive the client through
plain chat, streaming, a tool call and its follow-up.
