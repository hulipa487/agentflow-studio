# Extending AgentFlow without rebuilding Go

AgentFlow has four extension planes. The first three are configuration or Lua;
only the last one changes the binary.

## 1. Declarative HTTP tools

An external API becomes a model-visible tool with no Lua and no Go:

```yaml
tools:
  http:
    - name: crm_lookup
      description: Look up a customer by email address.
      method: GET
      url: https://api.example.com/v1/customers
      query:
        email: "{email}"
      auth:
        service: crm
      params:
        type: object
        properties:
          email: {type: string}
        required: [email]
      response: customer
      timeout: 15s
      hosts: [api.example.com]
```

The tool is registered as `http:crm_lookup`. The runtime owns:

- the JSON Schema the model sees;
- `{arg}` templating in the URL, query and JSON body;
- per-tenant credential resolution (`auth.service`; the value never reaches
  the model or the journal);
- the shared address guard (`net.http.allow_private`) and the optional host
  allow-list;
- the response cap and optional dotted-path selection (`customer.id`,
  `items[0].name`).

A non-GET tool defaults to `needs_confirm: true`; set it explicitly to change
that. Body templates preserve JSON types when a value is exactly `{arg}`.

Import an OpenAPI 3 document instead of writing every operation by hand:

```bash
agentflow tools import-openapi openapi.yaml -out tools.yaml \
  -base-url https://api.example.com -auth crm -prefix crm
```

The importer emits a `tools:` block for review; it does not fetch the spec at
runtime. Add `-read-only` to import only GET/HEAD.

## 2. MCP servers

MCP remains the arbitrary external-tool boundary. A stdio child or a
Streamable HTTP endpoint is declared under `mcp.servers`; its tools appear as
`<server>/<tool>`. HTTP servers accept a `token:` that may be a literal,
`${VAR}`, or `cred:<service>`, and are subject to the same address guard as
`http.request` and `builtin:fetch`.

## 3. Channel bridge

A chat platform that AgentFlow does not compile in can be added as an external
adapter. The adapter translates the platform's events into this protocol and
receives replies; AgentFlow needs no Go change:

```yaml
gateway:
  channels:
    - name: slack
      type: bridge
      agent: support
      path: /bridge/slack/
      deliver_url: http://127.0.0.1:9100/deliver
      secret: "${SLACK_BRIDGE_SECRET}"
      token: "${SLACK_BRIDGE_TOKEN}"   # optional bearer for deliver_url
```

Inbound (adapter → runtime):

```http
POST /bridge/slack/
X-Agentflow-Signature: sha256=<hex HMAC-SHA256 of the raw body>

{"from":"U123","chat_id":"C456","text":"hello","attachments":[]}
```

Outbound (runtime → adapter):

```json
{"to":"C456","text":"...","attachments":[{"type":"image","mime":"image/png","handle":"media:..."}]}
```

The HMAC is required. The adapter is trusted to relay the platform's sender
identity, but bridge handles are recorded as asserted; linking still requires
the normal in-channel `/link` challenge before a handle gets a profile scope.

## 4. Lua packages

A package is a directory of version-gated Lua source. Declare it and import it
from a loop:

```yaml
packages:
  greeting: ./packages/greeting
```

```lua
-- af-prelude-version: 1
-- af-import: greeting

function loop()
  while true do
    local msg = session.inbox()
    session.send(Greeting.hello(msg.text))
  end
end
```

A package may contain `manifest.yaml`:

```yaml
name: greeting
version: "1"
prelude_api: 1
files: [greeting.lua]
```

Without a manifest, `*.lua` files load in lexical order. Packages are source
composition, not isolated modules: they share the session's Lua state and
capability set (which the agent still controls). `package:<name>` is also
accepted directly as an agent's `loop:` reference. Editing a package restarts
the sessions that may import it.

## 5. What still needs Go

Compiled drivers remain compiled: a new LLM wire protocol, a new memory/vector
backend, a new search engine or shell provider, and new *kinds* of safety
filter. Model-backed moderation and classification no longer need Go — declare
a `safety.classifiers` entry and reference it as `classifier:<name>` in a
profile. Lua-coded filters would still need a native hook. The workarounds are:

- OpenAI-compatible endpoints for a new model vendor (`provider: openai` +
  `base_url`);
- MCP or a declarative HTTP tool for a new search/API provider;
- the bridge for a new chat platform;
- Lua packages for shared policy;
- a classifier prompt plus label policy for moderation without recompiling.

## 6. Tenant controls

Four deployment-level switches close the product-level gaps without changing
the inference path:

```yaml
runtime:
  users:
    unregistered: reject        # drop unlinked handles instead of the service stratum
    push_same_tenant: true      # session.push_user may only target the current tenant
  credentials:
    engine_wide: maintenance    # credential.get() only in engine-fired turns
  admin:
    operators:                  # named admin tokens; X-Operator attributes actions
      alice: "${ALICE_ADMIN_TOKEN}"
```

The tenant lifecycle endpoints are `GET /admin/api/usage`,
`GET /admin/api/users/{id}/export`, and
`DELETE /admin/api/users/{id}?confirm=<id>`. The delete path removes the
profile, handles, tokens, credentials, usage, journal and file metadata;
memory rows follow their store retention and content-addressed blobs are
reclaimed by the file GC. The admin API is still one authority level: the
operator names attribute actions, but they do not split read/write
permissions.

## 7. Test before deploying

```bash
agentflow validate -config agentflow.yaml
agentflow loop check loops/support.lua
agentflow loop test scenarios/refund.yaml
```

A scenario is data:

```yaml
agent: support
loop: ../loops/support.lua
llm:
  - match: refund
    text: "I can help with that."
messages:
  - from: alice
    text: "I need a refund"
expect_contains: ["help"]
```

`loop test` runs the real actor against fake LLM/tool/channel dependencies, so
it needs no credentials and no network. See `internal/looptest` for the full
scenario shape.

## Hooks and extension points

There is no hook registry. The engine dispatches three points itself, and everything else is a **naming convention the Lua calls**: a plugin contributes a global with the name its caller looks for, and that is the whole contract. The difference decides where a plugin's code has to live, so both are listed here.

### Dispatched by the core

These run on every turn whatever the loop does. Replacing the loop handler cannot uninstall them — that is the point of the first two.

| Point | Kind | Implemented by | Notes |
| --- | --- | --- | --- |
| `safety.ingress` | filter | Go, core-owned | Runs before the loop sees a message; may drop it, skip the turn, or rewrite the text. Of the five baseline filters only `affect-guard` ever drops; `classifier:<name>` filters can block on model output. |
| `safety.egress` | filter | Go, core-owned | Runs before the reply reaches the channel; may drop it (with an optional core-authored fallback). |
| `gateway.route` | handler | a Lua chunk named by `gateway.route`, default `plugin:per_chat` | One Luau state per process. Reads the message and returns a session address. A route handler may call only `inbox`, `deliver`, `runtime.triggers`, `router.state.*` and `log`; anything else is refused by name. |

### Naming conventions the Lua calls

These are ordinary globals, not entry points the engine invokes. The prelude or the loop calls them by name when the deployment configures that behaviour, and shadowing a builtin means defining the same global in `plugins/<name>.lua`.

| Configured as | Global the Lua must define | Called by |
| --- | --- | --- |
| memory profile `write: plugin:routing_table` | `memory_routing_table(record)` | the prelude's `memory.write` |
| memory profile `recall: plugin:recency` | `memory_recall_handler(query, opts)` | the prelude's `memory.recall` |
| memory profile `recall: plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | the prelude's `memory.recall` |
| support chunk `plugin:token_budget` | `token_budget(messages, budget)` | the loop, when it wants prompt trimming |
| support chunk `plugin:fact_extractor` | `fact_extractor_turn(turn)` | the loop, after a turn |
| loop plugin `plugin:per_chat` | `loop()` | nothing — it *is* a loop, not a helper |

> **What this section used to say**: It listed 26 hooks — `gateway.ingress`, `loop.before_turn`, `session.migrate`, `shell.before_exec`, `agent.start`, `core.on_reload` and others — with kinds, signatures and defaults, and stated that the core dispatched them. No dispatcher exists, so a plugin defining one of those names was silently never called. Two of them named builtins (`plugin:exec_policy` for `shell.before_exec`, `plugin:ttl` for `shell.teardown`) that were loaded into every session state and invoked by nothing; both are now removed. **Shell commands are gated by the `shell.exec` capability alone — there is no command policy filter.** Loops are restarted on a crash and hot-reloaded on a file change, which is what `session.resume` and `core.on_reload` were reaching for; a loop that needs to carry state across either uses `session.state.*`.
