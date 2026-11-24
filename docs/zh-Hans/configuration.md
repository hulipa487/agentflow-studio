## 配置指南

空部署也是有效的：`agents:` 可以整段省略，此时运行时会作为管理／Studio 控制平面启动，方便从 Studio 创建第一个智能体。不过渠道、触发器和其他引用仍然必须指向已存在的智能体。

AgentFlow 用单个 YAML 文件（`agentflow.yaml`）配置。所有值都在启动时校验 —— 未知的键就是启动错误，并给出文件和行号，绝不会拖到运行时才爆出来。凡是接受字符串的地方都支持环境变量插值（`${VAR}` 和 `${VAR:-default}`）。

### 密钥：环境变量引用、凭据存储与如实降级

含密钥的字段（`models.*.api_key`、`search.engines.*.api_key`、`gateway.channels[].token`/`secret`、`memory.backends.*.config.url`/`password`/`api_key`、`media.s3.access_key`/`secret_key`、`browser.api_token`、`profiles.shell.*.password`）接受三种写法：

- 字面值；
- `${VAR}`（或 `${VAR:-default}`）—— 从进程环境解析，找不到就回退到凭据存储里同名的那条；
- `cred:<service>` —— 只从凭据存储解析。

在 **configdir** 路径下，这些引用保持惰性，等使用方构造时才解析；在单文件 `-config` 路径下，整个文档在加载时按字节展开（旧行为，未改）。解析顺序是：环境变量 → 凭据存储 → 解析不出来。降级策略是统一的：*可选*组件（搜索引擎、telegram/ghhook 渠道、记忆后端、S3 媒体存储）上的凭据解析不出来，就**跳过该组件并打一条点名凭据的警告** —— 绝不会导致启动失败。模型的 `api_key` *不是*可选的：模型仍保持配置状态，第一次 LLM 调用会失败，错误里点名那个没解析出来的凭据。`runtime.credentials.enabled=false`（或没有凭据存储）会让 `cred:` 引用解析不出来，同样适用这套策略。

### 文件骨架

```
version: "1"

runtime:      { ... }   # 实例级：持久化、管理、集群、日志平面、
                        #   身份、凭据、用户、区域、timezone_offset_hours
models:       { ... }   # 具名的 LLM 提供商
memory:       { ... }   # 实例级后端实例
profiles:     { ... }   # 可复用配置：memory/、safety/、shell/、agent/
gateway:      { ... }   # 一个共享监听器及其渠道
mcp:          { ... }   # MCP 服务器
tools:        { ... }   # 全局工具策略
search:       { ... }   # 网页搜索引擎
legal_search: { ... }   # HKLII / NPC
browser:      { ... }   # Cloudflare Browser Run
net:          { ... }   # 出站连接策略
media:        { ... }   # blob 存储后端：fs（默认）| s3
files:        { ... }   # 用户范围的文件存储：fs（默认）| s3，临时文件 TTL
audit:        { ... }   # 消息日志：enabled、retention_days
usage:        { ... }   # 按用户的 Token 账本：配额、事件、保留期
triggers:     [ ... ]   # 由引擎触发的定时 / 事件任务
prompts:      { ... }   # 部署级提示词注册表
agents:       { ... }   # 智能体本身
plugins:      { ... }   # 插件目录、能力上限
```

### 设计理念

YAML 定义*拓扑* —— 有什么、怎么连。行为放在 TypeScript/JS 插件和 Wasm 外部 API 里，参数放在 YAML 里。这种分离意味着一个能用的聊天机器人只需约 12 行 YAML、零代码，同时流水线上每一处都仍可覆盖。

> **严格解析**：`KnownFields(true)` 意味着每个未知键都是启动错误。不存在「被忽略」的拼写错误。

### 配置目录（`-config <dir>`）

除了单文件，也可以给 `-config <dir>` 传一个目录来加载目录布局；只放了一个 `agentflow.yaml` 的目录（也就是 `agentflow init DIR` 写出来的）会加载那个文件。每个片段都按同样严格的方式解码，校验则在合并*之后*才跑，所以跨文件引用的检查方式和单文件完全一样。配置来源 —— 文件所在目录，或目录本身 —— 是所有相对路径的解析基准（循环、指令、提示词 `file:` 路径、路由器的路由、持久化、sqlite 后端路径、Shell 的 `key_file`、`plugins.dir`、`media.dir`）。单文件也一样：`agentflow run -config deploy/agentflow.yaml` 会在那个文件旁边解析 `./loops/x.ts`，与 Shell 的工作目录无关。

```
<dir>/
  system.yaml       # 必需：除 agents/channels 之外的顶层配置
  channels.yaml     # 可选：channels: [...]
  profiles/*.yaml   # 可选：每个文件一个智能体（按 glob 排序）
  triggers/*.yaml   # 可选：triggers: [...]（排序后合并，后面的文件优先）
```

`profiles/*.yaml` 是一份智能体配置，外加 `name` 和 `spawn` 标志：`spawn: false`（或不写）归到 `agents`，`spawn: true` 归到 `profiles.agent`（一个派生模板，包含它的 `extras` 和 `credentials` 白名单 —— 派生出的子会话两者都能看到）。在派生方案上写只属于智能体的字段（`persistent`、`singleton`、`taps`、`channels`、`lifecycle`）是启动错误。触发器的形状是 `{name, event{channel, match} | cron | every, run_on_boot, target{profile}, payload}` —— `every:` 和 `cron:` 触发器*由引擎*执行（见下文 `triggers`），合并后的列表通过 `runtime.triggers()` 作为数据提供给循环。目录部署会轮询重读 `triggers/*.yaml`，所以改排期无需重启即可生效。

## 完整配置参考

### `runtime` —— 实例级调优

| 键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `vm.memory_limit` | string | — | 纯 Go JS/TS VM（`goja`）每个状态的软内存上限（`b`/`kb`/`mb`/`gb`/`MiB`/`GiB`；留空 = 不设限）。限制超大字符串和集合的分配，超限时表现为脚本内存不足错误 |
| `scheduler.workers` | int | 8 | 操作池大小。这是唯一能设置它的地方 |
| `persistence` | string | sqlite://./data/agentflow.db | 消息日志、Token 台账及其逐次明细，以及引擎的键值行（文件树、会话状态、路由状态、Shell 句柄）存放的位置。调度器计时器*不*在这里：计时服务在内存里，所以重启会忘掉所有计时器，直到循环重新注册 |
| `topology` | string | single | 部署层级：`single`（单进程）或 `regional`（一个区域内多个实例共用一个 PostgreSQL）。不设置即 `single`。层级决定其他哪些设置合法；启动时会拒绝它兑现不了的设置。见[拓扑](#topology) |
| `zone` | string | "" | 区域内的可用区标签（字母、数字和 `. _ -`，最多 32 个字符）。会出现在指标和 `/admin/api/state` 里；它只是标签，绝不参与路由决策 |
| `shutdown_grace` | duration | 区域 5s · 单实例 0 | 关闭时先报告未就绪、继续服务多久才关闭监听器，好让负载均衡器排空、让在途的轮次跑完。`0` 表示不等待 |
| `sessions.idle_unload` | duration | 30m | 会话闲置这么久后卸载。它的 VM 局部变量随之消失；`session.state.*` 仍在，下一条消息会重建会话。会话 ID 不回收，租约释放。`0` 关闭卸载 |
| `admin.cors_origins` | list | \[\] | 允许调用 `/admin/api/*` 的精确浏览器来源（自建前端或别的浏览器客户端）。Studio 不需要它：桌面应用和网页应用的本地代理都是同源调用该 API。留空表示只允许同源；`"*"` 会被拒绝。预检在 Bearer 校验之前应答 |
| `admin.socket` | string | "" | 增加一个仅本地的管理监听：Unix 上是 unix socket 路径（相对路径按配置目录解析），Windows 上是 `\.\pipe\agentflow-admin` 这类命名管道。仍需 Bearer 令牌 |
| `admin.tcp` | bool | true | 在 HTTP 监听器（`gateway.listen`）上提供管理面，挂 `/admin/` 以及 `/healthz`、`/readyz`、`/metrics`。`false` 则只在 `admin.socket` 上提供；两种传输至少要留一个。不存在 `admin.listen`：管理面没有自己的端口，设置了会被拒绝 |
| `admin.operators` | map | {} | 具名管理令牌（`name: token`），每个的权限与 `ADMIN_TOKEN` 相同。请求在日志里归属到对应名字。值可以是 `${VAR}` 或 `cred:<service>` 引用，启动时解析；解析不出来的会被跳过并给出警告 |
| `limits.sessions_per_tenant` | int | 0 | 在 supervisor 的派生边界（路由首触和 `agent.spawn`）限制每个租户的活跃会话数。0 表示不限 |
| `limits.ephemeral_per_tenant` | int | 16 | 限制每个用户的并发单轮会话数（OpenAI 渠道的默认会话类型）。与 `sessions_per_tenant` 分开，所以无状态流量挤不掉对话名额。它需要有用户才能计数，因此没有用户的请求（`auth: key` 或 `none` 的 `openai` 渠道）不受它限制。0 表示不限 |
| `limits.egress_per_minute` | int | 0 | 限制每个租户每分钟的 `http.request` 和邮件操作次数（固定窗口）。0 表示不限。租户之间的操作池公平性始终开启，不可配置 |
| `studio.enabled` | bool | true | 提供 `/admin/api/studio/*`（构建器、演练场、追踪）。没有管理令牌时该 API 无实际作用；`false` 返回 503 |
| `studio.workspace` | string | <config dir>/studio | 单文件部署把 Studio 的循环和提示词写在这里。目录部署则写自己的 `loops/`、`prompts/` 和 `profiles/` |
| `studio.trace_events` | int | 2000 | 每个会话为追踪时间线保留的事件数（有界环形缓冲，最旧的先丢） |
| `studio.trace_sessions` | int | 200 | 保留多少个会话的追踪环；超过就淘汰最久未活动的 |
| `studio.trace_payloads` | bool | false | 在追踪事件里带上消息、回复和结果文本。元数据（操作、模型、耗时、Token、错误）始终保留 |
| `studio.chat_timeout` | duration | 120s | 演练场一轮最多等多久，超时则带着部分回复和 `timed_out` 返回 |
| `identity.enabled` | bool | false | 为每个入站生成稳定的用户 UUID；把 `from` 改写为 `user:<uuid>`，`session.push_user` 也随之可用。开启会重新划定现有状态的范围 —— 见表格下方的说明 |
| `identity.persistence` | string | <persistence dir>/identity.db | 身份注册表的 SQLite 路径或 PostgreSQL DSN（默认跟随 `runtime.persistence`） |
| `identity.require_registration` | boolean/true | true | 让未关联的账号留在共享 service 层（没有个人范围，没有配额身份），直到它关联到用户资料。`false` 则在首次接触时自动认领一个资料 |
| `identity.unregistered` | string | allow | 没有资料的发送者怎么处理：`allow` 继续在共享层里服务；`reject` 丢弃入站并要求其注册或关联 |
| `credentials.enabled` | bool | false | 加密的按租户凭据存储，支撑 `auth={service=...}` |
| `credentials.persistence` | string | <persistence dir>/credentials.db | 凭据存储的 SQLite 路径或 PostgreSQL DSN（默认跟随 `runtime.persistence`） |
| `credentials.master_key_env` | string | CREDENTIALS\_MASTER\_KEY | 存放主密钥的环境变量名（启动时读取，绝不从文件读） |
| `credentials.engine_wide` | string | allow | 门控 `credential.get()` —— 把原始密钥交给脚本的那个操作。`allow`（或留空）允许智能体读取其 `credentials:` 列表点名的内容；`maintenance` 则限制为引擎触发的轮次（启动、调度器、触发器），租户轮次因此永远看不到全局密钥。按租户的路径（`http.request` 和 `tools.http` 上的 `auth = {service = ...}`）从不返回值 |
| `users.enabled` | bool | false | 在网关监听器上提供公开的 `/v1/users` API（需要 `identity.enabled`） |
| `users.registration` | string | open | `open` 或 `invite`（一次性邀请码来自 `POST /admin/api/users/invites`） |
| `users.link_ttl` | duration | 10m | 渠道关联码的有效期 |
| `users.push_same_tenant` | bool | false | 让 `session.push_user` 拒绝当前轮次租户以外的目标。多租户聊天产品请开启 |
| `users.jit_provisioning` | bool | true | OIDC 首次登录验证通过时创建资料；`false` 则拒绝未知主体，直到操作员手动开通 |
| `users.oidc.issuer` · `audience` · `jwks_url` | string | — | 身份提供商的校验配置（见下文*身份提供商*） |
| `users.oidc.subject_claim` · `email_claim` · `name_claim` | string | sub · email · name | 承载身份和开通资料字段的 Token 声明 |
| `cluster.session_ttl` | duration | 30s | 会话认领在不续约的情况下能存活多久 |
| `cluster.poll_interval` | duration | 250ms | 实例多久查一次等待它拥有的会话的消息 —— 这就是消息落到错误实例上时多出的延迟 |
| `region` | string | "" | 给本实例所服务的区域命名：一个指标标签 `region`、`/admin/api/state` 里的一个字段，以及实例注册表的一部分。区域是一个独立的单元，而不是全局部署的分片 |
| `log_plane.persistence` | string | "" | 仅追加平面的目录 —— 消息日志和逐次用量明细 —— 每个 UTC 日写一个 JSONL 文件。留空则两者都留在运行时存储里 |
| `timezone_offset_hours` | float | 0 | cron 触发器按哪个 UTC 偏移匹配（例如 `8`、`-5.5`）；`every:` 不受影响 |

> **打开身份层会重新划定一切的范围**：`identity.enabled` 不是一个装饰性开关。开启后，每个入站的 `from` 都会从渠道原生发送者改写为稳定的 `user:<identity>`。个人范围要往后才来：一旦某个账号关联到资料 —— 或在首次接触时认领一个，那只在 `runtime.users.require_registration: false` 时发生 —— 引擎会在消息由核心掌管的 `provenance` 上盖 `user_uuid`（循环读的就是这个字段；它绝不会写进 payload，因为委派跳转会从脚本重建 payload），用户范围的各个子系统就按这个戳来解析。文件存储的范围随之从 `agent:<agent>` 变成 `user:<uuid>`，一轮的记忆从共享 service 层转到交互层，按用户的配额也有了可扣的账户。在关联之前，它完全不带个人范围：变的只有 `from`。循环里任何以 `from` 为键、或自己拼出范围的东西，都会跟着一起变。
>
>  **什么都不会自动迁移，引擎也判断不出什么挪了位置。** 在旧头部下写的行 —— 文件、记忆、循环自己分键的任何东西 —— 不会被认领，因为没法知道昨天的 `agent:bot` 就属于现在这个 `user:u_xxxx`。所以要么在有真实流量之前就打开这一层，要么自己写迁移：一个把旧头部的行复制到新头部的循环，在切换时跑一次。反过来也一样 —— 关掉它，或解除某个账号的关联，都会把个人范围留在原地。

管理服务器提供 `GET /healthz`、`GET /readyz`、`GET /metrics`（Prometheus 文本格式）、`GET /admin/sessions` 和 `/admin/credentials`。（会话列表以前是 `/v1/sessions`：那个前缀属于公开的用户 API，而这个私有监听器并不提供它。）实时计数器包括 `agentflow_sessions_active`、`agentflow_children_spawned`/`died`、`agentflow_llm_calls`、`agentflow_llm_tokens`、`agentflow_budget_denied` 和 `agentflow_trigger_fires`。

### 拓扑

`runtime.topology` 决定部署形态，而形态决定哪些设置合法。它是一个设置而不是十几个，是因为另一种做法会得到「配了一半、看着却健康」的部署：区域集群配本地 blob 目录，每个实例各留一份数据，还不报错。启动会拒绝这类组合，`agentflow validate` 会在启动前就拒绝。

#### `single`

默认值，也是这个设置出现之前写的所有配置的层级。单进程；以前合法的现在都合法：

| 后端 | `single` |
| --- | --- |
| 运行时、身份、凭据、租约、收件箱 | SQLite 或 PostgreSQL |
| 记忆：`sqlite`、`postgres`、`pgvector`、`qdrant`、`redis`、`redisvector`、`volatile` | 全部允许 |
| Blob（`media`、`files`） | 本地目录或 S3 |
| `runtime.log_plane.persistence` | 允许 |

同一存储上的第二个活跃进程会在启动时被拒绝，并给出对端的 id 和最后活跃时间。`-force-single` 可以强行启动；只在崩溃之后用，或者等过 30 秒的存活窗口。

单实例不安装会话中枢：没有第二个所有者可以接管对话，所以既不写 `session:` 租约行，也不写共享收件箱行。`-force-single` 会保留中枢 —— 用它意味着操作员声明两个进程可能共用一个存储，那么租约就是防止同一个对话跑两遍的唯一手段。

#### `regional`

一个区域内跨可用区的多个实例，共用一个 PostgreSQL。启动会拒绝：

| 设置 | 拒绝原因 |
| --- | --- |
| `runtime.persistence` 不是 PostgreSQL | 否则每个实例都会留自己的数据库 |
| `runtime.identity.persistence` / `runtime.credentials.path` 指向 SQLite | 否则每个实例都会留自己的身份或凭据。取消设置以跟随运行时存储，或指向 PostgreSQL |
| `runtime.cluster.persistence` 指向不同目标 | 租约和收件箱总是在运行时存储里 |
| 任何 `memory.backends` 条目使用 `sqlite` 提供商 | 否则每个实例都会留自己的对话历史 |
| `media.backend` 或 `files.backend` 不是 `s3` | 否则每个实例都会留自己的 blob |
| 设置了 `runtime.log_plane.persistence` | 本地目录只属于一个进程 |
| `ADMIN_TOKEN` 未设置且 `runtime.admin.operators` 为空 | 每次启动生成的令牌对每个对端都不认识，所以一个都不生成（启动时检查，因为它是环境设置） |

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require
  admin:
    operators: { alice: ${ALICE_ADMIN_TOKEN} }

media: { backend: s3, s3: { bucket: agentflow-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-files, region: eu-west-1 } }
```

Redis 是一个覆盖整个区域的集群（locality 为 `region`）；`volatile` 仍为实例本地。每个实例每 10 秒往 `instances` 表写一行心跳，最后心跳在 30 秒内就算存活；`/admin/api/state` 会报告 `topology`、`zone` 和每个活跃的 `instances` 条目，并对 `config_epoch` 不一致的对端打标记。Studio 的概览页显示同样的列表。

多区域部署就是每个区域一个独立的 `regional` 单元 —— 没有全局层。见[多区域](multi-region.md)。

`agentflow doctor -online`（区域配置的默认行为）检查运行侧：打开运行时存储、确认每个 pgvector 后端都有 `vector` 扩展、在每个 S3 桶里 put/get/delete 一个探针对象，并 ping 一下 Redis 和 Qdrant。每行都是 PASS/FAIL，附带修复方法。

### 命令行

`agentflow help` 列出命令，`agentflow help <command>`（或 `<command> -h`）打印某个命令的参数。这份帮助是从二进制自己的参数定义生成的，所以永远是最新参考。

`agentflow run [flags]`，或直接 `agentflow [flags]` 不带命令，就是运行引擎：

| 参数 | 默认值 | 含义 |
|---|---|---|
| `-config PATH` | `agentflow.yaml`，然后 `config.yaml` | 一个文件、一个配置目录，或一个含有 `agentflow.yaml` 的目录 |
| `-log-level` | `info` | `dev`、`debug`、`info`、`warn` 或 `error` |
| `-log-format` | `text` | `text` 或 `json` |
| `-log-file PATH` | | 同时把日志追加到这个文件 |
| `-force-single` | 关闭 | 即使存储上看起来还有别的实例在跑也启动（仅 `single`；崩溃之后用） |

所有命令遵循同样的约定：

- 参数可以放在位置参数之后（`agentflow cred set NAME -user u_1`），`-config` 也可以放在命令之前（`agentflow -config deploy/ validate`）。
- 命令作用的对象是位置参数：`init [DIR]`、`user export USER`、`user import BUNDLE.zip`、`user keygen [PREFIX]`、`migrate blobs DIR`、`cred set NAME`。
- `-out FILE` 写进文件；不给就输出到 stdout。唯一的例外是 `support-bundle`，它默认写一个带时间戳的 zip。
- `-json` 在 `doctor`、`config show`、`cred get` 和 `cred list` 上输出机器可读结果。
- 命令不适用的参数是错误，绝不静默忽略。
- `user export` 和 `user import` 找管理 API 的顺序是 `-url` → `$AGENTFLOW_URL` → 配置里的 `gateway.listen` → `http://127.0.0.1:8080`；令牌是 `-token` 或 `$ADMIN_TOKEN`。

工作池大小是 `runtime.scheduler.workers`（没有对应参数）。已移除的参数（`-configdir`、`-workers`、`-log-json`、`-no-tui`、`-no-console`、`-no-webui`、`init -dir`、`user export -id`、`user keygen -out`、`migrate blobs -from`、`cred -format`）会被拒绝，并给出替代写法。

### 凭据命令行

`agentflow cred set|get|list|delete <name>` 管理加载配置（`-config`）所指存储里的全局凭据。`set` 从交互式提示或 stdin 读值 —— 绝不从 argv（用管道：`printf %s "$TOKEN" | agentflow cred set github_token`）。主密钥来自 `credentials.master_key_env` 指定的环境变量（默认 `CREDENTIALS_MASTER_KEY`）；没设置就是一个明确的错误。`list` 只打印名字，从不打印值。这些记录正是 `credential.get()` 和配置里的 `cred:<service>` 引用所解析的对象。

### 迁移部署

从单实例升到区域单元是一次数据搬迁，`agentflow migrate` 不需要转换就能做：两种后端说的是同一种 SQL 方言，blob 又是内容寻址的。

```
agentflow migrate store -from sqlite://./data/agentflow.db -to postgres://...
agentflow migrate store -from <src> -to <dst> -tables files_meta -append
agentflow migrate blobs ./data/media -kind media -config agentflow.yaml
```

`migrate store` 复制运行时各平面、身份注册表和凭据存储。目标非空时会被拒绝，除非传 `-append`；它会报告哪些表被复制、哪些不存在。凭据按密文复制：目标必须使用同一个主密钥，否则每个密钥都解不开。`migrate blobs` 遍历本地 media 或 files 目录，把每个 blob PUT 进 `-kind` 指定的 S3 桶；内容寻址意味着重跑时先 HEAD 已存在的，只上传缺的。两个命令都可以中断后重跑。

### 用户 API

引擎**不带任何用户界面**。它在 HTTP 渠道使用的同一个公开监听器上提供一套纯 JSON 的用户资料 API（`runtime.users.enabled`，需要 `runtime.identity.enabled`），前端因此可以住在自己的仓库里、按自己的节奏部署。契约以 OpenAPI 形式发布在 `openapi/users.yaml`。

- **注册** —— `POST /v1/users/register` 创建资料，并*只返回一次*它的 API 令牌。`runtime.users.registration: invite` 改为仅限邀请，邀请码由操作员从管理 API 发放（`POST /admin/api/users/invites`）。
- **渠道关联** —— `POST /v1/users/me/links` 返回一个一次性验证码；用户*从要关联的那个账号*发送 `/link <code>`，引擎在那条消息到达时完成关联。只有资料所有者能看到验证码，只有账号所有者能从该账号发出，所以单靠任何一半都关联不了什么。带验证码的消息永远不会进到循环。只有平台会验证发送者身份的渠道才能关联 —— 目前是 telegram；webhook 自称的发送者和 ghhook 恒定的发送者都不行。
- **自助** —— `/me`（资料和关联账号）、`/me/usage`（今天、七天窗口和配额状况）、`/me/projects`（调用方的文件项目）、`/me/links/{id}` 解除关联，以及 `/me/token/rotate` 作废泄露的令牌。
- **认证永远是 Bearer 令牌** —— 要么是部署身份提供商发的访问令牌，要么是 `POST /register` 发的 `afu_…` API 令牌。没有 cookie，也没有服务端会话，所以别的站点没有可重放的背景凭据，也没有 CSRF 令牌要带。
- **跨源** —— 在操作员把某个来源列进 `runtime.users.cors_origins` 之前，CORS 是**关闭**的；只有列出的来源才会被应答，而 `Access-Control-Allow-Credentials` 是刻意永不下发的：请求带的是 Bearer 令牌，不是 cookie，浏览器没有可借给别的来源的背景权限。`GET /v1/users/config` 是公开的，告诉前端注册是否开放、能提供哪些渠道，以及本单元服务哪个区域（`runtime.region`）。

**身份提供商。** 设置 `runtime.users.oidc.issuer`（再加上 `audience`，以及发现端点不可用时的 `jwks_url`），引擎就会校验那里签发的令牌：先对签发方 JWKS 验签，再验 `iss`/`aud`/`exp`，最后把 subject 声明当身份 —— 以 `<issuer>|<sub>` 为键，两个签发方因此不会撞车。登录、密码、MFA 和会话时长都放在那个提供商里；引擎只负责校验。首次登录验证通过时，会用令牌的 `email`/`name` 声明开通资料，除非 `jit_provisioning: false` —— 那样未知主体会被拒绝，直到操作员手动开通。只接受非对称算法，且签发方在启动时必须可达。

**部署提示。** `POST /register` 按客户端地址限流，所以反代后面的前端也得在那层限流 —— 否则所有调用方共用一个地址。

### 管理 API

运行时本身不带内嵌的浏览器界面或终端面板 —— 它在 `/admin/api/` 下提供一套无头的 JSON 与 SSE 控制面，**AgentFlow Studio**（以及自动化脚本）连的就是它。

**一个端口。** 管理面和渠道共用 `gateway.listen`：`/admin/api/*`、`/admin/sessions`、`/healthz`、`/readyz` 和 `/metrics`。`/admin/`、`/healthz`、`/readyz`、`/metrics`、`/health` 和 `/v1/users/` 下的渠道路径会被拒绝。

**认证：** 每条管理 API 路由（以及 `/metrics` 和 `/admin/sessions`）都要 Bearer 令牌 —— 即使来自回环地址也一样。设置 `ADMIN_TOKEN`（或具名 `runtime.admin.operators`）即可固定；否则会生成一个随机的每次启动令牌，只打印一次到日志。每次启动的令牌对任何能读日志的人都不算秘密，所以在令牌是临时生成的情况下，管理面**只应答本地客户端**：来自另一台机器的请求，或带了 `Forwarded`、`X-Forwarded-For`、`X-Real-IP` 的请求（所以同机上的反代不算本地），一律 `403`；`/healthz` 和 `/readyz` 则对负载均衡器保持开放。配置了凭据就会把管理面开放给远程客户端，只靠令牌把守，所以前面要加 TLS。区域单元必须配置一个：每次启动的令牌别的实例不认识。密钥全程只写：列举端点返回元数据和指纹，从不返回值；配置视图会遮住敏感键，同时保留 `${VAR}` 占位符。

**本地传输：** `runtime.admin.socket` 增加第二个仅本地的管理监听 —— Unix 上是 unix domain socket，Windows 上是命名管道 —— 而 `runtime.admin.tcp: false` 把管理面从 HTTP 监听器上拿掉。socket 仅所有者可访问（0600；管道 DACL 授予所有者、Administrators 和 SYSTEM），Bearer 令牌仍然必需，Studio 桌面应用接受 `unix:///path` 或 `npipe://name` 形式的 URL。

**路由。** AgentFlow Studio 做的每件事都由 `/admin/api/` 下的一条 JSON 或 SSE 路由支撑，操作员也可以直接写脚本调。所有路由都要同一个 Bearer 令牌。

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/state` | 实例身份、运行时长、配置世代、活跃会话、智能体和渠道。 |
| `GET /admin/api/config` | 原始配置文本、脱敏的结构化视图，以及文件的 mtime。 |
| `POST /admin/api/config/validate` | 对提交的文本做与启动等价的校验（严格字段、环境变量展开、交叉引用）。 |
| `POST /admin/api/config/save` | 校验、备份成 `.bak`、原子替换。文件在你背后被改过就返回 409。 |
| `GET /admin/api/models` | 运行中 ∪ 文件的模型集；密钥遮成末四位指纹。 |
| `PUT/DELETE /admin/api/models/{name}` | 把模型热应用到*本进程*（见上文集群限制）。 |
| `POST /admin/api/models/{name}/test` | 通过运行中的管理器发一次 1 Token 调用，并分类结果（auth / not\_found / unreachable / …）。 |
| `POST /admin/api/models/persist` · `/revert` | 把运行中的集合写进文件，或把文件里的集合重载进运行中的管理器。 |
| `GET /admin/api/metrics` | 采样计数器序列（5 秒一次，约 2 小时），供迷你图使用。 |
| `GET /admin/api/logs` | SSE 日志尾部：先回放环形缓冲，再实时推送。 |
| `GET /admin/api/users` · `/{id}` | 带今日用量和配额状况的用户资料；单个资料连同最近调用、审计尾部与项目。 |
| `POST /admin/api/users/{id}/limit` | 设置资料的每日 Token 限额（0 表示沿用部署默认值）。 |
| `PUT /admin/api/users/{id}/settings` | 资料的模型与指令层 —— 这是设置它们的唯一途径，因为一个人用哪个模型是成本决策，不是自助选项。 |
| `POST /admin/api/users/{id}/unlink` | 解除一个渠道账号（最后一个会被拒绝）。 |
| `POST /admin/api/users/invites` | 生成一次性注册邀请。 |
| `GET /admin/api/agents` · `PUT/DELETE /admin/api/agents/{name}` | 在运行时按定义安装、替换或移除智能体（不写文件；Studio 的智能体路由才是会持久化的版本）。 |
| `GET/POST /admin/api/groups` · `GET/DELETE /{id}` | 租户群组：列举、创建、带成员查看、解散。 |
| `POST /admin/api/groups/{id}/members` · `DELETE /{id}/members/{user}` · `PUT /{id}/master` | 添加或移除成员，或把群组交给新群主。 |
| `GET/POST /admin/api/pools` · `GET/PUT/DELETE /{id}/grants` | 具名共享记忆池，以及决定谁能用它的授权（租户 → 读/写）。存储用 `pool: <name>` 绑定到池。 |
| `GET /admin/api/users/{id}/export` · `DELETE /admin/api/users/{id}` | 导出为某个资料保存的一切（隐私导出），或清除它。 |
| `GET /admin/api/users/{id}/bundle?public_key=…` | 全保真迁移包：资料、记忆行、文件字节、群组和凭据，封给调用方的 X25519 公钥。目标单元用私钥 `POST /admin/api/users/import`。见[多区域](multi-region.md)。 |
| `GET /admin/api/usage?day=YYYY-MM-DD` | 当天的按租户 Token 汇总，含总计。 |
| `GET /admin/api/credentials/users` | 存有凭据的用户分区。 |
| `GET/POST /admin/credentials` · `DELETE /admin/credentials/{service}?user=` | 凭据存储：列举元数据、开通按租户密钥、吊销一个。只写 —— 值永不返回。 |

浏览器控制台覆盖上面那四个标签页。这张表里的其他一切（用户、邀请、配额、租户、用量、日志和原始配置）在 AgentFlow Studio 里都有对应页面，或者可以用 `curl` 脚本化。

### Studio —— 智能体构建器、演练场与追踪

**AgentFlow Studio** 是一个用来试作智能体的桌面与网页应用：写循环和指令、校验、把定义实时应用、与之对话、观察这一轮的每一步、把结果持久化进部署配置。它住在自己的仓库 `agentflow-studio` 里 —— 没有嵌进二进制 —— 用和控制台相同的 Bearer 令牌与管理 API 通信。

- **智能体** —— 每个已配置的智能体，带模型、能力、活跃会话数，以及它只存在于运行时还是已持久化。可从模板创建（echo、LLM 对话、工具循环、一次性、带记忆的群聊、知识库、单轮 API）、编辑或删除。
- **构建器** —— 常见字段的表单（模型、安全、记忆方案、能力、技能、历史预算、常驻标志、`can_contact`、凭据、extras）、TypeScript/JS 编辑器、指令编辑器，以及用于其余一切的原始 YAML。*校验* 会检查 prelude 指令、编译代码块，并跑与启动相同的智能体校验；*应用* 把它装进运行时；*应用并保存* 还会写进配置（一个会记住选择的拆分按钮）。编辑器旁的停靠区要么是 **运行** 面板 —— 与正在编辑的智能体对话并看追踪 —— 要么是脚本参考（F1）；Ctrl+Enter 会实时应用待处理的修改并发送面板里的消息，或重发上一条，所以下一轮跑的就是屏幕上看到的。
- **演练场** —— 与单个智能体的对话串。每条对话串是一个独立会话（`<agent>|studio:<thread>`），所以对话不会串。回复经合成的 `studio` 渠道到达，也就是说它们走的是普通出站路径：安全过滤器、审计日志和投递都不变。可选的*以用户身份* UUID 会盖上租户，个人记忆范围、配额和池授权因此生效；不带它时轮次没有租户。
- **追踪** —— 这一轮的实时时间线：`turn.start`、每个 `op.start`/`op.end`（模型、工具、耗时、Token、工具调用）、安全丢弃、回复、重载和循环错误。文本载荷默认关闭（`studio.trace_payloads`）；元数据始终捕获。
- **渠道** —— 网关持久化的渠道集：添加、编辑、校验、删除 webhook、Telegram、GitHub-webhook 和 bridge 条目；密钥保持只写。保存后下次启动时绑定。
- **模型** —— 运行中的模型注册表：添加、编辑、用真实调用测试、删除，然后把运行中的集合持久化到配置或还原成文件内容。
- **记忆** —— 见[记忆](memory.md)指南。编辑 `memory.backends` 和 `profiles.memory`：后端按提供商呈现，只显示该提供商读取的键；方案则是存储区列表（后端、表、保留期、窗口、必需特性、范围或共享池）加召回、写入、嵌入和重排设置。后端 URL 和 API 密钥只写：页面显示 `${VAR}` 或 `cred:` 引用，或把密码打码后的 URL。校验与启动等价，仍在使用的方案或后端不能删除。没声明后端的部署会把内置的 `main_db` 显示在运行时存储上；保存第一个后端时会把它一并写进文件，用内置 `conversational` 方案的智能体因此不会丢存储。智能体在启动时打开存储区，所以保存要下次启动才生效，新建的方案也要等那次重启后才能在构建器里选。
- **安全** —— 编辑 `profiles.safety` 过滤链（有序的内置项和 `classifier:<name>` 引用）和 `safety.classifiers` 策略。校验与启动等价；仍被引用的方案和分类器不能删除。分派器是按智能体构建的，所以保存要下次启动才生效。
- **凭据** —— 按分区浏览加密存储（全局或按用户），创建、编辑、吊销，并设置注入的请求头/方案。值只写，只以末四位指纹显示；编辑即时生效，因为存储在请求时才读。
- **概览** —— 落地页：实例、区域、版本、运行时长和配置世代，活跃会话、智能体、渠道，以及采样计数器的迷你图（`/admin/api/state`、`/admin/api/metrics`）。
- **用户** —— 带今日配额的用户资料、关联账号（可解除）、项目、最近调用和审计尾部；设置每日限额以及按用户的模型和指令层；发放邀请；导出或清除资料（`/admin/api/users/*`）。
- **租户** —— 群组（成员、群主移交、解散）和记忆池（创建、按智能体和租户授权）（`/admin/api/groups`、`/admin/api/pools`）。
- **用量** —— 按租户、智能体或模型分组的每日 Token 汇总（`/admin/api/usage`）。
- **日志** —— 带级别和文本筛选的实时日志尾部（`/admin/api/logs`）。
- **配置** —— 原始配置文件，带与启动等价的校验和受 mtime 保护、会备份的保存（`/admin/api/config*`）；配置目录部署则通过结构化页面和文件本身来编辑。

`Ctrl+K` 会打开一个覆盖所有页面、智能体和操作的命令面板。

**语言。** Studio 界面提供英文、简体中文（简体中文）和繁体中文（繁體中文）。**EN / 简体 / 繁體** 切换在每个页面的命令栏、登录页，以及*设置 → 语言*下。切换立即生效，无需重启或刷新，并且会被记住（浏览器存在本地存储，桌面应用存在其 webview 的本地存储）；首次启动跟随操作系统或浏览器的语言，找不到就回退英文。这些指南跟着同一个开关走，尚未翻译的指南显示英文。配置键、标识符、模型和方案名、YAML、脚本，以及运行时返回的任何内容 —— 包括错误消息 —— 都按运行时的原样显示。

**持久化是配置原生的。** 目录配置下，Studio 写 `profiles/<name>.yaml`（标上 `# managed by agentflow studio`）、`loops/<name>.ts` 和 `prompts/<name>.md`。单配置文件下，它替换 `agents.<name>` 键（保留注释和其他小节），并把循环/提示词写到 `studio.workspace`；定义里存的是绝对路径，所以持久化的文件不依赖下一个进程怎么启动。两种方式都是当下实时应用、下次启动时从配置加载。不是 Studio 写的方案会被拒绝，除非强制覆盖。

**运行它。** 客户端是自己仓库里的独立 Go 模块：

```
cd ../agentflow-studio
make build        # 或：CGO_ENABLED=1 CC="zig cc" CXX="zig c++" go build -o agentflow-studio.exe .
./agentflow-studio
```

输入实例 URL（`http://127.0.0.1:8080`，即 `gateway.listen` 的地址）和管理令牌。作为原生客户端它直接发送令牌，所以不需要 `runtime.admin.cors_origins` 条目。要从另一台机器连接，运行时需要配置好的 `ADMIN_TOKEN`（每次启动的令牌只应答本地客户端）；前面加 TLS。那份白名单仍留给管理 API 的*浏览器*客户端，且只接受精确来源（`http://localhost:5173` 和 `http://127.0.0.1:5173` 是两个不同来源）；`"*"` 会被拒绝。

| Studio 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/catalog` | 模型、工具、能力、安全/记忆方案、提示词键、prelude 版本和模板。 |
| `GET /admin/api/studio/agents` · `/{name}` | 智能体列表；单个智能体连同其 YAML、循环、指令和持久化文件元数据。 |
| `POST /admin/api/studio/agents/validate` | 对提交的定义和循环做无副作用校验。 |
| `PUT /admin/api/studio/agents/{name}` | 校验、实时安装（`apply`）、可选持久化（`persist`）；返回实际应用了什么以及任何持久化错误。 |
| `DELETE /admin/api/studio/agents/{name}` | 停止它的会话、移除它，并只移除 Studio 管理的文件。`force` 可越过活跃会话和手写方案。 |
| `POST /admin/api/studio/chat` | 演练场的一轮。流式 SSE（`trace`、`reply`，然后 `done` 或 `error`），或 `stream: false` 时返回 JSON。一轮以 `done` 结束（完成、超时或被安全拦截）；投递不出去的消息或中途失败的循环以 `error` 结束，此时 `stream: false` 返回 `ok: false` 连同追踪。根本起不了一轮的请求（未知智能体、不是资料的 `act_as_user`）在任何流开始前就以 4xx 拒绝。 |
| `GET /admin/api/studio/threads` · `POST /threads/{thread}/stop` | 已观察的对话串，以及对某个活跃会话的停止请求。 |
| `GET /admin/api/studio/trace/{agent}/{thread}` | 缓冲的追踪；加 `?stream=true` 走实时 SSE。 |
| `GET/POST /admin/api/studio/channels` · `PUT/DELETE /{index}` | 持久化的渠道集；校验草稿、创建、替换或移除。保存需重启。 |
| `GET /admin/api/studio/safety` | 安全方案和分类器、它们的使用情况，以及编辑器可引用的内置过滤器和模型集合。 |
| `POST /admin/api/studio/safety/validate` | 对方案或分类器草稿做与启动等价的试运行。 |
| `POST/PUT/DELETE /admin/api/studio/safety/profiles` · `/classifiers` | 创建、替换或移除方案或分类器。正在使用中的会被拒绝；写入会校验、留 `.bak`，并需重启。 |
| `GET /admin/api/studio/memory` | 记忆后端（密钥以引用或打码 URL 呈现）和方案、它们的使用情况，以及编辑器可选的提供商、模型和插件。 |
| `POST /admin/api/studio/memory/validate` | 对方案或后端草稿做与启动等价的试运行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 创建、替换或移除方案或后端。名称不可变；空的 URL 或 API 密钥保留已存储的值；正在使用中的会被拒绝；写入会校验、留 `.bak`，并需重启。 |
| `GET/POST/DELETE /admin/api/credentials` | 列举某个分区的元数据（`?user=`；留空为全局）、创建或更新密钥（空密钥保留已存储的值）、吊销一个（`?service=&user=`）。从不返回值。 |

**限制。** 演练场是实例本地的：合成渠道注册在一个进程里，所以集群里某个对话串的会话归另一个实例所有时，这里就收不到回复。Studio 不存聊天历史 —— 智能体的记忆才是事实来源 —— 追踪环在内存里，重启即重置。派生方案编写、附件、回放和评测不在第一版范围内。

### 日志

日志用五个**累积**级别 —— 选中一个就打印它及以上。用 `-log-level` 选择（默认 `info`）：

| 级别 | 放什么 | 例子 |
| --- | --- | --- |
| `ERR` | 运行时退出或无法继续 | 配置加载失败、记忆后端打开失败、端口绑定失败 |
| `WARN` | 降级但继续 | 上游 429/502 + 重试、渠道投递错误、循环崩溃（重启中）、重载编译失败（保留旧版） |
| `INFO` | 生命周期 | 运行时启动、渠道注册、telegram webhook 设置、会话派生/退出、热重载被接受 |
| `DEBUG` | 每次交互 | LLM API 请求、记忆 put/get/query（redis 往返、pgvector 查询）、收到 telegram 更新、webhook/ghhook 入站、操作分派、imap 拉取 / smtp 发送 |
| `DEV` | 临时开发日志 | 调试时加的临时探针 —— `llog.Dev(log, ...)`；绝不有意提交 |

同样的记录也喂给 AgentFlow Studio 的实时日志尾部（`/admin/api/logs`，短标签 `ERR/WRN/INF/DBG/DEV`）。

### `models` —— 具名的 LLM 提供商

```
models:
  default:
    provider: anthropic        # anthropic | openai | openai-responses | gemini | rerank
    model: claude-sonnet-5
    api_key: ${ANTHROPIC_API_KEY}
    base_url: ~                # 覆盖端点主机
    timeout: 30s
    retry: 2
    max_tokens: 4096
    thinking: medium         # off | low | medium | high | xhigh | max；留空 = 提供商默认值
    server_tools: []           # 提供商原生的服务器端工具
```

`provider` 选择请求/响应形状，`base_url` 选择主机。OpenAI 兼容端点（Ollama、vLLM、LiteLLM）要在 `base_url` 里带上 `/v1`；`openai` 会追加 `/chat/completions`，`openai-responses` 会追加 `/responses`。Anthropic 兼容端点把 `base_url` 设到主机根；运行时会追加 `/v1/messages`。`gemini` 则把 `base_url` 指向版本前缀（默认 `https://generativelanguage.googleapis.com/v1beta`），运行时会追加 `/interactions`，并在 `x-goog-api-key` 请求头里发密钥。Gemini Interactions API 是非流式的，也不接受 Token 上限，所以 `llm.stream` 只产出一个缓冲的增量，`max_tokens` 不会发送。`provider: rerank` 是一个说 `POST {base_url}/rerank` 的交叉编码器端点（TEI、vLLM、Jina、Cohere），通过 `llm.rerank` / 记忆的 `rerank_model` 使用，而不是当对话模型。

`server_tools` 列出要在每个请求上启用的提供商原生服务端工具（例如 gemini 的 `google_search`、openai-responses 的 `web_search`/`x_search`）。与通过 `opts.tools` 按次传入的客户端函数工具不同，这些在提供商内部执行；运行时只注入原生的 `{type: <name>}` 条目，按你写的名字转发。值是与提供商相关的字符串，运行时因此保持通用。在 OpenAI 两种形状上，这个裸条目在 SDK 的工具联合体里没有对应变体（它的工具类型被钉在 `"function"`），所以用 `param.Override` 构造；在 `gemini` 上，`google_search` 会映射到原生工具，其他名字按原始条目透传。

### `memory.backends` —— 后端实例

下面每个后端都是真实的提供商 —— 没有桩。存储后端承载 KV/前缀/全文/TTL；向量后端承载语义召回。SQLite 会有意以明确错误拒绝向量查询，而不是静默返回空结果。

| 提供商 | 类别 | 特性 |
| --- | --- | --- |
| `sqlite` | 存储 | kv、prefix\_scan、text\_search、ttl、transaction、vector（对 BLOB 列做暴力余弦；无需扩展，受 `vector_scan_limit` 限制，默认 50000 行） |
| `redis` | 存储 | kv、prefix\_scan、ttl |
| `postgres` | 存储 | kv、prefix\_scan、text\_search、transaction —— 数据库装了 pgvector 扩展时再加 vector（启动时探测；同一张表） |
| `volatile` | 存储 | kv、prefix\_scan、ttl —— 内存里，不持久；受 `max_entries` 限制 |
| `pgvector` | 向量 | vector（余弦 k-NN；嵌入维度通过 `dim` 配置，默认 1536）。作为别名/兼容提供商保留；`postgres` 在扩展存在时已能提供向量 |
| `qdrant` | 向量 | vector（经 Qdrant 的 gRPC API 做余弦 k-NN —— `url` 的端口按 REST 读，所以 `:6333` 变成 gRPC 的 `:6334`；`dim` 默认 1536，`collection` 默认 `agentflow`，可选的 `api_key`） |
| `redisvector` | 向量 | kv、prefix\_scan、ttl、vector（需要 RediSearch 模块，例如 Redis Stack；余弦 k-NN，`dim` 默认 1536） |

每个后端有一个 **locality**，由提供商和 `runtime.topology` 推导，并以 `agent.info().memory.stores[*].locality` 暴露给循环：

| 提供商 | `single` | `regional` |
| --- | --- | --- |
| `postgres`、`pgvector`、`qdrant` | `deployment` | `deployment` |
| `sqlite` | `deployment`（这唯一实例就是部署） | 拒绝 |
| `redis`、`redisvector` | `deployment` | `region`（每个单元一个集群） |
| `volatile` | `instance` | `instance` |

在区域单元里，持久对话存储 —— `dialogue`、`facts`，或 `recall: plugin:semantic` 下的 `semantic` —— 必须放在 `deployment` 或 `region` 后端上。把持久存储放在 `volatile` 上会被校验拒绝（会话迁到别的实例就丢了历史），除非存储设了 `locality_ok: true`。无论哪个层级，易失记忆都必须有界：要么后端设了 `config.max_entries`（最旧的条目被淘汰），要么绑定到它的每个存储区都设了 `retention` TTL。

```yaml
memory:
  backends:
    cache:
      provider: volatile
      config: { max_entries: 10000 }

profiles:
  memory:
    p:
      stores:
        dialogue: { backend: main_db, table: dialogue, retention: 30d }
        scratch:  { backend: cache, table: scratch, retention: 1h }
```

```
memory:
  backends:
    main_db:
      provider: sqlite
      config:
        path: ./data/agentflow.db
    vec:                                # 任意一个向量存储都可服务 semantic 方案
      provider: pgvector
      config: { url: "postgres://user:pw@host/db", dim: 1536 }
    # provider: qdrant          # gRPC；url 保持 REST 形式，:6333 会拨号到 gRPC 的 :6334
    # config: { url: "http://localhost:6333", dim: 1536, collection: agentflow, api_key: "${QDRANT_KEY}" }
    # provider: redisvector      # Redis Stack（RediSearch），不是普通 Redis
    # config: { url: "redis://localhost:6379", dim: 1536 }
```

三种向量存储都只能做向量查询，和 `pgvector` 一样：其他任何查询种类都是明确错误，绝不静默返回空迭代器。`dim` 在存储首次创建时固定，必须和嵌入模型一致；之后要改需要手工迁移。`redisvector` 把值存在向量旁边，所以也提供 kv、prefix\_scan 和 ttl；`redis` 不变，仍是纯 KV 选项。

`qdrant` 说 gRPC，但它的 `url` 保持每个部署现有的形状 —— REST 的。协议决定传输（`https` 带 TLS 拨号），端口会被换算：显式的 `6333`（Qdrant 的 REST 默认端口）变成 gRPC 的 `6334`；其他显式端口按原样拨号，所以 gRPC 代理也能用；不带端口的 URL 得到 `6334`。之前经仅 REST 的代理访问 Qdrant 的部署，需要改指 gRPC 监听器。点本身仍用同样的确定性 ID 寻址，所以既有的 `agentflow` 集合可以原样读写。

完全没有 `memory.backends` 段时，引擎会在运行时存储上注册 `main_db`：文件是 `sqlite` 提供商加 `path`，DSN 是 `postgres` 提供商加 `url`。这正是内置 `memory: conversational` 方案在最小配置里也能用的原因，也意味着单实例部署不需要第二个数据库就能有持久记忆。`postgres` 运行时存储同理也不需要 `memory:` 段；装了 pgvector 扩展时，它的数据库就能提供向量。

`config.url`/`config.password`/`config.api_key` 的密钥引用解析不出来的后端会**被跳过并给出警告**，而不是让启动失败：绑定到它的存储区会重新绑到一个还活着的后端上（优先选提供 `text_search` 的，这样向量存储降级为全文检索；循环根据绑定的特性做能力探测），`requires` 下调到回退后端支持的项；没有后端存活时则被丢弃。真正未知的后端名仍然是启动错误。

### `profiles` —— 可复用配置

配置按种类分组放在 `profiles:` 下 —— `memory`、`safety`、`shell` 和 `agent` —— 智能体按名字引用它们（`memory: chat`、`safety: strict`、`shell: python`）。

#### 记忆方案

[记忆](memory.md)是完整指南：后端、方案和存储区怎么拼在一起，循环怎么写入和召回，隔离、配方，以及校验消息。这一节是参考。

```yaml
profiles:
  memory:
    chat:                                 # "conversational" 保留给内置方案
      stores:
        dialogue: { backend: main_db, table: dialogue, window: 1000, retention: 30d }
        facts: { backend: main_db, table: facts, requires: [kv, text_search] }
      write: [plugin:routing_table]
      recall: plugin:recency

    semantic:                             # 嵌入 + 重排召回
      stores:
        semantic: { backend: vec, table: semantic, requires: [vector] }
      write: [plugin:routing_table]
      recall: plugin:semantic
      embed_model: embed                  # models: 中的条目；写入向量存储时自动嵌入
      rerank_model: rerank                # 可选的 provider:"rerank" 条目
      oversample: 4                       # 取 k*4 个向量命中，重排后保留 k 个
```

`write` 会作为 `agent.info().memory.write` 传给循环；`memory.write` 始终走路由表（`plugin:routing_table`，或覆写了它的 `plugins/routing_table.ts`）。

用 `recall: plugin:semantic` 时，`memory.recall({ text = "...", k = 5 })` 会把查询转成嵌入、在向量存储上做余弦 k-NN，并在设了 `rerank_model` 时重排。没有文本的查询退回到按时间召回。

| 存储区键 | 说明 |
| --- | --- |
| `backend` | 必填。引用一个后端实例名。 |
| `table` | 这个存储区绑定的物理表（或集合）。 |
| `retention` | 以 `ms`、`s`、`m`、`h`、`d` 为单位的时长（允许复合与小数，例如 `2d12h`），或字面量 `forever`。**它是写入 TTL，不是清扫：** 它会成为任何未显式指定 TTL 的 `store.put` 的默认 TTL，所以记录在*写入*之后那个时长过期，而在这个设置存在之前写入的行永远不会过期。解析不出来的值是启动错误。 |
| `window` | 每张物理表最多保留的记录数，由定期 GC 裁剪。适用于 KV 后端；**向量后端（pgvector、Qdrant、Redis）不实现裁剪**，所以在这些上面设 window 或 retention 只是声明了一个没人执行的边界 —— 引擎会在启动时警告，而表会无限增长。 |
| `requires` | 启动时按提供商校验的特性列表。 |
| `pool` | 把存储区绑定到具名共享池（经租户管理 API 创建）。授权决定哪些租户能读写；绑定池的存储区不设 `scope`，且池在启动时必须存在。`shared: true` 是启动错误 —— 池取代了它。 |
| `scope` | `agent` 让私有存储区重新加入每个智能体一个池，而不是引擎的按用户键范围。 |
| `collection` / `policy` | **已弃用 —— 无效果。** 存储区由后端和表绑定，没有实现任何按存储区的策略开关。解析它只为让旧配置仍能启动；设置了任一项时引擎会在启动时警告。 |

> **隔离变化**：记忆存储**默认按智能体私有** —— 共用同一方案（或 `conversational`）的两个智能体不再共用物理表；每个绑定 `<agent>.<table>`（例如 `writer.dialogue`）。循环仍用方案的存储区名字；前缀在绑定时加上。**有意共享**：把存储区绑定到具名池（`pool: <name>`），再通过租户管理 API 授权给租户；`shared: true` 现在是启动错误。按智能体隔离之前写的既有行仍留在旧的未加前缀表里，需要显式迁移。

#### 安全方案

```
profiles:
  safety/default:
    filters: [source-attribution, signal-gate, steady-directive, support-offer, affect-guard]
```

`safety: none` 是显式退出。分派器由核心掌管，与循环处理器无关地运行。

#### 基于模型的分类器

`safety.classifiers` 定义由专用模型支撑的审核或分类策略。方案用 `classifier:<name>` 来选择启用：

```
models:
  moderation:
    provider: openai
    model: gpt-4.1-mini
    api_key: ${MODERATION_API_KEY}
    timeout: 3s
    retry: 0
    max_tokens: 64

safety:
  classifiers:
    moderation:
      model: moderation
      prompt: safety/moderation          # prompts: 注册表键；默认使用内嵌版本
      mode: enforce                      # enforce | shadow
      directions: [ingress, egress]
      sources: [channel, agent]
      threshold: 0.7
      categories:
        safe: allow
        self_harm: block
        violence: block
        harassment: review
        prompt_injection: block
      timeout: 2s
      on_error: block                    # block | pass
      on_saturated: block
      on_budget_exhausted: block
      max_tokens: 64
      max_input_chars: 8000
      max_concurrency: 8
      cache: { size: 10000, ttl: 5m }
      budget: { tokens_per_day: 500000 }
      refusal: ""                        # 可选的入站拒绝回复文本
      fallback: ""                       # 可选的出站替代回复文本

profiles:
  safety:
    strict:
      filters: [source-attribution, classifier:moderation, affect-guard]
```

模型只返回标签和分数；运行时套用 `categories` 和 `threshold`，所以模型输出无法自己决定动作。必须有一个 `safe: allow`（或其他 `allow` 标签）。分类器模型必须用引擎密钥（`key_source: config`，默认值）；`key_source: user` 的模型会在启动时被拒绝，否则一个没有密钥的用户就能关掉审核。调用模型时不带工具，也不带服务端工具。

失败遵循明确策略而不是静默放行：`on_error`（提供商错误、输出无效、超时）、`on_saturated`（并发上限满）、`on_budget_exhausted`（专用审核预算耗尽）。三者默认都是 `block`。设 `mode: shadow` 可以只记录判定而不执行；模型调用仍会发生（延迟也仍在），只是动作被抑制。`sources` 使用规范化词汇 `channel`、`agent`、`scheduler`、`system`（渠道驱动内部盖章 `user`，计时器盖章 `timer`）。

入站拦截会跳过本轮：循环根本看不到消息，配了 `refusal` 就发出它。出站拦截会压制回复，配了 `fallback` 就发出它。拒绝和兜底每个会话每 30 秒最多一次，免得被拦消息的洪峰变成出站洪峰。分类器开销记录在合成的 `__moderation_<name>` 台账桶下，所以既不消耗智能体的 `tokens_per_day` 预算，也不消耗用户的 LLM 配额。没有 `safety:` 的派生方案继承发起会话的过滤链；要在 `profiles.agent.*` 上设 `safety:` 才能给子会话不同的方案。

#### Shell 方案

Shell 方案说的是智能体的 Shell 是什么。用 `agents.<name>.shell` 绑定（智能体还需要 `shell.exec` 能力）。

```
profiles:
  shell:
    code:
      provider: sandbox          # sandbox（默认）| ssh
      environment: python        # environments 中的一个条目（必填）
      project: workspace         # 作业运行所在的文件存储项目（默认 workspace）
      workdir: app               # 命令的起始目录，相对于项目
      timeout: 90s               # 作业的时间上限，受 runtime.sandbox.max_timeout 限制
      mem_limit: 512MiB          # 可以降低 runtime.sandbox.memory，但不能提高
      cpu_limit: 1.0
      env: { CI: "1" }
      ignore: ["node_modules/", "__pycache__/"]   # 由作业创建，从不存储
      network: none              # 唯一的取值：沙箱作业没有网络
    box:
      provider: ssh              # 连接到一台真实的远程机器
      host: build.example.com:22
      user: dev
      key_file: /etc/agentflow/id_ed25519      # 或 password:（一个密钥引用）
      workdir: /srv/work
```

**`provider: sandbox`** 把每条命令作为一次性沙箱作业运行，用 [gVisor](https://gvisor.dev)（`provider: gvisor`，Linux 上的默认值）或 [microsandbox](https://github.com/superradcompany/microsandbox)（`provider: microsandbox`，Linux 上走 KVM 的硬件隔离微虚拟机、macOS 上走 Apple Silicon HVF、或 Windows 上走 WHP）：环境提供根工具链，项目以读写方式挂到 `/workspace`，命令写进项目的内容在结束时存下。别的都活不过一条命令，所以没什么要追踪的，任何实例都能跑下一条。`ignore` 用一个小的 `.gitignore` 子集：`node_modules/`（任意位置叫这个名字的目录）、`*.pyc`（任意位置的名字）、`build/out`（从项目根算起的路径）；结尾的斜杠把模式限定为目录。被忽略的路径既不存储也不清理 —— 它们作为尽力而为的缓存留在实例上。

项目可以用工作树里的 `.agentflow/shell.yaml` 定制它的智能体方案。它可以设 `environment`（部署声明过的）、`env`、`workdir`、`timeout` 和 `ignore`。它没有用于镜像、网络策略或资源限制的字段，未知字段是错误而不是静默无操作。

**`provider: ssh`** 每个会话保持一条到已存在机器的连接；这里不负责开通它。连接活在拨号的那个实例上，随会话关闭。

`provider: docker` 已移除。仍在用它配置会在加载时失败，并给出替代写法。

#### `environments` —— 声明的工具链

环境是沙箱作业启动时的根文件系统和变量。环境由操作员声明；Shell 方案选一个，项目也可以另选一个。

```
environments:
  python:
    image: docker.io/library/python:3.12-slim@sha256:<64 hex digits>
    env: { PYTHONUNBUFFERED: "1" }
  tools:
    rootfs: /srv/agentflow/envs/tools
```

`image` 与 `rootfs` 二选一：

- **`image`** —— 一个**按摘要钉住**的 OCI 镜像。只写标签会被拒绝：标签可能被指向不同内容，那样两个实例就会用同一个名字跑不同的工具链。在 `gvisor` 下，镜像首次使用时拉取并解包到 `<runtime.sandbox.data_dir>/images/<digest>`（公开/匿名拉取的仓库，gzip 压缩层）。在 `microsandbox` 下，`msb` 首次使用时原生拉取并缓存 OCI 镜像。两种情况下都会先应用镜像自带的 `Env`，再叠加这里的 `env`。
- **`rootfs`** —— 主机上的一个目录，里面已经是根文件系统（例如导出的容器，或 `debootstrap` 树）。它需要有 `/bin/sh`。

因为作业没有网络，命令需要的一切都必须已经在环境里：包是镜像的一部分，而不是命令装出来的。

#### `runtime.sandbox` —— Shell 作业怎么跑

每个设置都有默认值；已经声明了环境和 Shell 方案的部署一个都不用配。

| 键 | 默认值 | 含义 |
|---|---|---|
| `provider` | `gvisor` | 沙箱运行时后端：`gvisor`（`runsc`，Linux）或 `microsandbox`（`msb`，Linux KVM / macOS Apple Silicon / Windows WHP） |
| `runsc` | `PATH` 上的 `runsc` | `provider: gvisor` 时的 gVisor 运行时二进制。把该发行版的 `gvisor-bin/` 目录放在它旁边 |
| `microsandbox` | `PATH` 上的 `msb` | `provider: microsandbox` 时的 microsandbox CLI 二进制 |
| `data_dir` | `<data dir>/sandbox` | 工作区、解包后的镜像和运行时状态。这是缓存：里面的一切都能从文件存储和镜像仓库重建 |
| `concurrency` | 4 | 本实例同时运行的作业数 |
| `per_user` | 2 | 其中同一用户最多能占几个 |
| `queue_timeout` | 30s | 作业排队等轮到自己多久后失败 |
| `timeout` | 60s | 没有其他设置时的作业时限 |
| `max_timeout` | 10m | 调用、项目或方案最多能要求的时限 |
| `background_max_timeout` | 2h | `shell.start` 作业的时限 |
| `output_limit` | 256KiB | stdout 和 stderr 各自的上限；超出部分丢弃并置 `truncated` |
| `tmp_limit` | 256MiB | 作业的 `/tmp` 和家目录大小 |
| `memory` | 1GiB | 整个沙箱的内存上限。超限的作业被杀，调用抛错 |
| `cpus` | 无 | CPU 上限，单位为核 |
| `pids` | 512 | 沙箱内的进程数（`gvisor`） |
| `write_back.max_files` | 10000 | 一个作业最多能存多少个改动过的文件 |
| `write_back.max_bytes` | 1GiB | 一个作业能存的文件总大小 |

**要求。** **`gvisor`** 只在 Linux 上跑（x86_64 和 ARM64，内核 5.6 或更新，需要非特权用户命名空间；在没有 `/dev/kvm` 的云主机/容器里也能用）。**`microsandbox`** 通过 `msb` CLI 在 Linux（`/dev/kvm`）、macOS（Apple Silicon Hypervisor.framework）和 Windows（WHP）上跑（进程外 —— AgentFlow 仍是纯 Go `CGO_ENABLED=0`）。配置的运行时在主机上跑不起来时，引擎仍会启动，每次 Shell 调用都以一句说明缺什么的话失败。

**内存和 CPU 限制。** 在 `gvisor` 下，主机有 systemd manager 时由内核通过 systemd scope 强制执行（否则作业不带这两项限制运行，而 PID、tmpfs 大小、时限和输出上限仍然生效）。在 `microsandbox` 下，内存（`--memory`）和 CPU（`--cpus`）限制由微虚拟机 hypervisor 直接执行。`agentflow doctor` 会报告当前生效的提供商、它的版本、限制是否被强制执行，以及每个环境是否可用。

**一台主机，一个数据目录。** 两个引擎共用一个 `data_dir` 会在启动时互清对方的作业状态。

#### 智能体方案（可派生）

```
profiles:
  agent/coder:
    model: default
    loop: ./loop.ts
    skills: [builtin:fs.read, builtin:fs.write]
    capabilities: [llm.chat, tools]
    can_contact: []
    credentials: [deploy_token]   # 子智能体的 credential.get 允许列表
    extras:                       # 在子智能体中通过 agent.config() 只读访问
      workflow: release-train
      goal: { type: autonomous, max_turns: 12 }
    ephemeral:
      max_lifetime: 2h
      idle_ttl: 15m
      max_turns: 200
    budget:
      tokens_per_day: 100000
      window: 168h            # 可选的滚动窗口；省略则每日重置
```

声明了 `budget.tokens_per_day` 的派生方案会得到一个**由它派生出的每个子会话共享**的计量 LLM 池，所以一个项目类型（经理 + 工人）从同一个预算里花。设了 `window` 时，用量随提交逐步超过窗口而持续释放，而不是在日界处清零。

`extras` 和 `credentials` 会带到每个子会话，所以派生出的循环调用 `agent.config()` 时看到的是自己的方案选项和凭据白名单 —— 和静态智能体看到的一样。

### `gateway` —— 一个共享 HTTP 监听器

所有 HTTP 渠道（webhook、ghhook、bridge、telegram webhook/auto、qq webhook）都把路径挂到**一个共享 HTTP 服务器**上 —— 没有按渠道的 `listen`。监听器在 `gateway.listen` 设一次（默认 `:8080`），这个服务器还提供 `GET /health` → `200 {"ok":true}`，供探针和反代健康检查使用。`gateway.public_url` 是外部可达的基础地址（例如 `https://bot.example.com`），运行时需要替你注册 webhook 时用它。

| 键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `gateway.listen` | string | :8080 | 所有 HTTP 渠道共用的那一个 HTTP 监听器 |
| `gateway.public_url` | string | — | 外部基础 URL；telegram 的 `webhook`/`auto` 模式需要 |
| `gateway.channels` | list | — | 渠道定义（见下） |

#### 渠道类型

| 类型 | 说明 |
| --- | --- |
| `webhook` | POST → 路由 → 会话 → 回复。**同步**（默认）：响应挂起，直到智能体回复或 `timeout`（默认 `55s`）→ 504。**异步**（`async: true`）：立刻返回 202 和作业 ID；回复通过 `GET <path>result/<id>`（未完成时 202）收取，或 POST 到调用方给的 `callback_url` —— 异步作业不会被同步超时截断，15 分钟后过期。默认路径 `/webhook/` |
| `ghhook` | GitHub webhook 入口：用 `secret` 校验 `X-Hub-Signature-256` HMAC，立即回 200，再把事件作为智能体类型的消息转发（payload `{type: project_event, source: github, event, data}`）。没有回复路径。默认路径 `/hooks/github/` |
| `bridge` | 面向任何其他聊天平台（Slack、Discord、Teams 等）的进程外适配器。适配器把事件 POST 到 `path`，带 `X-Agentflow-Signature: sha256=<请求体的 HMAC>`，密钥是 `secret`；运行时把回复以 `{to, text, attachments}` POST 到 `deliver_url`，只用 `token` 作为 Bearer 认证（出站投递不做 HMAC 签名）。`timeout` 限制每次投递（默认 10s）。bridge 的发送者记为「声称身份」，因此无法关联到资料 |
| `telegram`（`mode: polling`） | 默认。长轮询 `getUpdates`，强制执行 `allow_users`。会先清掉 Telegram 侧残留的 webhook —— 有 webhook 注册时 getUpdates 会 409（例如从 webhook 模式退回时） |
| `telegram`（`mode: webhook`） | 在 `path` 接收 POST 更新（默认 `/webhook/telegram/`）；需要 `gateway.public_url`。每次投递都必须带 `secret_token` 请求头（否则 401/403，在解析之前检查）；按渠道配置，缺失时每次启动生成 |
| `telegram`（`mode: auto`） | 启动时探测 `<public_url>/health`：可达就注册 `setWebhook`，否则 `deleteWebhook` + 长轮询。`setWebhook` 失败时也会先清 webhook 再轮询。需要 `gateway.public_url` |
| `qq`（`mode: websocket`） | 默认。腾讯官方 QQ 机器人 API v2（QQ 开放平台）。运行时带着应用访问令牌主动连到 QQ 网关（令牌由 `app_id` + `secret` 换取，到期前刷新），订阅 `scenes` 所需的 intents，发送心跳，掉线后恢复会话；无需对公网开放任何入口。因 intents 不被允许或机器人被封而断开时，渠道会带错误停止 |
| `qq`（`mode: webhook`） | 平台把事件 POST 到 `path`（默认 `/webhook/qq/<name>/`）；在控制台把 `<公网 https 地址><路径>` 设为回调地址（端口 80、443、8080 或 8443）。每个请求（包括 URL 校验握手）都必须带由 `secret` 派生密钥签出的有效 Ed25519 签名（否则 401/403，在解析之前检查） |
| `openai` | 在 `<path>chat/completions` 提供 OpenAI Chat Completions API，在 `<path>models` 提供模型列表。`model` 是暴露的 `agents` 之一，每个跑自己的 `loop:`；把智能体指向 `./examples/loops/openai_passthrough.ts`，就能把客户端的消息、工具和采样参数原样转发给它的模型。`auth` 可以是 `user_token`（按用户 Bearer，需要 `runtime.identity.enabled`）、`key`（来自 `keys` 的共享密钥）或 `none`（仅回环监听器）。此类型的 `journal` 默认是 `metadata`，对话文本因此永不进入审计日志。见 [OpenAI 兼容 API](openai-api.md) |

每个 HTTP 渠道的 `path` 必须以 `/` 结尾（子树挂载）。UUID 风格的路径片段是推荐的不可猜密钥前缀。两个渠道挂同一个路径是启动错误，`openai` 渠道的端点与另一个渠道的子树重叠也是。Telegram 的 `token` 和 `secret_token`、ghhook 的 `secret`、qq 的 `secret`、bridge 的 `secret` 和 `token`、`openai` 渠道的 `keys` 都是密钥（见注册表）：单文件路径上做环境变量插值，configdir 路径上是惰性引用 —— 解析不出来的会跳过该渠道并给出警告，绝不导致启动失败。

**QQ 机器人**（`type: qq`）细节：

- **场景与 intents。** `c2c`（与机器人的私聊）和 `group`（QQ 群里的 @ 机器人）共用 `GROUP_AND_C2C_EVENT` intent；`guild`（频道里的 @）用 `PUBLIC_GUILD_MESSAGES`；`dm`（频道私信）用 `DIRECT_MESSAGE`，这个需要向平台申请开通，所以不在默认集合里。
- **会话。** 每条消息的回复目标，以及对路由器而言的 `payload.chat_id`，是 `c2c:<user_openid>`、`group:<group_openid>`、`guild:<channel_id>` 或 `dm:<guild_id>`，所以一个群或频道就是一个对话，每个私聊各自独立。发送者是 `user:qq:<openid>`（群里是 `member_openid`，频道里是作者 id）；payload 还带场景、消息 ID、事件 ID、`union_openid` 以及平台发来的昵称。QQ 会验证发送者身份，所以身份层把它们视为已验证、可关联。
- **回复。** 在平台的被动回复窗口内（私聊 60 分钟，群和频道 5 分钟），回复会引用最新的入站消息，带 `msg_id` 和递增的 `msg_seq`，私聊每条消息最多 4 条回复，群 5 条。超出窗口，或平台答复窗口已关闭时，回复改为主动消息发出，平台会限流，用户也可以关掉。长文本按最多 1800 个字符分块。
- **Markdown。** 开 `markdown: true` 时，私聊和群里的文本回复以 markdown 消息发出（`msg_type: 2`），在回复窗口内仍是被动的；频道和私信保持纯文本。Markdown 是平台按机器人开通的能力。若平台拒绝 markdown 消息，同样的文本会改以纯文本重发，并暂停 markdown 十分钟，所以机器人没有这项能力时也不会丢内容。被分块切断的围栏代码块会在一个块末尾关闭，并在下一个块开头重新打开。
- **媒体。** 入站附件按渠道的 `media` 策略下载；没有文字的语音消息使用平台的语音识别文本。出站时，QQ 只通过公开 URL 抓取媒体：渠道要么要求 `media.backend: s3`（短时效 SigV4 预签名 GET URL），要么要求 `media.temp_url.enabled: true` 加 `gateway.public_url`（HMAC-SHA256 临时 HTTP URL）；两者都没配置时会在启动时给出警告并跳过该渠道。blob 存储句柄和内联图片（例如模型输出的 WebP/PNG）会自动转码成 QQ 兼容格式（JPEG/PNG，上限 10 MiB），并在签 URL 之前缓存为派生变体（私聊和群里以富媒体上传，频道里作为图片链接发送）。
- **重复。** 平台可能重复投递同一条消息（webhook 重试、恢复后的重放）；30 分钟内重复的消息 ID 会被丢弃。
- **控制台设置。** 从 q.qq.com 的机器人页面复制 AppID 和 AppSecret。webhook 模式在那里填回调地址，并把运行时的出口 IP 加进控制台要求的 IP 白名单；websocket 模式只需要白名单。机器人处于沙箱状态时，只有在控制台配置过的沙箱群、频道和成员才能触达它。

**审计模式**按渠道设置：`journal: full`（除 `openai` 外每种类型的默认值）记录消息文本和附件描述符，`journal: metadata`（`openai` 的默认值）每条消息记一行，含 ID、大小、用户和耗时，**绝不含文本**，`journal: off` 对该渠道什么都不记。见 [`audit`](#audit--message-journal)。

**入站媒体**按渠道通过 `media:` 段选择性开启 —— 一组 MIME 模式白名单（`image/*`、`application/pdf`、`audio/*` 等）加 `max_bytes`（默认 8 MiB）。Telegram 下载文件（getFile），webhook 接受 base64 `attachments`；字节落到内容寻址的 blob 存储里（默认本地文件系统，或经顶层 `media:` 配置用 S3/MinIO），消息只带很小的片段描述符。没有 media 段时，附件被丢弃，只有文本/说明文字流过。

```
gateway:
  listen: ":8080"               # 唯一的共享监听器（默认 :8080）
  public_url: ""                # 外部基础地址，例如 https://bot.example.com
  channels:
    - type: webhook
      path: /webhook/dev/9f2c.../   # 子树；UUID 段 = 密钥前缀
      agent: bot
#     timeout: 120s                 # 同步等待回复的时长（默认 55s）
#     async: true                   # 或者发出后轮询：202 + 作业 id，然后
                                    # GET result/ 或 callback_url
#     media:                        # 可选——媒体接收默认关闭
#       max_bytes: 8388608
#       allow: [image/*, application/pdf]
    - type: ghhook
      path: /webhook/github/9f2c.../
      secret: ${GITHUB_WEBHOOK_SECRET}   # HMAC 密钥；留空则跳过校验（仅限开发）
      agent: bot
    - type: telegram
      token: ${TG_TOKEN}
#     secret_token: ${TG_WEBHOOK_SECRET}  # webhook 认证；留空 = 每次启动生成
      mode: auto                 # polling（默认）| webhook | auto
      allow_users: [123456789]
      agent: bot
      media:
        max_bytes: 8388608
        allow: [image/*, application/pdf]   # 图片 + 文档送入视觉模型
    - type: bridge
      name: teams
      path: /bridge/teams/
      agent: bot
      deliver_url: https://bridge.example.com/deliver
      secret: ${BRIDGE_SECRET}       # 入站事件的 HMAC 密钥（必填）
      token: ${BRIDGE_TOKEN}         # 出站投递时可选的 bearer
#     timeout: 10s                   # 每次投递（默认 10s）
    - type: qq
      name: qq
      agent: bot
      app_id: "102xxxxxx"            # 机器人的 AppID（QQ 开放平台控制台）
      secret: ${QQ_BOT_SECRET}       # AppSecret
      mode: websocket                # websocket（默认）| webhook
#     path: /webhook/qq/qq/          # 仅限 webhook 模式（这是默认值）
#     scenes: [c2c, group, guild]    # c2c | group | guild | dm（默认 c2c、group、guild）
#     api_base: https://api.bot.qq.com   # 默认值
#     markdown: true                 # 私聊和群中使用 markdown 回复
    - type: openai
      name: api
      path: /v1/                     # 提供 <path>chat/completions 和 <path>models
      agents: [bot]                  # 作为模型 id 开放
      auth: key                      # user_token | key | none（仅限回环）
      keys: ["${COMPAT_API_KEY}"]    # auth: key
#     timeout: 600s                  # 整个请求的时限（默认 600s）
#     max_request_bytes: 33554432    # POST 请求体上限（默认 32 MiB）
#     journal: metadata              # full | metadata | off；metadata 是此类型的默认值
#     expose_reasoning: false        # 以 reasoning_content 发送模型推理内容
```

> **破坏性变更**：按渠道的 `listen:` 字段已移除。仍带着它的配置会以 `field listen not found in type config.Channel` 加载失败。请把每个渠道的端口移到统一的 `gateway.listen`，给每个 HTTP 渠道一个以 `/` 结尾的 `path`，并为 telegram 的 webhook/auto 模式设置 `gateway.public_url`。见 README 的迁移清单。

### `mcp.servers`

```
mcp:
  servers:
    filesystem:                  # stdio：由运行时启动服务器
      command: npx
      args: ["-y", "@mcp/server-filesystem", "/data"]
    affiliate:                   # Streamable HTTP：运行时连接一个正在运行的服务器
      url: https://mcp.internal.example/mcp
      token: ${MCP_AFFILIATE_TOKEN}   # 以 Authorization: Bearer 发送
```

服务器声明 `command`（作为子进程派生，经 stdio 通信）**或** `url`（一个 Streamable HTTP 端点，直接拨号）二选一。两者都在启动时连接一次，连不上的服务器会被跳过并给出警告，而不是让运行停下来。HTTP 传输正是让集成能完全住在本仓库之外的原因 —— 一个独立部署、独立版本化的服务，运行时借此支持自有或授权的连接器而无须随附它们。`token` 只作用于 HTTP 传输，每次请求都作为 Bearer 凭据发送；不需要的端点留空即可。

连接时发现的工具以 `mcp:<server>/<tool>` 进入注册表。服务器断开后，调用返回结构化的 `unavailable`。

### `search`

```
search:
  default: ollama               # 调用省略 `engine` 时使用的引擎
  engines:
    doubao:
      api_key: ${DOUBAO_API_KEY}   # 火山引擎联网搜索 Custom API
    ollama:
      api_key: ${OLLAMA_API_KEY}   # ollama.com 托管的网页搜索
    stackoverflow:
      site: stackoverflow          # 可选；任意 StackExchange 站点（默认 stackoverflow）
      # api_key 可选——匿名配额为每天 300 次请求；免费的应用密钥可提升到 1 万次
    github:
      # 仓库搜索——api_key 可选（令牌可将每分钟 10 次提升到 30 次）
    youtube:
      api_key: ${YOUTUBE_API_KEY}  # YouTube Data API v3——必填；配额约每天 100 次搜索
```

配置 `builtin:web_search` 工具背后的引擎，可按调用经工具的 `engine` 参数选择（`tools.run("builtin:web_search", {query=..., engine="doubao"})`）；省略 `engine` 就用 `search.default`。没有配置任何引擎时，工具如实报告不可用（`{ok:false, unavailable:true}`）。按引擎的 `base_url` 和 `timeout` 是可选覆盖。结果归一为 `{title, url, site, summary, snippet, content, published, score}`；`score` 是引擎原生的（doubao 是 0..1 的相关度，stackoverflow 是原始赞同数，github 是星标数；youtube 没有）。doubao、ollama 和 youtube（以及带检索依据的 google\_search/x\_search）需要 `api_key`；stackoverflow 和 github 不需要（`site` 字段可选任意 StackExchange 站点，例如 `superuser` 或 `serverfault`）。密钥缺失或解析不出来会**跳过该引擎并给出启动警告**，而不是让启动失败；如果被跳过的是 `search.default`，默认值退到唯一还活着的引擎（或没有）。github 搜索仓库，查询里支持 GitHub 的完整限定符语法（`language:go stars:>100 pushed:>2024-01-01`）。youtube 经 YouTube Data API v3 搜索视频（site = 频道标题，content = 描述，`LIVE:` 前缀的片段标记直播）；共享的 `time_range` 映射到 `publishedAfter`/`publishedBefore` —— 注意 search.list 每次调用消耗 100 个配额单位（默认项目配额下约 100 次搜索/天），配额用完会如实报 `quotaExceeded` 错误。

### `legal_search`

```
legal_search:
  default: hklii                # 调用省略 `engine` 时使用的引擎
  engines:
    hklii: {}                   # 香港法律资讯中心——免费，无需密钥
    npc: {}                     # 国家法律法规数据库——免费，无需密钥
                                # （可选的 base_url / timeout 覆盖项）
```

配置两个工具背后的引擎，可按调用经 `engine` 参数选择（默认 `legal_search.default`）；没配置引擎时两者都如实报告不可用。两个引擎的回答方式不同，所以工具描述是按配置了哪些引擎拼出来的 —— 例如 `abbr`/`year`/`num`/`lang` 参数只在有 hklii 时才提供，因为 npc 一个都不读。结果同理：所选引擎不填的字段直接省略，而不是发空字符串，因为空字符串读起来像「未知」，而不是「不适用」。

`builtin:legal_search` 以 `{title, path, url, court, date, type?, effective?, status?, neutral?, parallel?, action?, parties?, coram?}` 返回命中，`total` 是完整匹配数。它的 `disable_fuzzy` 参数（默认 false）在每个引擎上含义不同 —— hklii 上是关键词 vs 概念，npc 上是仅标题 vs 全文。`builtin:legal_read` 取单个文档的完整纯文本，以命中的 `path` 为键，长正文可用 `start`/`max_chars` 分页。配置了多个引擎时，`path` 会被路由到签发它的引擎，所以不必在搜索调用之后再重复 `engine`：命中的 path 是一个只有它自己的引擎能读的不透明令牌，要求调用方再点一次引擎名字，等于在一个运行时已经知道的事情上让它出错。显式点名 `engine` 仍然优先，且绝不会被质疑。

**hklii** 覆盖香港判例法和法例。搜索命中带中性引注和平行引注、法院、日期、当事人和审理法官。读取*仅限判案书*：`path` 形如 `/en/cases/hkcfi/2020/1808`，或给出显式的 `abbr`/`year`/`num`，可选 `lang`（`en`|`tc`|`sc`）。法例命中 —— `court` 读作 `Hong Kong Ordinances`、路径形如 `/en/legis/ord/201/` 的那种 —— 在这里没有可读的全文；它的 path 会被明确报错拒绝，而不是返回错误的东西。

**npc** 覆盖中国国家法律、行政法规和司法解释。命中把制定机关作为 `court`、公布日期作为 `date`，并额外带 `type`（法律/行政法规/…）、`effective`（施行日期）和 `status`（有效/已修改/已废止/尚未生效）；npc 没有引注也没有当事人，所以那些字段缺省。读取只接受命中的 `path` —— 不透明的 `bbbs` 文档 ID —— 别的都不行：`abbr`、`year`、`num` 和 `lang` 在这里没有意义。返回的 `neutral` 是同一个 ID，不是引注。文本经内置的 doc/docx 解析器从返回的 Word 文档中提取。

### `browser`

```
browser:
  account_id: ${CLOUDFLARE_ACCOUNT_ID}   # Cloudflare 账户 id（不是密钥）
  api_token: ${CLOUDFLARE_API_TOKEN}     # 需要 "Browser Rendering - Edit" 权限
  base_url: https://api.cloudflare.com/client/v4   # 可选的端点覆盖
  timeout: 60s                           # 每个请求的时限（默认 60s）
```

配置 `builtin:browser`，它通过 Cloudflare **Browser Run**（原 Browser Rendering）的 Quick Actions 读取页面 —— 一个真正的无头浏览器，所以 JavaScript 渲染的内容在那里是存在的，而裸 HTTP 抓取只能拿到空壳。一个工具，按调用的 `action` 区分：`markdown` 返回 Markdown 化的页面，`content` 返回完全渲染的 HTML，`links` 返回所有链接，`scrape` 返回匹配 `selectors`（CSS）的元素，`json` 返回由 `prompt`/`schema` 描述的结构化数据，`accessibility_tree` 返回无障碍树。`url` 或 `html` 任选其一指定页面；`wait_until`（`load`|`domcontentloaded`|`networkidle0`|`networkidle2`）和 `wait_for_selector` 控制页面何时算加载完成 —— `networkidle` 那几个值正是单页应用能正常工作的关键。`json` 动作在 Cloudflare 侧跑 AI 模型，按 Workers AI 用量计费。

长结果有界：`max_chars`（默认 20000，`0` 表示不限）限制某个动作返回的文本，结果总是报告真实长度 `chars`，被截断时置 `truncated`，模型因此能分辨短页面和被裁剪的页面；`links` 同样限制 500 条。没配置账号时工具如实报告不可用（`{ok:false, unavailable:true}`），而解析不出来的 `api_token` 会**跳过该工具并给出启动警告**，而不是让启动失败 —— 和其他可选组件遵循同一策略。只设 `account_id` 和 `api_token` 其中一个而不设另一个是启动错误，因为 `account_id` 不是密钥，配了一半的段是值得立刻抓出来的错误。

有三个 Quick Actions 刻意不暴露：`screenshot` 和 `pdf` 返回原始字节，而它们目前还没有正确的去处（工具结果是进模型上下文的 JSON，而且整块进去，下游没有任何地方会截断它）；`crawl` 是异步的，返回一个必须轮询的作业 ID。

### `net` —— 出站连接策略

```
net:
  http:
    allow_private: false     # 默认值；true 会停用地址防护
```

用一个设置管辖**两条**出站 HTTP 路径 —— `builtin:fetch` 工具和脚本的 `http.request` 操作 —— 所以部署不会出现一条有防护、另一条没有的情况。

防护会拒绝连向回环、RFC 1918、运营商级 NAT、链路本地（包括 `169.254.169.254` 的云元数据端点）、唯一本地、组播，以及保留/特殊用途网段。它装在**拨号器**里，而不是解析 URL 时检查，这个位置正是关键：检查在 DNS 解析之后、连接建立之前运行，所以它看到的是实际要拨的地址。解析时检查会被 DNS 重绑定击穿 —— 名字被判定一次，等连接建起来时已经解析到别处了 —— 而每个重定向跳转都各自建立连接，所以每一跳都天然被检查到。

两个值得知道的细节。带 zone 的地址（`http://[fc00::1%25en0]/`）会最先被直接拒绝：Go 的前缀匹配匹配不到带 zone 的地址，若不显式拒绝，这种地址会从每条网段规则底下漏过去，落到唯一本地主机上而防护毫不知情。而 v4 映射的 v6 地址（`::ffff:10.0.0.1`）会在检查前被还原，因为它和它的 v4 形式指向同一台主机。

**取舍：** 有防护的传输层有意忽略 `HTTP_PROXY` / `HTTPS_PROXY`。在出口代理后面，拨号器连的是*代理*，防护就会去判断代理的地址，目标根本没被检查过。只经代理出网的部署会发现这些路径不走代理。另外注意 `198.18.0.0/15` 作为基准测试网段被封，而它正是 Clash/mihomo 和 sing-box 在 TUN 模式下的默认 fake-IP 池 —— 在跑了其中任何一个的开发机上，每个经代理的域名都会解析进去并被拒绝。

`allow_private: true` 会彻底关掉防护，并在启动时打一条点名风险的 WARN，所以它不可能被悄悄打开。它是给那些确实要调内部服务的部署准备的；默认关闭，因为防护是提示词注入的智能体和主机自身网络之间唯一的东西。防护只覆盖这两条由智能体控制的路径 —— 模型、搜索、法律和浏览器驱动用的是管理员配置的 `base_url`，刻意不覆盖，好让内部镜像源继续工作。

### `media` —— blob 存储后端

```
media:
  backend: fs                 # fs（默认）| s3
  dir: ./data/media           # fs 根目录（默认：在持久化数据库旁边）
  retention: 168h             # 按作用域的授权保留时长（默认 7 天；0 = 永久）
  derived_ttl: 24h            # 转码派生变体缓存 TTL（默认 24 小时）
  temp_url:                   # 可选的 HMAC-SHA256 临时 HTTP 端点（适用于 fs + QQ）
    enabled: false
    path: /media/t/           # 挂载在 gateway.listen 上的路由前缀（必须以 "/" 开头和结尾）
    ttl: 10m                  # 签名链接有效期（默认 10 分钟）
    secret: ${MEDIA_TEMP_URL_SECRET}  # 单节点可选（每次启动随机生成）；多实例集群必填
  s3:                         # backend: s3 时必填——适用于 AWS S3 和 MinIO
    bucket: my-media
    region: us-east-1
    endpoint: ~               # MinIO/S3 兼容服务时设置（path-style）；留空 = AWS（virtual-hosted）
    public_endpoint: ~        # 可选：当 endpoint 为内网地址时，供预签名 GET 链接使用的公网基础 URL
    prefix: agentflow         # 对象键前缀
    access_key: ${S3_ACCESS_KEY}
    secret_key: ${S3_SECRET_KEY}
```

选择集中式媒体平面，供从渠道注入的字节、LLM 输出（`reply.media`）、MCP 工具和脚本边界使用。不管后端是什么，对象都是内容寻址的（`media:<sha256>` 句柄），所以切换后端时循环和审计日志的形状不变；只有句柄过脚本桥，出站汇（模型与渠道）再把原始字节或签名 URL 落地，并带自动图片转码和派生变体缓存。访问按引擎盖章的作用域（`user:<uuid>` 或 `agent:<name>`）经运行时存储里的授权记录执行，同时保留内容去重；删除用户会清掉他的全部授权。`s3` 后端只说原生 SigV4（不用 SDK）：相同字节的重复写入在 PUT 前用 HEAD 去重，密钥绝不会出现在任何错误里。开 `temp_url.enabled: true`（需要 `gateway.public_url`）时，运行时在 `temp_url.path` 下提供短时效的 HMAC-SHA256 签名 URL，让 QQ 这类只吃 URL 的渠道在本地 `fs` 存储上也能工作，不必上 S3。

### `files` —— 用户范围的文件存储

```
files:
  backend: fs                 # fs（默认）| s3（AWS S3、MinIO、Cloudflare R2）
  dir: ./data/files           # fs 根目录（可选；省略时与 media 共用 blob 存储）
  s3:                         # 字段和解析方式与 media.s3 相同（可选独立存储桶）
    bucket: my-files
    region: auto
    endpoint: https://<accountid>.r2.cloudflarestorage.com
    access_key: ${R2_ACCESS_KEY_ID}
    secret_key: ${R2_SECRET_ACCESS_KEY}
  max_file_bytes: 33554432    # 单个文件上限（默认 32 MiB）
  scratch_ttl: 24h            # 每个会话临时空间的生存期；0 = 永不过期
  gc_grace: 24h               # blob GC 宽限期；0 = 停用 GC
```

支撑 `files.*` 脚本操作（能力 `files`）：带引擎原生快照版本管理的用户范围项目树，外加按会话的临时文件。默认情况下，`files` 没有显式设置 `backend`、`dir` 或 `s3` 段时，它会共用 `media` 的 blob 存储，一个统一的每小时标记清扫 GC 会保护 `files_meta` 引用与活跃媒体授权的并集，超过 `gc_grace` 才回收；设置了 `files.backend`、`files.dir` 或 `files.s3` 则保留一个专用的独立 blob 存储，同时仍让 `media` 作为回退从它读句柄。

文件字节和媒体一样在 blob 存储里内容寻址；快照元数据在运行时存储的 `files_meta` 表里（原生 sqlite，可直接查看）。S3 兼容端点需要真正的 SigV4 密钥对 —— Bearer 令牌说不了 S3。解析不出来的凭据对会在启动时停用存储并给出警告（之后操作如实失败），绝不导致启动失败。

### `audit` —— 消息日志

```
audit:
  enabled: true               # 默认开启
  retention_days: 90          # 清理期限；0 = 永久保留
```

核心把每条消息记进运行时 SQLite 存储的 `message_journal` 表：入站在路由器处（盖上稳定的消息 ID 和 `routed`/`dropped_queue` 状态），出站在每个会话的出站处（`delivered` / `failed` / `blocked_safety`，并关联到它所回复的入站 ID）。行里带渠道、聊天、发送者、智能体、会话、文本、附件描述符、文本与附件大小，以及来源信息。渠道的 `journal:` 设置会收窄它：`metadata` 保留行和大小但不含文本和附件，`off` 对该渠道不写任何行。这是一项信任边界特性：循环和渠道既绕不过也伪造不了它 —— 媒体只按句柄记录，审计链因此保持精简。清理在启动时和每天按 `retention_days` 执行。

### `usage` —— Token 台账

```
usage:
  events: false               # 逐次调用的明细行（默认关闭；每日汇总始终会写入）
  retention_days: 30          # 账本的清理期限；0 = 永久保留
  default_tokens_per_day: 0   # 每个用户的每日上限；0 = 不限制。用户资料自身的上限优先
```

每日汇总（`usage_daily`）喂养每一次配额和智能体预算决策，并留在运行时存储里。配置了日志平面时，逐次调用事件写入日志平面。`GET /admin/api/usage` 和 Studio 的用量页面读的是这份汇总。

### `tools.policy`

```
tools:
  policy:
    default: all          # all | none
    forbidden: [social_post, auto_purchase]
    overrides:
      "builtin:fs.write": { needs_confirm: true }
      "mcp:github/create_issue": { needs_confirm: true }
      "builtin:web_search":
        description: "Search the deployment's runbooks."   # 逐字替换，
                                                           # 或来自 prompts: 的 {prompt: }
        params:                            # 浅合并到 parameters.properties 中
          query: { description: "What to look up, in the user's words" }
        needs_confirm: false               # 策略字段：needs_confirm、permission
        autonomous: true                   #   （cost_level / user_visible / autonomous 会被
                                           #    解析但无人读取——启动时警告）
```

覆盖在启动时一次性地烤进注册表，在所有工具都注册完（内置、legal、shell、MCP）之后、任何智能体集合暴露之前。`params` 条目点名一个 schema 没有声明的参数时会被跳过并给出警告，绝不报错。策略字段按工具生效；`permission: forbidden` 把工具从每个暴露集合里移除 —— 也是其中唯一真正生效的一个。`cost_level`、`user_visible` 和 `autonomous` 会被接受、复制到工具上，然后没有任何地方读取它们；`tools.policy.write` 同样只解析不使用。设置了其中任何一个时，引擎会在启动时警告。

**触达脚本声明的工具**：覆盖也可以点名循环用 `tool.def` 声明的工具 —— 见下文。那些条目不在启动时烤入（代码块加载之前工具并不存在）；它们在循环构建列表时解析，对着活跃的提示词注册表，所以编辑一个基于 `file:` 的提示词，会在下一次 `tools.list()` 时生效。拼错的*提示词键*对两类工具都仍是启动错误。一个既没注册、也没被任何已加载循环声明的工具名，无法在启动时抓到，所以启动时会记一行「保留给脚本声明」（列出这些名字，让拼错在任何智能体跑起来之前就可见），之后在某个循环首次声明工具却没认领它时，按名字警告一次。

### `agents`

```
agents:
  bot:
    loop: ./loop.ts
    model: default
    instructions: ./prompts/bot.md     # 或 {prompt: assistant_system}——见 prompts:
    capabilities: [llm.chat, memory, tools, agent.send, shell.exec, net.mail]
    skills: [builtin:web_search, builtin:fs.read]
    memory: conversational     # 预设，或内联
    safety: default
    shell: sandboxed                    # 可选
    can_contact: [worker]               # ACL
    budget:
      tokens_per_day: 500000
      window: 24h                       # 可选的滚动窗口；省略则每日重置
    channels: [webhook]
    persistent: true                    # 可选——常驻智能体（见下文）
    credentials: [deploy_token]         # 可选——credential.get 允许列表
    inherit_user: false                 # 可选——即使被委托也以无租户身份运行
    extras:                             # 可选——通过 agent.config() 只读访问
      goal: { type: autonomous, success_signal: "PR merged", max_turns: 40 }
      workflow: release-train
```

**能力上限**：`plugins.allow_capabilities`（全局）⊇ `agents.<name>.capabilities`（按智能体）⊇ 派生时授予（子会话）。你无法委派自己没有的能力。智能体省略 `capabilities` 时得到默认值 `[llm.chat, memory, tools, agent.send, net.http, users]` —— 注意 `net.mail`、`shell.exec` 和 `session.state` *不是*默认值，必须显式声明。

**能力在运行时强制执行**，而不只是在启动时校验。每种能力门控其后的脚本操作：`llm.chat` → `llm.*`，`memory` → `store.*`，`tools` → `tools.*`，`apis` → `api.*`，`shell.exec` → `shell.*`，`net.http` → `http.request` 和 `os.env`，`net.mail` → `mail.*`，`files` → `files.*`。省略了某个能力的智能体会被拒绝，消息点名操作、智能体和缺失的能力 —— 操作仍然可达，只是明确失败，而不是消失成 undefined 属性错误。`agent.send`、`agent.request` 和 `agent.spawn` 一直就是这样强制的；其余现在也对齐了。`runtime.*` 和 `credential.get` 刻意不受门控 —— 前者是每个智能体都需要的循环机制，后者已经由智能体自己的凭据列表划定范围。启动警告会逐条列出从每个智能体和方案收回了什么。只有为了让一个依赖「每个智能体都有每个操作」的配置继续跑，才把 `plugins.enforce_capabilities: false`（默认 `true`）。

**`builtin:` 是工具命名空间。** 以前有四种词汇都用它来拼 —— 循环/插件名、工具名、记忆提供商名和内置记忆方案 —— 而字符串本身说不出一个名字属于哪一种。现在各自有自己的写法，前缀的含义因此变得唯一：

- **工具**保留它：`builtin:web_search`、`builtin:fetch`、`builtin:browser`、`builtin:legal_read`；
- **循环和插件**是 `plugin:<name>`：`plugin:per_chat`、`plugin:recency`、`plugin:semantic`。这也彻底解决了「循环还是路径」的问题，因为路径永远不会以 `plugin:` 开头 —— 而在以前，一个名为 `per_chat` 的文件是无从触达的；
- **记忆提供商**是裸名：`sqlite`、`qdrant`、`redisvector` —— `provider:` 键本身已经说明了这是什么名字；
- **内置记忆方案**是 `conversational`，这个名字现在被保留：`profiles.memory` 里同名条目是启动错误，而不是一个永远跑不起来的方案。

旧写法会按名字被识别，而不是报成未知，所以错误会告诉你该写什么：`loop:` 写 `builtin:per_chat` 会得到 `loops are spelled plugin: now`，提供商写 `builtin:sqlite` 会得到 `providers are unprefixed now`。另两种错误也会报 —— `loop:` 里放了*工具*名会让启动失败，并提示它该待在哪个字段；`skills:` 里放循环名会给出警告，因为那一种否则是静默的（匹配不到任何工具，被丢弃，智能体看起来就没有工具）。完全不匹配任何已注册工具的 `skills:` 条目*不会*被报，因为循环完全可以列出它自己用 `tool.def` 声明的工具。

**方案 extras 与凭据**：`extras` 是部署特有的、只读的方案数据，由 `agent.config()` 呈现给循环（`goal` 块会提升为顶层 `goal` 键，其余落在 `extras` 下）。*恰好*是密钥引用的值 —— `${VAR}`、`${VAR:-default}` 或 `cred:<service>` —— 在这个面上永远不会被解析：configdir 路径下它们保持原样，渲染成不透明标记（`{env="VAR"}` / `{cred="service"}`），所以解析后的密钥到不了循环（也到不了派生的子会话）；只是嵌入引用的字符串（`prefix-${VAR}`）是普通字面值，照常展开。`credentials` 是 `credential.get()` 的白名单：智能体只能取它在这里点名的已存储密钥；空或缺失 = 无权限。两个字段对派生方案同样生效，并到达每个子会话。

**常驻智能体**：会话通常按需在第一条消息时派生。`persistent: true` 的智能体在运行时启动后（渠道注册之后）会收到一条合成的 `{ type = "boot", from = "system:supervisor" }` 消息，所以 cron 调度器和队列消费者无需外部流量就能起来。

**租户范围跟随委派**：智能体跳转（`agent.send` / `agent.request` / `agent.reply`）会把发送轮次的租户带在消息由核心盖章的来源信息上，接收方以该租户身份行事 —— 所以为一群人服务的智能体集群会在每一跳写这个人的文件、记忆和 Shell 范围，而不是一个全集群共享的范围。这个戳取自发送方由引擎赋予的身份，绝不取自 payload，所以循环无法点名另一个租户（`payload.user_uuid` 无论谁写的都被忽略）。链条只能衰减：智能体拿到的是给它的那个租户，或者没有。`inherit_user: false` 让智能体退出跨跳继承 —— 它在共享的 `service` 层里无租户运行 —— 而一个人直接跟它说话时，仍会在自己的范围下触达它。派生方案上同样可用。

**预算**：`tokens_per_day` 用 reserve/commit/release 把 LLM 调用包在一个 Token 池外面。默认每天重置；设 `window`（例如 `24h`、`168h`）则按滚动窗口记账，用量随提交逐步超出窗口而持续释放 —— 没有午夜悬崖。会超预算的调用被拒绝（计入 `agentflow_budget_denied`）。

### `prompts` —— 部署级提示词注册表

```
prompts:
  assistant_system:
    file: ./prompts/assistant.md       # 相对于 configdir 解析
  distiller:
    inline: "You are a memory distiller..."   # 字面值（text: 是别名）
  kb_header:
    inline: "[Knowledge base — recalled facts with provenance]"
```

一个放系统提示词、工具提示词、注入提示词和蒸馏文本的地方，省得在脚本里硬编码或在多个文件里重复。每个条目只能写 `file:`、`inline:` 或 `text:`（`inline:` 的别名）其中之一；读不到的 `file:` 会让启动失败，方案或工具覆盖点名了未定义的键也会让启动失败 —— 缺失的提示词绝不会降级成静默的空字符串。提示词文件在加载时读取（变更时重读），相对 configdir 解析（单文件配置相对配置文件本身解析）。解析后的文本以只读方式呈现给每个循环的 `agent.config().prompts`（没声明提示词时为空对象），按同样的名字索引。

**引用提示词**：智能体或派生方案上的 `instructions` 接受文件路径（和以前一样）或 `{prompt: <key>}`，后者让注册表文本成为该智能体的系统提示词。工具覆盖接受同样的形式作为描述：`description: {prompt: <key>}`，与现有的字面值字符串并存。基于 `file:` 的条目是*实时的*：编辑文件会把文本重新发布到每个循环的 `agent.config().prompts`，并原地刷新任何源自该键的智能体的系统提示词，无需重启会话（读不到的文件会警告并保留已发布的文本）。`prompts:` 段本身不是实时的 —— 新增键、把某个键改指向别的文件，或在 `inline:` 与 `file:` 之间切换，仍需重启。

### `plugins`

```
plugins:
  dir: ./plugins
  allow_capabilities:           # 全局上限——任何东西都不能超过它
    - llm.chat
    - memory
    - tools
    - scheduler
    - shell.exec
    - net.http
    - net.mail
    - channel.push
    - agent.spawn
    - agent.send
    - agent.request
  enforce_capabilities: true    # 默认值；false 恢复强制执行之前的行为
```

`plugins.dir` 也会**按名字覆写内置实现**：文件 `<dir>/<name>.ts`（或 `.js`，例如 `./plugins/recency.ts`）会在所有地方替换同名内置实现 —— 循环/路由引用（`loop: plugin:recency`，包括热重载监听路径）以及加载进每个会话的支持片段。覆写文件受监听：被覆写的支持片段在每次会话启动时重读（第一次以及每次重载重启），编辑它会重启所有活跃会话，所以无需重启进程即可生效。未被覆写的片段是内嵌的，没有文件可改。

### `triggers` —— 定时与事件任务

```
triggers:                       # 顶层，或配置目录中的 triggers/*.yaml
  - name: morning-digest
    cron: "30 9 * * 1-5"        # 类型 = cron（5 个字段）
    every: 15m                  # 类型 = every（ms、s、m、h、d）
    event: { channel: webhook, match: urgent }   # 类型 = event
    run_on_boot: false          # 启动时触发一次，之后按计划执行
    target: { profile: pm }     # 一个已配置的智能体，或一个派生方案
    payload: { topic: news }    # 原样复制到触发的消息中
```

**`every:` 和 `cron:` 触发器由引擎执行**，不是由智能体执行：到期时运行时会把一条普通消息入队到目标邮箱 ——

```
{ id = "cron:<trigger-name>:<unix>", type = "cron", from = "system:scheduler",
  payload = { ... }, ts = ..., provenance = { kind = "scheduler", principal = "trigger:<name>" } }
```

—— 进入会话 `<target>|<trigger-name>`。没有任何智能体为此持有能力，也没有循环需要计时器：排期是引擎基础设施，所以部署可以删掉它以前养着的用户态调度智能体。投递总会计入日志并计数（`agentflow_trigger_fires`），并以 `info` 级别触发（`debug` 带下一次计算的触发时刻）。

- `every:` 接受 `ms`、`s`、`m`、`h` 或 `d` 单位的时长，允许复合和小数（`30s`、`15m`、`6h`、`1d`、`1.5h`、`2d12h`）。首次触发在启动后一个间隔（或 `run_on_boot` 时立即）；之后以那一次为锚，所以不会漂移。最短间隔 100ms。
- `cron:` 用经典的 5 字段（`minute hour day-of-month month day-of-week`），支持 `*`、`N`、`N-M`、`*/N`、`N-M/N` 和逗号分隔列表。星期接受 `0`–`7`（`0` 和 `7` 都是周日）。当 day-of-month *和* day-of-week 都受限时，匹配*任一*即触发（经典规则）。不支持名字（`mon`、`jan`），也没有秒字段。
- `cron:` 字段按 `runtime.timezone_offset_hours` 匹配（固定 UTC 偏移，默认 0 = UTC）。它是偏移不是命名时区，所以排期不会因别的区域的夏令时而漂移 —— `30 9 * * 1-5` 配 `8` 就是全年每天 UTC+8 的 09:30。
- `target.profile` 点名一个已配置的智能体或一个派生方案。派生方案解析为 `spawn:<profile>` 地址 —— 一个用该方案的循环、模型和授权运行的顶层会话（没有父级，也没有对父级的能力衰减）。
- `event:` 触发器**不**参与排期：路由器和循环自己匹配它们。`runtime.triggers()` 仍返回整个合并列表，形状不变，所以读它的循环照常工作。
- 排不上期的触发器 —— 表达式格式错误、间隔低于下限、或 `target.profile` 既不是智能体也不是派生方案 —— 会**记日志并跳过**，其余的照常运行。一个配置笔误只让你丢一个触发器和一行警告，绝不丢一次启动。
