# 编写脚本循环

一份用 TypeScript / JavaScript 写 AgentFlow 会话循环的实用指南：执行模型、完整的脚本 API、实战模式，以及那些容易踩的坑。

这些模式的可运行版本都在 [`examples/loops/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/loops)。

内置的 TypeScript 类型声明在 [ts/af.d.ts](ts/af.d.ts)；从本页打开就能在编辑器里查看 API 全貌。

## 目录

1. [最小的循环](#1-the-smallest-loop)
2. [执行模型](#2-execution-model)
3. [消息](#3-messages)
4. [状态与持久化](#4-state-and-persistence)
5. [能力](#5-capabilities)
6. [API 参考](#6-api-reference)
7. [模式](#7-patterns)
8. [容错](#8-resilience)
9. [提示词组装与 Token 预算](#9-prompt-assembly-and-token-budgets)
10. [测试与调试](#10-testing-and-debugging)
11. [坑点](#11-gotchas)
12. [速查表](#12-quick-reference-table)
13. [支持片段、覆写与路由处理器](#13-support-chunks-shadows-and-route-handlers)

---

## 1. 最小的循环

会话循环是一个定义了全局或导出函数 `loop()` 的 TypeScript / JavaScript 模块。运行时为每个会话新建一个隔离的 Goja VM 状态并调用 `loop()`；这个函数会在该会话的整个生命周期内持续运行。

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();      // 挂起，直到下一条消息到来
    session.send("echo: " + (msg.text || ""));
  }
}
```

让智能体指向这个文件：

```yaml
agents:
  echo:
    loop: ./examples/loops/echo.ts
```

启动运行时并给它发一条消息（示例配置带一个 webhook 渠道）：

```bash
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/local/ \
     -H 'Content-Type: application/json' \
     -d '{"from":"alice","text":"hello"}'
# echo: hello
```

### 唯一必需的一行

每一段脚本块 —— 循环、支持片段、路由处理器都一样 —— 都必须在开头的注释块里声明 prelude API 版本：

```ts
// af-prelude-version: 2
```

没有这个指令、或者版本不对的代码块，会在**运行之前**就被拒绝。没有兼容窗口：prelude 和使用它的脚本是一起升级的。版本号在源码里是 `vm.PreludeVersion`（脚本可见为 `af.version`），也可以从 `GET /admin/api/studio/catalog` 查到；当前值是 **2**。

### 先校验再运行

Studio 应用（`/admin/api/studio/*`）校验循环的方式是：检查指令、剥掉 TypeScript 注解、编译代码块，然后实时应用。`POST /admin/api/studio/agents/validate` 做同样的检查，但没有副作用。Go 里也可以直接做一次编译检查（`vm.CompileCheck`）。

编译检查只能证明语法没问题，证明不了行为：引用了不存在的全局、调用了没启用的能力，或者逻辑写错，都会在第一轮运行时报错，并在 Studio 的追踪里显示出来。

---

## 2. 执行模型

每个会话都是一个 **actor**：

- 一个 actor goroutine、一个邮箱、一个隔离的 Goja VM 状态加一个 worker goroutine；
- 消息按顺序逐条处理；
- 循环永远不会和自己并发。

暴露给 JS 的核心原生原语是 `__af_op(json_request)`。脚本调用某个操作时，VM 的 worker goroutine 挂在一个 Go channel 上；Go 执行请求（快的操作内联执行，阻塞的放到工作池），然后带着 `[json_response, ok]` 恢复 VM。prelude 把它包成 `op(req)`，并在 `ok` 为 false 时抛 `Error`。因为挂起的会话 goroutine 是在 Go channel 上等，一个挂起的会话不占用任何操作系统线程。

```ts
// 概念上，prelude 替你做的事情：
const [resp, ok] = __af_op(json.encode(req));
if (!ok) throw new Error(String(resp));
return json.decode(resp);
```

### 挂起点与重启

`session.inbox()` 就是挂起点。会话 VM 挂起等待下一条邮箱消息时，会话处于空闲、开销极低。不需要用 `time.sleep` 轮询 —— 永远不要写 `while (true) { ... time.sleep(0.1); ... }` 去找活干；邮箱本身就是工作队列。

`loop()` 周围会发生什么：

| 事件 | 结果 |
|---|---|
| `session.inbox()` | 挂起，直到有消息到达 |
| `session.send` / `push` / `llm.chat` / `tools.run` / `api.*` / `store.*` / `shell.*` / `http.*` / `mail.*` / `agent.*` | 跨 Go 桥让出，拿到结果后恢复 |
| `log.*`、`agent.info`、`agent.config`、`json.*` | 快速的宿主调用，不需要卸载到工作池 |
| `loop()` 返回 | 循环**立刻重启**（跑完的循环不等于退出的会话） |
| 未捕获的 JS 错误逃出 `loop()` | 循环崩溃，约 1 秒后重启，如此反复 |
| `session.exit()` | 会话结束；VM 被销毁；计时器和请求被回收 |

有两点值得记牢：

- **要结束会话，调 `session.exit()`。** 从 `loop()` 返回只会重启它。
- **把可能失败的工作包进 `try { ... } catch (err) { ... }`。** 未捕获的提供商错误会变成崩溃循环，而不是一次失败的轮次。

**JS 内存变量只是缓存，不是存储。** 一轮开始时 VM 状态可能是全新的：热重载、故障转移、租约丢失，或空闲卸载（`runtime.sessions.idle_unload`，默认 30m）都会让循环从头重跑。正确的循环需要跨轮记住的东西，都该放在 `session.state.*`、记忆或存储里 —— 今天熬过重启的模块变量，下次未必。把游标放在 JS 变量里的循环，只有在对话连续 30 分钟没消息之前才是对的。

### 热重载

改动循环文件，会在它下一个挂起点用全新状态重启会话 VM。指令和基于文件的提示词原地重新加载；路由处理器在消息之间重建；被覆写的支持片段会重启所有活跃会话。新版本编译不过的话，正在跑的版本原样留着。每个会话就是一个对话：一次编辑可能重启每个实例上该会话的副本，而重启会丢掉所有不在 `session.state`、记忆、存储或文件里的 JS 内存状态。

### 一次一个操作

每个会话跑一个单线程的 Goja 运行时，由会话 actor 一次驱动一个操作（没有 `setTimeout` / `setInterval` 事件循环）。辅助函数和 `tool.def` 的处理器被调用时就在同一个 VM goroutine 上同步执行，可以依次调用宿主操作，做完后正常交回控制权。

---

## 3. 消息

`session.inbox()` 返回这样的对象：

```ts
{
  id: "msg-...",                 // 稳定的消息 ID（审计记录以它关联）
  type: "user",                  // user | agent | timer | cron | boot | system
  from: "webhook:alice",         // 由核心标记的发送者
  text: "hello",                 // 渠道文本；可能为空
  channel: "webhook",            // 来源渠道（timer/cron/boot 消息没有）
  reply_to: "...",               // 回复时使用的渠道特定接收方
  ts: 1730000000,                // Unix 秒
  payload: { ... },              // op 特定的数据（见下文）
  attachments: [ ... ],          // 媒体部件描述符（如有）
  provenance: {                  // 由核心标记；循环永远无法控制
    kind: "channel",             // channel | agent | timer | scheduler | system | studio
                                 // timer = 一次 session.scheduler.* 触发，范围限于
                                 // 设置它的租户；scheduler = 引擎触发器，
                                 // 也是唯一的维护类定时器来源
    principal: "webhook:alice",
    parent: "...",               // 父会话 ID（agent 消息）
    request_id: "...",           // 关联 ID（agent.request / agent.reply）
    user_uuid: "u_...",          // 本轮次代表的租户，或 null/undefined
    membership_uuid: "...",      // 群组上下文（经过校验时才有）
  },
}
```

### 消息类型

| `type` | 来自哪里 | `payload` |
|---|---|---|
| `user` | 某个渠道，或 Studio 演练场 | 渠道相关（`chat_id`、`text` 等） |
| `agent` | `agent.send` / `agent.request` / `agent.spawn` 流量 | 发送方的 payload 对象 |
| `timer` | `scheduler.every/after/cron` | `{ timer_id: "..." }` |
| `shell.result` | 一个 `shell.start` 作业结束 | 该作业：`{ id, state, exit_code, stdout, stderr, changed, ... }` |
| `cron` | 引擎的 `every:` / `cron:` 触发器 | 触发器的 `payload:` 原样复制 |
| `boot` | supervisor 为 `persistent: true` 的智能体发出 | 无 |
| `system` | 引擎生命周期消息（例如 `agent.died`） | 事件相关 |

`timer`、`shell.result`、`cron` 和 `boot` 轮次**没有渠道**。`session.send` 只回复活跃的入站消息，否则会以 `no active message to reply to` 失败；有真实接收方时用 `session.push(channel, reply_to, text)`（能力 `channel.push`）或 `session.push_user(uuid, text)`，否则就把状态留到下一个用户轮次。

### 身份来自来源信息

身份从 `msg.provenance` 读，绝不从 `msg.payload` 读：

- `user_uuid` 是本轮所服务的一方。没有任何东西给消息盖章时**不存在**，消息不属于任何租户时**为空**。
- `membership_uuid` 在路由器提议了群组、且引擎校验了发送者成员关系时设置。
- 委派跳转会带上发送方的租户；由引擎盖章，`payload.user_uuid` 无论谁写的都被忽略。

循环没法靠往 payload 里塞 UUID 来扩大租户范围。

### 附件

渠道吃进媒体时，消息会带一些小的描述符：

```ts
{
  type: "image", mime: "image/png",
  handle: "media:<sha256>", name: "photo.png",
}
```

把它们原样转进 `llm.chat`（`parts: msg.attachments`）或 `session.send`（`{ attachments: msg.attachments }`）。原始字节永远不进 JS VM；运行时在提供商/渠道边界解析句柄。

### 发送

```ts
session.send(text);                              // 回复当前活跃的消息
session.send(text, { attachments: parts });      // 附带媒体
session.push(channel, reply_to, text);           // 主动出站；能力 channel.push
session.push_user(uuid, text);                   // 按身份 uuid 发送；能力 channel.push
```

`session.send` 和 `session.push` 都会经过由核心掌管的出站安全链和审计日志；循环绕不过其中任何一个。`session.push_user` 需要身份层和一个裸 UUID（prelude 会补上 `user:`）。

---

## 4. 状态与持久化

状态有四个地方可以放。挑能熬过你需要的那个事件的最小一个。

| 机制 | 作用域 | 熬过热重载 | 熬过重启 | 多实例 | 适用 |
|---|---|---|---|---|---|
| JS 局部变量 | 这个 VM 状态 | **否** | 否 | 否 | 一轮之内的临时数据 |
| `session.state` | 一个会话 | 是 | 是（运行时存储） | 是，跟着会话走 | 游标、计数器、计时器 ID |
| 记忆方案（`memory.*`） | 一个智能体（按用户/键范围） | 是 | 是 | 是 | 对话历史、事实 |
| `store.*` | 一张具名表，带作用域 | 是 | 是 | 是 | 原始键值或向量数据 |
| `files.*` | 一个用户/智能体项目树 | 是 | 是 | 是 | 文档与快照 |

### `session.state`

一个会话专属的小型持久键值存储。需要 `session.state` 能力（不在默认集合里）。

```ts
session.state.set(key, value);   // value 可以是任何可 JSON 编码的 JS 值
session.state.get(key);          // 从未设置时为 null/undefined
session.state.delete(key);
session.state.list();            // { [key]: value }
```

限制：键最长 128 个字符，可含字母、数字和 `. _ - :`；值上限 64 KiB；每会话 256 个键。它适合放计时器 ID、游标或步数 —— 不适合放对话历史。

### 记忆

`memory.write` 把记录按方案的写入链（默认 `plugin:routing_table`）送进已声明的存储区。`memory.recall` 分派到方案的召回处理器（默认 `plugin:recency`，或 `plugin:semantic`）。

```ts
// 一条轮次记录：默认链会把它路由到 "dialogue" 存储区。
memory.write({
  type: "turn",
  id: msg.id,
  text: msg.text,
  reply: reply.text,
});

// 一条事实：路由到 "facts" 存储区。
memory.write({ type: "fact", key: "name:" + msg.from, value: { text: "..." } });

// 显式指定目标时会绕过路由表。
memory.write({ targets: [{ store: "dialogue", key: "custom:" + msg.id, value: "..." }] });
```

`memory.recall` 返回记录的**列表**，每条是 `{ key, value, meta }` —— 不是 `{ records }` 包装。查询形状取决于处理器：

| 处理器 | 查询 |
|---|---|
| `plugin:recency`（默认） | `{ store?, kind: "all" \| "prefix" \| "text" \| "time", prefix?, text?, k?, from?, to? }` |
| `plugin:semantic` | `{ text, store?, k? }`（方案上需要有 `embed_model`） |

不带 `store` 时，按时间召回读 `dialogue`，所以要在 `facts` 上做全文检索就得指明存储区（`{ kind: "text", store: "facts", text: "...", k: 5 }`）。时间窗是 `kind: "time"` 加 RFC3339 的 `from` 和 `to`。召回返回处理器给什么就是什么；按时间和语义都返回记录列表。方案、存储区和隔离详见[记忆](memory.md)指南。

### 原始存储

`store.*` 是更底层的表 API，受 `memory` 能力门控，作用域由引擎决定（私有存储按用户给行分键，外加 service 行和升级前的行；绑定池的存储由租户授权治理）。

```ts
store.put("table", "key", value, { ttl: 3600, vector: [...] });
store.get("table", "key");                 // { value, found }
store.query("table", { kind: "all" });     // { records: [{ key, value, meta }, ...] }
store.delete("table", "key");
```

查询种类：`all`、`prefix`（`prefix: "..."`）、`text`（`text: "...", k: n`）、`vector`（`vector: [...], k: n`）、`time_range`（`from`、`to` 为 RFC3339）。后端必须支持该种类：声明了 `requires: ["vector"]`（或 `["text_search"]`）的存储区，后端不支持时启动就失败。给不支持 `vector` 特性的后端传向量是明确报错，绝不静默返回空结果。

### 文件

用户范围的文件存储（`files` 能力）存放项目树，带引擎原生的快照版本管理。字节留在引擎侧；`put` 接受文本或 base64，`read` 返回元数据和句柄。

```ts
files.put("notes", "inbox/1.md", msg.text);
files.list("notes", "inbox/");
files.commit("notes", { ref: "main", message: "note " + msg.id });
files.checkout("notes", "main");
```

临时文件按会话隔离并带 TTL：

```ts
files.scratch.put("page.html", body);
files.scratch.read("page.html");
files.scratch.list();
```

---

## 5. 能力

能力在运行时门控脚本面。被拒绝的操作会抛出一个明确的 `Error`，点名操作、智能体和缺失的能力 —— 不会变成莫名其妙的 undefined 属性错误。

| 能力 | 门控 |
|---|---|
| `llm.chat` | `llm.chat`、`llm.stream`、`llm.forward`、`llm.embed`、`llm.rerank`、`openai.request` |
| `memory` | `memory.*`、`store.*` |
| `tools` | `tools.list`、`tools.run`（声明的工具遵循可见性规则） |
| `apis` | `api.list`、`api.call`、`api.batch`、`api.<pkg>.<action>` |
| `agent.send` | `agent.send`（`agent.reply` 由请求归属授权，`agent.list` 按联系范围且不受门控） |
| `agent.request` | `agent.request` |
| `agent.spawn` | `agent.spawn` |
| `net.http` | `http.request`、`http.get`、`http.post`、`os.env` |
| `net.mail` | `mail.*` |
| `shell.exec` | `shell.*` |
| `net.ssh` | `ssh.*` |
| `files` | `files.*` |
| `session.state` | `session.state.*` |
| `users` | `user.current`、`user.usage`、`user.has_credential`、`user.get`、`user.list` |
| `channel.push` | `session.push`、`session.push_user` |

智能体不写 `capabilities` 时的默认值：`llm.chat`、`memory`、`tools`、`apis`、`agent.send`、`net.http`、`users`。`net.mail`、`shell.exec`、`net.ssh`、`files`、`session.state` 和 `channel.push` 必须显式声明。

不受此表门控的：

- `scheduler.*` —— 计时器是引擎投递的消息，不是权限；
- `runtime.triggers()` —— 只读的部署数据；
- `credential.get(name)` —— 不受门控，但按智能体白名单（`credentials: [...]`）；每次访问都记日志并计数。

`plugins.allow_capabilities` 是部署上限：智能体不能声明超出它的能力，派生出的子会话拿到父会话与模板的交集，`plugins.enforce_capabilities: false` 则为旧配置恢复「只声明」的行为。别依赖它。

---

## 6. API 参考

下面的签名都来自 prelude。失败会抛 `Error`；失败不该搞崩循环的地方，用 `try { ... } catch (err) { ... }`。

### 6.1 `session`

```ts
session.inbox() -> Message                  // 挂起
session.send(text, opts?)                   // 回复当前活跃的消息
session.send(text, { attachments: parts })
session.push(channel, reply_to, text, opts?) // 能力：channel.push
session.push_user(uuid, text)                // 能力：channel.push；需要身份层
session.exit()                               // 结束会话（不再恢复）
session.state.set(key, value)                // 能力：session.state
session.state.get(key) -> value | null | undefined
session.state.delete(key)
session.state.list() -> { [key]: value }
```

### 6.2 日志与时间

```ts
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
time.sleep(seconds)       // 让出执行；挂起期间会话处于空闲
os.time() -> Unix seconds // UTC；不接受参数
os.date([format,] t?) -> date object | string
os.env(name) -> string    // 引擎的环境变量；未设置时为 ""
```

`os.date` 默认返回对象：`{ year, month, day, hour, min, sec, wday, yday, isdst }`，`wday` 1 = 周日，全部 UTC。传 strftime 字符串则返回文本：`os.date("%Y-%m-%d %H:%M:%S", t)`。支持的指令：`%Y %y %m %d %e %H %I %M %S %p %A %a %B %b %j %w %u %Z %%`。没有本地时区；前导 `!` 当作无操作接受。相比之下，cron 触发器按 `runtime.timezone_offset_hours` 匹配。

### 6.3 `agent`

```ts
agent.info() -> {
  name, session_id, address, model, instructions,
  history_budget,
  memory: { stores: { [name]: { backend, table, features, locality } },
            recall, embed_model, rerank_model, oversample },
  shell, skills, capabilities,
  agent_model, instructions_append, ...          // 按用户的覆盖项
}
agent.config() -> {
  name, model, instructions_path | instructions_prompt,
  prompts: { [key]: "text" },
  goal?: { type, success_signal, max_turns, on_goal_met },
  extras: { ... }
}
agent.send(address, payload)                    // 能力：agent.send
agent.request(address, payload, timeout_s?) -> Message  // 能力：agent.request
agent.reply(request_id, payload)                // 由请求的归属方授权
agent.list() -> [ { session_id, agent, parent_id } ]    // 限于联系人范围
agent.spawn(profile, spec?) -> { address, session_id, agent } // 能力：agent.spawn
```

- `agent.info().instructions` 是应用了按用户层之后的系统提示词；`agent_model` 和 `instructions_append` 把覆盖片段暴露出来，循环想据此分支的话可以用。
- `agent.info().memory.stores[name].locality` 是 `deployment`、`region` 或 `instance`：存储区的数据能覆盖多远。持久对话数据（`dialogue`/`facts` 存储区）该放在 `deployment` 或 `region` 存储上；`instance` 存储随进程消亡，那它是缓存，不是历史。单实例部署可以有意用一个，区域单元里想这么做则必须在持久存储区上设 `locality_ok: true`。
- `agent.config()` 是作为数据的部署方案。密钥引用渲染成不透明标记 `{env: "VAR"}` / `{cred: "service"}`；解析后的值永远不过桥。
- 地址：配置好的静态智能体是 `agent:<name>`，活跃会话（以及派生出的子会话 —— 用 `spawn_result.address`）是 `session:<id>`。`send`/`request` 不接受 `:new`。
- `agent.request` 超时即取消；超时单位是**秒**。
- `agent.reply` 必须用核心盖章来源信息里的请求 ID：`msg.provenance.request_id`。绝不要从 `msg.payload` 里拼。
- `agent.spawn` 需要一个 `profiles.agent` 模板。子会话的能力是父会话 ∩ 模板，联系人也是父会话 ∩ 模板。`spec` 为兼容而接受，但在此版本中**不生效**；请改在模板里配置。

### 6.4 `scheduler`

```ts
scheduler.every(interval_s) -> { id }
scheduler.after(delay_s) -> { id }
scheduler.cron(expr) -> { id }
scheduler.cancel(id)
```

计时器把 `Message{ type: "timer", payload: { timer_id: id } }` 投递到同一个会话。它们在内存里，且是会话级的：重启就忘，而热重载会重启循环但**不清除**计时器，所以盲目重注册会重复。先取消记住的 ID（见 [`scheduled.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/scheduled.ts)）。

`scheduler.cron` 是个子集：`*/N * * * *` 和 `30 9 * * *` 这类固定形式；日、月、星期必须是 `*`，`*/N` 分钟不能和固定小时搭配。更复杂的排期请用 `every`/`after`（或引擎触发器）。

### 6.5 `llm`

```ts
llm.chat(messages, opts?) -> {
  text, thinking, thinking_blocks, usage, tool_calls
}
llm.stream(messages, opts?) -> Iterable<string>
llm.embed(inputs, opts?) -> { vectors, usage }
llm.rerank(query, documents, opts?) -> { results: [ { index, score } ] }
```

`messages` 是 `{ role, content }` 轮次的数组，多模态轮次用 `{ role, parts: [...] }`。工具历史用 `tool_calls`、`tool_call_id`，以及 `role: "tool"` 轮次上的 `tool_result`/`content`。开启 thinking 续接 Anthropic 工具循环时，在 assistant 轮重放 `thinking_blocks`。

`llm.chat` 的 `opts`：

```ts
{
  model: "default",          // 一个 models: 条目；"" = 智能体默认模型
  temperature: 0.7,
  max_tokens: 4096,
  thinking: "off"|"low"|"medium"|"high"|"xhigh"|"max",
  tools: tools.list(),       // 模型可见的工具定义
  tool_choice: "auto"|"none",
}
```

`usage` 是 `{ input, output, cached, cache_write, reasoning }`，取决于提供商上报了什么。

**流式。** `llm.stream` 返回标准的 ES6 可迭代对象。用 `for (const delta of llm.stream(messages, opts))` 迭代；提前跳出会触发 `return()`，自动关闭底层流（也可以手动调 `iter.next()` / `iter.close()`）。它只产出文本增量 —— 用量和工具调用请用 `llm.chat`。见 [`stream.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/stream.ts)。

**嵌入。** `llm.embed` 接受单个字符串、字符串数组，或字符串与片段对象（`{ type: "image", handle: ..., url: ... }`）混合的数组；`merged: true` 把所有东西折成一个向量（一个 Jina 内容组）。`llm.rerank` 返回指向 `documents` 的、从 0 开始的索引。

### 6.6 `memory` 与 `store`

```ts
memory.write(record) -> true
memory.recall(query, opts?) -> [ { key, value, meta } ]

store.put(table, key, value, opts?)   // opts：{ ttl: 秒数, vector: [...] }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records: [ { key, value, meta } ] }
store.delete(table, key)
store.scopes(table) -> [ "user:<uuid>", ... ]   // 仅限维护来源
```

记忆记录和查询的形状见[记忆](#memory)。`store.scopes` 只能从引擎触发的维护来源（`system` / `scheduler` 轮次）到达；渠道或智能体跳转轮次会被拒绝。夜间蒸馏器靠它枚举用户范围，而不必读某个用户的轮次。

### 6.7 `tools` 与 `tool.def`

```ts
tools.list() -> [ { type: "function", function: { name, description, parameters } } ]
tools.run(name, args, opts?) -> result
// opts: { idempotency_key?: "..." }
```

在 TypeScript/JS 顶层、`loop()` 之前声明一个模型可见的工具：

```ts
tool.def({
  name: "script:shout",
  description: "Uppercase the text and return it.",
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

声明的工具会并入 `tools.list()`，并由 `tools.run` 分派；抛错的处理器返回 `{ ok: false, error: ... }`，而不是把循环搞死。可见性和 Go 注册的工具完全一样，跟随智能体的 `skills` 和 `tools.policy.default` —— 被隐藏的声明工具既不列出也不可分派。Go 策略（`forbidden`、`needs_confirm`）管不到循环自己声明的处理器。

如果某个工具（Go 的，或通过覆盖声明的）带 `needs_confirm: true`，运行时会拦下调用，创建一条绑定会话、工作区、合资格审批人、动作修订版本、参数与目标地址的单次审批记录，并暂停等待已验证确认（渠道 `[ref:<id>]` / `confirm.decision` 或 Studio 内联脱敏审批卡片）。脚本输入不能伪造或传入 `confirmed: true`；`opts.idempotency_key` 会在运行时存储中持久化写入意图，从而在重试时重播已完成结果，并在中断导致结果不确定时故障关闭（提示等待上限为十分钟）。

### 6.8 `shell`

```ts
shell.run(command, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out,
                                 truncated, changed, deleted, conflicts, skipped }
shell.start(command, opts?) -> { id, state: "running" }   // 结果会投递到收件箱
shell.status(job_id) / shell.cancel(job_id)

// opts：{ project, environment, workdir, env, timeout }   // timeout 以秒为单位
```

每次调用都是文件存储里某个项目上、某个沙箱里的一条命令。命令在项目里写的东西结束时会被存下；别的一概活不到下一次调用，而下一次调用可以在任何实例上跑。非零退出码是结果；跑不起来的作业会抛错。`shell.spawn`、`shell.exec`、`shell.write` 和 `shell.destroy` 作为沙箱兼容垫片保留（需要 `shell.exec`）。远程 SSH 连接改用 `ssh.spawn`、`ssh.exec`、`ssh.read`、`ssh.write` 和 `ssh.close`（需要 `net.ssh`）。完整规则见脚本插件开发参考。

```ts
const conn = ssh.spawn({ host: "box:22", user: "dev" })
const out  = ssh.exec(conn.id, "uname -a")
ssh.write(conn.id, "/tmp/flag", "1")
const txt  = ssh.read(conn.id, "/tmp/flag")
ssh.close(conn.id)
```

### 6.9 `http` 与 `os`

```ts
http.request(opts) -> {
  ok, status, headers, body,
  truncated?, saved?, save_error?
}
http.get(url, opts?)
http.post(url, body, opts?)

// opts: {
//   method, url, headers, body, json, query, timeout,
//   auth: { service: "..." },      // 存储的凭据，由 Go 解析
//   save_to: "name",               // 响应体同时写入文件临时区
// }
```

任何 HTTP 状态都是成功的请求结果（`ok` 说请求完成了，`status` 说服务端说了什么）。响应体上限 10 MiB（被截断时置 `truncated`）；`save_to` 让大响应体不进入 JS，并返回 `saved: { name, handle, size }`。私有、回环、链路本地和保留地址在拨号器里就被拒绝。需要 `net.http`（它同时门控 `os.env`）。

`builtin:fetch` **工具**是另一套更丰富的接口：它额外支持 `follow`/`max_redirects`、`user_agent`、`max_bytes`，以及 `insecure`（关闭 TLS 校验，总会记日志并计数）。该由模型调用时用这个工具；循环自己要发请求时用 `http.request`。

### 6.10 `mail`

```ts
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
// opts: { host, port: 993, user, mailbox: "INBOX", unseen: false,
//         limit: 10, auth: { service: "..." } }
// messages: [ { seq, uid, flags, subject, from, to, date, body } ]

mail.smtp_send(opts) -> { ok, sent }
// opts: { host, port: 465, user, from, to: [...], subject,
//         text_body, auth: { service: "..." } }
```

密码在调用时从凭据存储解析，永远不过桥。`mail.*` 不受 HTTP 私有地址防护约束。需要 `net.mail`。

### 6.11 `files`

```ts
files.put(project, path, text | { data: base64, handle: "media:...", mime: "..." }, opts?) -> { ok, entry }
files.read(project, path) -> { ok, entry }
files.list(project, prefix?) -> { ok, entries }
files.delete(project, path) -> { ok: true }
files.projects() -> { ok, projects }
files.commit(project, { ref: "main", message: "..." }?) -> { ok, commit }
files.checkout(project, ref | commit_id) -> { ok, manifest }

files.scratch.put(name, text | { data: base64, mime: "..." }) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

作用域由引擎根据轮次身份解析；循环无法指定另一个用户的项目。一次提交是带父指针和具名引用的全树快照 —— 版本管理是引擎原生的，不是 git 服务器。需要 `files`。

### 6.12 `user`

```ts
user.current() -> { ok, registered, id, display_name, identities }
user.usage() -> { ok, input, output, cached, reasoning, calls, failed,
                  billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { ok, service, present }
user.get(id) -> profile          // 仅限维护来源
user.list() -> [ profile ]       // 仅限维护来源
```

关于本轮所服务用户的只读信息。`has_credential` 回答密钥存不存在，永远不是它的值。`get`/`list` 仅限引擎触发的维护轮次。

### 6.13 `runtime` 与 `credential`

```ts
runtime.triggers() -> {
  ok,
  triggers: [ {
    name, kind, run_on_boot,
    target: { profile }, payload,
    event: { channel, match } | cron | every,
  } ]
}

credential.get(name) -> { ok, name, value }
```

`runtime.triggers()` 是启动时快照的合并触发器列表，循环和路由处理器都能用。`credential.get` 只对智能体 `credentials:` 白名单里的名字成功；每次访问都记日志并计数，值绝不该发给用户。

### 6.14 `json`

```ts
json.encode(value) -> string
json.decode(text) -> value
```

过桥的一切都做 JSON 编码。后果是：

- JS 原生区分空数组 `[]` 和空对象 `{}`；数组序列化成 JSON 数组，普通对象序列化成 JSON 对象；
- 函数、symbol 和循环引用对象过不去；
- `undefined` 属性被省略，而 `undefined` 的数组槽位和 `NaN`/`±Infinity` 编码成 `null`；
- `json.decode` 要求字符串，格式有误时抛错。

### 6.15 `api`（外部 API）

```ts
api.list() -> [ { name, version, description, runtime: "script" | "wasm", actions } ]
api.call(pkg, action, args?, opts?) -> result   // opts: { idempotency_key?: "..." }
api.batch(calls) -> [ { ok, data?, error? } ]   // calls: [{ api, action, args, idempotency_key? }]
api.<pkg>.<action>(args?) -> result              // api.call 的 Proxy 简写
```

外部 API 是独立的 TypeScript（`goja`）或 WebAssembly（`wazero`）包，在 `apis:` 下配置（`apis` 能力，在默认集合里）。每个包在自己的隔离运行时、自己的权限和超时下运行。直接调用 `api.call` 与 `api.batch` 会通过与 `api:<pkg>.<action>` 工具封装相同的能力、`tools.policy`、托管模式 `kind`、审批和 `idempotency_key` 校验。`api.call` 和 `api.<pkg>.<action>` 出错会抛，而 `api.batch` 返回逐次调用的 `{ ok, data, error }`（任何需要确认的批量子项都会单独失败，必须改用 `api.call` 调用）。标记为 `expose_as_tool: true` 的操作，在智能体同时拥有 `apis` 和 `tools` 时，也会作为 `api:<pkg>.<action>` 工具出现。

---

## 7. 模式

下面每个模式在 `examples/loops/` 里都有可运行文件。想看成套的部署组合 —— 带记忆的 QQ 群机器人、Telegram RAG 机器人、GitHub 拉取请求代码智能体、chat/completions 端点 —— 见 [`examples/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples) 里的场景。

### 7.1 请求/响应

[`echo.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/echo.ts) —— 最小的循环。

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    session.send("echo: " + (msg.text || ""));
  }
}
```

### 7.2 带记忆的对话智能体

[`chat.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/chat.ts) 召回最近的轮次、组装提示词、裁到历史预算、回复，并记录本轮。

```ts
const info = agent.info();
let messages = [
  { role: "system", content: info.instructions || "You are a helpful assistant." },
];
const hits = memory.recall({ store: "dialogue", kind: "text", text: msg.text, k: 5 });
for (const hit of hits) {
  const v = hit.value;
  if (v && typeof v === "object") {
    if (v.text) messages.push({ role: "user", content: v.text });
    if (v.reply) messages.push({ role: "assistant", content: v.reply });
  }
}
messages.push({ role: "user", content: msg.text || "" });
messages = token_budget(messages, info.history_budget || 6000);
const reply = llm.chat(messages);
session.send(reply.text || "");
memory.write({ type: "turn", id: msg.id, text: msg.text, reply: reply.text });
```

`token_budget` 是加载进每个会话的支持片段；它保留系统消息和最新的消息，从最旧的历史开始丢，直到估算值放得下。一次工具往来 —— 请求了工具的 assistant 轮加上回答它的结果 —— 会作为一个整体保留或丢弃，所以裁剪过的历史一定是提供商能接受的历史。

### 7.3 流式

[`stream.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/stream.ts) —— 用 `for (const delta of llm.stream(messages, opts))` 迭代；提前跳出会触发 `return()` 自动关闭流（也可以手动调 `.close()`）。记住：只产出文本增量字符串。

### 7.4 多轮工具循环

[`tool_loop.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/tool_loop.ts) —— 完整形态：

1. 带上 `tools: tools.list()` 发送用户轮次；
2. 回复里有 `tool_calls`，就追加 **assistant** 轮（包括它的工具调用和思考块）；
3. 逐个执行，追加带 `tool_call_id` 的 `role: "tool"` 轮；
4. 再问一次 —— 并且继续循环，因为模型可能再要一个工具；
5. 在最终的文本回复或轮数上限处停下。

只跑工具却从不把结果送回去、或者从不追问的循环，会得到一个看起来无视工具的模型。

### 7.5 在 TypeScript/JS 里声明工具

在顶层用 `tool.def`（见 API 参考）。处理器在会话 VM 上跑，可以调用任何操作；抛出的错误变成 `{ ok: false, error: ... }`。

### 7.6 计时器与周期任务

[`scheduled.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/scheduled.ts) 幂等地注册计时器，并用 `payload.timer_id` 区分它们。计时器轮次没有渠道；不要在里面 `session.send`。

### 7.7 常驻智能体

[`daemon.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/daemon.ts) 处理 `persistent: true` 智能体启动时收到的合成 `boot` 消息，然后服务用户消息和自己的计时器触发。常驻智能体就是一个会话；在集群里恰好由一个实例拥有它。

### 7.8 多智能体委派

[`delegate.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/delegate.ts) 从 `profiles.agent` 模板派生一个子会话，并等待请求/回复：

```ts
const child = agent.spawn("researcher");
const answer = agent.request(child.address, { question: msg.text }, 120);
```

在子会话一侧，用核心盖章的 ID 应答请求：

```ts
if (msg.type === "agent") {
  const rid = msg.provenance && msg.provenance.request_id;
  if (rid) agent.reply(rid, { answer: "..." });
} else if (msg.type === "user") {
  session.send("...");
}
```

静态对等方可以按 `agent:<name>` 寻址（受 `can_contact` 约束）。租户跟着跳转走；子会话写的是同一个用户的范围，除非它用 `inherit_user: false` 退出了。

### 7.9 文件与快照

[`files.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/files.ts) 写一条笔记、列出项目、提交一个快照。要恢复某棵树用 `files.checkout`，要在工作树上跑命令用 `shell.run("make", { project: "..." })`。

### 7.10 带存储凭据的 HTTP

```ts
const r = http.request({
  url: "https://api.example.com/v1/items",
  headers: { Accept: "application/json" },
  auth: { service: "example_api" },   // 由 Go 解析；永远不会进入 JS
  query: { limit: 10 },
  timeout: 20,
});
if (r.status === 200) {
  const data = json.decode(r.body);
  session.send("found " + String((data.items || []).length) + " items");
} else {
  session.send("upstream said " + String(r.status));
}
```

大响应体可以加 `save_to: "response.json"`，再用 `files.scratch.read` 读。目标受地址防护约束，除非部署用 `net.http.allow_private` 退出。

### 7.11 邮件

```ts
const inbox = mail.imap_fetch({
  host: "imap.example.com", user: "bot@example.com",
  mailbox: "INBOX", unseen: true, limit: 5,
  auth: { service: "mail_password" },
});
```

### 7.12 维护循环

`system`/`scheduler` 来源的轮次可以枚举作用域和资料：

```ts
for (const scope of store.scopes("dialogue")) {
  // scope 形如 "user:<uuid>"；用显式带范围的键读写
}
```

引擎触发的工作（夜间压缩、蒸馏器）才是预期用法。渠道轮次会被拒绝，所以一个用户永远枚举不了另一个用户的作用域。

---

## 8. 容错

### 按轮捕获失败

```ts
try {
  handler(msg);
} catch (err) {
  log.error("turn failed: " + String(err));
  session.send("Sorry — that turn failed.");
}
```

未捕获的错误会让循环崩溃，然后每秒重启一次；给用户看的错误几乎总比崩溃循环好。

### 带退避的重试

```ts
function withRetry(fn: () => unknown, attempts: number): [boolean, unknown] {
  for (let i = 1; i <= attempts; i++) {
    try {
      return [true, fn()];
    } catch (_) {
      if (i < attempts) time.sleep(0.5 * i);
    }
  }
  return [false, "retries exhausted"];
}
```

`time.sleep` 会让给 Go，不阻塞操作系统线程。

### 幂等

会话可能在轮次中途重启（崩溃、热重载、实例故障转移）。如果某一轮有外部副作用，就在它之前或之后往 `session.state` 里记个标记，进来时检查。跨重启没有 exactly-once 保证。

### 给每个外部调用设边界

给 `http.request` 设 `timeout`；用 `agent.request` 的超时；限制工具轮数；用 `token_budget` 限制历史。能永远等下去的循环，最终一定会永远占住一个会话。

---

## 9. 提示词组装与 Token 预算

可靠的提示词形态：

```ts
const info = agent.info();
let messages = [
  { role: "system", content: info.instructions || "" },
];
// 召回的历史（从旧到新）
// 当前用户轮次
messages = token_budget(messages, info.history_budget || 6000);
const reply = llm.chat(messages, { model: info.model });
```

- `info.instructions` 已经包含任何按用户的指令层。
- `token_budget` 按 `ceil(chars / 4)` 估算 Token，保留系统消息和最新消息，从最旧的历史开始丢直到放得下。估算覆盖一条消息的文本、重放的思考块，以及它发起的工具调用的参数。
- **一次工具往来永不拆开。** 带 `tool_calls` 的 assistant 轮和它后面的 `role = "tool"` 消息一起保留或丢弃。工具循环的轮次之间，最新的消息就是工具结果，所以即使这一整个交换本身超预算，也会被完整保留。
- **大的工具结果按它的重量计费。** 它和其他消息一样计数，所以一个巨大的响应体可能挤掉真实历史，而引擎里没有任何地方会截断工具结果：限制工具输出（`builtin:fetch` 有 `max_bytes`，默认 1 MiB），在循环里对大的结果做摘要。
- `token_budget` 只裁剪，不修复。已经含有一个没有对应调用的工具结果的历史，会原样传下去。
- 预算在调用前强制执行：声明的 `budget.tokens_per_day` 是每个智能体的硬池；按用户配额从台账里扣。
- 思考 Token 在 `usage.reasoning` 里报告；它们计入预算。

---

## 10. 测试与调试

### 在 Studio 里

1. 启动运行时并连接 Studio（`runtime.studio.enabled`，默认开启）。
2. 在构建器里编辑循环，按 **校验** —— 它会检查 prelude 指令、剥掉 TypeScript 注解、编译代码块，并校验智能体定义。
3. **应用** 后打开演练场。每一轮都流式输出追踪：`turn.start`、`op.start`/`op.end`（模型、工具、耗时、Token）、安全决策、回复和循环错误。
4. 设置 `runtime.studio.trace_payloads: true` 可以把提示词和回复文本捕获进追踪（默认关闭）。

### 从终端

```bash
./agentflow run -config agentflow.yaml -log-level debug
```

`debug` 会记录每次操作分派、LLM 请求、记忆操作和工具调用。循环里的 `log.info` 出现在同一条流里。AgentFlow Studio 的日志视图（`/admin/api/logs` SSE）就是同一个源。

### 热重载

保存循环文件；运行中的会话在下一个挂起点用全新状态重启。新文件编译不过或声明了错误的 prelude 版本时，正在跑的版本原样留着。`session.state`、记忆、存储和文件熬得过重启；JS 局部变量不行。

### 示例循环

`examples/loops/` 下的每个文件都由测试套件检查（`examples.test.ts` 和 `vm.CompileCheck`）—— 它必须声明当前的 prelude 版本并干净编译。把它们当起点，然后把智能体的 `loop:` 指向你的副本。

---

## 11. 坑点

- **没有指令就不跑。** 开头注释块里的 `// af-prelude-version: N` 是强制的。版本不匹配是硬拒绝，不是警告。
- **从 `loop()` 返回就是重启它。** 要结束会话请调 `session.exit()`。
- **未捕获的错误就是崩溃循环。** 把轮次包进 `try { ... } catch (err) { ... }`。
- **`session.send` 需要活跃的入站消息。** 在 `timer`、`cron` 和 `boot` 轮次上会失败；用 `push`/`push_user`，或者留着状态。
- **计时器会跨热重载重复。** 重新注册前先取消记住的 ID；计时器在内存里，重启就丢。
- **`llm.stream` 只产出文本增量。** 用 `for (const delta of llm.stream(...))` 迭代；提前跳出会触发 `return()` 自动关闭流（也可以手动调 `iter.close()`）。
- **工具循环需要第二轮。** 追加 assistant 的工具调用轮和每个工具结果，然后再问一次。`token_budget` 不统计工具结果。
- **`agent.spawn(profile, spec)` 会忽略 `spec`。** 请配置派生模板。
- **`agent.request` 的超时是秒**，而 `agent.reply` 需要 `msg.provenance.request_id`，不是 payload 里的字段。
- **身份来自来源信息。** `payload.user_uuid` 被忽略；循环无法扩大租户。
- **`memory.recall` 返回列表**，而 `store.query` 返回 `{ records }`。
- **作用域由引擎侧强制执行。** `store.scopes` 和 `user.get/list` 仅限维护；渠道轮次会被拒绝。
- **`os` 不是完整的 Node/Lua `os` 库。** 只有 `os.env`、`os.time` 和 `os.date`，全部 UTC。没有 `require`、`fs`、`process`、`fetch`、`setTimeout`，也没有套接字库。
- **过桥的值都做 JSON 编码。** 空数组 `[]` 和空对象 `{}` 保留各自的 JSON 类型，而函数、symbol 和循环引用对象过不了桥。
- **能力是被强制执行的，不是建议。** 被收回的操作会抛出点名能力的拒绝；`files`、`net.mail`、`shell.exec`、`net.ssh`、`session.state` 和 `channel.push` 要显式声明（`apis` 在默认值里）。
- **钩子。** 引擎只直接分发 `safety.script_filters`、`safety.ingress`、`safety.egress`、`gateway.route`，以及配置好的外部 API（`apis`）操作；会话 VM 里其他一切都是全局约定，由 prelude 或你的循环按名字调用（`memory_routing_table`、`memory_recall_handler`、`token_budget` 等）。
- **`scheduler.cron` 是个子集。** 日/月/星期的排期请用 `every`/`after` 或引擎触发器。
- **`http.request` 在拨号器里拒绝私有目的地**（包括重定向跳转），除非部署设了 `net.http.allow_private`。

---

## 12. 速查表

| 操作 | 能力 | 阻塞 | 说明 |
|---|---|---|---|
| `session.inbox` | — | 挂起 | 邮件循环 |
| `session.send` | — | 是 | 回复活跃消息；安全 + 日志 |
| `session.push` / `push_user` | `channel.push` | 是 | 主动出站 |
| `session.exit` | — | 否 | 结束会话 |
| `session.state.*` | `session.state` | 是 | 持久化的会话级键值 |
| `log.*` | — | 内联 | 日志级别 |
| `time.sleep` | — | 让出 | 空闲，不阻塞 |
| `agent.info` / `config` | — | 内联 | 身份与方案 |
| `agent.send` | `agent.send` | 是 | `can_contact` |
| `agent.reply` / `agent.list` | — | 是/内联 | 请求归属 / 按联系范围 |
| `agent.request` | `agent.request` | 是 | 秒 |
| `agent.spawn` | `agent.spawn` | 是 | 派生模板 |
| `scheduler.*` | — | 是 | 会话计时器 |
| `llm.chat` / `stream` / `embed` / `rerank` | `llm.chat` | 是 | 计量/预算 |
| `memory.*` / `store.*` | `memory` | 是 | 带范围 |
| `tools.list` / `run` | `tools` | 是 | 确认拦截 |
| `api.*` | `apis` | 是 | 外部 TS/Wasm 包 |
| `shell.*` | `shell.exec` | 是 | 一次性沙箱作业；兼容句柄 |
| `ssh.*` | `net.ssh` | 是 | 持久化远程 SSH 连接 |
| `http.*` / `os.env` | `net.http` | 是 | 地址防护 |
| `mail.*` | `net.mail` | 是 | 无地址防护 |
| `files.*` | `files` | 是 | 用户范围 |
| `user.*` | `users` | 是 | get/list 仅限维护 |
| `runtime.triggers` | — | 内联 | 部署数据 |
| `credential.get` | 白名单 | 是 | 记日志；绝不回显 |
| `json.*` | — | 内联 | 桥编码 |

---

## 13. 支持片段、覆写与路由处理器

### 支持片段

四个内置实现在循环**之前**求值进每个会话状态：`token_budget`、`routing_table`、`recency`、`semantic`。它们定义循环可以调用的全局：

| 片段 | 全局 | 使用方 |
|---|---|---|
| `plugin:token_budget` | `token_budget(messages, budget)` | 你的循环 |
| `plugin:routing_table` | `memory_routing_table(record)` | `memory.write` |
| `plugin:recency` | `memory_recall_handler(query, opts)` | `memory.recall` |
| `plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | `memory.recall` |

要替换其中一个，就在 `plugins.dir` 里放一个同名文件（例如 `plugins/recency.ts`）。它会在整个部署范围内覆写内置实现；覆写文件受监听，且每次会话启动时加载，所以编辑后无需重启进程即可生效。覆写同样受版本门禁。

### 路由处理器

`gateway.route`（默认 `plugin:per_chat`）是另一个面：每进程一个 Goja VM 状态，位于任何会话之外。它的 `loop()` 接收 `{ message: Message, agent: "..." }` 项，决定消息去哪：

```ts
function loop() {
  while (true) {
    const item = session.inbox();
    const msg = item.message;
    af.op({
      type: "deliver",
      agent: item.agent,
      key: (msg.channel || "?") + ":" + (msg.from || "?"),
      message: msg,
    });
  }
}
```

`deliver` 接受 `{ agent, key, message, identity_group?, kind? }`。不给 `kind` 时，消息进入 `key` 对应的对话会话，首次消息时创建并一直保留。`kind: "ephemeral"` 给消息一个属于自己的会话，只服务一轮就消失：不占租约、不写收件箱行、ID 永不回收，所以它是无状态流量的正确选择。给它一个每次请求都唯一的键；内置处理器为 [OpenAI 兼容渠道](openai-api.md)就是这么做的：

```ts
if (msg.payload && msg.payload.api === "openai.chat") {
  const conv = msg.payload.conversation_id;
  if (conv !== null && conv !== undefined && conv !== "") {
    af.op({
      type: "deliver", agent: item.agent, message: msg,
      key: "conv:" + msg.channel + ":" + String(conv),
    });
  } else {
    af.op({
      type: "deliver", agent: item.agent, message: msg, kind: "ephemeral",
      key: "req:" + String(msg.payload.request_id),
    });
  }
}
```

未知的 `kind` 会被拒绝，消息不会投递；ephemeral 投递也不能携带 `identity_group`。像上面示例给聊天渠道那样用 `msg.from` 给这类请求分键，在共享密钥的渠道上是个错误：所有调用方都是同一个发送者，于是会共用一个会话。拿了消息却在取下一条之前没投递的处理器，等于把它丢了；正在等这个请求的渠道（OpenAI 兼容渠道）会立刻回 503。

路由处理器只能调 `inbox`、`deliver`、`router.state.*`、`runtime.triggers` 和 `log`；其他一律按名称拒绝。它不能发渠道回复，也不能调模型 —— 它负责路由，干活的是会话循环。`router.state` 是持久的、部署范围的状态（每个键默认 30 天 TTL），用于放聊天的路由模式这类东西。默认的 `per_chat` 处理器还实现了 `/group <uuid>` / `/personal` 切换，由引擎校验提议的成员关系；产品想要不同策略就整体替换处理器，但引擎侧的校验不是可选项。

---

## 延伸阅读

- [完整配置参考](configuration.md) —— 智能体、记忆、能力、触发器、MCP、渠道。
- [脚本插件开发](loops-reference.md) —— 内置文档里的 API 表和模态支持矩阵。
- [`examples/loops/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/loops) —— 上面每个可运行的模式。
- [`agentflow.yaml`](https://github.com/hulipa487/agentflow/blob/main/agentflow.yaml) —— 示例部署（仓库把这个文件 gitignore 掉了，因为它是本地实例配置；纳入版本管理的示例部件在 `examples/` 下）。
