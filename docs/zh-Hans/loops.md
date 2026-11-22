# 编写 Lua 循环

这是一份用 Lua 5.1 编写 AgentFlow 会话循环的实用指南：执行模型、完整的 Lua API、实际可用的模式，以及容易犯的错误。

这些模式的可运行版本位于 [`examples/loops/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/loops)。

内置的 Lua 类型定义位于 [lua/af.d.lua](lua/af.d.lua)；可以从本页打开，在编辑器中查看 API 全貌。

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
11. [常见陷阱](#11-gotchas)
12. [速查表](#12-quick-reference-table)
13. [辅助代码块、覆盖与路由处理器](#13-support-chunks-shadows-and-route-handlers)

---

## 1. 最小的循环

会话循环是一个定义了全局函数 `loop()` 的 Lua 代码块。运行时为每个会话加载一个全新的 Lua 状态并调用 `loop()`；该函数在整个会话生命周期内持续运行。

```lua
-- af-prelude-version: 1
function loop()
  while true do
    local msg = session.inbox()      -- 挂起，直到下一条消息到来
    session.send("echo: " .. (msg.text or ""))
  end
end
```

让智能体指向这个文件：

```yaml
agents:
  echo:
    loop: ./examples/loops/echo.lua
```

启动运行时并给它发一条消息（示例配置带有一个 webhook 渠道）：

```bash
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/local/ \
     -H 'Content-Type: application/json' \
     -d '{"from":"alice","text":"hello"}'
# echo: hello
```

### 唯一必需的一行

每个代码块——无论是循环、辅助代码块还是路由处理器——都必须在开头的注释块中声明 prelude API 版本：

```lua
-- af-prelude-version: 1
```

没有该指令或版本不符的代码块会在**运行之前**被拒绝。不存在兼容窗口：prelude 和使用它的 Lua 同步演进。该版本在源码中是 `vm.PreludeVersion`，可通过 `GET /admin/api/studio/catalog` 查看；当前值为 **1**。

### 运行前先校验

Studio 应用（`/admin/api/studio/*`）校验循环的方式是检查该指令并编译代码块，然后实时应用。`POST
/admin/api/studio/agents/validate` 执行同样的检查，但没有副作用。Go 中也提供单纯的编译检查（`vm.CompileCheck`）。

编译检查只能证明语法正确，不能证明行为正确：引用了不存在的全局变量、调用了未启用的能力，或在运行时行为出错的循环，会在第一个轮次失败，并显示在 Studio 的追踪中。

---

## 2. 执行模型

每个会话都是一个 **Actor**：

- 一个 goroutine、一个邮箱、一个 Lua 状态、一个循环协程；
- 消息按顺序逐条处理；
- 循环永远不会与自身并发运行。

暴露给 Lua 的唯一原生原语是 `__af_op(json_request)`。它让出协程的执行；Go 执行该请求（廉价的 op 直接内联执行，阻塞型 op 交给工作池），然后以 `(json_response, ok)` 恢复协程。prelude 将其封装为 `op(req)`，并在 `ok` 为 false 时把响应作为 Lua 错误抛出。由于每个阻塞调用都会让出执行，挂起的会话不占用任何操作系统线程。

```lua
-- 概念上，prelude 替你做的事情：
local resp, ok = __af_op(json.encode(req))
if not ok then error(resp, 0) end
return json.decode(resp)
```

### 挂起点与重启

`session.inbox()` 就是挂起点。协程挂起期间，会话处于空闲状态，开销很小。不需要用 `time.sleep` 轮询——永远不要写 `while true do ... time.sleep(0.1) ... end` 这样的代码去找活干；邮箱本身就是工作队列。

`loop()` 周围会发生什么：

| 事件 | 结果 |
|---|---|
| `session.inbox()` | 挂起，直到有消息到达 |
| `session.send` / `push` / `llm.chat` / `tools.run` / `store.*` / `shell.*` / `http.*` / `mail.*` / `agent.*` | 让出执行；带着结果恢复 |
| `log.*`, `agent.info`, `agent.config`, `json.*` | 内联执行；不让出 |
| `loop()` 返回 | 循环立即被**重启**（循环结束不等于会话退出） |
| Lua 错误逃出 `loop()` | 循环崩溃，约 1 秒后重启，无限重复 |
| `session.exit()` | 会话结束；协程不再恢复；定时器和请求都会被回收 |

有两点值得牢记：

- **要结束会话，请调用 `session.exit()`。** 从 `loop()` 返回只会让它重启。
- **把可能失败的操作包在 `pcall` 里。** 一个未捕获的提供商错误会导致崩溃循环，而不只是一个失败的轮次。

**Lua 局部变量只是缓存，不是存储。** 一个轮次可能从全新的 Lua 状态开始：热重载、故障转移、租约丢失或空闲卸载（`runtime.sessions.idle_unload`，默认 30m）都会让循环从头开始。正确的循环需要在轮次之间记住的任何东西，都应放在 `session.state.*`、记忆或存储区中——今天的重启中幸存的局部变量，下一次未必能幸存。把游标保存在局部变量里的循环，只在对话空闲不超过 30 分钟时才是正确的。

### 热重载

编辑循环文件后，会话的协程会在下一个挂起点从全新的 Lua 状态重新开始。指令和基于文件的提示词会原地重新加载；路由处理器在两条消息之间重建；被覆盖的辅助代码块会重启所有运行中的会话。无法编译的新版本不会替换正在运行的版本。每个会话就是一个对话：一次编辑可能会重启所有实例上该会话的副本，而重启会丢失所有不在 `session.state`、记忆、存储区或文件中的 Lua 状态。

### 一次一个 op

op 调用要放在循环协程上。不要在用 `coroutine.create` 创建的嵌套协程中调用 `session.inbox`、`llm.chat`、`store.*` 或任何其他会让出执行的 op：Actor 驱动的是循环协程，桥接层期望让出来自那里。普通 Lua 协程用于纯计算没有问题；任何要跨越桥接层的操作都应放在主循环中。

---

## 3. 消息

`session.inbox()` 返回如下形式的表：

```lua
{
  id = "msg-...",                 -- 稳定的消息 ID（审计记录以它关联）
  type = "user",                  -- user | agent | timer | cron | boot | system
  from = "webhook:alice",         -- 由核心标记的发送者
  text = "hello",                 -- 渠道文本；可能为空
  channel = "webhook",            -- 来源渠道（timer/cron/boot 消息没有）
  reply_to = "...",               -- 回复时使用的渠道特定接收方
  ts = 1730000000,                -- Unix 秒
  payload = { ... },              -- op 特定的数据（见下文）
  attachments = { ... },          -- 媒体部件描述符（如有）
  provenance = {                  -- 由核心标记；循环永远无法控制
    kind = "channel",             -- channel | agent | timer | scheduler | system | studio
                                  -- timer = 一次 session.scheduler.* 触发，范围限于
                                  -- 设置它的租户；scheduler = 引擎触发器，
                                  -- 也是唯一的维护类定时器来源
    principal = "webhook:alice",
    parent = "...",               -- 父会话 ID（agent 消息）
    request_id = "...",           -- 关联 ID（agent.request / agent.reply）
    user_uuid = "u_...",          -- 本轮次代表的租户，或 nil
    membership_uuid = "...",      -- 群组上下文（经过校验时才有）
  },
}
```

### 消息类型

| `type` | 来源 | `payload` |
|---|---|---|
| `user` | 渠道或 Studio 演练场 | 渠道特定（`chat_id`、`text`、...） |
| `agent` | `agent.send` / `agent.request` / `agent.spawn` 流量 | 发送方的载荷表 |
| `timer` | `scheduler.every/after/cron` | `{ timer_id = "..." }` |
| `shell.result` | 一个 `shell.start` 作业结束 | 该作业：`{ id, state, exit_code, stdout, stderr, changed, ... }` |
| `cron` | 引擎的 `every:` / `cron:` 触发器 | 原样复制触发器的 `payload:` |
| `boot` | 监管器，针对 `persistent: true` 的智能体 | 无 |
| `system` | 引擎生命周期消息（如 `agent.died`） | 视事件而定 |

`timer`、`shell.result`、`cron` 和 `boot` 轮次**没有渠道**。`session.send` 只回复当前活跃的入站消息，否则会以 `no active message to reply
to` 失败；有真实接收方时，请使用 `session.push(channel, reply_to, text)`（能力 `channel.push`）或 `session.push_user(uuid, text)`，或者先记录状态，留待下一个用户轮次处理。

### 身份来自来源信息

从 `msg.provenance` 读取身份，绝不要从 `msg.payload` 读取：

- `user_uuid` 是本轮次代表的租户。没有任何组件标记该消息时它**不存在**；消息不属于任何租户时它**为空**。
- 当路由器提议了一个群组且引擎校验了发送者的成员身份时，会设置 `membership_uuid`。
- 委派的每一跳都会携带发送方的租户；由引擎标记，无论是谁写的 `payload.user_uuid` 都会被忽略。

循环无法通过在载荷里放一个 UUID 来扩大租户范围。

### 附件

当渠道接收了媒体时，消息会携带小巧的描述符：

```lua
{
  type = "image", mime = "image/png",
  handle = "media:<sha256>", name = "photo.png",
}
```

把它们原样转交给 `llm.chat`（`parts = msg.attachments`）或 `session.send`（`{ attachments = msg.attachments }`）。原始字节永远不会进入 Lua；运行时在提供商/渠道边界解析句柄。

### 发送

```lua
session.send(text)                              -- 回复当前活跃的消息
session.send(text, { attachments = parts })     -- 附带媒体
session.push(channel, reply_to, text)           -- 主动出站；能力 channel.push
session.push_user(uuid, text)                   -- 按身份 uuid 发送；能力 channel.push
```

`session.send` 和 `session.push` 会经过核心掌管的安全出站过滤链和审计记录；循环无法绕过其中任何一个。`session.push_user` 需要启用身份层，并传入不带前缀的 UUID（prelude 会加上 `user:`）。

---

## 4. 状态与持久化

状态可以存放在四个地方。选择能挺过你所担心的那种情况的最小选项。

| 机制 | 范围 | 热重载后保留 | 重启后保留 | 多实例 | 用途 |
|---|---|---|---|---|---|
| Lua 局部变量 | 当前协程 | **否** | 否 | 否 | 单个轮次内的临时数据 |
| `session.state` | 单个会话 | 是 | 是（运行时存储） | 是，随会话迁移 | 游标、计数器、定时器 ID |
| 记忆方案（`memory.*`） | 单个智能体（按用户/键范围） | 是 | 是 | 是 | 对话历史、事实 |
| `store.*` | 一张具名表，带范围 | 是 | 是 | 是 | 原始键值或向量数据 |
| `files.*` | 一个用户/智能体的项目树 | 是 | 是 | 是 | 文档和快照 |

### `session.state`

单个会话的小型持久键值存储。需要 `session.state` 能力（不在默认集合中）。

```lua
session.state.set(key, value)   -- value 可以是任何可 JSON 编码的 Lua 值
session.state.get(key)          -- 从未设置时为 nil
session.state.delete(key)
session.state.list()            -- { key = value }
```

限制：键最长 128 个字符，由字母、数字和 `. _ - :` 组成；值最大 64 KiB；每个会话最多 256 个键。它适合存放定时器 ID、游标或步数——不适合存放对话历史。

### 记忆

`memory.write` 让一条记录经过方案的写入链（默认是 `plugin:routing_table`），路由到声明的存储区中。`memory.recall` 交给方案的召回处理器处理（默认是 `plugin:recency`，也可以是 `plugin:semantic`）。

```lua
-- 一条轮次记录：默认链会把它路由到 "dialogue" 存储区。
memory.write({
  type = "turn",
  id = msg.id,
  text = msg.text,
  reply = reply.text,
})

-- 一条事实：路由到 "facts" 存储区。
memory.write({ type = "fact", key = "name:" .. msg.from, value = { text = "..." } })

-- 显式指定目标时会绕过路由表。
memory.write({ targets = { { store = "dialogue", key = "custom:" .. msg.id, value = "..." } } })
```

`memory.recall` 返回记录**列表**，每条为 `{ key, value, meta }`——而不是 `{ records }` 包装。查询结构取决于处理器：

| 处理器 | 查询 |
|---|---|
| `plugin:recency`（默认） | `{ store?, kind = "all" \| "prefix" \| "text" \| "time", prefix?, text?, k?, from?, to? }` |
| `plugin:semantic` | `{ text, store?, k? }`（需要方案上配置 `embed_model`） |

不指定 `store` 时，recency 读取 `dialogue`，因此在 `facts` 上做文本搜索时要写明存储区（`{ kind = "text", store = "facts", text = "...", k = 5 }`）。时间窗口用 `kind = "time"`，并以 RFC3339 格式给出 `from` 和 `to`。召回返回处理器返回的任何内容；recency 和 semantic 都返回记录列表。[记忆](memory.md)指南介绍了方案、存储区和隔离。

### 原始存储区

`store.*` 是更底层的表 API。它由 `memory` 能力把关，范围由引擎确定（私有存储区按用户为行加键，外加服务行和升级前的行；绑定到记忆池的存储区由租户授权管理）。

```lua
store.put("table", "key", value, { ttl = 3600, vector = { ... } })
store.get("table", "key")                 -- { value, found }
store.query("table", { kind = "all" })    -- { records = { { key, value, meta }, ... } }
store.delete("table", "key")
```

查询类型：`all`、`prefix`（`prefix = "..."`）、`text`（`text = "...", k = n`）、`vector`（`vector = {...}, k = n`）、`time_range`（`from`、`to` 为 RFC3339）。后端必须支持该类型：声明了 `requires: ["vector"]`（或 `["text_search"]`）的存储区，如果后端不支持，会在启动时失败。向不具备 `vector` 特性的后端传入向量会明确报错，绝不会悄悄返回空结果。

### 文件

用户范围的文件存储（`files` 能力）保存项目树，并提供引擎原生的快照版本管理。字节数据始终留在引擎侧；`put` 接受文本或 base64，`read` 返回元数据和一个句柄。

```lua
files.put("notes", "inbox/1.md", msg.text)
files.list("notes", "inbox/")
files.commit("notes", { ref = "main", message = "note " .. msg.id })
files.checkout("notes", "main")
```

临时区按会话划分，并带 TTL：

```lua
files.scratch.put("page.html", body)
files.scratch.read("page.html")
files.scratch.list()
```

---

## 5. 能力

能力在运行时为 Lua 接口把关。被拒绝的 op 会抛出清晰的错误，指明该 op、智能体和缺少的能力——而不是消失在 `attempt to call a nil value` 里。

| 能力 | 把关的对象 |
|---|---|
| `llm.chat` | `llm.chat`, `llm.stream`, `llm.embed`, `llm.rerank` |
| `memory` | `memory.*`, `store.*` |
| `tools` | `tools.list`、`tools.run`（声明的工具遵循可见性规则） |
| `agent.send` | `agent.send`（`agent.reply` 由请求归属授权，`agent.list` 限于联系人范围且不受把关） |
| `agent.request` | `agent.request` |
| `agent.spawn` | `agent.spawn` |
| `net.http` | `http.request`, `http.get`, `http.post`, `os.env` |
| `net.mail` | `mail.*` |
| `shell.exec` | `shell.*` |
| `files` | `files.*` |
| `session.state` | `session.state.*` |
| `users` | `user.current`, `user.usage`, `user.has_credential`, `user.get`, `user.list` |
| `channel.push` | `session.push`, `session.push_user` |

智能体省略 `capabilities` 时的默认值：`llm.chat`、`memory`、`tools`、`agent.send`、`net.http`、`users`。`net.mail`、`shell.exec`、`files`、`session.state` 和 `channel.push` 需要显式声明。

不受此表把关的有：

- `scheduler.*`——定时器是引擎投递的消息，不是权限；
- `runtime.triggers()`——只读的部署数据；
- `credential.get(name)`——不受把关，但按智能体设有白名单（`credentials: [...]`）；每次访问都会被记录和计数。

`plugins.allow_capabilities` 是部署级上限：智能体声明的能力不能超出它的允许范围，派生出的子智能体获得父智能体与模板的交集，而 `plugins.enforce_capabilities: false` 会为旧配置恢复“仅声明”的行为。不要依赖它。

---

## 6. API 参考

下面所有签名都是 prelude 提供的。失败时会抛出 Lua 错误；在不希望失败导致循环崩溃的地方使用 `pcall`。

### 6.1 `session`

```lua
session.inbox() -> Message                  -- 挂起
session.send(text, opts?)                   -- 回复当前活跃的消息
session.send(text, { attachments = parts })
session.push(channel, reply_to, text, opts?) -- 能力：channel.push
session.push_user(uuid, text)                -- 能力：channel.push；需要身份层
session.exit()                               -- 结束会话（不再恢复）
session.state.set(key, value)                -- 能力：session.state
session.state.get(key) -> value | nil
session.state.delete(key)
session.state.list() -> { key = value }
```

### 6.2 日志与时间

```lua
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
time.sleep(seconds)       -- 让出执行；挂起期间会话处于空闲
os.time() -> Unix seconds -- UTC；不接受参数
os.date([format,] t?) -> date table | string
os.env(name) -> string    -- 引擎的环境变量；未设置时为 ""
```

`os.date` 默认返回一个表：`{ year, month, day, hour, min, sec, wday,
yday, isdst }`，`wday` 1 = 星期日，全部为 UTC。传入 strftime 格式字符串可得到文本：`os.date("%Y-%m-%d %H:%M:%S", t)`。支持的格式符：`%Y %y %m %d %e %H %I
%M %S %p %A %a %B %b %j %w %u %Z %%`。不存在本地时区；开头的 `!` 会被接受，但不起作用。相比之下，cron 触发器按 `runtime.timezone_offset_hours` 匹配。

### 6.3 `agent`

```lua
agent.info() -> {
  name, session_id, address, model, instructions,
  history_budget,
  memory = { stores = { [name] = { backend, table, features, locality } },
             recall, embed_model, rerank_model, oversample },
  shell, skills, capabilities,
  agent_model, instructions_append, ...          -- 按用户的覆盖项
}
agent.config() -> {
  name, model, instructions_path | instructions_prompt,
  prompts = { key = "text" },
  goal = { type, success_signal, max_turns, on_goal_met }?,
  extras = { ... }
}
agent.send(address, payload)                    -- 能力：agent.send
agent.request(address, payload, timeout_s?) -> Message  -- 能力：agent.request
agent.reply(request_id, payload)                -- 由请求的归属方授权
agent.list() -> [ { session_id, agent, parent_id } ]   -- 限于联系人范围
agent.spawn(profile, spec?) -> { address, session_id, agent } -- 能力：agent.spawn
```

- `agent.info().instructions` 是应用了按用户层之后的系统提示词；如果循环想据此分支，`agent_model` 和 `instructions_append` 会暴露覆盖的各个部分。
- `agent.info().memory.stores[name].locality` 为 `deployment`、`region` 或 `instance`：表示该存储区的数据能覆盖多远。持久的对话数据（`dialogue`/`facts` 存储区）应放在 `deployment` 或 `region` 存储区上；`instance` 存储区随进程消亡，因此它是缓存，而不是历史。单实例部署可以有意使用它，区域单元则必须在持久存储区上设置 `locality_ok: true` 才能这样做。
- `agent.config()` 以数据形式给出部署方案。密钥引用会渲染为不透明的标记 `{env="VAR"}` / `{cred="service"}`；解析后的值永远不会跨越桥接层。
- 地址：已配置的静态智能体用 `agent:<name>`，运行中的会话用 `session:<id>`（派生出的子智能体也是如此——请使用 `spawn_result.address`）。`send`/`request` 不接受 `:new`。
- `agent.request` 超时即取消；超时以**秒**为单位。
- `agent.reply` 必须使用核心标记的来源信息中的请求 ID：`msg.provenance.request_id`。绝不要从 `msg.payload` 构造它。
- `agent.spawn` 需要一个 `profiles.agent` 模板。子智能体的能力是父智能体 ∩ 模板，其联系人是父智能体的联系人 ∩ 模板的联系人。`spec` 为兼容而保留，但在此版本中**不会生效**；请改为配置模板。

### 6.4 `scheduler`

```lua
scheduler.every(interval_s) -> { id }
scheduler.after(delay_s) -> { id }
scheduler.cron(expr) -> { id }
scheduler.cancel(id)
```

定时器会把 `Message{ type = "timer", payload = { timer_id = id } }` 投递到同一个会话。它们保存在内存中，范围限于会话：重启会让它们丢失，而热重载会重启循环但**不会**清除定时器，所以不加判断地重新注册会产生重复。先取消记住的 ID（见 [`scheduled.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/scheduled.lua)）。

`scheduler.cron` 只支持一个子集：`*/N * * * *` 和 `30 9 * * *` 这样的固定形式；日、月和星期几必须是 `*`，并且 `*/N` 形式的分钟不能与固定的小时搭配。更复杂的计划请使用 `every`/`after`（或引擎触发器）。

### 6.5 `llm`

```lua
llm.chat(messages, opts?) -> {
  text, thinking, thinking_blocks, usage, tool_calls
}
llm.stream(messages, opts?) -> next_delta, close
llm.embed(inputs, opts?) -> { vectors, usage }
llm.rerank(query, documents, opts?) -> { results = [ { index, score } ] }
```

`messages` 是由 `{ role, content }` 轮次组成的数组，多模态轮次则用 `{ role, parts = {...} }`。工具历史在 `role = "tool"` 的轮次上使用 `tool_calls`、`tool_call_id` 和 `tool_result`/`content`。在开启思考的情况下继续 Anthropic 工具循环时，要在助手轮次上回放 `thinking_blocks`。

`llm.chat` 的 `opts`：

```lua
{
  model = "default",          -- 一个 models: 条目；"" = 智能体默认模型
  temperature = 0.7,
  max_tokens = 4096,
  thinking = "off"|"low"|"medium"|"high"|"xhigh"|"max",
  tools = tools.list(),       -- 模型可见的工具定义
  tool_choice = "auto"|"none",
}
```

`usage` 为 `{ input, output, cached, cache_write, reasoning }`，取决于提供商报告了哪些项。

**流式输出。** 直接调用 `next_delta`；泛型 `for` 会被拒绝，因为迭代器会跨越桥接层让出执行。迭代器在完成时自行关闭；提前退出后要调用 `close()`。它只返回文本增量——用量和工具调用请用 `llm.chat` 获取。见 [`stream.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/stream.lua)。

**嵌入。** `llm.embed` 接受单个字符串、字符串数组，或字符串与部件表（`{type="image", handle=..., url=...}`）混合的数组；`merged = true` 会把所有内容合并为一个向量（Jina 内容组）。`llm.rerank` 返回指向 `documents` 的从 0 开始的索引。

### 6.6 `memory` 和 `store`

```lua
memory.write(record) -> true
memory.recall(query, opts?) -> [ { key, value, meta } ]

store.put(table, key, value, opts?)   -- opts：{ ttl = 秒数, vector = {...} }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records = [ { key, value, meta } ] }
store.delete(table, key)
store.scopes(table) -> [ "user:<uuid>", ... ]   -- 仅限维护来源
```

记忆的记录和查询结构见[记忆](#memory)。`store.scopes` 只能从引擎触发的维护来源（`system` / `scheduler` 轮次）调用；渠道轮次或智能体跳转轮次会被拒绝。夜间提炼任务正是借此枚举用户范围，而无需读取某个用户的轮次。

### 6.7 `tools` 和 `tool.def`

```lua
tools.list() -> [ { type = "function", ["function"] = { name, description, parameters } } ]
tools.run(name, args, opts?) -> result
-- opts：{ confirmed = true, confirm_id = "..." }
```

在代码块顶层、`loop()` 之前用 Lua 声明一个模型可见的工具：

```lua
tool.def({
  name = "lua:shout",
  description = "Uppercase the text and return it.",
  params = {
    type = "object",
    properties = { text = { type = "string" } },
    required = { "text" },
  },
  handler = function(args)
    return { ok = true, shouted = string.upper(args.text or "") }
  end,
})
```

声明的工具会合并进 `tools.list()`，并由 `tools.run` 调度；处理函数抛错时会返回 `{ ok = false, error = ... }`，而不会终止循环。可见性完全与 Go 注册的工具一样，遵循智能体的 `skills` 和 `tools.policy.default`——被隐藏的声明工具既不会被列出，也无法被调度。Go 的策略（`forbidden`、`needs_confirm`）不约束循环自己声明的处理函数。

如果某个工具（Go 工具，或通过覆盖声明的工具）设置了 `needs_confirm: true`，运行时会拦截第一次 `tools.run`，询问用户，然后以 `confirmed = true` 重新调用；循环只会看到最终结果。等待确认的时间有上限（十分钟）。

### 6.8 `shell`

```lua
shell.run(command, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out,
                                 truncated, changed, deleted, conflicts, skipped }
shell.start(command, opts?) -> { id, state = "running" }   -- 结果会投递到收件箱
shell.status(job_id) / shell.cancel(job_id)

-- opts：{ project, environment, workdir, env, timeout }   -- timeout 以秒为单位
```

每次调用就是在一个沙箱中、针对文件存储中的一个项目执行一条命令。命令在项目中写入的内容会在结束时保存；除此之外什么都不会保留到下一次调用，而下一次调用可能在任意实例上运行。非零退出码属于正常结果；无法运行的作业才会抛错。`shell.spawn`、`shell.exec`、`shell.write` 和 `shell.destroy` 仍然存在：用于通过 SSH 连接远程机器（`provider = "ssh"`），也作为旧循环的兼容层。需要 `shell.exec`。完整规则见循环 API 参考。

### 6.9 `http` 和 `os`

```lua
http.request(opts) -> {
  ok, status, headers, body,
  truncated?, saved?, save_error?
}
http.get(url, opts?)
http.post(url, body, opts?)

-- opts: {
--   method, url, headers, body, json, query, timeout,
--   auth = { service = "..." },      -- 存储的凭据，由 Go 解析
--   save_to = "name",                -- 响应体同时写入文件临时区
-- }
```

任何 HTTP 状态码都算请求成功的结果（`ok` 表示请求已完成；`status` 表示服务器的答复）。响应体上限为 10 MiB（被截断时会设置 `truncated`）；`save_to` 让大型响应体不进入 Lua，并返回 `saved = { name, handle, size }`。私有地址、回环地址、链路本地地址和保留地址会在拨号器中被拒绝。需要 `net.http`（它同时把关 `os.env`）。

`builtin:fetch` **工具**是另一套更丰富的接口：它还支持 `follow`/`max_redirects`、`user_agent`、`max_bytes` 和 `insecure`（关闭 TLS 校验，始终会被记录和计数）。需要由模型调用时用该工具；循环自己需要发请求时用 `http.request`。

### 6.10 `mail`

```lua
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
-- opts: { host, port = 993, user, mailbox = "INBOX", unseen = false,
--         limit = 10, auth = { service = "..." } }
-- messages: [ { seq, uid, flags, subject, from, to, date, body } ]

mail.smtp_send(opts) -> { ok, sent }
-- opts: { host, port = 465, user, from, to = { ... }, subject,
--         text_body, auth = { service = "..." } }
```

密码在调用时从凭据存储中解析，永远不会跨越桥接层。`mail.*` 不受 HTTP 私有地址防护约束。需要 `net.mail`。

### 6.11 `files`

```lua
files.put(project, path, text | { data = base64, handle = "media:...", mime = "..." }, opts?) -> { ok, entry }
files.read(project, path) -> { ok, entry }
files.list(project, prefix?) -> { ok, entries }
files.delete(project, path) -> { ok = true }
files.projects() -> { ok, projects }
files.commit(project, { ref = "main", message = "..." }?) -> { ok, commit }
files.checkout(project, ref | commit_id) -> { ok, manifest }

files.scratch.put(name, text | { data = base64, mime = "..." }) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

范围由引擎根据轮次的身份确定；循环无法指定其他用户的项目。一次提交是一个完整的目录树快照，带有父指针和一个具名引用——版本管理是引擎原生的，而不是一个 git 服务器。需要 `files`。

### 6.12 `user`

```lua
user.current() -> { ok, registered, id, display_name, identities }
user.usage() -> { ok, input, output, cached, reasoning, calls, failed,
                  billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { ok, service, present }
user.get(id) -> profile          -- 仅限维护来源
user.list() -> [ profile ]       -- 仅限维护来源
```

关于本轮次所服务的人的只读信息。`has_credential` 只回答某个密钥是否存在，绝不返回其值。`get`/`list` 仅限引擎触发的维护轮次使用。

### 6.13 `runtime` 和 `credential`

```lua
runtime.triggers() -> {
  ok,
  triggers = [ {
    name, kind, run_on_boot,
    target = { profile }, payload,
    event = { channel, match } | cron | every,
  } ]
}

credential.get(name) -> { ok, name, value }
```

`runtime.triggers()` 是启动时快照下来的合并后触发器列表，循环和路由处理器都可以使用。`credential.get` 只对智能体 `credentials:` 白名单中的名称成功；每次访问都会被记录和计数，且该值绝不应发送给用户。

### 6.14 `json`

```lua
json.encode(value) -> string
json.decode(text) -> value
```

跨越桥接层的一切都经过 JSON 编码。由此带来：

- **空表编码为 `{}`**，而不是 `[]`；要发送空数组，请省略该字段或换一种信号；
- 数组必须是连续的 1..n 序列；混合键或非数字键会使其变成对象；
- 函数、userdata 和循环引用的表无法跨越；
- `nil` 字段会消失；`NaN`/`±Inf` 编码为 `null`；
- `json.decode` 要求传入字符串，遇到格式错误的输入会抛错。

---

## 7. 模式

下面每个模式在 `examples/loops/` 中都有一个可运行的文件。要看把它们组合起来的完整部署——带记忆的 QQ 群聊机器人、Telegram RAG 机器人、GitHub 拉取请求代码智能体、chat/completions 端点——请参阅 [`examples/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples) 中的场景。

### 7.1 请求/响应

[`echo.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/echo.lua)——最小的循环。

```lua
function loop()
  while true do
    local msg = session.inbox()
    session.send("echo: " .. (msg.text or ""))
  end
end
```

### 7.2 带记忆的对话智能体

[`chat.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/chat.lua) 召回最近的轮次，构建提示词，按历史预算裁剪，回复，然后记录本轮次。

```lua
local info = agent.info()
local messages = {
  { role = "system", content = info.instructions or "You are a helpful assistant." },
}
local hits = memory.recall({ store = "dialogue", kind = "text", text = msg.text, k = 5 })
for _, hit in ipairs(hits) do
  local v = hit.value
  if type(v) == "table" then
    if v.text then table.insert(messages, { role = "user", content = v.text }) end
    if v.reply then table.insert(messages, { role = "assistant", content = v.reply }) end
  end
end
table.insert(messages, { role = "user", content = msg.text or "" })
messages = token_budget(messages, info.history_budget or 6000)
local reply = llm.chat(messages)
session.send(reply.text or "")
memory.write({ type = "turn", id = msg.id, text = msg.text, reply = reply.text })
```

`token_budget` 是加载到每个会话中的辅助代码块；它保留系统消息和最新一条消息，并从最旧的历史开始丢弃，直到估算值符合预算。一次工具往返——请求调用工具的助手轮次，以及回应它的结果——会整体保留或整体丢弃，因此裁剪后的历史始终是提供商能接受的形式。

### 7.3 流式输出

[`stream.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/stream.lua)——直接使用迭代器，提前退出时调用 `close()`。记住：只有文本增量；泛型 `for` 不起作用。

### 7.4 多轮工具循环

[`tool_loop.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/tool_loop.lua)——完整结构：

1. 发送用户轮次，并带上 `tools = tools.list()`；
2. 如果回复包含 `tool_calls`，追加该**助手**轮次（包括其工具调用和思考块）；
3. 执行每个调用，追加一个带 `tool_call_id` 的 `role = "tool"` 轮次；
4. 再次询问——并持续循环，因为模型可能会请求另一个工具；
5. 在得到最终的文本回复或达到轮数上限时停止。

一个执行了工具却从不把结果发回去、或从不再次询问的循环，会让模型看起来像是无视了它的工具。

### 7.5 用 Lua 声明工具

在代码块顶层使用 `tool.def`（见 API 参考）。处理函数运行在循环协程上，可以调用任何 op；抛出的错误会变成 `{ ok = false, error = ... }`。

### 7.6 定时器与周期性工作

[`scheduled.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/scheduled.lua) 以幂等方式注册定时器，并通过 `payload.timer_id` 区分它们。定时器轮次没有渠道；不要在其中调用 `session.send`。

### 7.7 常驻智能体

[`daemon.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/daemon.lua) 处理 `persistent: true` 智能体在启动时收到的合成 `boot` 消息，然后处理用户消息和它自己的定时器触发。常驻智能体是一个会话；在集群中恰好由一个实例持有它。

### 7.8 多智能体委派

[`delegate.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/delegate.lua) 从一个 `profiles.agent` 模板派生子智能体，并等待请求/回复：

```lua
local child = agent.spawn("researcher")
local answer = agent.request(child.address, { question = msg.text }, 120)
```

在工作方一侧，用核心标记的 ID 回复请求：

```lua
if msg.type == "agent" then
  local rid = msg.provenance and msg.provenance.request_id
  if rid then agent.reply(rid, { answer = "..." }) end
elseif msg.type == "user" then
  session.send("...")
end
```

静态对等智能体可以用 `agent:<name>` 寻址（受 `can_contact` 约束）。租户随跳转传递；子智能体会写入同一用户的范围，除非它通过 `inherit_user: false` 选择不继承。

### 7.9 文件与快照

[`files.lua`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/files.lua) 写入一条笔记，列出项目，并提交一个快照。用 `files.checkout` 恢复目录树，或用 `shell.run("make", { project = "..." })` 在工作树上执行命令。

### 7.10 使用存储凭据的 HTTP 请求

```lua
local r = http.request({
  url = "https://api.example.com/v1/items",
  headers = { ["Accept"] = "application/json" },
  auth = { service = "example_api" },   -- 由 Go 解析；永远不会进入 Lua
  query = { limit = 10 },
  timeout = 20,
})
if r.status == 200 then
  local data = json.decode(r.body)
  session.send("found " .. tostring(#(data.items or {})) .. " items")
else
  session.send("upstream said " .. tostring(r.status))
end
```

对于大型响应体，加上 `save_to = "response.json"`，再用 `files.scratch.read` 读取。目标地址受地址防护约束，除非部署通过 `net.http.allow_private` 选择关闭。

### 7.11 邮件

```lua
local inbox = mail.imap_fetch({
  host = "imap.example.com", user = "bot@example.com",
  mailbox = "INBOX", unseen = true, limit = 5,
  auth = { service = "mail_password" },
})
```

### 7.12 维护循环

来源为 `system`/`scheduler` 的轮次可以枚举范围和用户资料：

```lua
for _, scope in ipairs(store.scopes("dialogue")) do
  -- scope 形如 "user:<uuid>"；用显式带范围的键读写
end
```

它的预期用途是引擎触发的工作（夜间压缩、提炼任务）。渠道轮次会被拒绝，因此一个用户永远无法枚举另一个用户的范围。

---

## 8. 容错

### 按轮次捕获失败

```lua
local ok, err = pcall(handler, msg)
if not ok then
  log.error("turn failed: " .. tostring(err))
  session.send("Sorry — that turn failed.")
end
```

未捕获的错误会让循环崩溃，并每秒重启一次；给用户一个错误提示几乎总比崩溃循环好。

### 带退避的重试

```lua
local function with_retry(fn, attempts)
  for i = 1, attempts do
    local ok, res = pcall(fn)
    if ok then return true, res end
    if i < attempts then time.sleep(0.5 * i) end
  end
  return false, "retries exhausted"
end
```

`time.sleep` 会让出执行；它不会阻塞线程。

### 幂等性

会话可能在轮次中途重启（崩溃、热重载、实例故障转移）。如果某个轮次会产生外部副作用，请在其之前或之后在 `session.state` 中记录一个标记，并在进入时检查它。跨重启不提供“恰好一次”的保证。

### 为每个外部调用设限

为 `http.request` 设置 `timeout`；使用 `agent.request` 的超时；限制工具轮数；用 `token_budget` 限制历史长度。一个可能永远等待的循环，终有一天会永远占住一个会话。

---

## 9. 提示词组装与 Token 预算

一种可靠的提示词结构：

```lua
local info = agent.info()
local messages = {
  { role = "system", content = info.instructions or "" },
}
-- 召回的历史（从旧到新）
-- 当前用户轮次
messages = token_budget(messages, info.history_budget or 6000)
local reply = llm.chat(messages, { model = info.model })
```

- `info.instructions` 已经包含所有按用户的指令层。
- `token_budget` 按 `ceil(chars / 4)` 估算 Token 数，保留系统消息和最新一条消息，并从最旧的历史开始丢弃，直到符合预算。估算范围包括消息的文本、回放的思考块，以及它发起的工具调用的参数。
- **一次工具往返永远不会被拆开。** 带 `tool_calls` 的助手轮次与其后的 `role = "tool"` 消息会一起保留或一起丢弃。在工具循环的各轮之间，最新一条消息是工具结果，因此即使当前这次往返本身就超出预算，它也会被完整保留。
- **大型工具结果有多重就占多少。** 它和其他消息一样计数，所以一个巨大的响应体可能挤掉真正的历史，而引擎中没有任何环节会截断工具结果：请限制工具输出（`builtin:fetch` 有 `max_bytes`，默认 1 MiB），并在循环中对大型结果做摘要。
- `token_budget` 只裁剪，不修复。已经包含没有对应调用的工具结果的历史，会原样传递。
- 预算在调用之前强制执行：声明的 `budget.tokens_per_day` 是每个智能体的硬性额度池；按用户的配额从账本中扣除。
- 思考 Token 在 `usage.reasoning` 中报告；它们计入预算。

---

## 10. 测试与调试

### 在 Studio 中

1. 启动运行时并连接 Studio（`runtime.studio.enabled`，默认开启）。
2. 在构建器中编辑循环，然后点击**校验**——它会检查 prelude 指令、编译代码块，并校验智能体定义。
3. 点击**应用**，然后打开演练场。每个轮次都会流式输出一条追踪：`turn.start`、`op.start`/`op.end`（模型、工具、耗时、Token）、安全决策、回复以及循环错误。
4. 设置 `runtime.studio.trace_payloads: true` 可以在追踪中记录提示词和回复的文本（默认关闭）。

### 在终端中

```bash
./agentflow run -config agentflow.yaml -log-level debug
```

`debug` 会记录每一次 op 调度、LLM 请求、记忆操作和工具调用。循环中的 `log.info` 也出现在同一个日志流中。AgentFlow Studio 的日志视图（`/admin/api/logs` SSE）就是同一份输出。

### 热重载

保存循环文件后，正在运行的会话会在下一个挂起点从全新状态重启。如果新文件无法编译，或声明了错误的 prelude 版本，正在运行的版本会保持不变。`session.state`、记忆、存储区和文件在重启后保留；Lua 局部变量则不会。

### 示例循环

`examples/loops/` 下的每个文件都由 `TestExampleLoopsCompile` 检查——它必须声明当前的 prelude 版本并能编译通过。可以把它们作为起点，然后让智能体的 `loop:` 指向你的副本。

---

## 11. 常见陷阱

- **没有指令就不运行。** 开头注释块中的 `-- af-prelude-version: N` 是必需的。版本不符会被直接拒绝，而不只是警告。
- **从 `loop()` 返回会让它重启。** 要结束会话，请调用 `session.exit()`。
- **未捕获的错误会导致崩溃循环。** 把轮次包在 `pcall` 里。
- **`session.send` 需要一条活跃的入站消息。** 在 `timer`、`cron` 和 `boot` 轮次中它会失败；请使用 `push`/`push_user` 或保存状态。
- **定时器会在热重载后重复。** 再次注册前先取消记住的 ID；定时器保存在内存中，重启后会丢失。
- **`llm.stream`：不能用泛型 `for`。** 直接调用 `next_delta()`，提前退出时调用 `close()`。它只产出文本。
- **工具循环需要第二轮。** 追加助手的工具调用轮次和每个工具结果，然后再次询问。`token_budget` 不计算工具结果。
- **`agent.spawn(profile, spec)` 会忽略 `spec`。** 请配置派生模板。
- **`agent.request` 的超时以秒为单位**，而 `agent.reply` 需要的是 `msg.provenance.request_id`，而不是载荷字段。
- **身份来自来源信息。** `payload.user_uuid` 会被忽略；循环无法扩大租户范围。
- **`memory.recall` 返回列表**，而 `store.query` 返回 `{ records }`。
- **范围在引擎侧强制执行。** `store.scopes` 和 `user.get/list` 仅限维护用途；渠道轮次会被拒绝。
- **`os` 不是真正的 Lua os。** 只有 `os.env`、`os.time` 和 `os.date`，全部为 UTC。没有 `io`、没有 `debug`，也没有套接字库。
- **空表编码为 `{}`。** 数组必须连续；函数和循环引用的表无法跨越桥接层。
- **能力是强制执行的，不是建议。** 被扣下的 op 会抛出指明该能力的拒绝错误；`files`、`net.mail`、`shell.exec`、`session.state` 和 `channel.push` 需要显式声明。
- **没有钩子。** 引擎只调度 `safety.ingress`、`safety.egress` 和 `gateway.route`；其余一切都是由 prelude 或你的循环调用的命名约定（`memory_routing_table`、`memory_recall_handler`、`token_budget`、...）。
- **`scheduler.cron` 只是一个子集。** 涉及日/月/星期几的计划请使用 `every`/`after` 或引擎触发器。
- **`http.request` 会在拨号器中拒绝私有目标地址**（包括重定向的各跳），除非部署设置了 `net.http.allow_private`。

---

## 12. 速查表

| Op | 能力 | 阻塞 | 说明 |
|---|---|---|---|
| `session.inbox` | — | 挂起 | 消息循环 |
| `session.send` | — | 是 | 回复当前活跃的消息；经过安全过滤和审计记录 |
| `session.push` / `push_user` | `channel.push` | 是 | 主动出站 |
| `session.exit` | — | 否 | 结束会话 |
| `session.state.*` | `session.state` | 是 | 按会话的持久 KV |
| `log.*` | — | 内联 | 日志级别 |
| `time.sleep` | — | 让出执行 | 空闲，不阻塞 |
| `agent.info` / `config` | — | 内联 | 身份与方案 |
| `agent.send` | `agent.send` | 是 | `can_contact` |
| `agent.reply` / `agent.list` | — | 是/内联 | 请求归属 / 限于联系人范围 |
| `agent.request` | `agent.request` | 是 | 秒 |
| `agent.spawn` | `agent.spawn` | 是 | 派生模板 |
| `scheduler.*` | — | 是 | 会话定时器 |
| `llm.chat` / `stream` / `embed` / `rerank` | `llm.chat` | 是 | 计量/受预算约束 |
| `memory.*` / `store.*` | `memory` | 是 | 有范围 |
| `tools.list` / `run` | `tools` | 是 | 确认拦截 |
| `shell.*` | `shell.exec` | 是 | 一次性沙箱作业；SSH 句柄 |
| `http.*` / `os.env` | `net.http` | 是 | 地址防护 |
| `mail.*` | `net.mail` | 是 | 无地址防护 |
| `files.*` | `files` | 是 | 用户范围 |
| `user.*` | `users` | 是 | get/list 仅限维护 |
| `runtime.triggers` | — | 内联 | 部署数据 |
| `credential.get` | 白名单 | 是 | 会被记录；切勿回显 |
| `json.*` | — | 内联 | 桥接层编码 |

---

## 13. 辅助代码块、覆盖与路由处理器

### 辅助代码块

四个内置代码块会在循环**之前**加载进每个会话状态：`token_budget`、`routing_table`、`recency`、`semantic`。它们定义了你的循环可以调用的全局变量：

| 代码块 | 全局变量 | 使用方 |
|---|---|---|
| `plugin:token_budget` | `token_budget(messages, budget)` | 你的循环 |
| `plugin:routing_table` | `memory_routing_table(record)` | `memory.write` |
| `plugin:recency` | `memory_recall_handler(query, opts)` | `memory.recall` |
| `plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | `memory.recall` |

要替换其中一个，在 `plugins.dir` 中放一个同名文件（例如 `plugins/recency.lua`）。它会在整个部署范围内覆盖该内置代码块；覆盖文件受到监视，并在每个会话启动时加载，因此编辑后无需重启进程即可生效。覆盖文件同样受版本约束。

### 路由处理器

`gateway.route`（默认 `plugin:per_chat`）是另一套接口：每个进程一个 Lua 状态，位于任何会话之外。它的 `loop()` 接收 `{ message = Message, agent = "..." }` 条目，并决定消息的去向：

```lua
function loop()
  while true do
    local item = session.inbox()
    local msg = item.message
    af.op({
      type = "deliver",
      agent = item.agent,
      key = (msg.channel or "?") .. ":" .. (msg.from or "?"),
      message = msg,
    })
  end
end
```

`deliver` 接受 `{ agent, key, message, identity_group?, kind? }`。不带 `kind` 时，消息会进入 `key` 对应的对话会话，该会话在第一条消息到来时创建并一直保留。`kind = "ephemeral"` 会给消息一个独立的会话，它只服务恰好一个轮次就消失：不获取租约、不写入收件箱行，其 ID 也永远不会被标记为已退役，因此适合无状态流量。请给它一个每个请求都唯一的键；内置处理器就是这样处理 [OpenAI 兼容渠道](openai-api.md)的：

```lua
if msg.payload and msg.payload.api == "openai.chat" then
  local conv = msg.payload.conversation_id
  if conv ~= nil and conv ~= "" then
    af.op({ type = "deliver", agent = item.agent, message = msg,
            key = "conv:" .. msg.channel .. ":" .. tostring(conv) })
  else
    af.op({ type = "deliver", agent = item.agent, message = msg, kind = "ephemeral",
            key = "req:" .. tostring(msg.payload.request_id) })
  end
end
```

未知的 `kind` 会被拒绝，消息不会被投递；临时投递也不能携带 `identity_group`。如果像上面的示例对聊天渠道那样，按 `msg.from` 为这类请求设置键，在使用共享密钥的渠道上就会出错：每个调用方的发送者都相同，于是他们会共用同一个会话。处理器取到一条消息后，如果在请求下一条之前没有投递它，这条消息就被丢弃了；正在等待该请求的渠道（OpenAI 兼容渠道）会立即返回 503。

路由处理器只能调用 `inbox`、`deliver`、`router.state.*`、`runtime.triggers` 和 `log`；其他调用都会按名称被拒绝。它不能发送渠道回复，也不能调用模型——它只负责路由，具体工作由会话循环完成。`router.state` 是持久的、部署范围的状态（每个键默认 TTL 为 30 天），用于存放诸如某个聊天的路由模式之类的信息。默认的 `per_chat` 处理器还实现了 `/group <uuid>` / `/personal` 切换，由引擎校验所提议的成员身份；如果你的产品需要不同的策略，可以整体替换该处理器，但引擎侧的校验不可省略。

---

## 延伸阅读

- [完整配置参考](configuration.md)——智能体、记忆、能力、触发器、MCP、渠道。
- [Lua 插件开发](loops-reference.md)——内置文档的 API 表和多模态矩阵。
- [`examples/loops/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/loops)——上面所有可运行的模式。
- [`agentflow.yaml`](https://github.com/hulipa487/agentflow/blob/main/agentflow.yaml)——示例部署（仓库在 gitignore 中忽略了该文件，因为它是本地实例的配置；纳入版本管理的示例片段位于 `examples/` 下）。
