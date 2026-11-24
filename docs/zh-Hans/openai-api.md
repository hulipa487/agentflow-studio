# OpenAI 兼容 API

AgentFlow 可以把一个智能体暴露成 OpenAI **chat completions** 端点。任何会说 Chat Completions API 的客户端 —— 智能体框架、编程助手、聊天前端，或者直接调 SDK 的代码 —— 只需要一个 base URL 和一个 API 密钥。不用装转换层，也不用另开端口：这个渠道和其他所有渠道一样挂在同一个 `gateway.listen` 上。

这个端点是一个**渠道**，不是代理。它负责认证、解析、关联回复、说 SSE。它不判断语义：路由处理器挑会话，智能体的循环决定这个请求是什么。用内置路由加上透传循环（`examples/loops/openai_passthrough.ts`）时，每个请求都得到自己的一轮会话，并原样转发给智能体的模型，所以一个本身就是智能体的客户端 —— 有自己的系统提示词、自己的工具、自己的对话 —— 拿到的就恰好是一个模型。

## 快速开始

当智能体的循环用 `llm.forward()` 转发请求时，它就以自己的模型身份作答（随附版本是 `examples/loops/openai_passthrough.ts`）。渠道不会替你挑循环：列在 `agents` 下的智能体跑的就是它自己 `loop:` 指定的循环。

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

把客户端指向 `http://<host>:8080/v1` 并带上密钥，模型名用 `coder`。

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
| `path` | string | `/v1/` | 各端点挂载的前缀，必须以 `/` 结尾。发往其下其他路径的请求会得到 OpenAI 格式的 404 信封 |
| `agents` | list | — | 以模型 ID 形式暴露的智能体。每个名字都必须是已配置的智能体。只有一个时可用简写 `agent:`；两者同时设置会报错 |
| `auth` | string | — | 必填。`user_token`（按用户的 Bearer 令牌，需要 `runtime.identity.enabled`）、`key`（来自 `keys` 的共享密钥，以 Bearer 令牌或 `x-api-key` 发送），或 `none`（仅当 `gateway.listen` 是回环地址时可用） |
| `keys` | list | — | `auth: key` 接受的共享密钥。`${VAR}` 和 `cred:<service>` 引用在启动时解析；解析不出来会跳过该渠道并给出警告 |
| `timeout` | duration | `600s` | 整个请求，包括工具轮次 |
| `max_request_bytes` | int | `33554432` | POST 请求体上限；更大的请求返回 413 `request_too_large` |
| `journal` | string | `metadata` | `full` 保留文本和附件，`metadata` 只留 ID 和大小、**绝不留文本**，`off` 什么都不写。其他渠道类型默认 `full` |
| `expose_reasoning` | bool | `false` | 以 `reasoning_content` 发送模型的推理内容。这会把模型的思考过程暴露给调用方 |
| `media` | map | 关闭 | 渠道的媒体策略。内联图片或文件（data URL）不在 `media.allow` 里就返回 400 |

`auth: user_token` 的身份来自 Bearer 令牌，并打进消息由核心掌管的来源信息里，而不是取自请求体。用 `auth: key` 或 `none` 时，请求没有用户：记忆范围、配额和下文按用户的限制对它都不适用。请求里的 `user` 字段和 `metadata` 会作为**不可信**数据传给循环。

## 请求

| 字段 | 处理方式 |
| --- | --- |
| `model` | `agents` 里的智能体名。未知则 404 `model_not_found` |
| `messages` | 角色有 `system`、`developer`、`user`、`assistant`、`tool`。内容是字符串或片段：`text`、`image_url`、`input_audio`、`file`、`video_url` |
| `stream`、`stream_options.include_usage` | 支持 |
| `tools`、`tool_choice`、`parallel_tool_calls` | 转发给模型。AgentFlow **绝不执行**：模型的工具调用会原样返回，客户端在下一个请求里把结果送回来 |
| `temperature`、`top_p`、`max_tokens`、`max_completion_tokens`、`stop`、`seed`、`response_format` | 智能体的提供商支持就转发（见[提供商](#providers)） |
| `n` | 只支持 1；更大的值返回 400 |
| `user`、`metadata` | 作为不可信数据传给循环。`metadata.conversation_id` 或 `X-AgentFlow-Conversation` 请求头可以要求一个对话会话 |
| `Idempotency-Key` 请求头 | 见[会话](#sessions) |
| 其他 | 接受并忽略 |

内联媒体（`data:` URL）按渠道的 `media:` 策略存储，以句柄的形式到达循环和模型。远程 URL 会交给自己去抓 URL 的提供商；运行时绝不抓取客户端给的 URL。

`GET <path>models` 列出暴露的智能体，`GET <path>models/{id}` 描述其中一个。

## 响应与错误

响应是标准的 `chat.completion` 和 `chat.completion.chunk` 对象。`finish_reason` 用模型自己的值（`stop`、`length`、`tool_calls`、`content_filter`）。工具调用的 `arguments` 是模型给的参数文本，原样不动。`usage` 把 `input`→`prompt_tokens`、`output`→`completion_tokens`、`cached`→`prompt_tokens_details.cached_tokens`、`reasoning`→`completion_tokens_details.reasoning_tokens`。

流式下，文本边生成边到达（见[流式](#streaming)），工具调用在最后以一个完整块到达 —— 协议允许只有一个 `arguments` 片段 —— 然后是 `finish_reason` 块、设置了 `stream_options.include_usage` 时的用量块，以及 `data: [DONE]`。空闲的流每 15 秒发一条 SSE 注释，让代理保持连接。

错误使用 OpenAI 的信封 `{"error": {"message", "type", "code"}}`：

| 情况 | 状态码 | `code` |
| --- | --- | --- |
| 凭据缺失或错误 | 401 | `invalid_api_key` |
| 未知模型 | 404 | `model_not_found` |
| JSON 有误、`n > 1`、不支持的内容片段、渠道不接受的媒体 | 400 | `invalid_request_error` |
| 智能体的模型无法兑现的选项（见[提供商](#providers)） | 400 | `invalid_request_error` |
| HTTP 方法不对 | 405 | `method_not_allowed` |
| 请求过大 | 413 | `request_too_large` |
| 用户配额或智能体预算耗尽 | 429 | `insufficient_quota` |
| 达到会话上限 | 429 | `rate_limit_exceeded`（带 `Retry-After`） |
| 输入被安全策略拦截 | 400 | `content_filter`；方案有拒绝文案时就是它 |
| 输出被安全策略拦截 | 200 | `finish_reason: "content_filter"`；内容是方案的兜底文案，或为空 |
| 模型调用失败 | 502 | `upstream_error`（上游自己的消息只记日志，不返回） |
| 智能体的循环失败 | 500 | `server_error`（只记日志，不返回） |
| 路由处理器没有投递请求 | 503 | `server_unavailable` |
| 正在排空（就绪为 false） | 503 | `server_unavailable` |
| 请求超时 | 504 | `timeout` |

**流式请求把晚到的错误写进流里。** 流式响应在本轮运行之前就开始（HTTP 200 加角色块），所以只有在那之前能确定的情况才有 HTTP 状态码：401、404、请求格式错误的 400、405、413，以及排空时的 503。之后的一切 —— 限额、输入被拦、模型失败、超时 —— 都以一条 `data: {"error": {...}}` 事件到达，`type` 和 `code` 同上，随后是 `data: [DONE]`。如果文本已经发出去了，流会以 `finish_reason: "stop"` 正常结束，错误只记日志。

## 会话

内置路由处理器（`plugin:per_chat`）给每个 OpenAI 请求一个自己的会话：一个**短暂**会话，服务恰好一轮就消失。它的键每次请求都不同，所以不占租约、不写收件箱行，也永远不会被回收。

并发短暂会话按用户限制（`runtime.limits.ephemeral_per_tenant`，默认 16，`0` 关闭），与对话会话分开计算。这个限制需要用户，所以只在 `auth: user_token` 下生效；`auth: key` 或 `none` 没有按调用方的限制，共享密钥应该放在你自己的限流器后面。

想要对话的客户端用 `metadata.conversation_id` 或 `X-AgentFlow-Conversation` 请求头显式声明。这会路由到正常的具名会话，键由渠道和对话 ID 组成。ID 由客户端选择，不按调用方隔离：用共享密钥时，两个调用方发同一个 ID 就会共用同一个会话。

`Idempotency-Key` 只对仍在运行的请求有效：同一调用方、同一请求体、同一键的重试会加入正在跑的那一轮，并收到完整答案（流式会从第一块重放）。一轮结束后的重试是新的一轮，换了请求体或换了调用方也是新的一轮。消息 ID 由键派生（哈希，绝不是请求头本身）；请求 ID 每轮都是新的。

客户端断开就结束请求；短暂会话没人听了，引擎就把它停掉，不会再把 Token 花在没人读的答案上。对话的轮次则会继续跑完。

**区域单元。** 单轮会话在任何实例上都能用。对话会话只有在请求到达拥有该对话的实例时才会被应答：发到别处的请求会被排给所有者，而所有者没有可应答的连接，`llm.forward` 在所有者那边也读不到落在另一个实例上的请求体。`agentflow validate` 和 `agentflow doctor` 会对这种组合给出警告。在限制解除之前，在负载均衡器上把对话固定到一个实例，或者干脆别在区域单元里用对话 ID。

## 循环收到什么

请求体不过脚本桥。一个大的编程请求留在 Go 里，消息只带元数据和一个句柄：

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

`msg.text` 是最后一条用户消息的文本（最后一条是工具结果时为空）；安全方案入站检查的就是它。

| 调用 | 用途 |
| --- | --- |
| `openai.request()` | 完整请求，作为对象：`{ model, stream, messages, tools, tool_choice, params, user, metadata }`。超过 4 MiB 的 JSON 会被拒绝 |
| `llm.forward(opts?)` | 把本轮的请求转发给模型，不必先实体化。返回 `{ text, tool_calls, finish_reason, usage, thinking, thinking_blocks }` |
| `session.reply(t)` | 以结构化答案结束本轮：`{ text, tool_calls, finish_reason, usage, reasoning }` |
| `session.send_delta(text)` | 发送答案的一个文本片段 |
| `session.send(text)` | 把文本追加到答案；循环去取下一条消息时本轮结束 |

`llm.forward`、`openai.request` 和其他 `llm.*` 调用需要 `llm.chat` 能力。

## 透传循环

`examples/loops/openai_passthrough.ts` 只有八行：

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

不带参数调用 `llm.forward()`，会把客户端自己的消息、工具、工具选择和采样参数送给本轮所跑的模型：智能体的 `model`，或者用户有个人资料时用他的模型。它和每次模型调用走同一条计量路径：用户配额、智能体预算和用量台账都适用，台账行的 kind 是 `forward`。

### 流式

客户端要求流式、且智能体的安全方案没有过滤器时，文本增量边到边转发。方案里有过滤器时，答案会先缓冲、检查，再整块发出 —— 安全优先于流式，客户端仍然收到合法的流。

### 推理

`expose_reasoning: true` 时，模型报告的推理内容会以 `reasoning_content` 发送，放在消息里或工具调用前的一个块里。文本被安全方案拒绝的回复不带推理内容。

## 自己写循环

渠道不附加任何策略；循环可以丰富请求，也可以不以模型身份、而以助手身份作答。下面这个透传循环会先补上召回到的事实：

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

`agentflow-studio` 里的 `examples/loops/openai_memory.ts` 是这个模式的完整版本。

`llm.forward` 的选项：`model`（另一个 `models:` 条目）、`messages`（替换客户端的）、`prepend` / `append`（在前后加轮次）、`tools`（在客户端的基础上加定义）、`params`（覆盖采样：`temperature`、`max_tokens`、`top_p`、`stop`、`seed`、`response_format`、`parallel_tool_calls`、`tool_choice`、`thinking`）以及 `max_tokens_cap`（给客户端的 `max_tokens` 设上限）。

循环自己处理回复时，有三点要知道：

- 回复里的工具调用有 `args`（对象，供读取）和 `arguments`（模型自己的 JSON 文本）。`session.reply` 在 `arguments` 存在时会发它，以保留模型序列化出的精确参数。如果你改了某个调用的 `args`，请把它的 `arguments` 设成 `undefined` 或 `null`。
- 用 `session.send_delta` 发过片段、再用完整文本回复的循环，文本只会发一次：回复只补上片段没有携带的部分。
- 不用 `session.reply` 结束本轮的循环照样会作答：它用 `session.send` 或 `session.send_delta` 发出去的内容就是答案，`finish_reason` 为 `"stop"`。什么也没产生的轮次就是一条空的助手消息。

别把持久状态放在脚本局部变量里。单轮会话没有状态要保，对话会话空闲时会被卸载（见 [`runtime.sessions.idle_unload`](configuration.md)）；请用 `session.state.*`、记忆或存储。

## 提供商

客户端发的东西会经由智能体模型的提供商转发过去，而各家是不一样的。

| 提供商 | 说明 |
| --- | --- |
| `openai` | 上面说的一切原样发送，包括 `developer` 消息。`max_completion_tokens` 以 `max_tokens` 发出 |
| `openai-responses` | `stop` 和 `seed` 没有对应项，会被丢弃 |
| `anthropic` | `system` 和 `developer` 消息会合并进系统提示词。`seed` 被丢弃。`response_format` 除 `{"type": "text"}` 之外一律返回 400 |
| `gemini` | 客户端的 `tools` 会被 400 拒绝，所以会发工具的编程客户端用不了 Gemini 智能体。`temperature`、`top_p`、`parallel_tool_calls` 被丢弃。`response_format` 除 `text` 之外，以及以远程 URL 给出的媒体，都会被 400 拒绝 |

**输出上限。** 客户端既没发 `max_tokens` 也没发 `max_completion_tokens` 时，用模型条目自己的 `max_tokens`，默认 4096。编程客户端经常不设上限，指望拿到模型自己的最大值，所以请把暴露智能体所用模型的 `max_tokens` 设成你希望这类请求获得的值。被上限截断的答案会以 `finish_reason: "length"` 报告。

只有当省略某个字段不会改变客户端拿到的结果种类时，才会把它丢掉；`response_format` 和具名 `tool_choice` 则直接拒绝 —— 因为要求 JSON 却悄悄收到散文的调用方根本分辨不出来。

## 运维

- **审计。** 用 `journal: metadata`（此类型的默认值）时，日志为每个请求和每个答案各留一行，含 ID、大小、用户和耗时，不含文本。
- **指标。** `agentflow_ephemeral_sessions`（活跃的单轮会话）、`agentflow_ephemeral_cap_drops`（被限额拒绝的请求）、`agentflow_sessions_unloaded`（被卸载的空闲对话），与常见的 `agentflow_llm_*` 和 `agentflow_egress_*` 计数器并列。
- **Doctor。** `agentflow doctor` 报告每个 `openai` 渠道的认证方式和暴露的智能体，并对 `auth: none` 给出警告。

## 兼容性

**目前还没有客户端被列为已验证。** 回放套件跑的是根据 OpenAI API 参考手写的样本。只有当某个客户端的一对真实请求/响应被提交到 `cmd/agentflow/testdata/chatapi` 并由测试回放后，这个客户端才会被加进列表。生成这类记录的录制器在运行时仓库的 `internal/drivers/chatapi/cmd/record`；把客户端的 base URL 指向它，然后让客户端走一遍普通聊天、流式、一次工具调用及其后续。
