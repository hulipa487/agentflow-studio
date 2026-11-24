## 脚本插件开发

AgentFlow 插件是用纯 Go 编译（编译前先剥掉 TypeScript 类型）、再由内置的纯 Go JavaScript VM（`goja`）执行的 TypeScript / JavaScript 脚本；此外还有用 TypeScript（`goja`）或 WebAssembly（`wazero`）写的外部 API 扩展。YAML 决定哪个智能体跑哪个插件，行为由插件定义。本套文档里的长文指南（脚本循环）讲执行模型、完整 API、可运行的模式和一份坑点清单；这一节是参考摘要。

插件分三种：

- **循环插件** —— 定义全局或导出的 `loop()` 函数，在它的整个生命周期内独占该会话的 VM。标准写法是 `while (true) { const msg = session.inbox(); ... }`。
- **支持片段** —— 在循环插件*之前*加载进每个会话状态。支持片段定义循环要读的全局和辅助函数。`plugin:token_budget`、`plugin:routing_table`、`plugin:recency` 和 `plugin:semantic` 这些内置实现都是支持片段。
- **覆写** —— 放在 `plugins/` 里、与某个内置同名的用户插件。它在加载时于实例范围内替换掉内置实现。

### 异步桥

暴露给 JS 的核心原生原语是 `__af_op(json_request)`。脚本调用某个操作时，VM 的工作 goroutine 挂在一个 Go channel 上；Go 执行请求（在 actor goroutine 上内联执行，或放到工作池里）；随后 VM 带着 `[json_response, ok]` 恢复。prelude 把这层原始原语包装成下文记录的冻结全局命名空间。因为挂起的会话 goroutine 是在 Go channel 上等待，忙的实例依然很轻 —— 没有操作系统线程卡在 JS 里等模型、数据库或 HTTP 响应。

```ts
// prelude 替你做的事（概念上）：
function op(req: Record<string, unknown>) {
  const [resp, ok] = __af_op(json.encode(req));
  if (!ok) throw new Error(String(resp));
  return json.decode(resp);
}
```

### 消息生命周期

会话的消息长这样：

```ts
{
  id: "msg-...",
  type: "user",           // "user" | "system" | "timer" | "cron" | "agent" | "shell.result" | ...
  from: "webhook:alice",  // 由核心标记的发送者
  text: "hello",
  channel: "webhook",
  reply_to: "...",
  ts: 1234567890,
  // 由核心分配，循环无法控制。user_uuid 是本轮次所属的租户；没有任何组件
  // 标记该消息时它不存在；消息不属于任何租户（未关联的账号）时它为 ""。
  // 身份要从这里读取，绝不要从 payload 读取——转发一跳时的 payload 是由调用方的脚本重建的。
  provenance: { kind: "agent", principal: "agent:planner", user_uuid: "u_xxxx" },
}
```

`session.send(text)` 通过活跃渠道回复（当前消息所属的渠道和回复目标）。`session.skip(reason?)` 跳过回复但不结束会话，并把原因记录到事件中心（群聊中常配合 Telegram `payload` 字段如 `chat_type`、`chat_title`、`mentioned`、`is_reply_to_bot`、`reply_to_text`、`message_thread_id` 使用）。`session.push(channel, reply_to, text)` 是主动发送，需要 `channel.push` 能力。`session.push_user(uuid, text)` 按身份 UUID 推送给用户。`session.exit()` 干净地结束会话。

### 编写循环插件

一个最小的 echo 循环长这样（每个代码块都以 prelude 版本声明开头）：

```ts
// af-prelude-version: 2
log.info("echo agent online");

function loop() {
  while (true) {
    const msg = session.inbox();
    log.info("message from " + (msg.from || "?"));
    time.sleep(0.3);
    session.send("echo: " + (msg.text || ""));
  }
}
```

ReAct 风格的循环会调 LLM，可选地调工具：用 `memory.recall` 取历史，把 `tools.list` 的工具定义传给 `llm.chat`，用 `tools.run` 执行返回的工具调用，最后用 `session.send` 回复。把它写成普通的 `.ts` 或 `.js` 循环文件，把 `loop:` 指向它即可。

### 内置脚本 API 参考

下面所有命名空间在加载时注入并冻结为只读；智能体缺少所需能力时，受能力门控的操作会在调用时直接报错。完整的 TypeScript 声明见 `docs/ts/af.d.ts`（文档和编辑器里都有）。

#### `session` —— 核心

```ts
session.inbox() -> Message            // 挂起，直到邮箱收到下一条消息
session.send(text)                    // 回复当前的入站消息
session.send(text, { attachments: parts }) // 附带媒体发送（见下文）
session.reply(t)                      // 以结构化回答结束本轮次：
                                      // { text, tool_calls, finish_reason, usage, reasoning }
session.send_delta(text)              // 流式回答的一个片段
session.push(channel, reply_to, text) // 主动出站；能力：channel.push
session.push(channel, reply_to, text, { attachments: parts })
session.push_user(uuid, text)         // 主动向某个用户 UUID 出站；能力：channel.push
session.skip(reason?)                 // 干净地跳过回复并记录原因
session.exit()                        // 结束本会话

session.state.set(key, value)         // 能力：session.state（需声明）
session.state.get(key)                // 该键从未设置过时返回 null/undefined
session.state.delete(key)
session.state.list()                  // 本会话的 { [key]: value }
```

渠道吃进媒体时，消息会带一个 `attachments` 数组，元素是媒体片段描述符（`{ type, mime, handle, name }`）。原样转进 `llm.chat` 或 `session.send` 即可；运行时会在一侧（提供商或渠道）把 blob 句柄（`"media:<sha256>"`）解析成字节 —— 原始媒体永远不进 JS VM。

`session.reply(t)` 是结构化回复：`tool_calls`（`{ id, name, args, arguments }`）、`finish_reason`（`stop`、`tool_calls`、`length`、`content_filter`）、`usage` 和 `reasoning` 会送到能表达它们的渠道，文本则走和 `session.send` 相同的安全与日志路径。`llm.forward` 或 `llm.chat` 的结果可以直接传进去：其中的 `thinking` 会被当作回复的推理内容，只有配置了对外暴露的渠道才会发送它。工具调用的 `arguments`（模型自己的原始 JSON 文本）优先于 `args` 对象发送，以保留提供商的精确格式；如果你改过某个调用的 `args`，就把它的 `arguments` 设为 `null` 或 `undefined`。每轮最多一条回复；在无法表达工具调用的渠道上，带工具调用的回复会报错，而不是悄悄丢掉。文本被出站安全拦截的回复以 `finish_reason = "content_filter"` 投递，不带工具调用也不带推理内容；其内容是方案的兜底文案（如果配了），否则为空。

`session.send_delta(text)` 用于流式回答。只有安全方案无法丢弃出站时，引擎才实时转发片段；否则先扣住，本轮结束时把检查过的完整内容一次放出。既流式又调 `session.reply` 的循环，应让回复带上完整文本：不管客户端有没有要求流式，回复只会补上片段尚未携带的部分。

`session.state` 是本会话专属的小型持久键值存储：一个游标、一个步数、最后说过的话。它熬得过重启，在集群里也会跟着会话走到下一个拥有它的实例，因为行存在共享存储里。键可含字母、数字和 `. _ - :`，最长 128 个字符；值是任何可 JSON 编码的 JS 值，上限 64 KiB；一个会话最多 256 个键。它需要显式开启 —— 在智能体的 `capabilities` 里声明 `session.state`，因为这是一块持久化面，不该默认发出去。

#### `llm` —— 能力：`llm.chat`

```ts
llm.chat(messages, opts?) -> { text, thinking, thinking_blocks, usage, tool_calls }
llm.stream(messages, opts?) -> Iterable<string>
llm.forward(opts?) -> { text, tool_calls, finish_reason, usage, thinking, thinking_blocks }
llm.embed(inputs, opts?) -> { vectors, usage }     // 字符串和 / 或部件对象
llm.rerank(query, documents, opts?) -> { results } // results：[{ index, score }]，从 0 开始

// opts: { model: "default", temperature: 0.7, max_tokens: 4096,
//         thinking: "low"|"medium"|"high"|"xhigh"|"max"|"off",
//         tools: [...], tool_choice: "auto" | "none" | "required" | "function:<name>" | { name: "<name>" },
//         parallel_tool_calls: true, top_p: 0.9, stop: [...] (or one string), seed: 1,
//         response_format: { type: "json_schema", json_schema: {...} } }

// forward opts: { model, messages, prepend, append, tools, params, max_tokens_cap }
```

`llm.forward` 是 OpenAI 渠道的透传。不带参数时，它把客户端自己的请求 —— 消息、工具、`tool_choice` 和采样参数 —— 转发给本轮所跑的模型（智能体的，或用户的个人模型），走与 `llm.stream` 同一条计量路径：预算和配额适用，用量台账会多一行 kind 为 `forward`。`opts.messages` 替换客户端的消息列表，`prepend`/`append` 在前后加轮次，`tools` 加定义，`params` 覆盖采样，`max_tokens_cap` 给客户端的值设上限。提供商表达不了的选项会被丢弃而不是拒绝，但 `response_format` 和具名 `tool_choice` 例外，它们会让调用失败。结果的 `finish_reason` 是模型自己的值（输出上限截断时为 `length`），每个工具调用都带 `arguments` —— 模型的参数文本 —— 与解码后的 `args` 并列。只有当客户端要求流式、且智能体的安全方案没有过滤器时，它才实时转发文本增量；否则整段文本返回给 `session.reply`。它不负责结束 HTTP 响应。模型调用失败会抛出以 `upstream_error:` 开头的错误。在 OpenAI 渠道轮次之外又不给 `opts.messages` 时，它返回错误。`openai.request()` 把完整请求作为对象返回，超过 4 MiB 的 JSON 会被拒绝 —— 该转发这个请求，而不是把它实体化。两者都需要 `llm.chat` 能力。`llm.chat` 的回复和 `llm.stream` 的最后一帧也带 `finish_reason`。

消息是 `{ role, content, tool_calls?, tool_call_id?, tool_result?, thinking_blocks? }` 的数组。**多模态轮次**用 `{ role, parts: [...] }`，每个片段是一个对象：

```ts
{ type: "text", text: "..." }
{ type: "image"|"audio"|"video"|"file", mime: "image/png", handle: "media:...",
  name: "photo.jpg" }   // handle 来自 msg.attachments（也可以用 data base64 / url）
```

提供商支持矩阵（不支持的片段类型会如实报错；来源可以是 base64 `data`、`url`，或提供商的文件引用）：

| 提供商 | image | audio | video | PDF / 文件 |
| --- | --- | --- | --- | --- |
| anthropic | ✅ base64/url | ❌ | ✅ base64/url（MiniMax video block） | ✅ document（base64） |
| openai (chat) | ✅ image\_url | ✅ input\_audio（wav/mp3） | ✅ video\_url —— MiniMax/Kimi/GLM 约定 | ✅ file\_url（url）—— GLM |
| openai-responses | ✅ input\_image | ✅ input\_audio（wav/mp3） | ✅ input\_video（url/file ref） | ✅ input\_file（data 或 file\_id） |
| gemini (interactions) | ✅ files api | ✅ files api | ✅ files api | ✅ files api |

视频来源遵循生态里的约定 —— MiniMax 的 `mm_file://<file_id>`、Kimi 的 `ms://<file_id>` —— 作为 URL/文件引用原样透传。SDK 的类型化请求联合体没有建模的形状（`video_url`、GLM 的 `file_url`、Anthropic 的 video block、Responses 的 `input_video` 和 `input_audio` 项）用 `param.Override` 构造 —— 那是 SDK 自带的原始 JSON 逃生口，未设置类型化变体时按原样序列化。blob 存储的 `handle` 会自动解析成 base64。

对 gemini，每个媒体片段都会上传到 **Gemini Files API**（可续传上传），再以 URI 引用 —— 绝不用内联 base64。`data`（base64）、`url` 和 blob 存储的 `handle` 三种来源都行（url 会先下载）；相同字节的重复上传在文件 48 小时生命周期内会按内容哈希去重，之后 Google 自动删除。Anthropic/openai 也直接接受 `url` 来源。`llm.stream` 返回标准的 ES6 可迭代对象（`[Symbol.iterator]`、`next()`、`close()`、`return()`）。用 `for (const delta of llm.stream(messages, opts))` 迭代；提前 break 会触发 `return()`，干净地关闭底层流（也可以手动调 `iter.next()` 和 `iter.close()`）。它只产出文本增量字符串 —— 要拿用量和工具调用请用 `llm.chat`。

**多模态嵌入**遵循 Jina 约定：`llm.embed` 接受字符串和片段对象混合的数组，带媒体的批次会把每项序列化成带类型的 doc（`{"text"}`、`{"image"|"video"|"audio"|"pdf": url|base64}`）—— 这正是 `jina-embeddings-v5-omni-small`（文本、图像、视频、音频和 PDF 在同一向量空间）和 `jina-clip-v2` 使用的形状。纯文本批次仍是普通字符串，所以任何 OpenAI 兼容的 `/embeddings` 端点（OpenAI、Ollama、vLLM、TEI、OpenRouter）看到的都是历史上那个一模一样的请求。选项：`task`（`retrieval.query` | `retrieval.passage` | `text-matching` | `clustering` | `classification`）、`dimensions`（Matryoshka 截断）、`merged: true`（把所有输入折成一个嵌入 —— 一个 Jina 内容组；PDF 只支持单输入）。Anthropic 和 Gemini 提供商在这里没有嵌入 API，会明确失败。`llm.rerank` 需要一个 `provider: rerank` 的模型条目，说 `POST {base_url}/rerank`（TEI、vLLM、Jina、Cohere）。智能体声明了预算时，`llm.embed` 按 Token 计费。`gemini` 提供商（Interactions API）是非流式的：对它调 `llm.stream` 会把完整回复作为单个增量产出。

**推理内容（响应侧）。** 提供商发来的推理内容会被动地暴露在回复上 —— `reply.thinking` 是给人读的文本（没有时为空字符串），`reply.thinking_blocks` 是提供商的原始块（Anthropic 的 `{type: "thinking", thinking, signature}` / `redacted_thinking`、Responses 的 reasoning 项、Gemini 的 `thought` 片段；Chat Completions 上为 null/undefined，那里推理只是一个纯字符串），而 `usage.reasoning` 在提供商单独上报时统计思考 Token。推理内容的有无是各提供商、各模型别名的运行时属性，不是契约：某个服务配置可以既按推理 Token 计费又从流里完全隐去推理内容，所以 `usage.reasoning` 才是可靠的审计信号，`reply.thinking` 为空本身并不说明出故障。引擎从不索取超出思考档位已经要求的推理内容，也从不存储它 —— 要不要持久化是循环的决定。Anthropic 开启 thinking 的多轮工具调用，要在 assistant 轮把块原样传回去：`thinking_blocks: reply.thinking_blocks`（否则 Messages API 会拒绝续接；其他提供商忽略该字段）。

```ts
// 多模态嵌入（通过 api.jina.ai/v1 使用 jina-embeddings-v5-omni-small）
const r = llm.embed([
  "find me the cat photo",                                // 文本
  { type: "image", handle: msg.attachments[0].handle },   // blob 存储中的媒体
], { model: "omni", task: "retrieval.query", dimensions: 512 });

// 为整条多模态记录生成一个向量（当方案指定了 omni embed_model 时，
// memory.write 会对带附件的记录这样做）：
const r2 = llm.embed(["trip to the beach", imgPart], { model: "omni", merged: true });
```

#### `agent` —— 能力：`agent.send`、`agent.request`、`agent.spawn`

```ts
agent.info() -> { name, session_id, address, model, instructions,
                  history_budget, memory, shell, skills, capabilities,
                  memory.stores: { [name]: { backend, table, features, locality }, ... } }
agent.config() -> { name, model, instructions_path, instructions_prompt?,
                    prompts: { [name]: "", ... },
                    goal?: { type, success_signal, max_turns, on_goal_met },
                    extras: { ... } }
agent.send(address, payload)
agent.request(address, payload, timeout?) -> Message
agent.reply(request_id, payload)
agent.list() -> [{ session_id, agent, parent_id }]
agent.spawn(profile) -> { address, session_id, agent }
```

存储区的 `locality` 是 `deployment`、`region` 或 `instance`。持久化的对话数据该放在部署级或区域级的存储上；实例本地的存储只是缓存，内容随进程一起消失（区域单元里，持久存储区放在这种存储上会在校验时被拒绝，除非设了 `locality_ok: true`）。

`agent.spawn` 需要一个 `profiles.agent` 模板；子会话的能力是父会话与模板能力的交集。早期版本接受的第二个参数已保留但不再生效 —— 请改在模板里配置。

`agent.config()` 把这个智能体自己的方案作为数据返回：配置名、模型、指令的*路径*（如果系统提示词来自 `prompts` 注册表则为 `instructions_prompt`，此时 `instructions_path` 为空）、部署解析后的 `prompts` 注册表（按名称索引）、方案里提升到 `goal` 的 `extras.goal`，以及其余的 `extras`（一个 pm 选项块、一个工作流名，等等）。里面的密钥引用渲染成不透明标记（`{env: "VAR"}` / `{cred: "service"}`）—— 解析后的密钥永远不会过脚本桥。

#### `user` —— 能力：`users`

```ts
user.current() -> { registered, id, display_name, identities }
user.usage() -> { registered, input, output, cached, reasoning, calls,
                  failed, billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { service, present }
user.get(id) -> { ... }    // 仅限维护来源
user.list() -> [ { ... } ] // 仅限维护来源
```

关于本轮所属用户的只读信息。`users` 是默认能力，所以循环不用声明就能调用。循环拿到的是一个名字和一个渠道，永远拿不到邮箱，也拿不到凭据值 —— `has_credential` 回答的是密钥存不存在，而不是它是什么。`get`/`list` 仅限引擎触发的维护来源（`system` 或 `scheduler` 轮次），正因如此才能写按用户的维护循环，又不会把一个用户的资料暴露给另一个用户的轮次。

#### `runtime` 与 `credential` —— 部署配置即数据

```ts
runtime.triggers() -> { ok, triggers: [ { name, kind, run_on_boot,
                          target: { profile }, payload,
                          event: { channel, match }   // kind = "event"
                          | cron                      // kind = "cron"
                          | every } ] }               // kind = "every"

credential.get(name) -> { ok, name, value }           // 仅限允许列表中的名称
```

`runtime.triggers()` 返回启动时快照的合并后 `triggers/*.yaml`（或顶层 `triggers:`）列表 —— 让循环从引擎读 CRON\_TASKS/ROUTES，而不是把表硬编码进脚本。它在**两个**脚本面上回答方式一致：智能体循环和网关路由处理器（路由器的操作分发返回完全相同的载荷），所以事件路由能读到驱动它的触发器列表。表达式原样透传，引擎的调度也不会改变形状：`every:`/`cron:` 条目同时由引擎自己触发（见配置参考里的 `triggers`），所以循环要调这个只是为了*观察*部署的排期。`credential.get(name)` 仅在智能体方案把该名字列在 `credentials: [...]` 下时，才去取全局存储的密钥（空列表或缺失 = 拒绝，并给出说明）。每次访问都会记日志（智能体、名称、允许/拒绝）并计数（`agentflow_credential_gets` / `agentflow_credential_gets_denied`）；值绝不写进日志，也绝不该回显到回复里。

`router.state` 是路由处理器自己的持久键值存储 —— 处理器把不能忘的东西放在这儿，比如某个聊天上次被路由到哪个智能体。它住在运行时存储里，所以每个实例读到同一个值，重启或故障转移也不会丢：`router.state.get(key)`（键从未设置时为 null/undefined）、`router.state.set(key, value, { ttl_seconds: n })`、`router.state.delete(key)` 和 `router.state.list(prefix)`。值是任何可 JSON 编码的 JS 值，上限 64 KiB；键可含字母、数字和 `. _ - :`（128 个字符）。键在最后一次写入后保留 30 天，除非处理器要求别的存活期（`ttl_seconds: 0` 表示保留到被删除），这正是处理器以高基数值为键时存储不至于膨胀的原因。该命名空间只在路由器的状态里定义，所以智能体循环够不到它 —— 而且它是「一致性」而非「串行化」：两个实例可能对同一个键交错读写，而投递本身由会话租约串行化。

#### `scheduler` —— 不受能力门控

```ts
scheduler.every(interval) -> timer_id    // 单位为秒；随会话结束而失效
scheduler.after(delay) -> timer_id
scheduler.cron(expr) -> timer_id          // cron 子集（见下文）
scheduler.cancel(timer_id)
```

计时器会把 `Message{ type: "timer" }` 放进所属会话的收件箱 —— 带上 `msg.payload.timer_id`，这样一个会话就能分辨自己的多个计时器。它们从不在计时器 goroutine 里调用脚本，所以每个会话 VM 始终保持单线程。`scheduler.cron` 只支持 5 字段 cron 的**子集**：`*/N * * * *`（每 N 分钟）和 `30 9 * * *` 这类固定分/时形式；日、月、星期字段必须是 `*`，`*/N` 分钟也不能和固定小时搭配（步长没法锚定到某个小时，所以会被悄悄丢掉）。其他写法一律拒绝而不是重新解释 —— 更复杂的排期请用 `every`/`after`（或自我重排）。

这些是*会话级*计时器：它们随会话一起消亡。**没有 `scheduler` 能力** —— 这些操作在 actor 自己的分发里处理，从不经过能力门控，所以任何循环都能注册计时器，无论它声明了什么。门控只管通过处理器映射能到达的操作（`llm.*`、`memory`/`store.*`、`tools.*`、`api.*`、`shell.*`、`http.request`、`mail.*`、`agent.*` 的地址）；计时器是引擎投递的消息，不是一项权限。在 YAML 里声明的部署级排期（`every:`/`cron:` 触发器）则属于引擎基础设施 —— 由引擎触发，不涉及任何能力，以 `Message{ type: "cron" }` 投递，所以循环不必自己跑一个计时循环才能被排期。循环需要自己判断时钟时，用 `os.time()`/`os.date()`（UTC）。另外注意计时器状态在内存里：重启会忘掉所有计时器，直到循环重新注册。

#### `memory` 与 `store` —— 能力：`memory`

```ts
memory.write(record) -> boolean                         // 由 plugin:routing_table 路由
memory.recall(query, opts?) -> [ { key, value, meta } ] // 当前生效的召回处理程序

store.put(table, key, value, opts?)                     // opts: { ttl: seconds, vector: [...] }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records }
store.delete(table, key)
```

存储查询接受 `{ kind: "all" | "prefix" | "text" | "vector" | "time_range", ... }`；向量查询用 `{ kind: "vector", vector: [...], k: n }`。给不支持 `vector` 特性的后端传 `vector` 是明确报错，绝不悄悄丢弃。智能体记忆优先用 `memory.write`/`memory.recall`；只有需要原始表访问时才用 `store.*`。

**按用户隔离（私有存储）。** 来自渠道的轮次只读写自己的用户范围和 `service` 行（由没有自身租户的上下文写入：引擎触发的工作、未关联的账号，以及用 `inherit_user: false` 退出的智能体）和 `legacy` 行（此功能上线前写入的 —— 升级前的历史对该智能体的每个用户仍可读，且不会再写入）。引擎触发的上下文（`system`/`scheduler` 来源 —— 像蒸馏器这样的夜间维护）用作用域可见的键读取所有范围，并在写入时显式指定范围；维护会话可以通过 `store.scopes` 枚举某个绑定的用户范围（仅限维护：渠道和智能体跳转会话会被拒绝）。这是引擎强制的，不靠循环代码过滤。绑定池的存储（`pool: <name>`）改由租户授权治理，私有存储也可以每个智能体用一个 `scope: agent` 重新加入某个池。

#### `tools` —— 能力：`tools`

```ts
tools.list() -> [ToolSpec]
tools.run(name, args, opts?) -> ToolResult
// opts: { idempotency_key?: "..." }

tool.def({
  name: "script:shout",
  description: "Shout the text back",
  params: {
    type: "object",
    properties: { text: { type: "string" } },
    required: ["text"],
  },
  handler: function(args) {
    return { ok: true, shouted: String(args.text || "").toUpperCase() };
  },
});
```

如果工具声明了 `needs_confirm: true`，运行时会创建一条绑定会话、工作区、合资格审批人、动作修订版本、归一化参数和目标地址的单次审批记录，暂停等待已验证确认（授权渠道回复或 Studio 审批卡片），并在一次执行时原子认领该审批。脚本选项不能传入 `confirmed: true`；`opts.idempotency_key` 会在重试时持久化写入意图并在超时或取消后结果不确定时故障关闭。你的循环只看到最终结果。（等待是有界的：十分钟，超时即拒绝 —— 调度器轮次上的确认式工具没有渠道可应答，无界等待会把会话永久挂住。）

**工具循环是循环自己的事。** 引擎只在被要求时执行工具并返回结果，不驱动对话。用 `tools` 调 `llm.chat`、把返回的东西通过 `tools.run` 跑一遍、把它们作为 `role: "tool"` 轮次发回去再问一次 —— 这都是代码块的工作，包括*第二*轮，而这正是最容易忘的：看到第一个结果后还要再要工具调用的模型，只有代码块再检查一次 `reply.tool_calls` 才能拿到它。循环持久化什么也是同理：工具调用及其结果是下一轮该看到的历史，只存助手文本的代码块，会让模型在后续对话里引用一个已经不存在的工具结果。

`tool.def` 在 TypeScript/JS 里声明模型可见的工具 —— 这是替代手工拼装额外工具定义的标准做法。声明的工具会并入 `tools.list`（与 Go 工具同样的 `{ type: "function", function: {...} }` 形状，所以会原样流进 `llm.chat`），`tools.run` 分派给 JS 处理器。处理器在循环 VM 上同步执行，自身可以调用任何操作（`llm.chat`、`memory.*` 等）。声明的名字会在列表和执行两处覆盖同名 Go 工具（覆盖发生在 `skills` 过滤之后，所以此智能体看不到的工具不会压制它指向的 Go 工具）。处理器抛错会以 `{ ok: false, error: ... }` 返回，而不是杀掉循环。声明的工具是循环自己的代码：Go 的工具策略（`forbidden`、`needs_confirm`、`permission: forbidden`）仍然管不到它们。请在代码块顶层、`loop()` 之前声明。

**可见性**：声明的工具按治理 Go 工具的同一条规则被 `skills` 和 `tools.policy.default` 过滤 —— `skills` 点名了就列出，`skills` 非空但没点名就隐藏，`skills` 为空则完全按 `default` 处理。正因如此，一个共享循环目录才能承载只给部分智能体用的工具。对 `default: none` 且没有 `skills` 的部署来说这是行为变化：声明的工具过去无条件列出，现在不是了。`tools.run` 遵循同一规则 —— 智能体看不到的声明名字会落到 Go 路径并以不可用拒绝，与看不见的已注册工具完全一样。已注册工具本来就总在执行时分派处校验，而 `tools.run` 正是模型可见、幻觉或注入的工具调用会走的路径，所以两者一致。想调自己处理器的循环，直接调那个函数即可。

**覆盖已声明工具的文字**：`tools.policy.overrides` 可以点名一个已声明的工具，把面向模型的文字从代码块移到配置里 —— 描述（字面值，或来自 `prompts` 注册表的 `{prompt: <key>}`）和 `params.<param>.description`。没有覆盖点名时，传给 `tool.def` 的规格仍是后备，而覆盖永远不改动声明 schema 的类型或 `required`。解析发生在构建 `tools.list()` 的时候，所以重新加载的 `file:` 提示词在下次调用就生效，无需重启。覆盖从不增删列表项：可见性只来自 `skills` 和策略默认值。已声明但被隐藏的工具仍算已声明，所以点名它的覆盖不会被报成「什么也没点到」。

#### `api` —— 能力：`apis`（外部 API）

```ts
api.list() -> [APIDescriptor]                 // { name, version, description, runtime: "script"|"wasm", actions }
api.call(pkg, action, args?, opts?) -> unknown  // opts: { idempotency_key?: "..." }
api.batch(calls) -> [{ ok, data?, error? }]     // calls: [{ api, action, args, idempotency_key? }]
api.<pkg>.<action>(args?) -> unknown            // api.call(pkg, action, args) 的只读 Proxy 简写
```

外部 API 是可插拔的包，用沙箱化的 TypeScript（`goja`）脚本（`apis/<pkg>/...`）或 WebAssembly（`wazero`）模块实现 —— 与会话状态隔离，经宿主边界调用。直接的 `api.call` / `api.batch` 调用与声明为 `expose_as_tool: true` 的工具封装（`api:<pkg>.<action>`）在共享执行边界上强制执行相同的能力、`tools.policy`、托管模式 `kind`、审批和 `idempotency_key` 策略。每个操作都在自己的包权限和超时下执行；`api.call` / `api.<pkg>.<action>` 的错误会抛出，而 `api.batch` 返回逐次调用的 `{ ok, data, error }`（需要审批的子项会单独失败）。

#### `shell` —— 能力：`shell.exec`

```ts
shell.run(cmd, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out, truncated,
                             project, environment, changed, deleted, conflicts, skipped }
shell.start(cmd, opts?) -> { id, state: "running", ... }     // 结果会送到收件箱
shell.status(job_id)    -> job
shell.cancel(job_id)    -> job

// opts: { project: "site", environment: "python", workdir: "app",
//         env: { KEY: "val" }, timeout: 120 }                 // timeout 单位为秒

// 沙箱句柄：把作业选项编码进 id 的兼容垫片。
shell.spawn(opts) -> { id, provider, state }
shell.exec(id, cmd, opts?) -> the same result as shell.run
shell.write(id, path, content)
shell.destroy(id)
```

**沙箱 Shell 并不是一个持续存在的东西。** `shell.run` 是在一次性沙箱（[gVisor](https://gvisor.dev) `runsc` 或 [microsandbox](https://github.com/superradcompany/microsandbox) `msb`）里跑一条命令，为这条命令启动、跑完销毁。沙箱拿到两样东西：一个声明的**环境**作为工具链根，以及从文件存储挂到 `/workspace`（命令的起始目录）的一个**项目**，可读写。`/workspace` 之外的临时状态会被丢弃。没有网络。命令结束后，引擎对比项目目录和它放进去的内容，把差异写回项目的工作树 —— 也就是 `files.*` 读写的那棵树。什么时候 `files.commit` 由循环决定。

所以循环的规则很短：

- **命令在项目里写的东西会留下，别的一概不留。** 后台进程、Shell 变量、装的包、`/workspace` 之外的文件，都活不到下一次调用。
- **非零 `exit_code` 和 `timed_out` 也是结果**，就像 `http.request` 收到 500 一样。根本跑不起来的作业 —— 这台主机没沙箱、环境未知、队列超时、沙箱内存超限被杀 —— 会抛错，所以请把调用包在 `try { ... } catch (err) { ... }` 里。命令在被杀之前写下的东西仍会存下来。
- **一个实例上每个项目同时只跑一个作业**；同一项目上的第二次调用会等第一次（直到 `runtime.sandbox.queue_timeout`）。
- **回写永不覆盖更新的文件。** 作业改过、但期间被别人改过的路径保留存储里的版本，并列在 `conflicts` 中。存不进去的路径（超过单文件上限、超过作业回写上限、符号链接）带原因列在 `skipped` 中，并从目录里移除，免得目录里留着存储中没有的东西。
- **任何实例都能跑下一条命令。** 实例上的项目目录只是文件存储的缓存。

设置分层：调用的 `opts` 覆盖项目的 `.agentflow/shell.yaml`，后者覆盖智能体的 Shell 方案。环境必须是部署声明过的；时限受 `runtime.sandbox.max_timeout` 限制。项目清单可以设 `environment`、`env`、`workdir`、`timeout` 和 `ignore`，别的都不行：

```yaml
# .agentflow/shell.yaml，位于项目的工作树中
environment: node
env: { NODE_ENV: test }
workdir: app
timeout: 120s
ignore: ["node_modules/", "dist/"]
```

被 `ignore`（来自方案或清单）匹配的路径既不存储也不清理：它们作为尽力而为的缓存留在创建它们的实例上 —— 依赖目录、构建缓存 —— 在另一个实例上就是不存在。

**后台作业。** `shell.start` 把同样的作业排队后立即返回，时限更长（`runtime.sandbox.background_max_timeout`）。结果以 `type: "shell.result"` 的消息到达会话收件箱，作业在 `msg.payload` 里（`state` 为 `finished`、`failed` 或 `cancelled`；`stdout` 和 `stderr` 是最后 16 KiB）。结果轮次以启动作业的用户身份运行。`shell.status` 可以在作业运行时（从任何实例）读取，`shell.cancel` 在运行它的实例上停止它。实例死掉的作业会报为 `failed`，不会重跑。会话结束时，它的后台作业会被取消。

**沙箱句柄。** `shell.spawn` 返回一个*沙箱句柄*，它存在的意义是让为容器 Shell 写的循环在重启和跨实例时也能干净工作：它只是把拿到的选项编码进 id。对它 `shell.exec` 会跑作业，`shell.write` 写进本地项目，`shell.destroy` 什么也不做。它不需要存储，在任何实例上都能用。有三个选项在沙箱里有含义：`image` 指定一个声明的环境，`volumes[0]` 的容器路径是项目的挂载点（而不是 `/workspace`），而 `network` 非 `"none"`、`ref` 非 `"main"` 以及 `scratch_mount` 会被拒绝，并说明原因。传入 `provider: "ssh"` 或 SSH 句柄会被拒绝 —— 请改用 `ssh.*`。

绑定了 Shell 方案的智能体（`agents.<name>.shell`）还会得到隐式操作本地沙箱项目的注册表工具，模型因此从不直接接触句柄：`builtin:shell.exec`（`{ command }` → `shell.run` 的结果）、`builtin:fs.read`（`{ path, handle_id? }` → `{ ok, content, path, truncated }`）和 `builtin:fs.write`（`{ path, content, handle_id? }`）。这些文件工具直接读写项目 —— 不会为它们启沙箱 —— 路径可以相对于项目，也可以写在 `/workspace` 下；可选的 `handle_id` 只接受 `shell.spawn` 返回的沙箱句柄。没绑定方案的智能体拿到的是如实的不可用，沙箱跑不起来的主机也一样；没有 `shell.exec` 能力的智能体根本看不到这些工具，即使 `tools.policy.default: all` 也一样。`builtin:shell.exec` 和 `builtin:fs.write` 默认 `needs_confirm: true` —— 可通过 `tools.policy.overrides` 按部署放开。

#### `ssh` —— 能力：`net.ssh`

```ts
ssh.spawn(opts?)             -> { id, provider, state }
ssh.exec(id, cmd, opts?)     -> { stdout, stderr, exit_code, duration_ms }
ssh.read(id, path)           -> { ok, path, content }
ssh.write(id, path, content) -> true
ssh.close(id)                -> true
```

**远程 SSH 句柄。** `ssh.spawn` 会连到真实远程机器（`host`、`user`、`password` 或 `key_file`、`workdir`、`env`、`shell_opts`），除非在 `opts` 中覆盖，否则默认继承绑定 `agents.<name>.ssh` 方案的设置；`ssh.exec`、`ssh.read`、`ssh.write` 和 `ssh.close` 作用其上。连接活在建连的那个实例上，并在调用 `ssh.close` 或会话结束时关闭。`ssh.spawn` 也可以带 `project` + `ref` 和一项 `volumes: ["x:/dir"]`，在句柄报告就绪前把文件快照复制到远程的 `/dir`，或用 `scratch_mount` 复制会话的临时文件。

绑定了 SSH 方案的智能体（`agents.<name>.ssh`）还会得到注册表工具，每个会话按需开启并复用一条 SSH 连接（或接收来自 `ssh.spawn` 的显式 `handle_id`）：`builtin:ssh.exec`（`{ command, handle_id? }` → `{ ok, tool, handle_id, stdout, stderr, exit_code, duration_ms }`）、`builtin:ssh.read`（`{ path, handle_id? }` → `{ ok, tool, handle_id, path, content }`）、`builtin:ssh.write`（`{ path, content, handle_id? }`）和 `builtin:ssh.close`（`{ handle_id? }`）。没绑定 SSH 方案（且未提供 `handle_id`）的智能体拿到的是如实的不可用；没有 `net.ssh` 能力的智能体根本看不到这些工具，即使 `tools.policy.default: all` 也一样。除 `builtin:ssh.read` 外都默认 `needs_confirm: true` —— 可通过 `tools.policy.overrides` 按部署放开。

`builtin:fetch` 是面向 API 和普通页面的通用 HTTP 客户端，不需要账号或凭据 —— 所以和 `web_search`、`browser` 不同，它永远不会如实的不可用。选项：`url`（必填）、`method`、`headers`、`body`（原始）或 `json`（序列化；除非已有请求头，否则会设 `Content-Type`）、`query`、`timeout_ms`、`follow` / `max_redirects`、`user_agent`、`max_bytes`、`insecure`，以及 `save_to`（响应体会同时落到本会话的临时文件区，可用 `files.scratch.read` 读取）。它返回 `{ok, status, status_text, headers, body, final_url, redirects?, content_type?, content_length?, size, truncated?, tls_verified, time_cost_ms}` —— 对*任何* HTTP 状态都返回，因为 404 是模型该读到的答案，而不是工具失败；`ok` 表示请求是否完成，`status` 表示服务端说了什么。

响应体有上限（`max_bytes`，默认 1 MiB），结果总会报告真实的 `size`，被截断时置 `truncated`。这个上限不是锦上添花：工具结果整块交给模型，下游不做任何截断，而循环的 Token 预算器根本不统计工具结果 —— 所以超大的响应体会挤掉真实的对话历史，而不是被裁掉。目标受前文所述的 `net` 防护约束，拒绝会以点名地址的错误返回，计入 `agentflow_http_private_blocked`，并记在调用方智能体名下。

`insecure: true` 跳过 TLS 证书校验。它永远被允许 —— 没有配置开关 —— 但绝不静默：跳过会递增 `agentflow_http_insecure_tls`，记一条点名智能体和 URL 的 WARN，并在结果里报 `tls_verified: false`。它只跳过证书校验；地址防护仍然生效，因为「对端自称是谁」和「连接实际去哪」是两个不同的问题。这个工具和 `web_search` 一样是 `Autonomous: true`；想要确认的部署可以通过 `tools.policy.overrides` 设 `needs_confirm`。

#### `http` 与 `os` —— 能力：`net.http`

```ts
http.request(opts) -> { ok, status, headers, body, truncated? }
http.get(url, opts?)
http.post(url, body, opts?)

// opts: { method, url, headers, body, json, query, timeout, auth,
//         save_to: "name" }  // 响应体还会保存到本会话的文件临时空间
// auth: { service: "..." } 会在请求时解析已存储的凭据。

os.env(name) -> string | null | undefined
os.time() -> number                        // Unix 秒数（UTC）
os.date([format,] time?) -> DateFields | string // UTC；除非给出格式，否则返回对象
```

**墙上时钟。** `os.time()` 返回整秒 Unix 时间，不接受参数。`os.date()` 返回日期对象 —— `{ year, month, day, hour, min, sec, wday, yday, isdst }`，其中 `wday` 1 = 周日 … 7 = 周六，`yday` 1–366，`isdst` 恒为 `false` —— 接受一个可选的 Unix 秒时间（默认当前）。该对象可 JSON 编码，所以能放进载荷。传 strftime 字符串则返回文本：`os.date("%Y-%m-%d %H:%M:%S", t)`。支持的指令有 `%Y %y %m %d %e %H %I %M %S %p %A %a %B %b %j %w %u %Z %%`；未知指令原样保留。**一切都是 UTC** —— 沙箱没有本地时区，所以字段在每台主机上都一样，前导 `!`（`os.date("!*t")`）会被当作无操作接受。相比之下，cron 触发器按 `runtime.timezone_offset_hours` 匹配；两者之间的换算由你的循环负责。

#### `files` —— 能力：`files`

```ts
files.put(project, path, content | { data, handle?, mime? }, { mime }?) -> { ok, entry }
files.read(project, path) -> { ok, entry }        // entry: { path, handle, size, mime, ts, revision }
files.list(project, prefix?) -> { ok, entries }
files.projects() -> { ok, projects }              // 本轮次范围内的项目
files.delete(project, path) -> { ok: true }
files.commit(project, { ref: "main", message }?) -> { ok, commit } // 快照；父链即历史
files.checkout(project, ref | commit_id) -> { ok, manifest }       // manifest: { commit, files: [{ path, handle, size }] }
files.scratch.put(name, content | { data, mime? }) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

一个**用户范围的文件存储**：具名项目树，其范围由引擎根据轮次身份解析（来自渠道的轮次和继承了租户的委派轮次是 `user:<uuid>`，否则是 `agent:<name>`）—— 循环永远无法指定别的范围。字节存在内容寻址的 blob 存储里（`files:` 配置；本地文件系统或 S3 兼容 —— 未变的文件在快照之间自动去重）；快照元数据（工作树、提交、引用、临时文件）存在运行时存储的 `files_meta` 表里，由引擎掌管，可以用普通 SQL 查看。版本管理是**引擎原生快照，不是 git 服务器**：一次提交记录完整树（路径 → 句柄）并带父指针和说明，同时移动一个具名引用；checkout 解析引用或提交 ID。原始字节不过脚本桥 —— `put` 接受 utf-8 内容或 base64 `data`，`read` 返回句柄加元数据，实体化在引擎侧完成：Shell 派生时的 checkout、`scratch_mount`，或 `http.request` / `builtin:fetch` / `builtin:browser` 的 `save_to`。临时文件按会话隔离（键是调用会话，不是用户），TTL 由 `files.scratch_ttl` 控制（默认 24h，读取时惰性过期 + 启动时清扫）。一个 `files` 能力管住这一整族；它不在默认集合里，需要显式声明。启动时存储被禁用（例如 S3 凭据解析不出来）的话，每个操作都会如实失败。

#### `mail` —— 能力：`net.mail`

```ts
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
// opts: { host, port: 993, user, mailbox: "INBOX", unseen: false,
//         limit: 10, auth: { service: "..." } }
// messages: [{ seq, uid, flags, subject, from, to, date, body }]
// 通过隐式 TLS 连接；邮件正文上限 256 KiB（超出部分截断并加标记）

mail.smtp_send(opts) -> { ok, sent }
// opts: { host, port: 465, user, from, to: [...], subject, text_body, auth }
// 端口 465 = 隐式 TLS；587/25 = STARTTLS
```

和 `http.request` 一样，`auth: { service: "..." }` 指向一个由 Go 在调用时解析的已存储凭据 —— 密码永远不过脚本桥。主机、端口和用户名来自循环（通常经 `os.env`），脚本源码里因此不存任何密钥。`net.mail` 不在默认能力集里；智能体必须声明它。

#### 核心工具

```ts
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
json.encode(value) / json.decode(text)
time.sleep(seconds)        // 让出到 Go 计时器；从不阻塞操作系统线程
```

### 编写支持片段

支持片段在循环插件之前求值进会话状态。它们可以定义循环或其他支持片段要读的全局。例如，要换掉默认的召回策略，就在 `plugins/recency.ts` 里覆写 `plugin:recency`，定义一个 `memory_recall_handler(query, opts)`。

```ts
// plugins/recency.ts（覆盖插件示例）
function memory_recall_handler(query: Record<string, unknown>, opts?: Record<string, unknown>) {
  const info = agent.info();
  const storeName = (info.memory && info.memory.stores.dialogue) || "dialogue";
  const res = store.query(storeName, { kind: "all" });
  return res.records || [];
}
```

### 错误处理

外部调用一律包在 `try { ... } catch (err) { ... }` 里，免得某个抽风的提供商或坏命令把会话搞成崩溃循环：

```ts
try {
  const reply = llm.chat(messages, { model: info.model });
} catch (err) {
  log.warn("llm.chat failed: " + String(err));
  session.send("⚠️ I couldn't reach my model right now.");
  return;
}
```

桥抛出的是从 Go 响应解包出来的 `Error`，所以堆栈指向你的代码，而不是 prelude。

### 确认与权限

工具、Shell 内置项与外部 API 动作可以声明 `needs_confirm`。需要确认时，运行时会创建一条绑定会话、工作区、合资格审批人、动作修订版本、归一化参数和目标地址的单次审批记录，向用户询问（或向 Studio 推送脱敏审批提示），并在单次执行时原子消耗该审批；脚本不能传入 `confirmed: true`。在有副作用的调用上使用 `opts.idempotency_key` 以便在不确定结果时对账或重播。你的循环只看最终结果，所以多数情况下不必手工处理确认。

### 限制与注意事项

- `llm.stream` 返回 ES6 可迭代对象 —— 用 `for (const delta of llm.stream(...))`，或手动调 `iter.next()` / `iter.close()`。
- 标准库就是 Goja 的 ECMAScript 运行时（`Object`、`Array`、`Map`、`Set`、`JSON`、`RegExp`、`Math`、`Date`、`Proxy`、没有事件循环的 `Promise`，以及类型化数组）。Node/浏览器 API（`require`、`fs`、`process`、`fetch`、`setTimeout`、`setInterval`）都不可用，也没有宿主 `os` 库：prelude 只提供 `os.env`、`os.time` 和 `os.date`（全部 UTC —— 沙箱里没有本地时区）。
- 脚本的原始网络访问只能经由 `http.request`、`api.*`（外部 API 被授予宿主 HTTP 时）和 `mail.*` 操作。没有套接字库。HTTP 那几条路径 —— `http.request`、`ExtHost.http` 和 `builtin:fetch` —— 在拨号器里拒绝私有、回环、链路本地和保留地址，所以光有 URL 并不足以够到宿主自己的网络；见 `net` 块。`mail.*` *不*受该防护约束，仍会拨向循环指定的任何主机。
- 过桥的所有值都做 JSON 编码。对象和数组必须可序列化；函数和 symbol 过不去。
- 插件可热重载：重载监听器总会运行（`runtime.reload.watch` 无效）。改循环文件或指令文件会在下一轮从零重启会话 VM；改 `gateway.route` 背后的文件会在消息之间重建路由器状态（路由在那儿跑，不在会话里）；改被覆写的支持片段会重启所有活跃会话，新片段就在那时加载。都不需要重启进程，而且新版本编译不过就不会被应用 —— 正在跑的版本原样留着。

### 实用示例

把下面任意一个放进循环文件，把智能体的 `loop:` 指向它即可：

#### 工具循环

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.type === "user" && msg.text) {
      const info = agent.info();
      const toolsList = tools.list();
      try {
        const reply = llm.chat(
          [{ role: "user", content: msg.text }],
          { model: info.model, tools: toolsList, tool_choice: "auto" },
        );
        if (reply.tool_calls && reply.tool_calls.length > 0) {
          for (const tc of reply.tool_calls) {
            const r = tools.run(tc.name, tc.args || {});
            log.info("tool result: " + json.encode(r));
          }
          session.send(reply.text || "done");
        } else {
          session.send(reply.text || "");
        }
      } catch (err) {
        session.send("error: " + String(err));
      }
    }
  }
}
```

#### Shell 循环

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.type === "user" && msg.text) {
      try {
        const r = shell.run("echo hello from " + msg.text);
        session.send("exit=" + r.exit_code + " stdout=" + r.stdout);
      } catch (err) {
        session.send("the shell is not available: " + String(err));
      }
    }
  }
}
```

#### 多智能体请求

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.type === "user" && msg.text) {
      try {
        const answer = agent.request("agent:expert", { question: msg.text }, 30);
        session.send("expert says: " + ((answer.payload && answer.payload.answer) || "?"));
      } catch (err) {
        session.send("expert unavailable: " + String(err));
      }
    }
  }
}
```
