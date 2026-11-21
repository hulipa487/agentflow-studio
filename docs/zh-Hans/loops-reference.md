## Lua 插件开发

AgentFlow 插件是标准的 Lua 5.1 脚本，由内置纯 Go 虚拟机（`gopher-lua`，带可挂起的 `pcall`/`xpcall`、`utf8`、`bit32` 与 `table.unpack`/`pack` 辅助函数）执行。YAML 决定哪个智能体运行哪个插件；插件决定行为。本文档中的长篇指南（循环）介绍执行模型、完整 API、可直接运行的模式以及常见陷阱清单；本节是参考摘要。

插件分三类：

- **循环插件**——定义一个全局函数 `loop()`，在会话协程的整个生命周期内掌管它。标准写法是 `while true do local msg = session.inbox() ... end`。
- **辅助代码块**——在循环插件*之前*加载到每个会话状态中。辅助代码块定义循环要读取的全局变量和辅助函数。`plugin:token_budget`、`plugin:routing_table`、`plugin:recency` 和 `plugin:semantic` 等内置插件都是辅助代码块。
- **覆盖插件（shadow）**——放在 `plugins/` 中、与某个内置插件同名的用户插件。它会在加载时于整个实例范围内替换该内置插件。

### 异步桥接

暴露给 Luau 的唯一原生原语是 `__af_op(json_request)`。它让出协程；Go 执行该请求（内联执行或交给工作池）；随后协程以 `(json_response, ok)` 恢复。prelude 把这个原始原语封装成下文所述的易用表。由于每次调用都会让出执行，繁忙的实例开销依然很低——没有任何操作系统线程会因等待模型、数据库或 HTTP 响应而阻塞。

```
-- prelude 替你做的事（概念上）：
local function op(req)
  local resp, ok = __af_op(json.encode(req))
  if not ok then error(resp, 0) end
  return json.decode(resp)
end
```

阻塞操作包括 `send`、`llm.chat`、`tools.run`、所有 `store.*`、所有 `shell.*`、`http.request`、各 `mail.*` 操作、`agent.*` 和 `scheduler.*`。`inbox`、`log`、`agent.info` 等快速操作在 actor goroutine 上内联执行。

### 消息生命周期

`session.inbox()` 会挂起协程，直到会话邮箱收到一条消息。消息是一个表，至少包含：

```
{
  id = "msg-...",
  type = "user",           -- "user" | "system" | "timer" | "cron" | "agent" | "shell.result" | ...
  from = "webhook:alice",  -- 由核心标记的发送者
  text = "hello",
  channel = "webhook",
  reply_to = "...",
  ts = 1234567890,
  -- 由核心分配，循环无法控制。user_uuid 是本轮次所属的租户；没有任何组件
  -- 标记该消息时它不存在；消息不属于任何租户（未关联的账号）时它为 ""。
  -- 身份要从这里读取，绝不要从 payload 读取——转发一跳时的 payload 是由调用方的 Lua 重建的。
  provenance = { kind = "agent", principal = "agent:planner", user_uuid = "u_xxxx" }
}
```

`session.send(text)` 通过当前渠道回复（即当前消息的渠道和回复目标）。`session.push(channel, reply_to, text)` 主动发送，需要 `channel.push` 能力。`session.push_user(uuid, text)` 按身份 UUID 向用户推送。`session.exit()` 干净地结束会话。

### 编写循环插件

一个最小的回声循环如下（每个代码块都以 prelude 版本开头）：

```
-- af-prelude-version: 1
log.info("echo agent online")

function loop()
  while true do
    local msg = session.inbox()
    log.info("message from " .. (msg.from or "?"))
    time.sleep(0.3)
    session.send("echo: " .. (msg.text or ""))
  end
end
```

ReAct 风格的循环会调用 LLM，并可选择调用工具：用 `memory.recall` 获取历史，把 `tools.list` 中的工具定义传给 `llm.chat`，用 `tools.run` 执行返回的工具调用，再用 `session.send` 回复。把它写成一个普通的循环文件，并让 `loop:` 指向它。

### 内置 Lua API 参考

下面所有的表只有在智能体的能力允许时才会注入。调用已停用的 API 会抛出错误。

#### `session` — 核心

```
session.inbox() -> Message            -- 挂起，直到邮箱收到下一条消息
session.send(text)                    -- 回复当前的入站消息
session.send(text, { attachments = parts }) -- 附带媒体发送（见下文）
session.reply(t)                      -- 以结构化回答结束本轮次：
                                      -- { text, tool_calls, finish_reason, usage, reasoning }
session.send_delta(text)              -- 流式回答的一个片段
session.push(channel, reply_to, text) -- 主动出站；能力：channel.push
session.push(channel, reply_to, text, { attachments = parts })
session.push_user(uuid, text)         -- 主动向某个用户 UUID 出站；能力：channel.push
session.exit()                        -- 结束本会话

session.state.set(key, value)         -- 能力：session.state（需声明）
session.state.get(key)                -- 该键从未设置过时返回 nil
session.state.delete(key)
session.state.list()                  -- 本会话的 { key = value }
```

渠道接收了媒体时，消息会带有一个 `attachments` 数组，其中是媒体部件描述符（`{type, mime, handle, name}`）。把它们原样传给 `llm.chat` 或 `session.send`；运行时会在提供商 / 渠道边界把 blob 句柄（"media:<sha256>"）解析为字节——原始媒体从不进入 Lua。

`session.reply(t)` 是回复的结构化形式：`tool_calls`（`{ id, name, args, arguments }`）、`finish_reason`（`stop`、`tool_calls`、`length`、`content_filter`）、`usage` 和 `reasoning` 会传递给能够表达它们的渠道，其文本与 `session.send` 走同一条安全与审计记录路径。`llm.forward` 或 `llm.chat` 的结果可以直接传入：其中的 `thinking` 被当作回复的推理内容，只有配置为公开推理内容的渠道才会发送它。工具调用的 `arguments`（模型自己生成的 JSON 文本）会优先于其 `args` 表发送，因为 Lua 表无法区分 `[]` 和 `{}`；如果你修改了某个调用的 `args`，请把它的 `arguments` 设为 `nil`。每个轮次最多一次回复；在无法表达工具调用的渠道上，携带工具调用的回复会直接失败，而不会悄悄丢弃它们。如果回复文本被出站安全拒绝，该回复会以 `finish_reason = "content_filter"` 送达，不带工具调用和推理内容；内容是方案中的替代回复文本（若有），否则为空。

`session.send_delta(text)` 用于流式回答。只有当安全方案不可能丢弃出站内容时，引擎才会实时转发片段；否则会先扣留，在轮次结束时放出检查过的完整内容。既流式输出又调用 `session.reply` 的循环应让回复携带完整文本：无论客户端是否请求了流式输出，回复只会补上片段尚未携带的部分。

`session.state` 是仅供本会话使用的小型持久键值存储：一个游标、一个步数、最后说过的话。它在重启后依然保留；在集群中，它会跟随会话转到下一个拥有该会话的实例，因为这些条目保存在共享存储中。键可以包含字母、数字和 `. _ - :`，最长 128 个字符；值可以是任何能编码为 JSON 的 Lua 值，最大 64 KiB；一个会话最多可保存 256 个键。它需要主动启用——在智能体的 `capabilities` 中声明 `session.state`，因为它是一个持久化入口，不应默认开放。

#### `llm` — 能力：`llm.chat`

```
llm.chat(messages, opts?) -> { text, thinking, thinking_blocks, usage, tool_calls }
llm.stream(messages, opts?) -> iterator
llm.forward(opts?) -> { text, tool_calls, finish_reason, usage, thinking, thinking_blocks }
llm.embed(inputs, opts?) -> { vectors, usage }   -- 字符串和 / 或部件表
llm.rerank(query, documents, opts?) -> { results } -- results：[{ index, score }]，从 0 开始

-- opts: { model = "default", temperature = 0.7, max_tokens = 4096,
--         thinking = "low"|"medium"|"high"|"xhigh"|"max"|"off",
--         tools = [...], tool_choice = "auto" | "none" | "required" | "function:<name>" | { name = "<name>" },
--         parallel_tool_calls = true, top_p = 0.9, stop = {...} (or one string), seed = 1,
--         response_format = { type = "json_schema", json_schema = {...} } }

-- forward opts: { model, messages, prepend, append, tools, params, max_tokens_cap }
```

`llm.forward` 是 OpenAI 渠道的透传。不带参数时，它把客户端自己的请求——消息、工具、tool_choice 和采样参数——转发给本轮次所用的模型（智能体的模型，或用户资料中的模型），走与 `llm.stream` 相同的计量流式路径：预算和配额照常生效，用量账本会记下一条类型为 `forward` 的记录。`opts.messages` 替换客户端的消息列表，`prepend`/`append` 在其前后添加轮次，`tools` 添加定义，`params` 覆盖采样参数，`max_tokens_cap` 限制客户端给出的值。提供商无法表达的选项会被丢弃而不是拒绝，但 `response_format` 和指定名称的 `tool_choice` 例外，它们会使调用失败。结果中的 `finish_reason` 是模型自己给出的（输出上限截断回答时为 `length`），每个工具调用除了解码后的 `args` 之外，还带有 `arguments`，即模型的参数文本。只有当客户端请求了流式输出且智能体的安全方案没有任何过滤器时，它才会实时转发文本增量；否则返回完整文本供 `session.reply` 使用。它不会结束 HTTP 响应。模型调用失败时会抛出以 `upstream_error:` 开头的错误。在 OpenAI 渠道的轮次之外，且没有 `opts.messages` 时，它会返回错误。`openai.request()` 以表的形式返回完整请求，JSON 超过 4 MiB 时拒绝——这种情况下应转发该请求，而不是把它物化出来。两者都需要 `llm.chat` 能力。`llm.chat` 的回复和 `llm.stream` 的最后一帧也带有 `finish_reason`。

消息是 `{ role, content, tool_calls?, tool_call_id?, tool_result?, thinking_blocks? }` 的数组。**多模态轮次**使用 `{ role, parts = {...} }`；每个部件是一个表：

```
{ type = "text",  text = "..." }
{ type = "image"|"audio"|"video"|"file", mime = "image/png", handle = "media:...",
  name = "photo.jpg" }   -- handle 来自 msg.attachments（也可以用 data base64 / url）
```

提供商支持矩阵（不支持的部件类型会如实报错；来源可以是 base64 `data`、`url` 或提供商的文件引用）：

| 提供商 | 图片 | 音频 | 视频 | PDF / 文件 |
| --- | --- | --- | --- | --- |
| anthropic | ✅ base64/url | ❌ | ✅ base64/url（MiniMax 视频块） | ✅ document（base64） |
| openai (chat) | ✅ image\_url | ✅ input\_audio (wav/mp3) | ✅ video\_url——MiniMax/Kimi/GLM 约定 | ✅ file\_url (url)——GLM |
| openai-responses | ✅ input\_image | ✅ input\_audio (wav/mp3) | ✅ input\_video（url/文件引用） | ✅ input\_file（data 或 file\_id） |
| gemini (interactions) | ✅ files api | ✅ files api | ✅ files api | ✅ files api |

视频来源遵循生态中的约定——MiniMax `mm_file://<file_id>`、Kimi `ms://<file_id>`——并作为 URL / 文件引用原样传递。SDK 的类型化请求联合类型没有建模的结构（`video_url`、GLM 的 `file_url`、Anthropic 视频块、Responses 的 `input_video` 和 `input_audio` 项）用 `param.Override` 构建——这是 SDK 自带的原始 JSON 逃生口，在没有设置类型化变体时会原样序列化。blob 存储的 `handle` 会自动解析为 base64。

对于 gemini，每个媒体部件都会上传到 **Gemini Files API**（可续传上传）并通过 URI 引用——从不内联 base64。`data`（base64）、`url` 和 blob 存储 `handle` 这几种来源都可用（url 会先下载）；在文件 48 小时的有效期内，相同字节的重复上传会按内容哈希去重，到期后 Google 会自动删除该文件。Anthropic / openai 也直接接受 `url` 来源。`llm.stream` 返回两个值——`next_delta, close`。请直接调用 `next_delta()`（结束时返回 nil），绝不要放在泛型 `for ... in` 中：迭代器会跨桥接让出执行，而 Luau 不允许从泛型 for 的 C 边界让出。提前退出后请调用 `close()`；迭代器在结束时会自行关闭。它只返回文本增量——需要用量和工具调用时请用 `llm.chat`。

**多模态嵌入**遵循 Jina 约定：`llm.embed` 接受字符串与部件表混合的数组，携带媒体的批次会把每一项序列化为类型化文档（`{"text"}`、`{"image"|"video"|"audio"|"pdf": url|base64}`）——这正是 `jina-embeddings-v5-omni-small`（文本、图片、视频、音频和 PDF 位于同一向量空间）和 `jina-clip-v2` 使用的结构。纯文本批次仍是普通字符串，因此任何兼容 OpenAI 的 `/embeddings` 端点（OpenAI、Ollama、vLLM、TEI、OpenRouter）看到的请求与以往完全一致。选项：`task`（`retrieval.query` | `retrieval.passage` | `text-matching` | `clustering` | `classification`）、`dimensions`（Matryoshka 截断）、`merged = true`（把所有输入合并为一个嵌入——即 Jina 的内容组；PDF 只能单独输入）。Anthropic 和 Gemini 提供商在这里没有嵌入 API，会明确报错。`llm.rerank` 需要一个 `provider: rerank` 的模型条目，它使用 `POST {base_url}/rerank` 协议（TEI、vLLM、Jina、Cohere）。智能体声明了预算时，`llm.embed` 会按 Token 计量。`gemini` 提供商（Interactions API）不支持流式：在它上面调用 `llm.stream` 会把完整回复作为单个增量返回。

**思考内容（响应侧）。** 提供商发送的任何推理内容都会被动地呈现在回复上——`reply.thinking` 是人类可读的文本（没有时为空字符串），`reply.thinking_blocks` 是提供商的原始块（Anthropic 的 `{type="thinking", thinking, signature}` / `redacted_thinking`、Responses 的 reasoning 项、Gemini 的 `thought` 部件；在 Chat Completions 上为 nil，因为那里的推理内容只是一个字符串），`usage.reasoning` 则在提供商单独报告时统计思考 Token。是否有推理内容是因提供商、因模型别名而异的运行时属性，而不是一项约定：某种服务配置可能对推理 Token 计费，却完全不在流中返回推理内容，所以 `usage.reasoning` 才是可靠的审计信号，`reply.thinking` 为空本身并不意味着故障。引擎从不在思考级别已请求的范围之外额外索取推理内容，也从不存储它——是否持久化由循环决定。对于启用了思考的 Anthropic 多轮工具调用，请在 assistant 轮次中把这些块原样传回：`thinking_blocks = reply.thinking_blocks`（否则 Messages API 会拒绝后续请求；其他提供商会忽略该字段）。

```
-- 多模态嵌入（通过 api.jina.ai/v1 使用 jina-embeddings-v5-omni-small）
local r = llm.embed({
    "find me the cat photo",                    -- 文本
    { type = "image", handle = msg.attachments[1].handle },  -- blob 存储中的媒体
  }, { model = "omni", task = "retrieval.query", dimensions = 512 })

-- 为整条多模态记录生成一个向量（当方案指定了 omni embed_model 时，
-- memory.write 会对带附件的记录这样做）：
local r = llm.embed({ "trip to the beach", img_part }, { model = "omni", merged = true })
```

#### `agent` — 能力：`agent.send`、`agent.request`、`agent.spawn`

```
agent.info() -> { name, session_id, address, model, instructions,
                  history_budget, memory, shell, skills, capabilities,
                  memory.stores[name] = { backend, table, features, locality }, ... }
agent.config() -> { name, model, instructions_path, instructions_prompt?,
                    prompts = {  = "", ... },
                    goal = {type, success_signal, max_turns, on_goal_met}?,
                    extras = { ... } }
agent.send(address, payload)
agent.request(address, payload, timeout?) -> Message
agent.reply(request_id, payload)
agent.list() -> [{ session_id, agent, parent_id }]
agent.spawn(profile) -> { address, session_id, agent }
```

存储的 `locality` 为 `deployment`、`region` 或 `instance`。持久的对话数据应放在覆盖整个部署或整个区域的存储上；实例本地的存储只是缓存，其内容随进程一起消失（在 regional 单元中，放在实例本地存储上的持久存储会在校验时被拒绝，除非它设置了 `locality_ok: true`）。

`agent.spawn` 需要一个 `profiles.agent` 模板；子智能体的能力是父智能体与模板能力的交集。早期版本接受的第二个参数现已保留，不再生效——请改为配置模板。

`agent.config()` 以数据形式返回该智能体自己的方案：配置名称、模型、指令的*路径*（如果系统提示词来自 `prompts` 注册表，则为 `instructions_prompt`，此时 `instructions_path` 为空）、部署中已解析、按名称索引的 `prompts` 注册表、提升为 `goal` 的方案 `extras.goal`，以及其余的 `extras`（一个 pm 选项块、一个工作流名称……）。其中的密钥引用会渲染为不透明标记（`{env="VAR"}` / `{cred="service"}`）——已解析的密钥从不会跨过 Lua 桥接。

#### `user` — 能力：`users`

```
user.current() -> { registered, id, display_name, identities }
user.usage() -> { registered, input, output, cached, reasoning, calls,
                  failed, billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { service, present }
user.get(id) -> { ... }    -- 仅限维护来源
user.list() -> [ { ... } ] -- 仅限维护来源
```

关于本轮次所属用户的只读信息。`users` 是默认能力，所以循环无需声明任何东西就能调用这些函数。循环能获知名字和渠道，但永远拿不到邮箱，也拿不到凭据的值——`has_credential` 回答的是某个密钥是否存在，而不是它是什么。`get`/`list` 仅限由引擎触发的维护来源（`system` 或 `scheduler` 轮次）使用，正因如此，才能实现按用户执行的维护循环，又不会把一个用户的资料暴露给另一个用户的轮次。

#### `runtime` 和 `credential` — 以数据形式提供的部署配置

```
runtime.triggers() -> { ok, triggers = [ { name, kind, run_on_boot,
                          target = { profile }, payload,
                          event = { channel, match }   -- kind = "event"
                          | cron                        -- kind = "cron"
                          | every } ] }                 -- kind = "every"

credential.get(name) -> { ok, name, value }              -- 仅限允许列表中的名称
```

`runtime.triggers()` 返回启动时快照的合并后的 `triggers/*.yaml`（或顶层 `triggers:`）列表——从引擎读取你的 CRON\_TASKS/ROUTES，而不要把这些表硬编码进 Lua 源码。它在**两种** Lua 环境中的回答方式相同：智能体循环和网关路由处理程序（路由器的操作分发提供完全相同的载荷），因此事件路由可以读取驱动它自己的触发器列表。表达式原样传递，其结构也不受引擎调度影响：`every:`/`cron:` 条目同样由引擎自己触发（见配置参考中的 `triggers`），所以循环只有在需要*观察*部署的调度时才用得上这个调用。`credential.get(name)` 获取引擎全局存储的密钥，**仅当**智能体的方案在 `credentials: [...]` 中列出了该名称时才可用（列表为空或缺失 = 拒绝，并返回说明原因的错误）。每次访问都会记录日志（智能体、名称、允许 / 拒绝）并计数（`agentflow_credential_gets` / `agentflow_credential_gets_denied`）；该值从不写入审计记录，也绝不应回显到回复中。

`router.state` 是路由处理程序自己的持久键值存储——处理程序在这里保存不能忘记的东西，例如某个聊天上次被路由到了哪个智能体。它位于运行时存储中，所以每个实例读到的值都相同，重启或故障切换也不会丢失：`router.state.get(key)`（该键从未设置过时为 nil）、`router.state.set(key, value, {ttl_seconds = n})`、`router.state.delete(key)` 以及 `router.state.list(prefix)`。值可以是任何能编码为 JSON 的 Lua 值，最大 64 KiB；键可以包含字母、数字和 `. _ - :`（128 个字符）。除非处理程序要求不同的有效期（`ttl_seconds = 0` 表示保留到被删除为止），否则键会在最后一次写入后保留 30 天，这就是当处理程序以高基数的值作为键时限制存储规模的机制。该命名空间只在路由器的状态中定义，因此智能体循环无法访问——它提供的是一致性而不是串行化：两个实例可能对同一个键交错执行读-改-写，而消息投递本身由会话租约串行化。

#### `scheduler` — 不受能力控制

```
scheduler.every(interval) -> timer_id    -- 单位为秒；随会话结束而失效
scheduler.after(delay) -> timer_id
scheduler.cron(expr) -> timer_id          -- cron 子集（见下文）
scheduler.cancel(timer_id)
```

计时器会把一条 `Message{ type = "timer" }` 放入所属会话的邮箱——其中带有 `msg.payload.timer_id`，让会话能区分自己的各个计时器。它们从不在计时器 goroutine 中调用 Lua，因此 VM 始终保持单线程。`scheduler.cron` 只支持 5 字段 cron 的一个**子集**：`*/N * * * *`（每 N 分钟）以及 `30 9 * * *` 这类固定分钟 / 小时的形式；日、月和星期字段必须为 `*`，而且 `*/N` 分钟不能与固定小时搭配（步长无法锚定在某个小时上，否则会被悄悄丢弃）。其他写法一律拒绝而不是重新解释——更复杂的调度请使用 `every`/`after`（或自行重新调度）。

这些是*会话范围*的计时器：它们随会话结束而失效。**不存在 `scheduler` 能力**——这些操作在 actor 自己的分发逻辑中处理，从不经过能力关卡，所以任何循环无论声明了什么都可以注册计时器。能力关卡覆盖的是可通过处理程序映射访问的操作（`llm.*`、`memory`/`store.*`、`tools.*`、`shell.*`、`http.request`、`mail.*`、`agent.*` 地址）；计时器是引擎投递的一条消息，而不是一种权限。在 YAML 中声明的部署级调度（`every:`/`cron:` 触发器）则属于引擎基础设施——由引擎触发，不涉及任何能力，以 `Message{ type = "cron" }` 的形式投递，所以循环不必自己跑一个计时器循环来接受调度。循环需要推算时钟本身时，请使用 `os.time()`/`os.date()`（UTC）。还要注意，计时器状态保存在内存中：重启后所有计时器都会被遗忘，直到其循环重新注册。

#### `memory` 和 `store` — 能力：`memory`

```
memory.write(record) -> boolean          -- 由 plugin:routing_table 路由
memory.recall(query, opts?) -> [ { key, value, meta } ] -- 当前生效的召回处理程序

store.put(table, key, value, opts?)      -- opts: { ttl = seconds, vector = {...} }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records }
store.delete(table, key)
```

存储查询接受 `{ kind = "all" | "prefix" | "text" | "vector" | "time_range", ... }`；向量查询的形式为 `{ kind = "vector", vector = {...}, k = n }`。向后端不具备 `vector` 特性的存储传入 `vector` 会明确报错，绝不会悄悄丢弃。智能体记忆请优先使用 `memory.write`/`memory.recall`；只有需要直接访问原始表时才使用 `store.*`。

**按用户划分范围（私有存储）。** 由渠道发起的轮次只读写它自己的用户范围，外加 `service` 条目（由没有自己租户的上下文写入：引擎触发的工作、未关联的账号，以及通过 `inherit_user: false` 选择退出的智能体）和 `legacy` 条目（在此功能上线前写入——升级前的历史对该智能体的所有用户保持可读，且不会再被写入）。引擎触发的上下文（`system`/`scheduler` 来源——例如蒸馏器这样的夜间维护）以带范围标识的键读取所有范围，并显式指定范围写入；维护会话可以通过 \`store.scopes\` 枚举某个绑定的用户范围（仅限维护：渠道会话和智能体转发会话会被拒绝）。这由引擎强制执行：无需在循环代码里筛选。绑定到池的存储（`pool: <name>`）则改由租户授权管理，私有存储也可以用 `scope: agent` 为每个智能体重新接入一个池。

#### `tools` — 能力：`tools`

```
tools.list() -> [ToolSpec]
tools.run(name, args, opts?) -> ToolResult
-- opts: { confirmed = true, confirm_id = "..." }

tool.def({ name = "lua:shout",
           description = "Shout the text back",
           params = { type = "object",
                      properties = { text = { type = "string" } },
                      required = { "text" } },
           handler = function(args)
             return { ok = true, shouted = string.upper(args.text) }
           end })
```

如果某个工具声明了 `needs_confirm: true`，第一次调用会返回 `needs_confirm` 响应。运行时会询问用户，然后以 `opts.confirmed = true` 重新调用该工具。你的循环只会看到最终结果。（等待确认的时间是有限的：十分钟，超时后调用被拒绝——在调度器轮次中，需要确认的工具没有可供回答的渠道，而无限期等待会让会话永远挂起。）

**工具循环由循环负责。** 引擎在被要求时执行工具并返回结果；它不驱动对话。带 `tools` 调用 `llm.chat`、把返回的内容交给 `tools.run` 执行、把结果作为一个 `role = "tool"` 轮次发出后再次询问——这些都是代码块的工作，包括走*第二*轮，而这一步最容易被忘记：模型看到第一个结果后若还要调用另一个工具，除非代码块再次检查 `reply.tool_calls`，否则它得不到。循环持久化的内容也是如此：工具调用及其结果是下一轮次应当看到的历史；只保存 assistant 文本的代码块，召回的会是一段模型提到某个工具结果、而那个结果已不复存在的对话。

`tool.def` 在 Lua 中声明一个对模型可见的工具——这是引擎标准的方式，用来取代手工拼装的额外工具定义。声明的工具会合并到 `tools.list` 中（与 Go 工具的 `{ type = "function", function = {...} }` 结构相同，因此可以原样传入 `llm.chat`），`tools.run` 会把调用分派给 Lua 处理程序。处理程序在循环协程上同步运行，自身也可以调用任何操作（`llm.chat`、`memory.*`……）。在 list 和 run 中，声明的名称都会覆盖同名的 Go 注册工具（覆盖在 `skills` 筛选之后应用，因此该智能体看不到的工具绝不会屏蔽它所同名的 Go 工具）。处理程序出错时会返回 `{ ok = false, error = ... }`，而不会终止循环。声明的工具是循环自己的代码：Go 工具策略（`forbidden`、`needs_confirm`、`permission: forbidden`）依然不管控它们。请在代码块顶层、`loop()` 之前声明。

**可见性**：声明的工具按与 Go 工具完全相同的规则，由 `skills` 和 `tools.policy.default` 筛选——`skills` 列出它时可见；`skills` 非空但没有列出它时隐藏；`skills` 为空时按 `default` 全部可见或全部隐藏。正因如此，一个共享的循环目录才能携带只有部分智能体能用的工具。对于设置了 `default: none` 且没有 `skills` 的部署，这是一个行为变化：声明的工具以前会无条件列出，现在不会了。`tools.run` 应用同样的规则——智能体看不到的声明名称会落到 Go 路径上，并被当作不可用而拒绝，与不可见的注册工具完全一样。注册工具一直在分派时受到强制约束，而 `tools.run` 正是模型幻觉出的或被注入的工具调用所走的面向模型的路径，所以两者保持一致。想调用自己处理程序的循环，直接调用该函数即可。

**覆盖声明工具的文本**：`tools.policy.overrides` 可以指定某个声明的工具，从而把它面向模型的文本从代码块移到配置中——描述（字面量，或来自 `prompts` 注册表的 `{prompt: <key>}`）以及 `params.<param>.description`。没有覆盖项指定该工具时，传给 `tool.def` 的定义仍作为后备，覆盖项也绝不会改动声明的 schema 中的类型或 `required`。解析在构建 `tools.list()` 时进行，因此重新加载的 `file:` 提示词会在下一次调用时生效，无需重启。覆盖项从不增删列表条目：可见性来自 `skills` 和策略默认值，而不是覆盖项。已声明但被隐藏的工具仍算作已声明，所以指定它的覆盖项不会被报告为没有指向任何工具。

#### `shell` — 能力：`shell.exec`

```
shell.run(cmd, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out, truncated,
                             project, environment, changed, deleted, conflicts, skipped }
shell.start(cmd, opts?) -> { id, state = "running", ... }     -- 结果会送到收件箱
shell.status(job_id)    -> job
shell.cancel(job_id)    -> job

-- opts: { project = "site", environment = "python", workdir = "app",
--         env = { KEY = "val" }, timeout = 120 }             -- timeout 单位为秒

-- 句柄：对 SSH 是真实连接，对沙箱是兼容垫片。
shell.spawn(opts) -> { id, provider, state }
shell.exec(id, cmd, opts?) -> the same result as shell.run
shell.write(id, path, content)
shell.destroy(id)
```

**并不存在一个持续存在的沙箱 shell。** `shell.run` 是在一个临时沙箱（[gVisor](https://gvisor.dev) `runsc` 或 [microsandbox](https://github.com/superradcompany/microsandbox) `msb`）中执行的一条命令，沙箱为这条命令启动，执行完即销毁。沙箱得到两样东西：一个已声明的**环境**作为工具链根目录，以及来自文件存储的一个**项目**，以读写方式挂载在 `/workspace`，命令就从这里开始执行。`/workspace` 之外的临时状态会在结束后丢弃。没有网络。命令结束时，引擎会把项目目录与它放进去的内容作比较，把差异写回项目的工作树——也就是 `files.*` 读写的同一棵树。何时 `files.commit` 由循环决定。

所以循环要遵守的规则很简单：

- **命令在项目中写入的内容会保留，其他一律不保留。** 后台进程、shell 变量、安装的软件包以及 `/workspace` 之外的文件都不会留到下一次调用。
- **非零的 `exit_code` 和 `timed_out` 都是结果**，就像 `http.request` 返回的 HTTP 500。根本无法运行的作业——本主机上没有沙箱、未知的环境、排队超时、沙箱因超出内存限制被杀掉——会抛出错误，所以请用 `pcall` 包裹调用。命令被杀掉之前写入的内容仍会保存。
- 在一个实例上，**每个项目同一时间只运行一个作业**；对同一项目的第二次调用会等待第一次完成（最长 `runtime.sandbox.queue_timeout`）。
- **写回绝不会覆盖更新的文件。** 作业修改的某个路径如果在作业运行期间被别人修改过，就保留存储中的版本，并列入 `conflicts`。无法存储的路径（超过单文件上限、超出作业的写回限制、符号链接）会连同原因列入 `skipped`，并从目录中移除，使目录中永远不会有存储里没有的东西。
- **下一条命令可以在任何实例上运行。** 实例上的项目目录只是文件存储的缓存。

设置分层：调用的 `opts` 覆盖项目的 `.agentflow/shell.yaml`，后者又覆盖智能体的 shell 方案。环境必须是部署中已声明的环境；时间上限受 `runtime.sandbox.max_timeout` 限制。项目的清单文件可以设置 `environment`、`env`、`workdir`、`timeout` 和 `ignore`，除此之外不能设置别的：

```
# .agentflow/shell.yaml，位于项目的工作树中
environment: node
env: { NODE_ENV: test }
workdir: app
timeout: 120s
ignore: ["node_modules/", "dist/"]
```

被 `ignore`（来自方案或清单文件）匹配的路径既不存储也不清理：它们作为尽力而为的缓存——依赖目录、构建缓存——留在生成它们的实例上，在其他实例上则直接不存在。

**后台作业。** `shell.start` 把同样的作业排入队列并立即返回，时间上限更长（`runtime.sandbox.background_max_timeout`）。其结果以一条 `type = "shell.result"` 的消息送达会话收件箱，作业信息在 `msg.payload` 中（`state` 为 `finished`、`failed` 或 `cancelled`；`stdout` 和 `stderr` 为最后 16 KiB）。结果轮次以启动该作业的用户身份运行。`shell.status` 在作业运行期间读取其状态——可从任何实例读取——`shell.cancel` 则在运行该作业的实例上停止它。所在实例已宕掉的作业会报告为 `failed`；它不会被重新运行。会话结束时，其后台作业会被取消。

**句柄。** 带 `provider = "ssh"` 的 `shell.spawn` 会打开一条到真实远程机器的连接（`host`、`user`、`password` 或 `key_file`）；`exec`、`write` 和 `destroy` 作用于该连接，它存在于发起连接的实例中，并在会话结束时关闭。SSH spawn 可以携带 `project` + `ref` 以及一个 `volumes = { "x:/dir" }` 条目，在句柄报告就绪之前把文件快照复制到远程机器的 `/dir`，或者用 `scratch_mount` 复制会话的临时文件。

不带 provider 的 `shell.spawn` 返回一个*沙箱句柄*，它的存在是为了让为旧容器 shell 编写的循环继续工作：它只是把收到的选项编码进 id。对它调用 `shell.exec` 会运行一个作业，`shell.write` 写入项目，`shell.destroy` 什么也不做。它不需要存储，可在任何实例上使用。有三个选项的含义变了：`image` 指定一个已声明的环境，`volumes[1]` 的容器路径就是项目的挂载位置（取代 `/workspace`），而 `"none"` 以外的 `network`、`"main"` 以外的 `ref` 以及 `scratch_mount` 会被拒绝，并附带说明原因的消息。

绑定了 shell 方案（`agents.<name>.shell`）的智能体还会获得一些注册表工具，它们隐式解析该方案，因此模型从不需要处理句柄：`builtin:shell.exec`（`{ command }` → `shell.run` 的结果）、`builtin:shell.write`（`{ path, content }`）、`builtin:fs.read`（`{ path }` → `{ ok, content, path, truncated }`）、`builtin:fs.write`（`{ path, content }`）和 `builtin:shell.destroy`。使用沙箱方案时，文件工具直接读写项目——不会为它们启动沙箱——路径可以相对于项目给出，也可以位于 `/workspace` 下。使用 SSH 方案时，这些工具每个会话保持一条连接，在首次使用时打开。没有绑定方案的智能体会得到如实的“不可用”结果，无法运行沙箱的主机也是如此；没有 `shell.exec` 能力的智能体根本看不到这些工具，即使设置了 `tools.policy.default: all` 也一样。除 `fs.read` 外，其余工具默认都是 `needs_confirm: true`——可按部署通过 `tools.policy.overrides` 放宽。

`builtin:fetch` 是面向 API 和普通网页的通用 HTTP 客户端，不需要任何账号或凭据——所以与 `web_search` 和 `browser` 不同，它从不会报告不可用。选项：`url`（必填）、`method`、`headers`、`body`（原始内容）或 `json`（会被序列化；除非已有请求头设置了 `Content-Type`，否则会设置它）、`query`、`timeout_ms`、`follow` / `max_redirects`、`user_agent`、`max_bytes`、`insecure`，以及 `save_to`（响应体还会保存到本会话的文件临时空间中，可通过 `files.scratch.read` 读取）。它返回 `{ok, status, status_text, headers, body, final_url, redirects?, content_type?, content_length?, size, truncated?, tls_verified, time_cost_ms}`——*任何* HTTP 状态都会返回，因为 404 是模型应当读取的答案，而不是工具失败；`ok` 表示请求是否完成，`status` 表示服务器说了什么。

响应体有上限（`max_bytes`，默认 1 MiB），结果始终报告真实的 `size`，发生截断时会设置 `truncated`。这个上限不是锦上添花：工具结果会被完整交给模型，下游没有任何环节会截断它，循环的 Token 预算器也根本不统计工具结果——所以过大的响应体会挤掉真正的对话历史，而不是被裁剪。目标地址受上文所述 `net` 防护的约束，被拒绝时会返回一个指明该地址的错误，计入 `agentflow_http_private_blocked`，并记录在发起调用的智能体名下。

`insecure: true` 会跳过 TLS 证书校验。它始终允许——没有配置开关——但绝不会悄无声息：跳过时会递增 `agentflow_http_insecure_tls`，记录一条注明智能体和 URL 的 WARN 日志，并在结果中报告 `tls_verified: false`。它只跳过证书检查；地址防护依然生效，因为对端声称自己是谁和连接通往哪里是两个不同的问题。该工具与 `web_search` 一样是 `Autonomous: true`；希望对它进行确认的部署可以通过 `tools.policy.overrides` 设置 `needs_confirm`。

#### `http` 和 `os` — 能力：`net.http`

```
http.request(opts) -> { ok, status, headers, body, truncated? }
http.get(url, opts?)
http.post(url, body, opts?)

-- opts: { method, url, headers, body, json, query, timeout, auth,
--         save_to = "name" }  -- 响应体还会保存到本会话的文件临时空间
-- auth = { service = "..." } 会在请求时解析已存储的凭据。

os.env(name) -> string | nil
os.time() -> number                        -- Unix 秒数（UTC）
os.date([format,] time?) -> table | string -- UTC；除非给出格式，否则返回表
```

**挂钟时间。** `os.time()` 返回整数 Unix 秒数，不接受参数（这不是 Lua 的 `os.time(table)`）。`os.date()` 返回日期*表*——`{ year, month, day, hour, min, sec, wday, yday, isdst }`，其中 `wday` 1 = 星期日 … 7 = 星期六，`yday` 为 1–366，`isdst` 始终为 `false`——并接受一个可选的 Unix 秒数时间（默认：当前时间）。该表可以编码为 JSON，因此可以放进载荷中传递。传入 strftime 字符串则返回文本：`os.date("%Y-%m-%d %H:%M:%S", t)`。支持的格式符为 `%Y %y %m %d %e %H %I %M %S %p %A %a %B %b %j %w %u %Z %%`；未知的格式符原样保留。**一切都是 UTC**——沙箱没有本地时区，所以各字段在每台主机上都相同，开头的 `!`（`os.date("!*t")`）会被接受但不起作用。相比之下，cron 触发器按 `runtime.timezone_offset_hours` 匹配；两者之间的换算由你的循环负责。

#### `files` — 能力：`files`

```
files.put(project, path, content|{data=base64, handle=..., mime=...}, {mime}?) -> { ok, entry }
files.read(project, path) -> { ok, entry }        -- entry: {path, handle, size, mime, ts, revision}
files.list(project, prefix?) -> { ok, entries }
files.projects() -> { ok, projects }             -- 本轮次范围内的项目
files.delete(project, path) -> { ok = true }
files.commit(project, {ref="main", message}?) -> { ok, commit }   -- 快照；父链即历史
files.checkout(project, ref|commit_id) -> { ok, manifest }        -- manifest: {commit, files=[{path,handle,size}]}
files.scratch.put(name, content|{data=base64, mime=...}) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

一个**按用户划分范围的文件存储**：若干命名的项目树，其范围由引擎根据轮次的身份解析（由渠道发起的轮次以及继承了租户的委派轮次为 `user:<uuid>`，其他情况为 `agent:<name>`）——循环永远无法指定其他范围。字节保存在按内容寻址的 blob 存储中（`files:` 配置；本地文件系统或兼容 S3 的存储——未变化的文件在各快照之间自动去重，不额外占用空间）；快照元数据（工作树、提交、ref、临时文件）保存在运行时存储的 `files_meta` 表中，由引擎掌管，可以用普通 SQL 查看。版本管理是**引擎原生的快照，而不是 git 服务器**：一次提交记录完整的树（路径 → 句柄），附带父指针和提交信息，并移动一个命名 ref；checkout 解析一个 ref 或提交 id。原始字节从不跨过 Lua 桥接——`put` 接受 utf-8 内容或 base64 `data`，`read` 返回句柄和元数据，实体化在引擎侧完成：shell spawn 的 checkout、`scratch_mount`，或者 `http.request` / `builtin:fetch` / `builtin:browser` 的 `save_to`。临时空间按会话划分（以调用方会话而非用户为键），有效期由 `files.scratch_ttl` 决定（默认 24h，读取时惰性过期，并在启动时清扫）。一个 `files` 能力管控整个函数族；它不在默认集合中——需要显式声明。如果存储在启动时被停用（例如 S3 凭据无法解析），每个操作都会如实失败。

#### `mail` — 能力：`net.mail`

```
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
-- opts: { host, port = 993, user, mailbox = "INBOX", unseen = false,
--          limit = 10, auth = { service = "..." } }
-- messages: [{ seq, uid, flags, subject, from, to, date, body }]
-- 通过隐式 TLS 连接；邮件正文上限 256 KiB（超出部分截断并加标记）

mail.smtp_send(opts) -> { ok, sent }
-- opts: { host, port = 465, user, from, to = {...}, subject, text_body, auth }
-- 端口 465 = 隐式 TLS；587/25 = STARTTLS
```

与 `http.request` 一样，`auth = { service = "..." }` 指定一个已存储的凭据，由 Go 在调用时解析——密码从不跨过 Lua 桥接。主机、端口和用户名由循环提供（通常通过 `os.env`），因此 Lua 源码中不会有任何密钥。`net.mail` 不在默认能力集合中；智能体必须声明它。

#### 核心工具函数

```
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
json.encode(value) / json.decode(text)
time.sleep(seconds)        -- 让出执行；从不阻塞线程
```

### 编写辅助代码块

辅助代码块会在循环插件之前求值到会话状态中。它们可以定义供循环或其他辅助代码块读取的全局变量。例如，要替换默认的召回策略，可以在 `plugins/recency.lua` 中放一个定义了 `memory_recall_handler(query, opts)` 的文件来覆盖 `plugin:recency`。

```
-- plugins/recency.lua（覆盖插件示例）
function memory_recall_handler(query, opts)
  local info = agent.info()
  local store_name = (info.memory and info.memory.stores.dialogue) or "dialogue"
  local res = store.query(store_name, { kind = "all" })
  return res.records or {}
end
```

### 错误处理

始终用 `pcall` 包裹外部调用，这样出问题的提供商或错误的命令就不会让会话陷入崩溃循环：

```
local ok, reply = pcall(llm.chat, messages, { model = info.model })
if not ok then
  log.warn("llm.chat failed: " .. tostring(reply))
  session.send("⚠️ I couldn't reach my model right now.")
  return
end
```

桥接以 JSON 字符串返回错误；prelude 会将其解包，使堆栈跟踪指向你的代码，而不是 prelude。

### 确认与权限

工具和 shell 内置工具可以声明 `needs_confirm`。第一次 `tools.run` 会返回一个带有 `needs_confirm = true` 和 `confirm_id` 的结果；actor 询问用户，然后以 `opts.confirmed = true` 和同一个 `confirm_id` 重新调用该工具。你的循环只会看到最终结果，所以大多数情况下无需手动处理确认。

### 限制与注意事项

- `llm.stream` 返回一个闭包——请直接调用它，绝不要放在泛型 `for ... in` 中。
- 标准库包括 base、coroutine、table、string、math、bit32、utf8 和 buffer。`io` 和 `debug` 不可用，真正的 `os` 库也不可用：prelude 只提供 `os.env`、`os.time` 和 `os.date`（全部为 UTC——沙箱中没有本地时区）。
- Lua 访问原始网络只能通过 `http.request` 和 `mail.*` 操作。没有套接字库。HTTP 路径——这个操作和 `builtin:fetch` 工具——会在拨号器中拒绝私有、回环、链路本地和保留地址，所以仅凭一个 URL 无法访问主机自身的网络；参见 `net` 配置块。`mail.*` *不*受该防护约束，仍会连接循环指定的任何主机。
- 跨过桥接的所有值都会编码为 JSON。表必须可序列化；函数和 userdata 无法跨过桥接。
- 插件支持热重载：重载监视器始终运行（`runtime.reload.watch` 不起作用）。修改循环文件或指令文件后，会话协程会在下一轮次从头重新启动；修改 `gateway.route` 背后的文件会在两条消息之间重建路由器的状态（路由在那里运行，而不是在会话中）；修改被覆盖的辅助代码块会重启每个运行中的会话，新的代码块就在那时加载。这些都不需要重启进程；如果新版本无法编译，也都不会被应用——已在运行的版本保持不变。

### 实用示例

把下面任意一个放进循环文件，再让智能体的 `loop:` 指向它：

#### 工具循环

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local info = agent.info()
      local tools = tools.list()
      local ok, reply = pcall(llm.chat,
        { { role = "user", content = msg.text } },
        { model = info.model, tools = tools, tool_choice = "auto" })
      if ok and reply.tool_calls then
        for _, tc in ipairs(reply.tool_calls) do
          local r = tools.run(tc.name, tc.args)
          log.info("tool result: " .. json.encode(r))
        end
        session.send(reply.text or "done")
      elseif ok then
        session.send(reply.text or "")
      else
        session.send("error: " .. tostring(reply))
      end
    end
  end
end
```

#### Shell 循环

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local ok, r = pcall(shell.run, "echo hello from " .. msg.text)
      if ok then
        session.send("exit=" .. r.exit_code .. " stdout=" .. r.stdout)
      else
        session.send("the shell is not available: " .. tostring(r))
      end
    end
  end
end
```

#### 多智能体请求

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local ok, answer = pcall(agent.request, "agent:expert", { question = msg.text }, 30)
      if ok then
        session.send("expert says: " .. (answer.payload and answer.payload.answer or "?"))
      else
        session.send("expert unavailable: " .. tostring(answer))
      end
    end
  end
end
```
