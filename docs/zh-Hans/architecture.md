## 架构

一个请求自上而下穿过一个不深的分层栈。渠道驱动把平台字节转换成 `session.Message`。可选的身份层判断是谁发送的，路由器的 Lua 处理器选出一个会话键。监督器找到或创建会话参与者（actor），它运行一个 Luau 循环。循环通过异步桥调用由能力支撑的操作，驱动再去访问外部系统。安全过滤器、审计记录和来源信息都由核心掌管。它们就在这条路径上，因此循环无法移除或伪造它们。

```
┌─ Channels (bytes in / replies out) ─────────────────────────────────────────┐
│  webhook (sync | async)   ghhook (GitHub HMAC)   bridge (HMAC adapter)       │
│  telegram: polling | webhook | auto     qq: websocket gateway | webhook       │
│  studio (synthetic, playground only)                                          │
│  media: per-channel allow-list → blob store (fs | s3)   ── one shared httpd ─ │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ session.Message{id, from, text, channel, reply_to, attachments}
            ▼
┌─ Identity sink (runtime.identity.enabled; optional) ────────────────────────┐
│  handle → user:<uuid> · /link codes consumed here · unregistered allow|reject │
└───────────┬──────────────────────────────────────────────────────────────────┘
            ▼
┌─ Router (one Luau service state per process) ───────────────────────────────┐
│  audit journal (inbound) ──► gateway.route handler (default plugin:per_chat)  │
│  computes the session key, may propose a group; router.state.* is durable     │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ deliver{agent, key, message, identity_group?, kind?}
            ▼
┌─ Supervisor (session authority) ────────────────────────────────────────────┐
│  one actor per (agent, key), spawned on first message · provenance stamped    │
│  group membership validated · per-tenant session cap · can_contact on hops    │
│  sessionhub: lease + shared inbox route to the owning instance in a fleet     │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ one mailbox per session
            ▼
┌─ Actor (1 goroutine + 1 Luau state per session) ────────────────────────────┐
│  session.inbox() ──► safety ingress ──► loop() continues                      │
│  llm.* / memory.* / tools.run / shell.* / http.* / files.* / agent.* / …      │
│  __af_op(json) yields ── inline op, or worker pool (fair-share per tenant)    │
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ op handlers, gated per agent by `capabilities`
            ▼
┌─ Capabilities & Drivers ────────────────────────────────────────────────────┐
│  llm:    anthropic · openai · openai-responses · gemini · rerank (base_url)   │
│  memory: sqlite · redis · postgres · volatile · pgvector · qdrant · redisvector│
│  tools:  builtin:* · tools.http (declarative) · MCP (stdio | Streamable HTTP) │
│  shell:  gVisor jobs · ssh   mail: imap + smtp   files: fs | s3   net: netguard│
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ session.send / session.push / session.push_user
            ▼
┌─ Egress (actor-side, core-owned) ───────────────────────────────────────────┐
│  safety egress ──► audit journal (outbound) ──► gateway.Send ──► channel      │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

### 功能流程：一轮用户交互

```
user ─► [channel driver]
          │ normalize → session.Message{from, text, attachments}
          ▼
       identity sink (optional) ─► router: journal "routed" ─► gateway.route (Lua)
          │                                          (per_chat → session key)
          ▼
       supervisor.Deliver ◄──────────────────────────┘
          │ stamp provenance (kind, principal, user_uuid), enqueue mailbox
          ▼
       actor mailbox ◄──── scheduler.every / after / cron ── (session timers)
                     ◄──── triggers: every: / cron: ──────── (engine-fired messages)
          ▼
       session.inbox() returns ◄── safety ingress ran here (drop | skip | rewrite)
          │  ├─ memory.recall(...)            ── async bridge ──► store backend
          │  ├─ llm.chat(messages, {tools})   ── async bridge ──► llm driver
          │  ├─ tools.run(name, args)         ── async bridge ──► builtin / http / mcp
          │  └─ memory.write(record)          ── async bridge ──► store backend
          ▼
       session.send(reply)
          ▼
       safety egress ─► journal "delivered" ─► gateway.Send ─► channel ─► user
```

何时以及如何调用工具由循环决定。没有 `on_tool_call` 钩子：循环从 `llm.chat` 读取 `reply.tool_calls`，然后自己调用 `tools.run`（见 `examples/loops/tool_loop.lua`）。如果 `tools.run` 返回 `needs_confirm`，协程会挂起，由参与者向用户确认，之后再恢复。

---

### 多智能体编排

静态智能体（各有自己的模型和指令）在 `can_contact` 访问控制下，通过 `agent.send`/`agent.request` 协作；临时子智能体从 `profiles.agent` 模板派生，并收窄能力。参考编排器（main + expert + project manager + workers）是基于这些原语构建的常见模式 — 它不在这个运行时中，而是在一个单独的产品仓库里：

```
chit-chat:      user ──telegram/webhook──► main ──llm──► user

expert consult: user ──► main ──[directive: consult]──► expert_code (agent.request)
                                    ◄── opinion ── (agent.reply)
                          main ──llm (fold opinion)──► user

project launch: user ──► main ──[directive: start_project]──► agent.spawn("project_manager")
                          main ─► PM: brief{goal, channel, reply_to, chat_key}
                          PM ── decompose → store tasks ── spawn N workers
                          PM ──► worker: task      worker ──► PM: report
                          PM ──► main: {kind: milestone|decision|notify, ...}
                          main ──session.push(channel, reply_to, text)──► user  (idle-time push)
                          PM ── shutdown workers ── session.exit()
```

---

`session.push`（能力 `channel.push`）是主动出站 — 后台智能体无需当前有入站消息就能联系用户。`session.exit` 干净地终止一个派生的会话（父会话会收到 `agent.died`）。编排器的连接方式和各角色的提示词随参考产品仓库提供，不在这个运行时中。

### 各层职责

| 层 | 负责 | 说明 |
| --- | --- | --- |
| channels | 传输 | 把平台原始字节规范化为共享的 `session.Message`，并把回复送回去。`studio` 是为演练场注册的合成渠道。 |
| identity | 身份 | 可选。把渠道关联账号解析为 `user:<uuid>`，消费 `/link` 码，并在 `runtime.users.unregistered: reject` 时拒绝未注册的发送者。 |
| router | 路由 | 先把入站消息记入审计记录，然后在每个进程一个的服务状态中运行 `gateway.route` Lua 处理器，计算会话键（并可选地提议一个群组身份）。可热重载。 |
| supervisor | 权威 | 查找和派生会话、加盖来源信息、校验群组成员关系、按租户限制会话数、在智能体之间跳转时检查 `can_contact`、派生/请求/回复，以及通过会话中心在集群内移交。 |
| actor | 执行 | 每个会话一个 goroutine + 一个 Luau 状态。驱动循环，在 `__af_op` 上挂起，拿到结果后恢复，在 `session.inbox()` 处运行入站安全检查，在每次发送时运行出站安全检查。 |
| capabilities | 操作 | 面向 Lua 的表（`llm`、`memory`、`store`、`tools`、`shell`、`http`、`mail`、`files`、`agent`、`scheduler`、`session.state`、`user`、`credential`），都建立在唯一的异步桥原语之上。每个智能体只获得其 `capabilities` 授予的处理器。 |
| drivers | I/O | 访问真实系统：LLM 提供商、存储和向量后端、shell 提供方、MCP 服务器、搜索引擎、浏览器、邮件、S3。 |
| safety | 防护 | 由核心掌管的过滤链（`profiles.safety`、基于模型的 `safety.classifiers`），不论循环如何，都在参与者内部的入站和出站处运行。无法从 Lua 中卸载。 |
| admin plane | 操作员 | `/healthz`、`/readyz`、`/metrics`，以及 Studio 和浏览器控制台使用的 `/admin/api/*` JSON API，通过 TCP 和/或本地套接字或管道提供。 |

## 部署：两种层级

引擎要么作为单个进程运行，要么作为一个区域单元运行。层级需要显式设置（`runtime.topology`），它决定哪些后端可用。对于该层级无法兑现的设置，启动时会直接拒绝，而不是接受它，再悄悄为每个实例保留一份数据副本。未设置该键即为 `single`，因此在这个设置出现之前写的所有配置都能照常启动。

### `single` — 单个实例

`runtime.persistence: sqlite://./data/agentflow.db`（或者什么都不写）就能为你提供消息日志、文件存储的元数据、Token 账本、身份注册表、凭据存储、租约表、会话收件箱和 shell 句柄记录，每一项都是运行时数据库旁边的一个本地文件。无需安装任何东西，无需通过网络访问任何东西，而且引擎对单实例做出的每项保证都成立：审计记录完整，配额依据重启后仍保留的账本执行，定时任务会按时触发。

支持两种形态：

- **单数据库模式** — 全部使用 SQLite（包括向量，依靠 SQLite 的向量支持），或者使用带 pgvector 的 PostgreSQL。
- **拆分模式** — 对话状态使用 SQLite 或 PostgreSQL，向量使用 Qdrant 或 redisvector。

Blob 可以放在本地目录或 S3。Redis 和 volatile 记忆后端本来就是实例本地的（实例就是整个部署）。允许使用日志平面目录，它就是单个进程的日志。配合自托管模型（Ollama、vLLM、置于 OpenAI 兼容端点之后的 llama.cpp），这一层级完全不需要任何第三方服务。

`single` 部署不允许第二个运行中的进程使用同一个存储。引擎维护着一个实例注册表（运行时存储中每个进程一行心跳），第二个进程会在启动时停止，并给出对方的 ID、主机和最后在线时间。崩溃的对等进程也属于这种情况：等过 30 秒的存活窗口，或者用 `-force-single` 启动一次。

### `regional` — 一个区域内的多个实例

把 `runtime.persistence` 指向一个 PostgreSQL DSN，并设置层级：

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require

media: { backend: s3, s3: { bucket: agentflow-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-files, region: eu-west-1 } }
```

该层级的要求由 `agentflow validate` 强制执行，因此每个实例都会在同一处拒绝同样的错误配置：

| 设置 | 区域层级要求 | 原因 |
| --- | --- | --- |
| `runtime.persistence` | PostgreSQL | 一个所有实例都能看到的存储。 |
| `runtime.identity.persistence`、`runtime.credentials.path` | 不设置（跟随运行时存储），或使用 PostgreSQL。SQLite 会被拒绝 | 共享的身份注册表和凭据存储；两个实例必须把同一个关联账号解析为同一个用户。 |
| `runtime.cluster.persistence` | 已弃用。不设置，或与 `runtime.persistence` 指向同一目标 | 租约和会话收件箱现在总是位于运行时存储中；在区域单元中指向不同目标会被拒绝。 |
| `memory.backends` | 不能使用 `sqlite` 提供商 | 否则每个实例都会保留自己的一份对话历史。请使用 `postgres`、`pgvector`、`qdrant`、`redis` 或 `redisvector`。 |
| `media.backend`、`files.backend` | `s3` | 否则每个实例都会把自己的 blob 存在本地目录里。 |
| `runtime.log_plane.persistence` | 不设置 | 日志平面目录只属于一个进程；单元的消息日志必须共享。 |
| `ADMIN_TOKEN` 或 `runtime.admin.operators` | 至少设置一个 | 每次启动生成的管理令牌对其他对等实例都是未知的，所以启动时会拒绝生成。具名操作员可以完全取代共享令牌。 |

每个实例都在同一个运行时存储中注册，每 10 秒发送一次心跳，只要最近一次心跳在 30 秒之内就视为运行中。`/admin/api/state` 报告 `topology`、`zone` 以及每一行运行中的 `instances`，并标出 `config_epoch` 不一致的对等实例；Studio 的概览显示同一份列表。心跳只是存活标记，不是锁 — 谁拥有哪些工作仍以租约表为准。

### 多区域

没有全局层级。多区域部署就是每个区域一个独立的 `regional` 单元，各单元在运行时不共享任何东西。前端的应用把每个用户固定到一个归属区域，并把注册、邀请、凭据设置和流量都发往那里；每个区域是独立的故障域，热路径上没有跨区域延迟，数据也留在本区域内。共享的聊天集成（一个 Telegram 机器人只有一个接收方）属于这个前端入口，由它通过 `bridge` 渠道转发到用户所在的区域。管理和计费按区域分别进行，灾难恢复则是提升一个备用 PostgreSQL，以建立一个替代单元。完整的模式 — 包括 bridge、按区域计费和灾难恢复手册 — 见[多区域](multi-region.md)。

目标由协议和地址组成：服务器用 `postgres://…` 或 `postgresql://…`，本地文件用 `sqlite://<path>` 或直接写路径，日志平面则是一个目录 — `./data/log` 或 `file://./data/log`。所有目标都在启动时于同一处解析，所以引擎没有对应后端的协议会成为启动错误，并列出它支持的协议：拼写错误不会悄悄变成一个以它命名的目录。

### 日志平面

引擎的状态有三种形态，形态决定了它应该放在哪里：

- **在热路径上读取以做出决策** — 配额检查和预算刷新在每次调用前读取的 Token 汇总、文件存储元数据、会话状态、路由状态、shell 句柄。这些留在事务型存储中，在那里求和是精确的，并且对每个实例立即可见。
- **持续写入、很少读取、按时间过期** — 消息日志和每次调用的用量明细。这就是日志平面，也是部署可以放到别处的部分：`runtime.log_plane.persistence: ./data/log` 会把两者写成按行分隔的 JSON，每个 UTC 日一个文件，并按整天删除来清理。

存储之上的一切都不变：操作员查看的审计记录和方案页面上的每次调用明细，都来自日志平面所指向的位置，而 `usage_daily` — 这份汇总不能挪走 — 让每一次配额和预算决策都保持精确。在把它指向目录之前，有三个限制值得了解。机器崩溃可能丢失最后写入的记录，因为它们没有 fsync（消息日志在两个调用点本来就是尽力而为的，所以这并没有削弱任何已承诺的东西）。该目录只属于一个进程。而且存放在本地文件中的消息日志是*按实例*的，这恰恰把审计记录本该汇总的东西拆散了 — 区域单元把消息日志保存在共享存储中，校验器会拒绝在那里使用日志平面目录。保留期仍按平面分别设置：`audit.retention_days` 清理消息日志（无论它在哪里），`usage.retention_days` 清理用量明细，两者都会报告删除了什么。

### 多活能保证什么

| 保证 | 机制 | 值得了解的限制 |
| --- | --- | --- |
| 一个对话只在一处运行 | 一个 `session:<agent\|key>` 租约；拿不到租约的实例会把消息排队，而不是投递。对等实例拿到租约后，旧所有者的参与者会停止，定时器触发也像其他消息一样经由会话中心路由 | 被强制终止的持有者，其租约会在 TTL（默认 30s）内继续有效；正常关闭则会立即释放。会话空闲时，丢失租约会在 TTL 的三分之一时间内被发现；有消息等待时会立即发现 |
| 每条消息都能到达其会话 | 共享收件箱，由所有者取出，按入队时间排序 | 至少一次：在投递和确认之间崩溃会导致重新投递。同一实例重复提交同一消息 ID 只算一行 |
| 接管会带上积压的消息 | 未投递的行仍可被认领；新所有者会把它们取完。已投递的行一小时后清除，因此收件箱不会无限增长 | 进行中的一轮会丢失并被重新投递，所以循环可能会看到同一条消息两次 |
| 常驻智能体能在实例失效后存活 | 每隔半个会话 TTL 执行一次故障转移检查，向租约表询问每个持久智能体的 `boot` 会话是否有所有者；没有人拥有时，某个实例会认领并启动它 | 启动轮次会在新所有者上运行一次。常驻智能体的对话保存在共享记忆后端中，所以它会接着继续，而不是从空白开始 |
| 故障转移后会话保留其 shell | 没有什么需要保留：shell 命令是在共享文件存储中某个项目上运行的一次性沙箱作业，所以下一条命令会在当前拥有该会话的实例上运行 | 在失效实例上运行的作业会丢失，连同尚未写回的内容；后台作业随后会被报告为失败，而不会重新运行。SSH 句柄不会迁移：循环需要派生一个新的 |
| 定时触发器只触发一次 | 一个 `trigger:<name>` 租约，按触发器自身的间隔认领 | 被终止的持有者会让其触发器停滞，直到认领过期（一个间隔再加一点余量） |
| 维护任务每个单元只运行一次 | `files-gc`、`files-scratch`、`journal-prune`、`usage-prune`、`rows-sweep` 租约，各自持有该任务自身的间隔再加一点余量 | 运行时间超过租约的清理任务仍可能与另一个实例的重叠 |
| 正常关闭会先排空 | 就绪状态变为 false，然后经过 `runtime.shutdown_grace`（regional 为 5s，single 为 0）再关闭监听器；会话中心退出时会释放它的会话 | 崩溃会跳过排空；对等实例改为在租约 TTL 到期时恢复 |
| 预算在总体上成立 | 每日智能体预算读取账本，而不是自己的计数器（最多每 15s 刷新一次） | 超支上限取决于整个集群在一个刷新间隔内的花费，而不是实例数量。滚动窗口预算仍按实例计算 |
| 按用户配额成立 | 共享账本，加上进行中的预占额度：在区域单元中共享，在 `single` 中为进程本地 | 区域层级的预占每次模型调用需要一个事务；正是按用户的建议锁让两个实例的预留成为同一个决策 |

### 部署必须遵守的不变量

- **路由处理器把状态保存在存储中，而不是 Lua 中。** 路由器是每个进程一个 Luau 状态，由收到入站消息的那个实例运行，所以在 Lua 全局变量中记住任何东西的处理器，会让每个实例得出各自的答案，重启后也会忘掉。应该用 `router.state.*` 来存放：持久、部署范围内共享、每个实例上的值都相同，并且能在故障转移后保留。
- **要么显式设置 `ADMIN_TOKEN`，要么把每个实例都命名为操作员。** 在区域单元中，每次启动生成的令牌会被拒绝而不是生成，因为每个控制台都会拒绝其他实例的令牌。
- **消息 ID 按实例唯一，而不是按集群唯一。** 渠道驱动通常自行给消息编号（`wh-1`）。收件箱会用接收消息的实例加以限定，所以不会丢失任何东西 — 但仅按 ID 在消息日志中做审计查询可能匹配到两个实例的行。查询时请把 ID 当作按实例的键来处理。
- **比较配置纪元。** `/admin/api/state` 报告 `config_epoch`，即按原样写入的配置片段的短哈希，并标出纪元不一致的运行中实例。两个实例不一致，说明其中一个运行着不同的部署；轮换密钥不会改变它（它哈希的是写入的内容，而不是解析后的内容）。
- **在控制台中编辑配置只会影响一个实例。** 模型标签页会热应用到提供该控制台的进程；重载监视器覆盖循环、指令和提示词文件，不包括模型注册表。请持久化到配置文件，然后滚动更新集群。
- **租约和收件箱位于运行时存储中。** 它们必须共用一个目标：租约在一处而收件箱在另一处，会把会话路由给一个永远看不到其消息的所有者。正因如此，`runtime.cluster.persistence` 已被弃用。
- **每个实例都需要沙箱才能运行 shell 作业。** 作业在拥有该会话的实例上运行，所以每个实例都需要 `runsc` 和非特权用户命名空间，并在第一次使用某个环境时拉取它的镜像。项目和后台作业记录都在共享存储中；实例上的工作区目录只是它们的缓存。
- **每个项目每个实例同时只运行一个作业，而不是每个单元一个。** 当同一用户的两个会话落在不同实例上时，两个实例可能同时在同一个项目上运行命令。双方不会互相覆盖：两边都改动过的路径保留先存储的版本，后完成的作业会将其报告为冲突。
