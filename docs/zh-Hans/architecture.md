## 架构

一个请求沿着一条不长的链路往下走。渠道驱动把平台字节变成 `session.Message`；可选的身份层判定发送者是谁；路由器的脚本处理器算出会话键。supervisor 找到或创建会话 actor，它在一个 Goja VM 里跑一段 TypeScript/JS 循环。循环通过异步桥调用由能力支撑的操作（以及配置好的 Goja 或 `wazero` Wasm 外部 API），驱动再去够外部系统。安全过滤器、审计日志和来源标记由核心掌管，它们就长在链路上，循环既删不掉也伪造不了。

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
┌─ Router (one Goja service VM per process) ──────────────────────────────────┐
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
┌─ Actor (1 goroutine + 1 Goja VM per session) ───────────────────────────────┐
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
│  shell:  sandbox (gVisor · microsandbox) · ssh   mail: imap+smtp   files: fs│
└───────────┬──────────────────────────────────────────────────────────────────┘
            │ session.send / session.push / session.push_user
            ▼
┌─ Egress (actor-side, core-owned) ───────────────────────────────────────────┐
│  safety egress ──► audit journal (outbound) ──► gateway.Send ──► channel      │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

### 功能流程：一轮用户请求

```
user ─► [channel driver]
          │ normalize → session.Message{from, text, attachments}
          ▼
       identity sink (optional) ─► router: journal "routed" ─► gateway.route (script)
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

循环自己决定何时、如何调用工具。没有 `on_tool_call` 钩子：循环从 `llm.chat` 的 `reply.tool_calls` 里读，然后自己调 `tools.run`（见 `examples/loops/tool_loop.ts`）。`tools.run` 遇到 `needs_confirm` 会把脚本执行挂起，等 actor 向用户确认后再恢复。

---

### 多智能体编排

静态智能体（各自带模型和指令）通过 `agent.send`/`agent.request` 在 `can_contact` ACL 下协作；临时子智能体从 `profiles.agent` 模板派生，能力逐层收窄。参考实现的编排器（主控 + 专家 + 项目经理 + 工人）就是建在这些原语上的常见形态 —— 它不在本运行时里，而是一个独立的产品仓库：

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

`session.push`（能力 `channel.push`）是主动出站 —— 后台智能体无需有正在等待的入站消息也能触达用户。`session.exit` 干净地结束一个派生会话（父会话会收到 `agent.died`）。编排器的接线和各角色提示词随参考产品仓库提供，不在本运行时中。

### 各层职责

| 层 | 负责 | 说明 |
| --- | --- | --- |
| channels | 传输 | 把平台原始字节转成共享的 `session.Message`，再把回复送回去。`studio` 是为演练场注册的合成渠道。 |
| identity | 身份 | 可选。把渠道账号解析成 `user:<uuid>`，消费 `/link` 码；当 `runtime.users.unregistered: reject` 时拒绝未注册的发送者。 |
| router | 路由 | 先记录入站，再在每个进程一个 Goja 服务 VM 里运行 `gateway.route` 脚本处理器，算出会话键（可选地提议群组身份）。可热重载。 |
| supervisor | 权威 | 会话查找与派生、来源标记、群组成员校验、按租户的会话上限、智能体跳转时的 `can_contact`、派生/请求/回复，以及通过会话中枢完成的集群交接。 |
| actor | 执行 | 每个会话一个 goroutine 加一个 Goja VM。驱动循环，在 `__af_op` 上挂起、拿到结果后恢复，在 `session.inbox()` 处跑安全入站，在每次发送时跑安全出站。 |
| capabilities | 操作 | 面向脚本的 API（`llm`、`memory`、`store`、`tools`、`api`、`shell`、`http`、`mail`、`files`、`agent`、`scheduler`、`session.state`、`user`、`credential`），都架在同一个异步桥原语上。每个智能体只拿到 `capabilities` 授予的处理器。 |
| drivers | I/O | 够到真实系统：LLM 提供商、存储与向量后端、Shell 提供方、MCP 服务器、`wazero` Wasm 外部 API 模块、搜索引擎、浏览器、邮件、S3。 |
| safety | 防护 | 由核心掌管的过滤链（`profiles.safety`、基于模型的 `safety.classifiers`、`safety.script_filters`），在 actor 内部的入站和出站处运行，与循环怎么写无关。循环脚本卸不掉它。 |
| admin plane | 运维 | `/healthz`、`/readyz`、`/metrics`，以及供 Studio 和浏览器控制台使用的 `/admin/api/*` JSON API，走 TCP 和/或本地套接字、命名管道。 |

## 部署：两个层级

引擎要么作为单进程运行，要么作为一个区域单元。层级是显式的 —— `runtime.topology` —— 由它决定哪些后端合法。启动时会拒绝该层级无法兑现的设置，而不是先接受、再悄悄留一份实例本地的数据副本。未设置时按 `single` 处理，所以在这个设置出现之前写的配置都能原样启动。

### `single` —— 单实例

`runtime.persistence: sqlite://./data/agentflow.db`（或者干脆不写）就把审计日志、文件存储元数据、Token 台账、身份注册表、凭据存储、租约表、会话收件箱和 Shell 句柄记录都放在运行时数据库旁边的本地文件里。不用装东西，不用走网络，引擎对单实例承诺的每一条都成立：审计链完整、配额由重启后依然存在的台账强制执行、定时任务照常触发。

支持两种形态：

- **单库模式** —— 全部用 SQLite（向量也在里面，来自 SQLite 的向量支持），或用 PostgreSQL 加 pgvector。
- **拆分模式** —— 对话状态用 SQLite 或 PostgreSQL，向量另用 Qdrant 或 redisvector。

blob 可以是本地目录或 S3。Redis 和易失记忆后端按定义就是实例本地的（实例就是部署）。允许用日志平面目录，它恰好只属于一个进程。配上自托管模型（Ollama、vLLM、llama.cpp 挂 OpenAI 兼容端点），这个层级完全不需要第三方服务。

`single` 部署会拒绝同一个存储上的第二个活跃进程。引擎维护一份实例注册表 —— 运行时存储里每个进程一行心跳 —— 第二个进程启动时就会带着对端的 id、主机和最后活跃时间停下。对端崩溃的情况也一样：等过 30 秒的存活窗口，或者用 `-force-single` 启动一次。

### `regional` —— 一个区域内的多个实例

把 `runtime.persistence` 指向 PostgreSQL DSN，并设置层级：

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require

media: { backend: s3, s3: { bucket: agentflow-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-files, region: eu-west-1 } }
```

这一层级的硬性要求由 `agentflow validate` 强制执行，所以每个实例都会在同一点拒绝同样的错误配置：

| 设置 | 区域部署的要求 | 原因 |
| --- | --- | --- |
| `runtime.persistence` | PostgreSQL | 需要一个所有实例都看得见的存储。 |
| `runtime.identity.persistence`、`runtime.credentials.path` | 不设置（跟随运行时存储），或使用 PostgreSQL。SQLite 会被拒绝 | 共享的身份注册表和凭据存储；两个实例必须把同一个账号解析成同一个用户。 |
| `runtime.cluster.persistence` | 已弃用。不设置，或与 `runtime.persistence` 指向同一目标 | 租约和会话收件箱现在总是在运行时存储中；区域单元里指向不同目标会被拒绝。 |
| `memory.backends` | 不能用 `sqlite` 提供商 | 否则每个实例都会留一份自己的对话历史。请用 `postgres`、`pgvector`、`qdrant`、`redis` 或 `redisvector`。 |
| `media.backend`、`files.backend` | `s3` | 否则每个实例都会在本地目录留一份自己的 blob。 |
| `runtime.log_plane.persistence` | 不设置 | 日志平面目录只属于一个进程，单元的审计日志必须共享。 |
| `ADMIN_TOKEN` 或 `runtime.admin.operators` | 至少设置一个 | 每次启动生成的令牌对等实例一无所知，所以启动会拒绝生成。具名操作员可以完全取代共享令牌。 |

每个实例都注册到同一个运行时存储，每 10 秒一次心跳，最后心跳在 30 秒内就算存活。`/admin/api/state` 会报告 `topology`、`zone` 和每个活跃的 `instances` 行，并对 `config_epoch` 不一致的对端打上标记；Studio 的概览页显示同样的列表。心跳只是存活标记，不是锁 —— 谁拥有工作仍然由租约表说了算。

### 多区域

没有全局层。多区域部署就是每个区域一个独立的 `regional` 单元，单元之间运行时什么都不共享。前面的应用把每个用户固定到一个归属区域，注册、邀请、凭据设置和流量都发往那里；每个区域是独立的故障域，热路径上没有跨区域延迟，数据也留在本区域。共享的聊天集成（一个 Telegram 机器人只有一个接收方）属于那个入口服务，由它通过 `bridge` 渠道转发到用户所在区域。管理和计费按区域扇出，灾难恢复则是提升一个备用 PostgreSQL 来起一个替代单元。完整做法 —— 包括 bridge、按区域计费和灾备手册 —— 见[多区域](multi-region.md)。

目标是「协议 + 地址」：服务器用 `postgres://…` 或 `postgresql://…`，本地文件用 `sqlite://<path>` 或裸路径，日志平面则用目录 —— `./data/log` 或 `file://./data/log`。所有目标都在启动时于同一处解析，所以引擎没有后端的协议会直接报启动错误，并列出它支持的协议：拼错也不会悄悄变成一个同名的目录。

### 日志平面

引擎的状态有三种形态，形态决定它该放哪儿：

- **热路径上需要读来做决策** —— 配额检查和预算刷新在每次调用前都要读的 Token 汇总、文件存储元数据、会话状态、路由状态、Shell 句柄。这些留在事务存储里，求和精确，并且对所有实例立即可见。
- **写得勤、读得少、按时间过期** —— 消息日志和逐次调用的用量明细。这就是日志平面，也是部署可以换个地方放的部分：`runtime.log_plane.persistence: ./data/log` 把两者都写成换行分隔的 JSON，每个 UTC 日一个文件，并按整天删除来清理。
存储之上的东西一律不变：操作员查看的审计链和资料页上的逐次明细，都来自平面所指的位置；而 `usage_daily` —— 这份不能挪走的汇总 —— 让每一次配额和预算决策都保持精确。在把它指向目录之前，有三条限制值得知道。机器崩溃可能丢掉最后写入的记录，因为它们没有 fsync（消息日志在两个调用点本来就是尽力而为，所以这并没有削弱任何已承诺的东西）。这个目录只属于一个进程。而放在本地文件上的消息日志是*按实例*的，这恰好把审计链本该汇总的东西拆散了 —— 区域单元把消息日志留在共享存储里，校验器也会拒绝在那里用日志平面目录。保留期仍按平面分别设置：`audit.retention_days` 清理消息日志（无论它在哪儿），`usage.retention_days` 清理用量明细，各自报告删了什么。

### 多活保证了什么

| 保证 | 机制 | 值得知道的限制 |
| --- | --- | --- |
| 一个对话只在一处运行 | 一个 `session:<agent\|key>` 租约；拿不到租约的实例会把消息排队，而不是投递。对等实例拿到租约后，旧所有者的 actor 会停止，定时器触发也像其他消息一样经会话中枢路由 | 被硬杀的持有者，其租约在 TTL（默认 30s）内存活；正常关闭则立即释放。会话空闲时，租约丢失最多在 TTL 三分之一内被发现；有消息在等则立即发现 |
| 每条消息都能到达它的会话 | 共享收件箱，由所有者排空，按入队时间排序 | 至少一次：投递与确认之间崩溃会重投。同一实例重发同一条消息 ID 只算一行 |
| 接管会带走积压 | 未投递的行仍可认领，新所有者会排空它们。已投递的行一小时后清扫，收件箱不会无限增长 | 跑到一半的轮次会丢失并重新投递，所以循环可能看到同一条消息两次 |
| 常驻智能体能熬过实例故障 | 每半个会话 TTL 跑一次故障转移检查，向租约表询问每个常驻智能体的 `boot` 会话归谁所有；无人拥有时，由某个实例认领并拉起 | 启动轮只在新所有者上跑一次。常驻智能体的对话存在共享记忆后端里，所以是接着聊而不是从空白开始 |
| 会话在故障转移后仍保有它的 Shell | 没什么要保的：Shell 命令是共享文件存储里某个项目上的一次性沙箱作业，下一条命令在哪台实例上跑都行 | 在故障实例上运行的作业会丢，连同尚未写回的内容；后台作业随后会报为失败而不是重跑。SSH 句柄不会迁移：循环需要重新派生一个 |
| 定时触发器只触发一次 | 一个 `trigger:<name>` 租约，按触发器自身的间隔认领 | 被杀的持有者会让触发器卡住，直到认领过期（一个间隔再加点余量） |
| 清理任务每个单元只跑一次 | `files-gc`、`files-scratch`、`journal-prune`、`usage-prune`、`rows-sweep` 各自的租约，持有时间为作业自身间隔加余量 | 跑超时的清扫仍可能和另一个实例重叠 |
| 正常关机会排空 | 就绪探针先转 false，再过 `runtime.shutdown_grace`（区域 5s，单实例 0）才关闭监听器；会话中枢退出时释放会话 | 崩溃不排空；对等实例改在租约 TTL 时接管 |
| 预算在总体上成立 | 每日智能体预算读台账而不是自己的计数器（最多每 15s 刷新一次） | 超支幅度取决于整个集群在一个刷新间隔内的花费，与实例数无关。滚动窗口预算仍是按实例的 |
| 按用户配额成立 | 共享台账，加上在途预占 —— 区域单元中共享，`single` 中进程本地 | 区域部署里每次模型调用要一次事务做预占；真正让两个实例的预留变成同一个决策的，是按用户建议锁 |

### 部署必须守住的几条不变量

- **路由处理器的状态放存储里，别放脚本全局。** 路由器是每进程一个 Goja VM，哪个实例收到入站就由它运行 —— 所以在模块全局里记东西的处理器，会让每个实例给出各自的答案，重启还会忘光。该放 `router.state.*`：持久、全部署一致、每个实例同一个值，而且熬得过故障转移。
- **要么显式设置 `ADMIN_TOKEN`，要么给每个实例都取操作员名。** 区域单元里会拒绝生成每次启动的令牌，因为各控制台都会拒绝别人的令牌。
- **消息 ID 按实例唯一，不是按集群唯一。** 渠道驱动常给自己的消息编号（`wh-1`）。收件箱会用接收它的实例来限定，所以不会丢东西 —— 但只按 ID 在消息日志里做审计查询，可能匹配到两个实例的行。查询时把 ID 当作按实例的键。
- **比一比配置世代。** `/admin/api/state` 会报告 `config_epoch` —— 按写入原样算出的配置片段短哈希 —— 并标记世代不一致的活跃实例。两个实例不一致，说明有一个跑的是别的部署；轮换密钥不会改变它（哈希的是写下的内容，不是解析后的值）。
- **控制台的配置修改只会到达一个实例。** 模型标签页热应用到服务该控制台的进程；重载监听器只覆盖循环、指令和提示词文件，不覆盖模型注册表。要保存到配置文件，再滚动更新整个集群。
- **租约和收件箱都在运行时存储里。** 它们必须共用一个目标：租约在一处、收件箱在另一处，会把会话路由给一个永远看不到消息的所有者。`runtime.cluster.persistence` 正因此被弃用。
- **每个实例都需要沙箱才能跑 Shell 作业。** 作业在拥有该会话的实例上运行，所以每个实例都要配好沙箱运行时（`gvisor` 用 `runsc` 加非特权用户命名空间，`microsandbox` 用可访问 hypervisor 的 `msb`）及其环境镜像。项目和后台作业记录在共享存储里；实例上的工作目录只是它们的缓存。
- **每个项目在每个实例上同时只跑一个作业，不是每个单元一个。** 同一用户的两个会话落在不同实例时，两个实例可能同时在同一个项目上跑命令。两边不会互相覆盖：两边都改过的路径保留先存的版本，后完成的作业会把它报为冲突。
