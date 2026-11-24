# OpenAI 兼容 API

AgentFlow 可以把一个智能体暴露为 OpenAI **chat completions** 端点。任何使用 Chat Completions API 的客户端 — 智能体框架、编程助手、聊天前端，或直接调用 SDK 的代码 — 只需要一个基础 URL 和一个 API 密钥。无需安装任何转换层，也不需要第二个端口：该渠道与其他所有渠道一样挂载在同一个 `gateway.listen` 上。

这个端点是一个**渠道**，而不是代理。它负责认证、解析、关联回复，并使用 SSE 通信。它不对含义做任何判断：路由处理器选择会话，智能体的循环决定这个请求是什么。使用内置路由和透传循环（`examples/loops/openai_passthrough.ts`）时，每个请求都获得自己的单轮会话，并被原样转发给智能体的模型，因此本身就是智能体的客户端 — 有自己的系统提示词、自己的工具、自己的对话 — 拿到的就是一个纯粹的模型。

## 快速开始

当智能体的循环使用 `llm.forward()` 转发请求时（示例位于 `examples/loops/openai_passthrough.ts`），它就以其模型的身份作答。渠道不会替你选择循环：列在 `agents` 下的智能体运行的是它自己的 `loop:` 所指定的循环。

```yaml
agents:
  coder:
    loop: ./examples/loops/openai_passthrough.ts
    model: default
    capabilities: [llm.chat]      # llm.forward 和 openai.request 受它控制

gateway:
  listen: ":8080"
  channels:
    - name: api
      type: openai
      agents: [coder]
      auth: key
      keys: ["${COMPAT_API_KEY}"]
```

把客户端指向 `http://<host>:8080/v1` 并带上密钥，模型名称使用 `coder`。

## 配置

```yaml
gateway:
  channels:
    - name: api
      type: openai
      path: /v1/                  # 提供 <path>chat/completions 和 <path>models
      agents: [coder, helper]     # 以模型 ID 的形式暴露（只有一个时可用 `agent:`）
      auth: key                   # user_token | key | none
      keys: ["${COMPAT_API_KEY}"] # 仅用于 auth: key；支持密钥引用
      timeout: 600s               # 整个请求的时限
      max_request_bytes: 33554432 # POST 请求体上限（默认 32 MiB）
      journal: metadata           # full | metadata | off；metadata 是此类型的默认值
      expose_reasoning: false     # 以 reasoning_content 发送模型的推理内容
      media:                      # 内联图片和文件需要它
        allow: ["image/*"]
```

| 键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `path` | string | `/v1/` | 各端点所挂载的前缀；必须以 `/` 结尾。发往其下任何其他路径的请求会收到 OpenAI 格式的 404 错误信封 |
| `agents` | list | — | 以模型 ID 形式暴露的智能体。每个名称都必须是已配置的智能体。只有一个时可用简写 `agent:`；两者同时设置会报错 |
| `auth` | string | — | 必填。`user_token`（按用户的 bearer 令牌；需要 `runtime.identity.enabled`）、`key`（来自 `keys` 的共享密钥，以 bearer 令牌或 `x-api-key` 发送），或 `none`（仅当 `gateway.listen` 是回环地址时可用） |
| `keys` | list | — | `auth: key` 接受的共享密钥。`${VAR}` 和 `cred:<service>` 引用在启动时解析；任何一个无法解析时，该渠道会被跳过并给出警告 |
| `timeout` | duration | `600s` | 整个请求的时限，包括工具调用的往返 |
| `max_request_bytes` | int | `33554432` | POST 请求体上限；更大的请求会收到 413 `request_too_large` |
| `journal` | string | `metadata` | `full` 保留文本和附件，`metadata` 保留 ID 和大小，**从不保留文本**，`off` 不写入任何内容。其他渠道类型默认为 `full` |
| `expose_reasoning` | bool | `false` | 以 `reasoning_content` 发送模型的推理内容。这会把模型的思考过程展示给调用方 |
| `media` | map | 关闭 | 渠道的媒体策略。内联图片或文件（data URL）除非其类型在 `media.allow` 中，否则会以 400 拒绝 |

`auth: user_token` 下的身份来自 bearer 令牌，并被加盖到消息中由核心掌管的来源信息上，而不是从请求体中读取。使用 `auth: key` 或 `none` 时，请求没有用户：记忆范围、配额以及下文的按用户限制都不适用。请求中的 `user` 字段和 `metadata` 会作为**不受信任**的数据传给循环。

## 请求

| 字段 | 处理方式 |
| --- | --- |
| `model` | `agents` 中的一个智能体名称。未知时：404 `model_not_found` |
| `messages` | 角色 `system`、`developer`、`user`、`assistant`、`tool`。内容是字符串或若干部分：`text`、`image_url`、`input_audio`、`file`、`video_url` |
| `stream`, `stream_options.include_usage` | 支持 |
| `tools`, `tool_choice`, `parallel_tool_calls` | 转发给模型。AgentFlow **从不执行**：返回的是模型的工具调用，由客户端在下一次请求中把结果发回 |
| `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stop`, `seed`, `response_format` | 在智能体的提供商支持时转发（见[提供商](#providers)） |
| `n` | 只能为 1；更大的值返回 400 |
| `user`, `metadata` | 作为不受信任的数据传给循环。`metadata.conversation_id` 或 `X-AgentFlow-Conversation` 请求头用于请求一个对话会话 |
| `Idempotency-Key` 请求头 | 见[会话](#sessions) |
| 其他任何字段 | 接受并忽略 |

内联媒体（`data:` URL）按渠道的 `media:` 策略存储，并以句柄的形式送达循环和模型。远程 URL 会传给自行获取 URL 的提供商；运行时从不获取客户端提供的 URL。

`GET <path>models` 列出暴露的智能体，`GET <path>models/{id}` 描述其中一个。

## 响应和错误

响应是标准的 `chat.completion` 和 `chat.completion.chunk` 对象。`finish_reason` 就是模型自己给出的（`stop`、`length`、`tool_calls`、`content_filter`）。工具调用的 `arguments` 是模型给出的参数文本，原样不变。`usage` 的映射为 `input`→`prompt_tokens`、`output`→`completion_tokens`、`cached`→`prompt_tokens_details.cached_tokens` 以及 `reasoning`→`completion_tokens_details.reasoning_tokens`。

在流中，文本随生成随到（见[流式传输](#streaming)），工具调用在最后作为一个完整的数据块到达 — 协议允许单个 `arguments` 片段 — 然后是 `finish_reason` 数据块，设置了 `stream_options.include_usage` 时还有一个用量数据块，最后是 `data: [DONE]`。空闲的流每 15 秒发送一条 SSE 注释，以免代理把它断开。

错误使用 OpenAI 的信封格式 `{"error": {"message", "type", "code"}}`：

| 情况 | 状态码 | `code` |
| --- | --- | --- |
| 凭据缺失或无效 | 401 | `invalid_api_key` |
| 未知模型 | 404 | `model_not_found` |
| JSON 错误、`n > 1`、不支持的内容部分、渠道不接受的媒体 | 400 | `invalid_request_error` |
| 智能体的模型无法满足的选项（见[提供商](#providers)） | 400 | `invalid_request_error` |
| HTTP 方法错误 | 405 | `method_not_allowed` |
| 请求过大 | 413 | `request_too_large` |
| 用户配额或智能体预算已用尽 | 429 | `insufficient_quota` |
| 已达到会话上限 | 429 | `rate_limit_exceeded`（附带 `Retry-After`） |
| 输入被安全过滤拦截 | 400 | `content_filter`；如果方案设置了拒绝回复文本，它就是错误消息 |
| 输出被安全过滤拦截 | 200 | `finish_reason: "content_filter"`；内容是方案的替代回复文本，或为空 |
| 模型调用失败 | 502 | `upstream_error`（上游自己的消息会记入日志，不会返回） |
| 智能体的循环失败 | 500 | `server_error`（记入日志，不会返回） |
| 路由处理器没有投递该请求 | 503 | `server_unavailable` |
| 正在排空（就绪状态为 false） | 503 | `server_unavailable` |
| 请求超时 | 504 | `timeout` |

**流式请求在流中报告较晚出现的错误。** 流式响应在这一轮运行之前就已开始（HTTP 200 和 role 数据块），所以只有在此之前就能确定的情况才有 HTTP 状态码：401、404、格式错误请求的 400、405、413 以及排空时的 503。之后发生的一切 — 触及限制、输入被拦截、模型失败、超时 — 都以一个 `data: {"error": {...}}` 事件到达，带有相同的 `type` 和 `code`，随后是 `data: [DONE]`。如果此前已经发送了文本，流会以 `finish_reason: "stop"` 正常结束，错误则记入日志。

## 会话

内置路由处理器（`plugin:per_chat`）为每个 OpenAI 请求分配一个专属会话：一个只服务一轮就消失的**临时**会话。它的键对每个请求都是唯一的，所以它不获取租约，不写收件箱行，也从不需要退役。

并发的临时会话按用户限制数量（`runtime.limits.ephemeral_per_tenant`，默认 16，`0` 表示不限制），与对话会话分开计算。这个限制需要有用户，所以在 `auth: user_token` 下才生效；使用 `auth: key` 或 `none` 时没有按调用方的限制，共享密钥应当放在你自己的限流器之后。

需要对话的客户端可以通过 `metadata.conversation_id` 或 `X-AgentFlow-Conversation` 请求头选择启用。这会路由到一个普通的带键会话，键由渠道和对话 ID 组成。该 ID 由客户端选择，且不限定于调用方：使用共享密钥时，发送相同 ID 的两个调用方会共享同一个会话。

对于仍在运行的请求，会遵循 `Idempotency-Key`：同一调用方用相同的键和相同的请求体重试，会加入正在运行的这一轮，并收到完整的回答（流会从第一个数据块开始重放）。在这一轮结束后重试是新的一轮；相同的键配上不同的请求体或来自不同的调用方，也是新的一轮。消息 ID 由该键派生（一个哈希，绝不是请求头本身）；请求 ID 每一轮都是新的。

客户端断开连接会结束请求；当临时会话已经没有人在收听时，引擎会停止它，这样就不会再为没人读的回答消耗 Token。对话的这一轮则会继续运行到结束。

**区域单元。** 单轮会话可以在任何实例上运行。对话会话只有在请求到达拥有该对话的实例时才会得到回答：落到其他实例上的请求会排队交给所有者，而所有者并没有可用来回答的连接，并且所有者上的 `llm.forward` 也无法读取到达另一个实例的请求体。`agentflow validate` 和 `agentflow doctor` 会对这种组合发出警告。在这个限制解除之前，请在负载均衡器上把对话固定到一个实例，或者不要在区域单元中使用对话 ID。

## 循环收到什么

请求体不会穿过脚本桥。一个很大的编程请求会留在 Go 中，消息只携带元数据和一个句柄：

```ts
{
  api: "openai.chat",
  request_id: "…", request_ref: "…",
  model: "coder", stream: true,
  message_count: 12, has_tools: true, bytes: 48211,
  conversation_id: null,          // 来自请求头或 metadata，不受信任
  user: "…", metadata: { … },   // 不受信任
}
```

`msg.text` 是最后一条用户消息的文本（最后一条消息是工具结果时为空）；安全方案在入站时检查的就是它。

| 调用 | 用途 |
| --- | --- |
| `openai.request()` | 以对象形式返回完整请求：`{ model, stream, messages, tools, tool_choice, params, user, metadata }`。JSON 超过 4 MiB 时拒绝 |
| `llm.forward(opts?)` | 把这一轮的请求转发给模型，而无需把它物化。返回 `{ text, tool_calls, finish_reason, usage, thinking, thinking_blocks }` |
| `session.reply(t)` | 以结构化的回答结束这一轮：`{ text, tool_calls, finish_reason, usage, reasoning }` |
| `session.send_delta(text)` | 发送回答中的一个文本片段 |
| `session.send(text)` | 向回答追加文本；当循环请求下一条消息时，这一轮结束 |

`llm.forward`、`openai.request` 以及其他 `llm.*` 调用都需要 `llm.chat` 能力。

## 透传循环

`examples/loops/openai_passthrough.ts` 只有几行：

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    session.inbox();
    const reply = llm.forward();
    session.reply(reply);
  }
}
```

不带参数的 `llm.forward()` 会把客户端自己的消息、工具、工具选择和采样参数发送给这一轮所用的模型：智能体的 `model`，或者用户在其用户资料中设置了模型时使用该模型。它与每次模型调用走同一条计量路径：用户的配额、智能体的预算和用量账本都适用，账本行的类型为 `forward`。

### 流式传输

当客户端请求流式响应、且智能体的安全方案没有过滤器时，文本增量会随到随转。如果方案有任何过滤器，回答会先缓冲、检查，再整体发送 — 安全优先于流式，客户端仍会收到一个有效的流。

### 推理

设置 `expose_reasoning: true` 时，模型报告的推理内容会以 `reasoning_content` 发送，放在消息上，或者放在工具调用之前的一个数据块中。文本被安全方案拒绝的回复不携带推理内容。

## 编写你自己的循环

渠道不附加任何策略；循环可以丰富请求，也可以作为一个助手（而不是模型）来回答它。下面是一个在前面加上已记住事实的透传循环：

```ts
function loop() {
  while (true) {
    const msg = session.inbox();
    const facts = memory.recall({ store: "facts", kind: "text", text: msg.text || "", k: 5 });
    let opts = undefined;
    if (facts && facts.length > 0) {
      const lines = facts.map((hit) => "- " + String(hit.value.text || ""));
      opts = { prepend: [{ role: "system", content: lines.join("\n") }] };
    }
    session.reply(llm.forward(opts));
  }
}
```

`agentflow-studio` 仓库中的 `examples/loops/openai_memory.ts` 是这种模式的完整版本。

`llm.forward` 的选项：`model`（另一个 `models:` 条目）、`messages`（替换客户端的消息）、`prepend` / `append`（在其前后添加轮次）、`tools`（在客户端的工具之外添加定义）、`params`（覆盖采样参数：`temperature`、`max_tokens`、`top_p`、`stop`、`seed`、`response_format`、`parallel_tool_calls`、`tool_choice`、`thinking`）以及 `max_tokens_cap`（限制客户端的 `max_tokens`）。

循环自己处理回复时，需要了解三点：

- 回复中的工具调用同时带有 `args`（对象，供读取）和 `arguments`（模型自己的 JSON 文本）。只要有 `arguments`，`session.reply` 就会原样发送它，以保留模型输出的原始 JSON 文本。如果你修改了某个调用的 `args`，请把它的 `arguments` 设为 `undefined` 或 `null`。
- 循环先用 `session.send_delta` 发送片段、再用完整文本回复时，文本只会发送一次：回复只补上片段尚未包含的部分。
- 循环不调用 `session.reply` 就结束这一轮时仍会作答：它用 `session.send` 或 `session.send_delta` 发送的内容就是回答内容，`finish_reason: "stop"`。什么都没产生的一轮是一条空的 assistant 消息。

不要把持久状态放在脚本局部变量中。单轮会话没有需要保留的状态，而对话会话在空闲时会被卸载（见 [`runtime.sessions.idle_unload`](configuration.md)）；请使用 `session.state.*`、记忆或存储。

## 提供商

客户端发送的内容会通过智能体模型的提供商转发给该模型，而各提供商之间存在差异。

| 提供商 | 说明 |
| --- | --- |
| `openai` | 上述所有内容都按原样发送，包括 `developer` 消息。`max_completion_tokens` 作为 `max_tokens` 发送 |
| `openai-responses` | `stop` 和 `seed` 没有对应项，会被丢弃 |
| `anthropic` | `system` 和 `developer` 消息会合并到系统提示词中。`seed` 会被丢弃。`{"type": "text"}` 以外的 `response_format` 会以 400 拒绝 |
| `gemini` | 客户端的 `tools` 会以 400 拒绝，所以发送工具的编程客户端无法使用 Gemini 智能体。`temperature`、`top_p` 和 `parallel_tool_calls` 会被丢弃。`text` 以外的 `response_format` 以及以远程 URL 给出的媒体会以 400 拒绝 |

**输出上限。** 当客户端既没有发送 `max_tokens` 也没有发送 `max_completion_tokens` 时，使用模型条目的 `max_tokens`，其默认值为 4096。编程客户端常常省略这个上限，期望得到模型自身的最大值，所以请把暴露的智能体所用模型的 `max_tokens` 设为你希望这类请求获得的值。被上限截断的回答会以 `finish_reason: "length"` 报告。

只有在省略某个请求字段不会改变客户端拿回的内容类型时，才会丢弃它；`response_format` 和指定了名称的 `tool_choice` 则会被拒绝，因为请求 JSON 却悄悄收到一段普通文本的调用方无从察觉。

## 运维

- **审计。** 使用 `journal: metadata`（此处的默认值）时，消息日志为每个请求和每个回答各保留一行，包含 ID、大小、用户和耗时，不含文本。
- **指标。** `agentflow_ephemeral_sessions`（运行中的单轮会话）、`agentflow_ephemeral_cap_drops`（因限制被拒绝的请求）和 `agentflow_sessions_unloaded`（被卸载的空闲对话），以及常规的 `agentflow_llm_*` 和 `agentflow_egress_*` 计数器。
- **Doctor。** `agentflow doctor` 会报告每个 `openai` 渠道的认证模式和暴露的智能体，并对 `auth: none` 发出警告。

## 兼容性

**目前还没有任何客户端被列为已验证。** 回放测试套件运行的是根据 OpenAI API 参考手写的测试夹具。只有当某个客户端的一对实际捕获的请求/响应被提交到 `cmd/agentflow/testdata/chatapi` 下并由测试回放后，该客户端才会被加入此列表。用于生成这些捕获的录制器是运行时仓库中的 `internal/drivers/chatapi/cmd/record`；把客户端的基础 URL 指向它，然后让客户端依次完成普通聊天、流式传输、一次工具调用及其后续请求。
