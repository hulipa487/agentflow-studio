## 配置指南

空部署是有效的：`agents:` 可以完全省略，此时运行时会作为管理 / Studio 控制平面启动，以便从 Studio 创建第一个智能体。渠道、触发器和其他引用仍必须指向已存在的智能体。

AgentFlow 通过单个 YAML 文件（`agentflow.yaml`）进行配置。每个值都会在启动时校验——未知的键会导致启动错误并给出文件和行号，绝不会在运行时才冒出来。凡是接受字符串的地方都支持环境变量插值（`${VAR}` 和 `${VAR:-default}`）。

### 密钥：环境变量引用、凭据存储与如实降级

含密钥的字段（`models.*.api_key`、`search.engines.*.api_key`、`gateway.channels[].token`/`secret`、`memory.backends.*.config.url`/`password`/`api_key`、`media.s3.access_key`/`secret_key`、`browser.api_token`、`profiles.shell.*.password`）接受三种形式：

- 字面值；
- `${VAR}`（或 `${VAR:-default}`）——从进程环境中解析，找不到时回退到凭据存储中同名的条目；
- `cred:<service>`——只从凭据存储中解析。

在 **configdir** 路径下，这些引用保持惰性，在使用方构造时才解析；在单文件 `-config` 路径下，整个文档在加载时按字节展开（旧行为，未改变）。解析顺序为：环境变量 → 凭据存储 → 无法解析。降级策略是统一的：*可选*组件（搜索引擎、telegram/ghhook 渠道、记忆后端、S3 媒体存储）上的凭据无法解析时，会**跳过该组件并输出一条点名该凭据的警告**——绝不会导致启动失败。模型的 `api_key` *不是*可选的：该模型仍保持配置状态，第一次 LLM 调用会失败，并给出点名未解析凭据的明确错误。`runtime.credentials.enabled=false`（或没有凭据存储）会使 `cred:` 引用无法解析，同样适用上述策略。

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

YAML 定义*拓扑*——有哪些东西、它们如何连接。行为放在 Lua 插件中，参数放在 YAML 中。这种分离意味着一个可用的聊天机器人只需约 12 行 YAML、零代码，而流水线上的每个环节仍可被覆盖。

> **严格解析**：`KnownFields(true)` 意味着每个未知的键都是启动错误。不存在被“忽略”的拼写错误。

### 配置目录（`-config <dir>`）

除了单个文件，也可以向 `-config <dir>` 传入一个目录来加载目录布局；只包含一个 `agentflow.yaml` 的目录（即 `agentflow init DIR` 生成的内容）会加载该文件。每个片段都按同样严格的校验解码，并且校验在合并*之后*进行，因此跨文件引用的检查方式与单文件完全相同。配置来源——文件所在目录或目录本身——是所有相对路径的解析基准（循环、指令、提示词 `file:` 路径、路由器路由、持久化、sqlite 后端路径、shell 的 `key_file`、`plugins.dir`、`media.dir`）。单文件同样如此：`agentflow run -config deploy/agentflow.yaml` 会在该文件旁边解析 `./loops/x.lua`，与 shell 的工作目录无关。

```
<dir>/
  system.yaml       # 必需：除 agents/channels 之外的顶层配置
  channels.yaml     # 可选：channels: [...]
  profiles/*.yaml   # 可选：每个文件一个智能体（按 glob 排序）
  triggers/*.yaml   # 可选：triggers: [...]（排序后合并，后面的文件优先）
```

`profiles/*.yaml` 文件是一份智能体配置，外加 `name` 和 `spawn` 标志：`spawn: false`（或不写）将它归入 `agents`，`spawn: true` 将它归入 `profiles.agent`（一个派生模板，包括它的 `extras` 和 `credentials` 允许列表——派生出的子智能体两者都能看到）。在派生方案上设置仅限智能体的字段（`persistent`、`singleton`、`taps`、`channels`、`lifecycle`）会导致启动错误。触发器的形式为 `{name, event{channel, match} | cron | every, run_on_boot, target{profile}, payload}`——`every:` 和 `cron:` 触发器*由引擎*执行（见下文 `triggers`），合并后的列表通过 `runtime.triggers()` 作为数据提供给循环。目录部署会轮询重新读取 `triggers/*.yaml`，因此修改计划无需重启即可生效。

## 完整配置参考

### `runtime`——实例级调优

| 键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `vm.memory_limit` | string | — | 限制每个 Luau 状态的总分配量（`b`/`kb`/`mb`/`gb`；留空 = 不限制）。会加到每个新会话上；超额分配表现为 Luau 内存不足错误。与 `limits.sessions_per_tenant` 一起，可限定一个租户的 VM 占用 |
| `vm.instruction_budget` | string | — | **已弃用——无效果。** 每次恢复执行的预算固定为 5,000,000。设置时会发出警告 |
| `scheduler.workers` | int | 8 | 操作池大小。这是唯一设置它的地方 |
| `reload.watch` | bool | — | **已弃用——无效果。** 重新加载监视器始终运行。设置时会发出警告 |
| `persistence` | string | sqlite://./data/agentflow.db | 消息日志、Token 账本及其逐次调用明细，以及引擎的键值行（文件树、会话状态、路由状态、shell 句柄）存放的位置。调度器定时器*不*在这里：定时器服务在内存中，因此重启会丢失所有定时器，直到其循环重新注册 |
| `topology` | string | single | 部署层级：`single`（单进程）或 `regional`（同一区域内共享一个 PostgreSQL 的多个实例）。未设置即为 `single`。层级决定了其他哪些设置有效；启动时会拒绝它无法满足的设置。见[拓扑](#topology) |
| `zone` | string | "" | 区域内的可用区标签（字母、数字和 `. _ -`，最多 32 个字符）。在指标和 `/admin/api/state` 中报告；它只是一个标签，绝不用于路由决策 |
| `shutdown_grace` | duration | 5s regional · 0 single | 关闭时先报告未就绪并继续服务多长时间再关闭监听器，以便负载均衡器排空流量、进行中的轮次得以完成。`0` 表示不等待 |
| `sessions.idle_unload` | duration | 30m | 对话无活动这么长时间后将其卸载。其 Lua 局部变量会丢失；`session.state.*` 保留，下一条消息会重新创建会话。会话 id 不会作废，其租约会被释放。`0` 表示不卸载 |
| `admin.cors_origins` | list | \[\] | 允许调用 `/admin/api/*` 的精确浏览器源（自定义前端或其他浏览器客户端）。原生 Studio 桌面应用直接发送令牌，不需要它。留空则 API 仅限同源；`"*"` 会被拒绝。预检请求在 bearer 检查之前应答 |
| `admin.socket` | string | "" | 添加一个仅限本地的管理监听器：Unix 上为 unix 套接字路径（相对路径相对于配置目录解析），Windows 上为命名管道，例如 `\\.\pipe\agentflow-admin`。仍然需要 bearer 令牌 |
| `admin.tcp` | bool | true | 在 HTTP 监听器（`gateway.listen`）上提供管理平面，路径为 `/admin/` 以及 `/healthz`、`/readyz` 和 `/metrics`。`false` 则只在 `admin.socket` 上提供；必须至少保留一种传输方式。没有 `admin.listen`：管理平面没有自己的端口，设置它的配置会被拒绝 |
| `admin.operators` | map | {} | 具名的管理令牌（`name: token`），每个都与 `ADMIN_TOKEN` 具有相同权限。请求在日志中归属到该名称。值可以是 `${VAR}` 或 `cred:<service>` 引用，在启动时解析；无法解析的会被跳过并发出警告 |
| `limits.sessions_per_tenant` | int | 0 | 在监督器的派生边界（路由的首次接触和 `agent.spawn`）上限制每个租户的运行中会话数。0 表示不限制 |
| `limits.ephemeral_per_tenant` | int | 16 | 限制每个用户的并发单轮会话数（OpenAI 渠道的默认会话类型）。与 `sessions_per_tenant` 分开计算，因此无状态流量不会占用对话名额。它需要有用户可计数，所以没有用户的请求（`auth: key` 或 `none` 的 `openai` 渠道）不受它限制。0 表示不限制 |
| `limits.egress_per_minute` | int | 0 | 限制每个租户每分钟的 `http.request` 和邮件操作次数（固定窗口）。0 表示不限制。租户之间的操作池公平调度始终开启，不可配置 |
| `studio.enabled` | bool | true | 开放 `/admin/api/studio/*`（构建器、演练场、追踪）。没有管理令牌时该 API 不起作用；`false` 时返回 503 |
| `studio.workspace` | string | <config dir>/studio | 单文件部署写入 Studio 循环和提示词的位置。目录部署则写入它自己的 `loops/`、`prompts/` 和 `profiles/` |
| `studio.trace_events` | int | 2000 | 每个会话为追踪时间线保留的事件数（有界环形缓冲，丢弃最旧的） |
| `studio.trace_sessions` | int | 200 | 保留的会话追踪环数量；超出时驱逐最久未活动的 |
| `studio.trace_payloads` | bool | false | 在追踪事件中包含消息、回复和结果文本。元数据（操作、模型、耗时、Token、错误）始终保留 |
| `studio.chat_timeout` | duration | 120s | 一次演练场轮次最多等待多久，超时后带着部分回复和 `timed_out` 返回 |
| `identity.enabled` | bool | false | 为每条入站消息生成稳定的用户 UUID；将 `from` 改写为 `user:<uuid>`，从而启用 `session.push_user`。开启它会重新划定现有状态的范围——见表格下方的说明 |
| `identity.persistence` | string | <persistence dir>/identity.db | 身份注册表的 sqlite 路径 |
| `credentials.enabled` | bool | false | 加密的按租户凭据存储，为 `auth={service=...}` 提供支持 |
| `credentials.path` | string | <persistence dir>/credentials.db | 凭据存储的 sqlite 路径 |
| `credentials.master_key_env` | string | CREDENTIALS\_MASTER\_KEY | 存放主密钥的环境变量（启动时读取，绝不从文件读取） |
| `credentials.engine_wide` | string | allow | 控制 `credential.get()`，即把原始密钥返回给 Lua 的操作。`allow`（或留空）允许智能体读取其 `credentials:` 列表中列出的项；`maintenance` 将其限制为引擎触发的轮次（启动、调度器、触发器），因此租户轮次永远看不到引擎全局密钥。按租户的路径——`http.request` 和 `tools.http` 上的 `auth = {service = ...}`——永远不会返回该值 |
| `users.enabled` | bool | false | 在网关监听器上提供公开的 `/v1/users` API（需要 `identity.enabled`） |
| `users.registration` | string | open | `open` 或 `invite`（来自 `POST /admin/api/users/invites` 的一次性邀请码） |
| `users.require_registration` | bool | true | 在未关联的关联账号绑定到用户资料之前，将其保留在共享服务层（没有个人范围，没有配额身份）。`false` 则在首次接触时自动认领一份用户资料 |
| `users.unregistered` | string | allow | 没有用户资料的发送者如何处理：`allow` 继续在共享层为其服务；`reject` 丢弃入站消息并要求其注册或关联 |
| `users.link_ttl` | duration | 10m | 渠道关联码的有效时长 |
| `users.push_same_tenant` | bool | false | 让 `session.push_user` 拒绝当前轮次租户以外的目标。多租户聊天产品应设置它 |
| `users.jit_provisioning` | bool | true | 在经过验证的首次 OIDC 登录时创建用户资料；`false` 则拒绝未知主体，直到操作员为其开通 |
| `users.oidc.issuer` · `audience` · `jwks_url` | string | — | 身份提供商验证（见下文*身份提供商*） |
| `users.oidc.subject_claim` · `email_claim` · `name_claim` | string | sub · email · name | 携带身份及所开通用户资料字段的令牌声明 |
| `cluster.persistence` | string | `persistence` | **已弃用——设置时会发出警告。** 租约和会话收件箱现在始终存放在 `runtime.persistence` 中。在区域单元中，除非它指向同一目标，否则会被拒绝 |
| `cluster.session_ttl` | duration | 30s | 会话认领在未续期的情况下能存活多久 |
| `cluster.poll_interval` | duration | 250ms | 实例多久查找一次等待其所拥有会话的消息——即消息到达错误实例时增加的延迟 |
| `region` | string | "" | 本实例服务的区域名称：作为指标标签 `region`、`/admin/api/state` 中的一个字段，以及实例注册表的一部分。一个区域是一个独立单元，而不是全局部署的一个分片 |
| `log_plane.persistence` | string | "" | 仅追加平面——消息日志和逐次调用的用量明细——所在的目录，按 UTC 日期每天写一个 JSONL 文件。留空则两者都保存在运行时存储中 |
| `timezone_offset_hours` | float | 0 | 匹配 cron 触发器所用的 UTC 偏移（例如 `8`、`-5.5`）；不影响 `every:` |

> **开启身份层会重新划定所有内容的范围**：`identity.enabled` 不是一个表面开关。开启后，每条入站消息的 `from` 都会从渠道原生发送者改写为稳定的 `user:<identity>`。个人范围随后才出现：一旦关联账号绑定到用户资料——或在首次接触时认领一份，这只在 `runtime.users.require_registration: false` 时发生——引擎就会在消息由核心拥有的 `provenance`（循环读取的字段；它永远不会写入载荷，因为委托跳转会从 Lua 重建载荷）上标记 `user_uuid`，用户范围的子系统正是依据这个标记进行解析。随后文件存储的范围从 `agent:<agent>` 变为 `user:<uuid>`，一个轮次的记忆从共享服务层移到交互层，按用户的配额也有了可计费的账户。在关联账号绑定之前，它完全没有个人范围：只有 `from` 变了。循环中凡是以 `from` 为键或用它组成自定义范围的内容，都会随之移动。
>
>  **不会进行任何迁移，引擎也无法得知哪些内容移动了。** 在旧标识下写入的行——文件、记忆、循环自己作为键的任何内容——都不会被接管，因为无法知道昨天的 `agent:bot` 属于现在的 `user:u_xxxx` 这个人。所以请在真实流量到来之前开启身份层，或者自己编写接管逻辑：一个把旧标识的行复制到新标识的循环，在切换时运行一次。反之亦然——关闭它或解除关联账号，都会把个人范围留在原处。

管理服务器提供 `GET /healthz`、`GET /readyz`、`GET /metrics`（Prometheus 文本格式）、`GET /admin/sessions` 和 `/admin/credentials`。（会话列表以前是 `/v1/sessions`：该前缀属于公开的用户 API，而这个私有监听器不提供它。）实时计数器包括 `agentflow_sessions_active`、`agentflow_children_spawned`/`died`、`agentflow_llm_calls`、`agentflow_llm_tokens`、`agentflow_budget_denied` 和 `agentflow_trigger_fires`。

### 拓扑

`runtime.topology` 选择部署形态，而形态决定了哪些设置有效。它之所以是一个设置而不是十几个，是因为否则就可能出现配置了一半却看似健康的部署：一个使用本地 blob 目录的区域集群会让每个实例各自保存一份数据，而且没有任何报错。启动时会拒绝这些组合，`agentflow validate` 会在启动前就拒绝它们。

#### `single`

默认值，也是该设置出现之前编写的所有配置。单个进程；之前有效的一切仍然有效：

| 后端 | `single` |
| --- | --- |
| 运行时、身份、凭据、租约、收件箱 | SQLite 或 PostgreSQL |
| 记忆：`sqlite`、`postgres`、`pgvector`、`qdrant`、`redis`、`redisvector`、`volatile` | 全部允许 |
| Blob（`media`、`files`） | 本地目录或 S3 |
| `runtime.log_plane.persistence` | 允许 |

同一存储上若有第二个运行中的进程，启动时会被拒绝，并给出对方的 id 和最后在线时间。`-force-single` 会强行启动；仅在崩溃后使用，或者等待 30 秒的存活窗口过去。

单实例不安装会话中枢：没有第二个所有者可以接手对话，因此不会写入 `session:` 租约行和共享收件箱行。`-force-single` 会保留中枢——使用它的操作员等于在声明两个进程可能共享该存储，此时租约就是防止同一对话运行两次的唯一保障。

#### `regional`

同一区域内跨多个可用区的多个实例，共享一个 PostgreSQL。启动时会拒绝：

| 设置 | 拒绝原因 |
| --- | --- |
| `runtime.persistence` 不是 PostgreSQL | 每个实例会各自保存一个数据库 |
| `runtime.identity.persistence` / `runtime.credentials.path` 设为 SQLite | 每个实例会各自保存身份或凭据。取消设置以跟随运行时存储，或将它们指向 PostgreSQL |
| `runtime.cluster.persistence` 指向不同的目标 | 租约和收件箱始终存放在运行时存储中 |
| 任何 provider 为 `sqlite` 的 `memory.backends` 条目 | 每个实例会各自保存对话历史 |
| `media.backend` 或 `files.backend` 不是 `s3` | 每个实例会各自保存 blob |
| 设置了 `runtime.log_plane.persistence` | 本地目录只属于一个进程 |
| 未设置 `ADMIN_TOKEN` 且 `runtime.admin.operators` 为空 | 每次启动生成的令牌其他对等实例都不知道，因此不会生成（在启动时检查，因为它是环境设置） |

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

Redis 是一个区域范围的集群（locality 为 `region`）；`volatile` 仍是实例本地的。每个实例每 10 秒向 `instances` 表写入一行心跳，只要最近一次心跳在 30 秒内就视为存活；`/admin/api/state` 报告 `topology`、`zone` 以及每个存活的 `instances` 条目，并标出 `config_epoch` 不同的对等实例。Studio 的“概览”显示同样的列表。

多区域部署就是每个区域一个独立的 `regional` 单元——不存在全局层级。见[多区域](multi-region.md)。

`agentflow doctor -online`（区域配置的默认方式）检查在线的一侧：它打开运行时存储，确认每个 pgvector 后端上都有 `vector` 扩展，在每个 S3 存储桶中放入 / 读取 / 删除一个探测对象，并 ping Redis 和 Qdrant。每一行都是 PASS/FAIL 并附带修复方法。

### 命令行

`agentflow help` 列出所有命令，`agentflow help <command>`（或 `<command> -h`）打印某个命令的标志。这些帮助由二进制文件自身的标志定义生成，因此是始终最新的参考。

`agentflow run [flags]`，或不带命令的 `agentflow [flags]`，运行引擎：

| 标志 | 默认值 | 含义 |
|---|---|---|
| `-config PATH` | `agentflow.yaml`，其次 `config.yaml` | 一个文件、一个配置目录，或包含 `agentflow.yaml` 的目录 |
| `-log-level` | `info` | `dev`、`debug`、`info`、`warn` 或 `error` |
| `-log-format` | `text` | `text` 或 `json` |
| `-log-file PATH` | | 同时将日志追加到此文件 |
| `-no-tui` | 关 | 不显示终端仪表板；在 stderr 上输出纯日志 |
| `-no-console` | 关 | 提供管理 API 但不提供网页控制台 |
| `-force-single` | 关 | 即使存储上似乎有另一个实例在运行也照样启动（仅限 `single`；用于崩溃后） |

所有命令遵循相同的约定：

- 标志可以放在位置参数之后（`agentflow cred set NAME -user u_1`），`-config` 可以放在命令之前（`agentflow -config deploy/ validate`）。
- 命令作用的对象是一个位置参数：`init [DIR]`、`user export USER`、`user import BUNDLE.zip`、`user keygen [PREFIX]`、`migrate blobs DIR`、`cred set NAME`。
- `-out FILE` 写入文件；不带它时输出到 stdout。唯一的例外是 `support-bundle`，它默认写入一个带时间戳的 zip。
- `-json` 在 `doctor`、`config show`、`cred get` 和 `cred list` 上打印机器可读的输出。
- 不适用于某个命令的标志会报错，绝不会被静默忽略。
- `user export` 和 `user import` 依次从 `-url`、`$AGENTFLOW_URL`、配置的 `gateway.listen`、`http://127.0.0.1:8080` 查找管理 API；令牌为 `-token` 或 `$ADMIN_TOKEN`。

工作池大小由 `runtime.scheduler.workers` 设置（没有对应的标志）。已移除的标志（`-configdir`、`-workers`、`-log-json`、`-no-webui`、`init -dir`、`user export -id`、`user keygen -out`、`migrate blobs -from`、`cred -format`）会被拒绝，并给出指明替代项的提示。

### 凭据 CLI

`agentflow cred set|get|list|delete <name>` 管理已加载配置（`-config`）所指定存储中的引擎全局凭据。`set` 从交互式提示或 stdin 读取值——绝不从 argv 读取（用管道传入：`printf %s "$TOKEN" | agentflow cred set github_token`）。主密钥来自 `credentials.master_key_env` 所指定的环境变量（默认 `CREDENTIALS_MASTER_KEY`）；未设置该密钥会给出明确的错误。`list` 只打印名称，从不打印值。`credential.get()` 和 `cred:<service>` 配置引用解析的正是这些记录。

### 迁移部署

从单实例到区域单元的层级变更是一次数据搬迁，`agentflow migrate` 无需任何转换即可完成：两种后端使用相同的 SQL 方言，blob 按内容寻址。

```
agentflow migrate store -from sqlite://./data/agentflow.db -to postgres://...
agentflow migrate store -from <src> -to <dst> -tables files_meta -append
agentflow migrate blobs ./data/media -kind media -config agentflow.yaml
```

`migrate store` 复制运行时平面、身份注册表和凭据存储。除非传入 `-append`，否则它会拒绝非空的目标，并报告哪些表已复制、哪些不存在。凭据以密文形式复制：目标必须使用相同的主密钥，否则所有密钥都会解密失败。`migrate blobs` 遍历本地的 media 或 files 目录，把每个 blob PUT 到 `-kind` 所指定的 S3 存储桶中；由于按内容寻址，重新运行时会先 HEAD 已存在的对象，只上传缺失的部分。两个命令都可以安全地中断并重新运行。

### 用户 API

引擎**不附带任何用户界面**。它在 HTTP 渠道所用的同一个公开监听器上提供一个纯 JSON 的用户资料 API（`runtime.users.enabled`，需要 `runtime.identity.enabled`），这样前端可以放在自己的仓库中，按自己的节奏部署。接口契约以 OpenAPI 形式发布在 `openapi/users.yaml`。

- **注册**——`POST /v1/users/register` 创建一份用户资料，并*仅返回一次*它的 API 令牌。`runtime.users.registration: invite` 使其仅限邀请注册，邀请码由操作员通过管理 API（`POST /admin/api/users/invites`）签发。
- **渠道关联**——`POST /v1/users/me/links` 返回一个一次性代码；用户*从要关联的账号*发送 `/link <code>`，引擎在该消息到达时完成关联。只有用户资料的所有者能看到代码，只有该账号的所有者能从该账号发送消息，因此任何一方单独都无法完成关联。携带代码的消息永远不会到达循环。只有平台对发送者身份做了认证的渠道才能关联——目前是 telegram；webhook 的自报发送者和 ghhook 的固定发送者都不行。
- **自助服务**——`/me`（用户资料和关联账号）、`/me/usage`（今天、七天窗口以及配额状态）、`/me/projects`（调用者的文件项目）、用于解除关联的 `/me/links/{id}`，以及用于作废泄露令牌的 `/me/token/rotate`。
- **认证始终使用 bearer 令牌**——要么是部署所用身份提供商签发的访问令牌，要么是 `POST /register` 返回的 `afu_…` API 令牌。没有 cookie，也没有服务器端会话，因此没有可供其他站点重放的环境凭据，也不需要携带 CSRF 令牌。
- **跨源**——在操作员于 `runtime.users.cors_origins` 中列出源之前，CORS **一直关闭**；只有列出的源会得到应答，并且有意从不发送 `Access-Control-Allow-Credentials`：请求携带的是 bearer 令牌而不是 cookie，所以浏览器没有可借给其他源的环境权限。`GET /v1/users/config` 是公开的，它告诉前端注册是否开放、可以提供哪些渠道，以及本单元服务的是哪个区域（`runtime.region`）。

**身份提供商。** 设置 `runtime.users.oidc.issuer`（外加 `audience`，在无法使用发现机制时还需 `jwks_url`），引擎就会验证该提供商签发的令牌：先用签发方的 JWKS 验证签名，再检查 `iss`/`aud`/`exp`，然后把主体声明作为身份——以 `<issuer>|<sub>` 为键，因此两个签发方不会冲突。登录、密码、MFA 和会话有效期都由该提供商负责；引擎只做验证。经过验证的首次登录会根据令牌的 `email`/`name` 声明开通一份用户资料，除非设置了 `jit_provisioning: false`，此时未知主体会被拒绝，直到操作员为其开通。只接受非对称算法，并且启动时必须能访问签发方。

**部署说明。** `POST /register` 按客户端地址限流，因此位于反向代理后面的前端也必须在代理处限流——否则所有调用者共用一个地址。

### 网页控制台

管理服务器还在 `http://<gateway.listen>/admin/` 托管一个内嵌的**网页控制台**（默认开启；`-no-console` 会停用这些静态文件）——一个面向操作员的单页界面，无需构建步骤或单独的端口。它有四个标签页：

- **Dashboard（仪表板）**——实例信息、运行中的会话、智能体 / 渠道、基于凭据存储的 API 密钥管理器（按用户创建 / 撤销，密钥只能写入），以及流式日志尾部。
- **Models（模型）**——列出每个具名模型（密钥脱敏为末 4 位指纹），可编辑 / 添加 / 移除，还有一个*测试*按钮，通过运行中的管理器发起一次 1 Token 的调用并对结果分类（auth / not\_found / unreachable / …）。编辑会立即**热应用到正在运行的进程**，之后显式点击 *Persist to config*（持久化到配置）把它们写入文件（备份保存为 `.bak`），点击 *Revert*（还原）则从文件重新加载。只要运行时与文件不一致就会显示偏差标记——仅针对本实例：比较的是本进程的模型集合与文件，因此在集群中每个控制台看到的都是各自的偏差。这里的编辑会立即应用到**本实例**；其他实例在重启前保持当前模型，因此要在整个集群中生效，应先 *Persist to config*，再滚动重启。
- **Config（配置）**——原始 YAML 编辑器，带有与启动等效的 *Validate*（校验：严格字段、环境变量展开、交叉引用——错误会指出行号），先备份再原子写入的 *Save*（保存；如果文件在你编辑期间被磁盘上的改动修改则拒绝），以及脱敏的结构化视图。保存后会提示重启；运行时绝不会静默地热重载拓扑。
- **Metrics（指标）**——以迷你折线图显示计数器，在进程内采样（每 5 秒一次，约 2 小时），因此无需外部 TSDB；`/metrics` 仍是 Prometheus 的集成点。

**一个端口。** 管理平面与渠道共享 `gateway.listen`：`/admin/`（控制台、`/admin/api/*`、`/admin/sessions`、`/admin/credentials`）、`/healthz`、`/readyz` 和 `/metrics`。位于这些路径之下的渠道路径会被拒绝，`/health` 和 `/v1/users/` 同样如此。

**认证：** 每个控制台 API 路由（以及 `/metrics`、`/admin/sessions`、`/admin/credentials`）都需要 bearer 令牌——即使来自回环地址也一样。设置 `ADMIN_TOKEN`（或配置 `runtime.admin.operators`）来固定令牌；否则每次启动会生成一个随机令牌，并在日志中打印一次。对于能读取该日志的人来说，每次启动生成的令牌不是秘密，因此在使用生成令牌时，管理平面**只应答本地客户端**：来自其他机器的请求，或携带 `Forwarded`、`X-Forwarded-For` 或 `X-Real-IP` 的请求（因此同一主机上的反向代理不算本地），都会得到 `403`；`/healthz` 和 `/readyz` 对负载均衡器保持开放。配置了凭据后，管理平面向远程客户端开放，仅由令牌保护，因此请在其前面加上 TLS。区域单元必须配置一个：每次启动生成的令牌其他实例都不知道。`-no-console` 停用的是静态控制台文件，*不是*管理或 Studio API，也不是认证。静态 SPA 文件可公开访问，但没有令牌就不起作用。密钥始终只能写入：列表端点返回元数据和指纹，从不返回值；配置视图会对敏感键脱敏，同时保留 `${VAR}` 占位符。

**本地传输：** `runtime.admin.socket` 会添加第二个仅限本地的管理监听器——Unix 上是 unix 域套接字，Windows 上是命名管道——而 `runtime.admin.tcp: false` 会把管理平面从 HTTP 监听器上移除。套接字仅限所有者访问（0600；管道的 DACL 授权给所有者、Administrators 和 SYSTEM），仍然需要 bearer 令牌，Studio 桌面应用接受 `unix:///path` 或 `npipe://name` 形式的 URL。浏览器控制台在 HTTP 监听器上提供，因此如果要使用它，请保持 TCP 开启。文档现在位于 AgentFlow Studio 中，可以通过本地套接字或管道使用。

**管理 API。** 控制台所做的一切都是 `/admin/api/` 下的 JSON 路由，因此操作员可以用脚本完成。所有路由都需要同一个 bearer 令牌。

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/state` | 实例身份、运行时长、配置纪元、运行中的会话、智能体和渠道。 |
| `GET /admin/api/config` | 原始配置文本、脱敏的结构化视图，以及文件的 mtime。 |
| `POST /admin/api/config/validate` | 对提交的文本进行与启动等效的校验（严格字段、环境变量展开、交叉引用）。 |
| `POST /admin/api/config/save` | 校验，备份为 `.bak`，原子替换。如果文件在你编辑期间被修改，则以 409 拒绝。 |
| `GET /admin/api/models` | 运行中 ∪ 文件中的模型集合；密钥脱敏为末 4 位指纹。 |
| `PUT/DELETE /admin/api/models/{name}` | 将模型热应用到*本进程*（见上文的集群限制）。 |
| `POST /admin/api/models/{name}/test` | 通过运行中的管理器发起一次 1 Token 的调用，并对结果分类（auth / not\_found / unreachable / …）。 |
| `POST /admin/api/models/persist` · `/revert` | 将运行中的集合写入文件，或把文件中的集合重新加载到运行中的管理器。 |
| `GET /admin/api/metrics` | 供迷你折线图使用的采样计数器序列（每 5 秒一次，约 2 小时）。 |
| `GET /admin/api/logs` | SSE 日志尾部：先重放环形缓冲区，然后实时输出。 |
| `GET /admin/api/users` · `/{id}` | 带有今日用量和配额状态的用户资料；单个用户资料及其最近调用、审计记录尾部和项目。 |
| `POST /admin/api/users/{id}/limit` | 设置用户资料的每日 Token 上限（0 表示继承部署默认值）。 |
| `PUT /admin/api/users/{id}/settings` | 用户资料的模型和指令层——这是设置它们的唯一途径，因为一个人使用哪个模型是成本决策，而不是自助选项。 |
| `POST /admin/api/users/{id}/unlink` | 解除一个渠道关联账号（最后一个会被拒绝）。 |
| `POST /admin/api/users/invites` | 生成一个一次性注册邀请。 |
| `GET /admin/api/agents` · `PUT/DELETE /admin/api/agents/{name}` | 在运行时根据定义安装、替换或移除智能体（不写入文件；Studio 的智能体路由是会持久化的版本）。 |
| `GET/POST /admin/api/groups` · `GET/DELETE /{id}` | 租户群组：列出、创建、查看（含成员）、解散。 |
| `POST /admin/api/groups/{id}/members` · `DELETE /{id}/members/{user}` · `PUT /{id}/master` | 添加或移除成员，或把群组移交给新的群主。 |
| `GET/POST /admin/api/pools` · `GET/PUT/DELETE /{id}/grants` | 具名的共享记忆池，以及决定谁可以使用它们的授权（租户 → 读 / 写）。存储区用 `pool: <name>` 绑定到记忆池。 |
| `GET /admin/api/users/{id}/export` · `DELETE /admin/api/users/{id}` | 导出为某个用户资料保存的全部内容（隐私导出），或将其清除。 |
| `GET /admin/api/users/{id}/bundle?public_key=…` | 完整保真的迁移包：用户资料、记忆行、文件字节、群组和凭据，用调用者的 X25519 密钥封装。目标单元上由 `POST /admin/api/users/import` 用私钥接收它。见[多区域](multi-region.md)。 |
| `GET /admin/api/usage?day=YYYY-MM-DD` | 当天按租户汇总的 Token 用量，附总计。 |
| `GET /admin/api/credentials/users` | 存有凭据的用户分区。 |
| `GET/POST /admin/credentials` · `DELETE /admin/credentials/{service}?user=` | 凭据存储：列出元数据、开通按租户的密钥、撤销一个密钥。只能写入——绝不返回值。 |

浏览器控制台涵盖上述四个标签页。此表中的其他内容（用户、邀请、配额、租户、用量、日志和原始配置）在 AgentFlow Studio 中都有对应页面，也可以用 `curl` 编写脚本。

### Studio——智能体构建器、演练场与追踪

**AgentFlow Studio** 是一个用于智能体原型开发的原生桌面应用（Go + Fyne）：编写循环及其指令、校验、实时应用定义、与它对话、观察轮次的每一步，并把结果持久化到部署配置中。它位于自己的仓库 `agentflow-studio` 中——没有嵌入到二进制文件里——并使用与控制台相同的 bearer 令牌与管理 API 通信。

- **智能体**——每个已配置的智能体，包括模型、能力、运行中会话数，以及它是仅存在于运行时还是已持久化。可以从模板创建（echo、LLM 聊天、工具循环、单次执行、带记忆的群聊、知识库、单轮 API）、编辑或删除。
- **构建器**——一个用于常用字段的表单（模型、安全、记忆方案、能力、技能、历史预算、常驻标志、`can_contact`、凭据、附加字段），一个 Luau 编辑器，一个指令编辑器，以及用于其他所有内容的原始 YAML。*校验*会检查 prelude 指令、编译代码块，并运行与启动时相同的智能体校验；*应用*将其安装到运行时；*应用并保存*还会将其写入配置（一个会记住选择的拆分按钮）。编辑器旁的停靠栏中有一个**运行**面板——与正在编辑的智能体对话并查看追踪——或者是 Luau 参考（F1）；Ctrl+Enter 会把待处理的编辑实时应用并发送面板中的消息，或重复上一条消息，因此下一个轮次运行的就是屏幕上的内容。
- **演练场**——针对单个智能体的多对话聊天。每个对话都是独立的会话（`<agent>|studio:<thread>`），因此对话之间不会混杂。回复通过合成的 `studio` 渠道到达，这意味着它们走的是普通的出站路径：安全过滤器、审计记录和投递都不变。可选的*以用户身份* uuid 会标记一个租户，从而应用个人记忆范围、配额和记忆池授权；不填则轮次没有租户。
- **追踪**——轮次的实时时间线：`turn.start`、每个 `op.start`/`op.end`（模型、工具、耗时、Token、工具调用）、安全丢弃、回复、重新加载和循环错误。文本载荷默认关闭（`studio.trace_payloads`）；元数据始终会被记录。
- **渠道**——网关已持久化的渠道集合：添加、编辑、校验和删除 webhook、Telegram、GitHub-webhook 和 bridge 条目；密钥只能写入。保存的内容在下次启动时绑定。
- **模型**——运行中的模型注册表：添加、编辑、用真实调用测试、删除，然后把运行中的集合持久化到配置，或还原为文件中的内容。
- **记忆**——见[记忆](memory.md)指南。编辑 `memory.backends` 和 `profiles.memory`：后端按提供商划分，只包含该提供商读取的键；方案是存储区列表（后端、表、保留期、窗口、所需特性、范围或共享池），外加召回、写入、嵌入和重排设置。后端 URL 和 API 密钥只能写入：页面显示 `${VAR}` 或 `cred:` 引用，或密码已脱敏的 URL。校验与启动等效，仍在使用中的方案或后端不能删除。没有声明任何后端的部署会显示运行时存储上的内置 `main_db`；保存第一个后端时会把它写入文件，这样使用内置 `conversational` 方案的智能体仍保留其存储区。智能体在启动时打开其存储区，因此保存的内容在下次启动时生效，新方案在重启后才能在构建器中选择。
- **安全**——编辑 `profiles.safety` 过滤链（有序的内置项和 `classifier:<name>` 引用）以及 `safety.classifiers` 策略。校验与启动等效；仍被引用的方案和分类器不能删除。调度器按智能体构建，因此保存的内容在下次启动时绑定。
- **凭据**——按分区（引擎全局或按用户）浏览加密存储，创建、编辑、撤销，并设置注入的请求头 / 认证方案。值只能写入，仅以末四位指纹显示；编辑实时生效，因为存储是在请求时读取的。
- **概览**——首页：实例、区域、版本、运行时长和配置纪元、运行中的会话、智能体、渠道，以及采样计数器的迷你折线图（`/admin/api/state`、`/admin/api/metrics`）。
- **用户**——带有今日配额的用户资料、关联账号（可解除关联）、项目、最近调用和审计记录尾部；设置每日上限以及按用户的模型和指令层；签发邀请；导出或清除用户资料（`/admin/api/users/*`）。
- **租户**——群组（成员、群主移交、解散）和记忆池（创建、按智能体和租户授权）（`/admin/api/groups`、`/admin/api/pools`）。
- **用量**——按租户、智能体或模型分组的每日 Token 汇总（`/admin/api/usage`）。
- **日志**——带级别和文本筛选的实时日志尾部（`/admin/api/logs`）。
- **配置**——原始配置文件，带有与启动等效的校验，以及受 mtime 保护、带备份的保存（`/admin/api/config*`）；配置目录部署则通过结构化页面和文件本身进行编辑。

`Ctrl+K` 打开一个涵盖所有页面、智能体和操作的命令面板。

**语言。** Studio 的界面提供英语（English）、简体中文和繁体中文（繁體中文）。**EN / 简体 / 繁體** 切换按钮位于每个页面的命令栏、登录页面，以及*设置 → 语言*中。更改立即生效，无需重启或重新加载，并会被记住（桌面应用保存在其配置文件中，网页应用保存在浏览器的本地存储中）；首次启动时跟随操作系统或浏览器的语言，否则回退到英语。这些指南也跟随同一个切换按钮，尚未翻译的指南显示英文。配置键、标识符、模型和方案名称、YAML、Luau，以及运行时返回的任何内容——包括错误消息——都完全按运行时写出的样子显示。

**持久化以配置为原生形式。** 使用目录配置时，Studio 写入 `profiles/<name>.yaml`（标记为 `# managed by agentflow studio`）、`loops/<name>.lua` 和 `prompts/<name>.md`。使用单个配置文件时，它替换 `agents.<name>` 键（保留注释和其他部分），并把循环 / 提示词写到 `studio.workspace` 下；定义中保存绝对路径，因此持久化的文件不依赖于下一个进程的启动方式。无论哪种方式，更改都会立即实时应用，并在下次启动时从配置加载。不是 Studio 写入的方案会被拒绝，除非强制执行。

**运行它。** 客户端是一个独立的 Go 模块，位于自己的仓库中：

```
cd ../agentflow-studio
make build        # 或：CGO_ENABLED=1 CC="zig cc" CXX="zig c++" go build -o agentflow-studio.exe .
./agentflow-studio
```

输入实例 URL（`http://127.0.0.1:8080`，即 `gateway.listen` 地址）和管理令牌。作为原生客户端，它直接发送令牌，因此不需要 `runtime.admin.cors_origins` 条目。要从另一台机器连接，运行时需要配置 `ADMIN_TOKEN`（每次启动生成的令牌只应答本地客户端）；请在其前面加上 TLS。该允许列表仍用于管理 API 的*浏览器*客户端，并且只接受精确的源（`http://localhost:5173` 和 `http://127.0.0.1:5173` 是不同的源）；`"*"` 会被拒绝。

| Studio 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/catalog` | 模型、工具、能力、安全 / 记忆方案、提示词键、prelude 版本和模板。 |
| `GET /admin/api/studio/agents` · `/{name}` | 智能体列表；单个智能体及其 YAML、循环、指令和持久化文件的元数据。 |
| `POST /admin/api/studio/agents/validate` | 对提交的定义和循环进行无副作用的校验。 |
| `PUT /admin/api/studio/agents/{name}` | 校验，实时安装（`apply`），可选地持久化（`persist`）；返回已应用的内容以及任何持久化错误。 |
| `DELETE /admin/api/studio/agents/{name}` | 停止其会话，将其移除，并且只移除 Studio 管理的文件。`force` 会无视运行中的会话和手写的方案。 |
| `POST /admin/api/studio/chat` | 一次演练场轮次。以 SSE 流式返回（`trace`、`reply`，然后是 `done` 或 `error`），或在 `stream: false` 时返回 JSON。轮次以 `done` 结束（已完成、超时或被安全拒绝）；无法投递的消息或在轮次中途失败的循环则以 `error` 结束，此时 `stream: false` 会返回带追踪的 `ok: false`。永远无法开始轮次的请求（未知智能体、不是用户资料的 `act_as_user`）会在任何数据流开始之前以 4xx 拒绝。 |
| `GET /admin/api/studio/threads` · `POST /threads/{thread}/stop` | 观察到的对话，以及对某个运行中会话的停止请求。 |
| `GET /admin/api/studio/trace/{agent}/{thread}` | 缓冲的追踪；加上 `?stream=true` 获取实时 SSE。 |
| `GET/POST /admin/api/studio/channels` · `PUT/DELETE /{index}` | 已持久化的渠道集合；校验草稿、创建、替换或移除一个渠道。保存后需要重启。 |
| `GET /admin/api/studio/safety` | 安全方案和分类器、它们的使用情况，以及编辑器可以引用的内置过滤器和模型集合。 |
| `POST /admin/api/studio/safety/validate` | 对方案或分类器草稿进行与启动等效的试运行。 |
| `POST/PUT/DELETE /admin/api/studio/safety/profiles` · `/classifiers` | 创建、替换或移除方案或分类器。使用中的条目会被拒绝；写入时会校验、保留 `.bak`，且需要重启。 |
| `GET /admin/api/studio/memory` | 记忆后端（密钥显示为引用或脱敏的 URL）和方案、它们的使用情况，以及编辑器可提供的提供商、模型和插件。 |
| `POST /admin/api/studio/memory/validate` | 对方案或后端草稿进行与启动等效的试运行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 创建、替换或移除方案或后端。名称不可更改；URL 或 API 密钥留空则保留已存储的值；使用中的条目会被拒绝；写入时会校验、保留 `.bak`，且需要重启。 |
| `GET/POST/DELETE /admin/api/credentials` | 列出一个分区的元数据（`?user=`；留空为引擎全局），创建或更新密钥（密钥留空则保留已存储的值），撤销一个密钥（`?service=&user=`）。绝不返回值。 |

**限制。** 演练场是实例本地的：合成渠道只注册在一个进程中，因此在集群中，如果某个对话的会话由另一个实例拥有，这里就捕获不到回复。Studio 不存储聊天历史——智能体的记忆才是事实来源——追踪环保存在内存中，重启后清空。派生方案的编写、附件、重放和评估不在第一个版本的范围内。

### 终端仪表板

当 stdout 是 TTY 时，运行时还会在备用屏幕中挂载一个 **Bubbletea 终端仪表板**（用 `-no-tui` 停用）。它相当于终端里的控制台 Metrics 标签页：同样的计数器，按子系统分组，带有进程内的迷你折线图，标题栏中显示运行中的会话数。它是一个**只读的指标显示**——没有任何操作员操作，所以它显示的内容不会改变运行时；`q`/`esc` 退出并恢复屏幕。指标面板根据宽度排成一到三列，日志页脚随终端大小伸缩（3–8 行）；终端高度不足以同时容纳两者时，面板会被裁剪并显示可见的标记，而不是静默丢弃行；低于最小高度时则完全隐藏面板，以保证画面始终能放下。管道输出（systemd、`| tee`）则仍使用普通的 stderr 日志器。

### 日志

日志使用五个**累加**的级别——选择某一级别会打印该级别及其以上的所有内容。用 `-log-level` 选择（默认 `info`）：

| 级别 | 记录什么 | 示例 |
| --- | --- | --- |
| `ERR` | 运行时退出或无法继续 | 配置加载失败、记忆后端打开失败、端口绑定失败 |
| `WARN` | 降级但仍在继续 | 上游 429/502 + 重试、渠道投递错误、循环崩溃（正在重启）、重新加载时编译失败（保留旧版本） |
| `INFO` | 生命周期 | 运行时启动、渠道已注册、telegram webhook 已设置、会话已派生 / 已退出、热重载已接受 |
| `DEBUG` | 每一次交互 | LLM API 请求、记忆 put/get/query（redis 往返、pgvector 查找）、收到 telegram 更新、webhook/ghhook 入站、操作分派、imap 拉取 / smtp 发送 |
| `DEV` | 临时开发日志 | 调试时临时添加的插桩——`llog.Dev(log, ...)`；有意从不提交 |

同样的记录也会输送到 TUI 日志面板和网页控制台的日志尾部（短标签 `ERR/WRN/INF/DBG/DEV`）。

### `models`——具名的 LLM 提供商

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

`provider` 选择请求 / 响应的形态，`base_url` 选择主机。对于 OpenAI 兼容端点（Ollama、vLLM、LiteLLM），请在 `base_url` 中包含 `/v1`；`openai` 会追加 `/chat/completions`，`openai-responses` 会追加 `/responses`。对于 Anthropic 兼容端点，把 `base_url` 设为主机根地址；运行时会追加 `/v1/messages`。对于 `gemini`，把 `base_url` 指向版本前缀（默认 `https://generativelanguage.googleapis.com/v1beta`）；运行时会追加 `/interactions`，并在 `x-goog-api-key` 请求头中发送密钥。Gemini Interactions API 不支持流式输出，也不接受 Token 上限，因此 `llm.stream` 只产出一个缓冲后的增量，且不会发送 `max_tokens`。`provider: rerank` 是一个使用 `POST {base_url}/rerank` 的交叉编码器端点（TEI、vLLM、Jina、Cohere），通过 `llm.rerank` / 记忆的 `rerank_model` 使用，而不是作为聊天模型。

`server_tools` 列出要在每个请求上启用的提供商原生服务器端工具（例如 gemini 上的 `google_search`，openai-responses 上的 `web_search`/`x_search`）。与通过 `opts.tools` 按调用传入的客户端函数工具不同，这些工具在提供商内部执行；运行时只注入原生的 `{type: <name>}` 条目，并以你所写的名称转发。值是提供商特定的字符串，使运行时保持通用。在 OpenAI 的两种形态上，这个裸条目在 SDK 的工具联合类型中没有对应的变体（其工具类型固定为 `"function"`），所以用 `param.Override` 构造；在 `gemini` 上，`google_search` 映射到原生工具，其他名称都作为原始条目透传。

### `memory.backends`——后端实例

下面的每个后端都是真实的提供商——没有桩实现。存储类后端提供 KV / 前缀 / 文本 / TTL；向量类后端提供语义召回。SQLite 有意以明确的错误拒绝向量查询，而不是静默返回空结果。

| 提供商 | 类型 | 特性 |
| --- | --- | --- |
| `sqlite` | 存储 | kv、prefix\_scan、text\_search、ttl、transaction、vector（对 BLOB 列进行暴力余弦计算；无需扩展，受 `vector_scan_limit` 限制，默认 50000 行） |
| `redis` | 存储 | kv、prefix\_scan、ttl |
| `postgres` | 存储 | kv、prefix\_scan、text\_search、transaction——数据库安装了 pgvector 扩展时还支持 vector（启动时检测；同一张表） |
| `volatile` | 存储 | kv、prefix\_scan、ttl——内存中、不持久化；受 `max_entries` 限制 |
| `pgvector` | 向量 | vector（余弦 k-NN；嵌入维度可通过 `dim` 配置，默认 1536）。作为别名 / 兼容性提供商保留；现在 `postgres` 在扩展存在时即可提供向量功能 |
| `qdrant` | 向量 | vector（通过 Qdrant 的 gRPC API 进行余弦 k-NN——`url` 的端口按 REST 端口解读，因此 `:6333` 会变为 gRPC 的 `:6334`；`dim` 默认 1536，`collection` 默认 `agentflow`，可选 `api_key`） |
| `redisvector` | 向量 | kv、prefix\_scan、ttl、vector（需要 RediSearch 模块，例如 Redis Stack；余弦 k-NN，`dim` 默认 1536） |

每个后端都有一个**位置属性**（locality），由其提供商和 `runtime.topology` 推导而来，并以 `agent.info().memory.stores[*].locality` 的形式提供给循环：

| 提供商 | `single` | `regional` |
| --- | --- | --- |
| `postgres`、`pgvector`、`qdrant` | `deployment` | `deployment` |
| `sqlite` | `deployment`（唯一的实例就是整个部署） | 拒绝 |
| `redis`、`redisvector` | `deployment` | `region`（每个单元一个集群） |
| `volatile` | `instance` | `instance` |

在区域单元中，持久的对话存储区——`dialogue`、`facts`，或 `recall: plugin:semantic` 下的 `semantic`——必须位于 `deployment` 或 `region` 后端上。校验会拒绝位于 `volatile` 上的持久存储区（迁移到另一个实例的会话会丢失历史），除非该存储区设置了 `locality_ok: true`。易失记忆在每个层级中也都是有界的：要么后端设置 `config.max_entries`（驱逐最旧的条目），要么绑定到它的每个存储区都设置 `retention` TTL。

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

三种向量存储都与 `pgvector` 一样只支持向量查询：其他任何查询类型都会给出明确的错误，绝不会静默返回空迭代器。`dim` 在存储首次创建时固定，因此必须与嵌入模型匹配；之后修改需要手动迁移。`redisvector` 把值存放在向量旁边，所以它也支持 kv、prefix\_scan 和 ttl；`redis` 不变，仍是纯 KV 的选项。

`qdrant` 使用 gRPC，但它的 `url` 保持每个部署原有的形式——REST 形式。协议头决定传输方式（`https` 使用 TLS 拨号），端口会被转换：显式写出的 `6333`（Qdrant 的 REST 默认端口）会变为 gRPC 的 `6334`；其他显式端口按原样拨号，因此 gRPC 代理仍然可用；没有端口的 URL 使用 `6334`。原先通过仅支持 REST 的代理访问 Qdrant 的部署，必须改为指向 gRPC 监听器。数据点本身仍用与之前相同的确定性 id 寻址，因此现有的 `agentflow` 集合会被原样读写。

如果完全没有 `memory.backends` 块，引擎会在运行时存储上注册 `main_db`：文件路径对应 provider `sqlite` 加 `path`，DSN 对应 provider `postgres` 加 `url`。这正是内置的 `memory: conversational` 方案能在最小配置中工作的原因，也意味着单实例部署无需第二个数据库即可拥有持久记忆。因此，`postgres` 运行时存储也不需要 `memory:` 部分；安装了 pgvector 扩展时，它的数据库就能提供向量功能。

如果某个后端的 `config.url`/`config.password`/`config.api_key` 密钥引用无法解析，它会被**跳过并发出警告**，而不会导致启动失败：绑定到它的存储区会重新绑定到一个存活的后端（优先选择提供 `text_search` 的提供商，这样向量存储会降级为全文搜索；循环根据绑定的特性进行特性检测），并把 `requires` 降低为回退后端所提供的特性；如果没有任何后端存活，这些存储区就会被丢弃。真正未知的后端名称仍然是启动错误。

### `profiles`——可复用配置

方案按类型分组放在 `profiles:` 下——`memory`、`safety`、`shell` 和 `agent`——智能体按名称引用它们（`memory: chat`、`safety: strict`、`shell: python`）。

#### 记忆方案

[记忆](memory.md)是完整指南：后端、方案和存储区如何组合，循环如何写入和召回，隔离、实用方案和校验消息。本节是参考。

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

`write` 以 `agent.info().memory.write` 的形式传给循环；`memory.write` 始终运行路由表（`plugin:routing_table`，或覆盖它的 `plugins/routing_table.lua`）。

使用 `recall: plugin:semantic` 时，`memory.recall({ text = "...", k = 5 })` 会对查询进行嵌入，在向量存储上运行余弦 k-NN，并在设置了 `rerank_model` 时进行重排。不带文本的查询回退为按时间远近召回。

| 存储区键 | 说明 |
| --- | --- |
| `backend` | 必填。引用一个后端实例名称。 |
| `table` | 该存储区绑定的物理表（或集合）。 |
| `retention` | 以 `ms`、`s`、`m`、`h`、`d` 为单位的时长（允许组合和小数，例如 `2d12h`），或字面值 `forever`。**它是写入 TTL，而不是定期清扫：** 它成为所有未显式指定 TTL 的 `store.put` 的默认 TTL，因此记录在*写入*后经过这么长时间过期，而该设置出现之前写入的行永远不会过期。无法解析的值是启动错误。 |
| `window` | 每张物理表保留的最大记录数，由定期 GC 修剪。适用于 KV 后端；**向量后端（pgvector、Qdrant、Redis）不实现修剪**，因此在它们上面设置 window 或 retention 只是声明了一个无人执行的上限——引擎会在启动时警告，表会无限增长。 |
| `requires` | 特性列表，启动时根据提供商进行校验。 |
| `pool` | 将存储区绑定到一个具名共享池（通过租户管理 API 创建）。授权决定哪些租户可以读写它；绑定到池的存储区不接受 `scope`，且该池在启动时必须存在。`shared: true` 是启动错误——它已被池取代。 |
| `scope` | `agent` 让私有存储区改回每个智能体一个池，而不是使用引擎按用户划分的键范围。 |
| `collection` / `policy` | **已弃用——无效果。** 存储区按后端和表绑定，没有实现任何按存储区的策略开关。仍会被解析，以便现有配置能够启动；设置其中任何一个时，引擎都会在启动时警告。 |

> **隔离方式变更**：记忆存储区**默认按智能体私有**——使用同一方案（或 `conversational`）的两个智能体不再共享物理表；每个智能体绑定 `<agent>.<table>`（例如 `writer.dialogue`）。循环继续使用方案中的表名；前缀在绑定时添加。**有意共享：** 将存储区绑定到一个具名池（`pool: <name>`），并通过租户管理 API 授予租户访问权限；`shared: true` 现在是启动错误。在按智能体隔离之前写入的现有行仍保留在旧的无前缀表中，需要显式迁移。

#### 安全方案

```
profiles:
  safety/default:
    filters: [source-attribution, signal-gate, steady-directive, support-offer, affect-guard]
```

`safety: none` 表示显式退出。调度器由核心拥有，无论循环处理程序如何都会运行。

#### 基于模型的分类器

`safety.classifiers` 定义由专用模型支持的审核或分类策略。方案通过 `classifier:<name>` 选用：

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

模型只返回一个标签和一个分数；由运行时应用 `categories` 和 `threshold`，因此模型输出无法自行选择动作。必须包含 `safe: allow`（或另一个 `allow` 标签）。分类器模型必须使用引擎密钥（`key_source: config`，即默认值）；`key_source: user` 的模型会在启动时被拒绝，否则没有密钥的用户就能使审核失效。调用该模型时不带任何工具，也不带服务器端工具。

失败遵循显式策略，而不是静默放行：`on_error`（提供商错误、无效输出、超时）、`on_saturated`（并发上限已满）和 `on_budget_exhausted`（专用审核预算耗尽）。三者的默认值都是 `block`。设置 `mode: shadow` 可以只记录判定而不执行；模型调用仍然会发生（其延迟也仍然存在），只是动作被抑制。`sources` 使用规范化的词汇 `channel`、`agent`、`scheduler`、`system`（渠道驱动在内部标记 `user`，定时器在内部标记 `timer`）。

入站拦截会跳过该轮次：循环永远看不到这条消息，如果配置了 `refusal` 则发送它。出站拦截会抑制回复，如果配置了 `fallback` 则发送它。拒绝回复和替代回复限流为每个会话每 30 秒一条，这样大量被拦截的消息不会变成大量出站消息。分类器的花费记录在合成的 `__moderation_<name>` 账本分区下，因此不会消耗智能体的 `tokens_per_day` 预算或用户的 LLM 配额。没有 `safety:` 的派生方案会继承派生它的会话的过滤链；在 `profiles.agent.*` 上设置 `safety:` 可以给子智能体使用不同的方案。

#### Shell 方案

Shell 方案说明智能体的 shell 是什么。用 `agents.<name>.shell` 绑定一个方案（该智能体还需要 `shell.exec` 能力）。

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

**`provider: sandbox`** 把每条命令作为一次性的 [gVisor](https://gvisor.dev) 作业运行：环境是只读的根文件系统，项目以读写方式挂载在 `/workspace`，命令结束时会存储它写入项目的内容。除此之外，命令结束后什么都不会保留，因此无需跟踪任何状态，任何实例都可以运行下一条命令。`ignore` 使用 `.gitignore` 的一个小子集：`node_modules/`（任意位置的同名目录）、`*.pyc`（任意位置的名称）、`build/out`（从项目根开始的路径）；末尾的斜杠把模式限定为目录。被忽略的路径既不会存储也不会清理——它们作为尽力而为的缓存留在实例上。

项目可以在其工作树中用 `.agentflow/shell.yaml` 文件定制其智能体的方案。它可以设置 `environment`（部署中声明过的一个）、`env`、`workdir`、`timeout` 和 `ignore`。它没有用于镜像、网络策略或资源限制的字段，未知字段会报错，而不是静默无效。

**`provider: ssh`** 为每个会话保持一个到已有机器的连接；这里不负责开通该机器。连接存在于拨号的那个实例中，随会话一起关闭。

`provider: docker` 已被移除。仍在使用它的配置会在加载时失败，并给出指明替代项的提示。

#### `environments`——声明的工具链

环境是沙箱作业启动时使用的根文件系统和变量。环境由操作员声明；Shell 方案选择其中一个，项目也可以另选一个。

```
environments:
  python:
    image: docker.io/library/python:3.12-slim@sha256:<64 hex digits>
    env: { PYTHONUNBUFFERED: "1" }
  tools:
    rootfs: /srv/agentflow/envs/tools
```

以下两项必须且只能设置一项：

- **`image`**——一个**按摘要固定**的 OCI 镜像。只写标签会被拒绝：标签可以被移到不同的内容上，那样两个实例就会在同一个名称下运行不同的工具链。镜像在某个实例上首次使用该环境时拉取（第一个作业会等待下载完成），并保存在 `<runtime.sandbox.data_dir>/images/<digest>` 下，因此只需修改固定的摘要即可切换到新镜像。只支持公开镜像，或镜像仓库允许匿名拉取的镜像。镜像层必须使用 gzip 压缩。会应用镜像自身的 `Env`，再叠加这里的 `env`。
- **`rootfs`**——主机上一个已包含根文件系统的目录（例如导出的容器，或 `debootstrap` 目录树）。它需要有 `/bin/sh`。

由于作业没有网络，命令所需的一切都必须已经存在于环境中：软件包是镜像的一部分，而不是由命令安装的东西。

#### `runtime.sandbox`——Shell 作业如何运行

每个设置都有默认值；声明了一个环境和一个 Shell 方案的部署完全不需要这些设置。

| 键 | 默认值 | 含义 |
|---|---|---|
| `runsc` | `PATH` 上的 `runsc` | gVisor 运行时二进制文件。请把发行包中的 `gvisor-bin/` 目录放在它旁边 |
| `data_dir` | `<data dir>/sandbox` | 工作区、解包后的镜像以及 runsc 自身的状态。它是缓存：其中的一切都可以从文件存储和镜像仓库重建 |
| `concurrency` | 4 | 本实例上同时运行的作业数 |
| `per_user` | 2 | 其中一个用户最多可占用多少个 |
| `queue_timeout` | 30s | 作业排队等待多久后失败 |
| `timeout` | 60s | 没有其他设置时作业的时间上限 |
| `max_timeout` | 10m | 调用、项目或方案最多可以申请的时长 |
| `background_max_timeout` | 2h | `shell.start` 作业的上限 |
| `output_limit` | 256KiB | stdout 和 stderr 各自的上限；超出部分被丢弃并设置 `truncated` |
| `tmp_limit` | 256MiB | 作业的 `/tmp` 和主目录的大小 |
| `memory` | 1GiB | 整个沙箱的内存上限。超出的作业会被终止，调用会抛出错误 |
| `cpus` | 无 | 以核数计的 CPU 上限 |
| `pids` | 512 | 沙箱内的进程数 |
| `write_back.max_files` | 10000 | 一个作业可以存储的已更改文件数 |
| `write_back.max_bytes` | 1GiB | 一个作业可以存储的文件总大小 |

**要求。** gVisor 只能在 Linux 上运行（x86_64 和 ARM64，内核 5.6 或更新）。不需要 root：沙箱在用户命名空间中运行，因此必须启用非特权用户命名空间。在 Windows、macOS 或没有 `runsc` 的 Linux 主机上，引擎仍会启动，而每个 shell 调用都会失败，并用一句话说明缺少什么。

**内存和 CPU 上限**在主机有一个愿意创建 systemd scope 的 systemd 管理器时，由内核通过该 scope 强制执行——对服务或登录会话而言这是常见情况。如果不满足（例如没有 systemd 的容器），作业运行时就没有这两项限制，应从外部加以约束；进程数、tmpfs 大小、时间上限和输出上限在任何环境下都生效。`agentflow doctor` 会报告主机属于哪种情况、`runsc` 是否可用，以及每个环境是否可用。

**一台主机，一个数据目录。** 两个引擎共享同一个 `data_dir` 会在启动时清除彼此的作业状态。

#### 智能体方案（可派生）

```
profiles:
  agent/coder:
    model: default
    loop: ./loop.lua
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

声明了 `budget.tokens_per_day` 的派生方案会获得一个计量的 LLM 池，**由从它派生的所有子智能体共享**，这样一种项目类型（管理者 + 工作者）就从同一个预算中支出。设置 `window` 后，用量会随着提交记录超出窗口而持续回落，而不是在日界处重置。

`extras` 和 `credentials` 会传递给每个子智能体，因此派生出的循环调用 `agent.config()` 时能看到自己的方案选项和凭据允许列表——与静态智能体看到的形态相同。

### `gateway`——一个共享的 HTTP 监听器

所有 HTTP 渠道（webhook、ghhook、bridge、telegram webhook/auto、qq webhook）都把路径挂载在**一个共享的 HTTP 服务器**上——不存在按渠道的 `listen`。监听器只在 `gateway.listen` 设置一次（默认 `:8080`），该服务器还为探针和反向代理健康检查提供 `GET /health` → `200 {"ok":true}`。`gateway.public_url` 是外部可访问的基础地址（例如 `https://bot.example.com`），在需要运行时替你注册 webhook 时使用。

| 键 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `gateway.listen` | string | :8080 | 所有 HTTP 渠道共用的唯一 HTTP 监听器 |
| `gateway.public_url` | string | — | 外部基础 URL；telegram 的 `webhook`/`auto` 必需 |
| `gateway.channels` | list | — | 渠道定义（见下文） |

#### 渠道类型

| 类型 | 说明 |
| --- | --- |
| `webhook` | POST → 路由器 → 会话 → 回复。**同步**（默认）：响应挂起，直到智能体回复，或到达 `timeout`（默认 `55s`）后返回 504。**异步**（`async: true`）：立即返回 202 和一个作业 id；回复通过 `GET <path>result/<id>` 获取（未完成时返回 202），或 POST 到调用方提供的 `callback_url`——异步作业永远不会被同步超时截断，在 15 分钟后过期。默认路径 `/webhook/` |
| `ghhook` | GitHub webhook 接收：用 `secret` 校验 `X-Hub-Signature-256` HMAC，立即返回 200 确认，并把事件作为智能体类型的消息转发（载荷 `{type: project_event, source: github, event, data}`）。没有回复路径。默认路径 `/hooks/github/` |
| `bridge` | 用于其他任何聊天平台（Slack、Discord、Teams 等）的进程外适配器。适配器把事件 POST 到 `path`，并附带以 `secret` 为密钥的 `X-Agentflow-Signature: sha256=<HMAC of the body>`；运行时把回复以 `{to, text, attachments}` 的形式 POST 到 `deliver_url`，仅以 `token` 作为 bearer 认证（出站投递不做 HMAC 签名）。`timeout` 限制每次投递的时长（默认 10s）。bridge 发送者被记录为自报身份，因此不能关联到用户资料 |
| `telegram`（`mode: polling`） | 默认。长轮询 `getUpdates`，强制执行 `allow_users`。会先清除 Telegram 端残留的 webhook——注册了 webhook 时 getUpdates 会返回 409（例如从 webhook 模式切回时） |
| `telegram`（`mode: webhook`） | 在 `path`（默认 `/webhook/telegram/`）接收 POST 更新；需要 `gateway.public_url`。每次投递都必须携带 `secret_token` 请求头（否则返回 401/403，在解析前检查）；按渠道配置，未配置时每次启动生成 |
| `telegram`（`mode: auto`） | 启动时探测 `<public_url>/health`：可访问时注册 `setWebhook`，否则执行 `deleteWebhook` 并长轮询。`setWebhook` 失败时也会先清除 webhook 再轮询。需要 `gateway.public_url` |
| `qq`（`mode: websocket`） | 默认。腾讯官方 QQ 机器人 API v2（QQ 开放平台）。运行时使用应用访问令牌（由 `app_id` + `secret` 获取，在过期前刷新）主动连接 QQ 网关，订阅其 `scenes` 所需的 intents，发送心跳，并在连接断开后恢复会话；不需要任何可从互联网访问的地址。因 intents 不被允许或机器人被封禁而关闭连接时，渠道会以错误停止 |
| `qq`（`mode: webhook`） | 平台把事件 POST 到 `path`（默认 `/webhook/qq/<name>/`）；在控制台中把 `<public https url><path>` 设为回调地址（端口 80、443、8080 或 8443）。每个请求——包括 URL 校验握手——都必须携带有效的 Ed25519 签名，签名密钥由 `secret` 派生（否则返回 401/403，在解析前检查） |
| `openai` | 在 `<path>chat/completions` 提供 OpenAI Chat Completions API，在 `<path>models` 提供模型列表。`model` 指向所开放的某个 `agents`，每个智能体运行自己的 `loop:`；给智能体设置 `loop: plugin:openai_passthrough`，即可把客户端的消息、工具和采样参数原样转发给其模型。`auth` 可以是 `user_token`（按用户的 bearer，需要 `runtime.identity.enabled`）、`key`（来自 `keys` 的共享密钥）或 `none`（仅限回环监听器）。此类型的 `journal` 默认为 `metadata`，因此对话文本永远不会进入审计记录。见[OpenAI 兼容 API](openai-api.md) |

每个 HTTP 渠道的 `path` 都必须以 `/` 结尾（挂载一个子树）。建议使用 UUID 形式的路径段作为不可猜测的密钥前缀。两个渠道挂载相同路径是启动错误，`openai` 渠道的端点与另一个渠道的子树重叠也是如此。Telegram 的 `token` 和 `secret_token`、ghhook 的 `secret`、qq 的 `secret`、bridge 的 `secret` 和 `token`，以及 `openai` 渠道的 `keys` 都是密钥（见注册表）：在单文件路径下进行环境变量插值，在 configdir 路径下是惰性引用——无法解析时跳过该渠道并发出警告，绝不会导致启动失败。

**QQ 机器人**（`type: qq`）详情：

- **场景与 intents。** `c2c`（与机器人的私聊）和 `group`（QQ 群中 @ 机器人的消息）共用 `GROUP_AND_C2C_EVENT` intent；`guild`（QQ 频道中的 @ 消息）使用 `PUBLIC_GUILD_MESSAGES`；`dm`（QQ 频道私信）使用 `DIRECT_MESSAGE`，该权限需向平台申请才能获得，因此不在默认集合中。
- **会话。** 每条消息的回复目标，以及供路由器使用的 `payload.chat_id`，是 `c2c:<user_openid>`、`group:<group_openid>`、`guild:<channel_id>` 或 `dm:<guild_id>`，因此一个群或一个 QQ 频道是一个对话，而每个私聊各自是一个对话。发送者是 `user:qq:<openid>`（群中为 `member_openid`，QQ 频道中为作者 id）；平台提供时，载荷中还会包含场景、消息 id、事件 id、`union_openid` 和昵称。QQ 会对发送者进行认证，因此身份层把他们视为已验证、可关联的。
- **回复。** 在平台的被动回复窗口内（私聊 60 分钟，群和 QQ 频道 5 分钟），回复会用 `msg_id` 和递增的 `msg_seq` 引用最近一条入站消息，私聊中每条消息最多 4 条回复，群中最多 5 条。超出之后，或平台回应窗口已关闭时，回复会作为主动消息发出，平台会对主动消息限流，且用户可以关闭它。长文本会被拆分为每块最多 1800 个字符。
- **Markdown。** 设置 `markdown: true` 后，私聊和群中的文本回复会以 markdown 消息（`msg_type: 2`）发出，在回复窗口内仍是被动回复；QQ 频道和私信仍为纯文本。Markdown 是平台按机器人授予的能力。如果平台拒绝 markdown 消息，同样的文本会以纯文本重新发送，并暂停 markdown 10 分钟，因此在机器人没有该能力时也不会丢失任何内容。被分块切断的围栏代码块会在一块的末尾闭合，并在下一块的开头重新打开。
- **媒体。** 入站附件按渠道的 `media` 策略下载；没有文本的语音消息使用平台的语音识别文本。出站时，QQ 按 URL 拉取媒体：附件需要一个公开的 `http(s)` `url`（在私聊和群中作为富媒体上传，在 QQ 频道中作为图片链接发送）。没有 URL 的 blob 存储附件会报告为错误，而不是被丢弃。
- **重复消息。** 平台可能会多次投递同一条消息（webhook 重试、恢复后的重放）；30 分钟内重复的消息 id 会被丢弃。
- **控制台设置。** 从 q.qq.com 上机器人的页面复制 AppID 和 AppSecret。对于 webhook 模式，在那里填写回调地址，并把运行时的出口 IP 加入控制台要求的 IP 白名单；对于 websocket 模式只需配置白名单。机器人处于沙箱状态时，只有你在控制台中配置的沙箱群、QQ 频道和成员可以访问它。

**审计模式**按渠道设置：`journal: full`（除 `openai` 外所有类型的默认值）记录消息文本和附件描述符，`journal: metadata`（`openai` 的默认值）每条消息记录一行，包括 id、大小、用户和时间，**从不记录文本**，`journal: off` 则不为该渠道记录任何内容。见 [`audit`](#audit--message-journal)。

**入站媒体**按渠道通过 `media:` 块选择启用——一个 MIME 模式允许列表（`image/*`、`application/pdf`、`audio/*`、…）外加 `max_bytes`（默认 8 MiB）。Telegram 会下载文件（getFile），webhook 接受 base64 的 `attachments`；字节落入按内容寻址的 blob 存储（默认本地文件系统，或通过顶层 `media:` 配置使用 S3/MinIO），消息只携带很小的部件描述符。没有 media 块时，附件会被丢弃，只有文本 / 说明文字会传递。

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

> **破坏性变更**：按渠道的 `listen:` 字段已被移除。仍带有该字段的配置会加载失败，报错 `field listen not found in type config.Channel`。请把每个渠道的端口移到唯一的 `gateway.listen`，为每个 HTTP 渠道设置以 `/` 结尾的 `path`，并为 telegram 的 webhook/auto 设置 `gateway.public_url`。见 README 中的迁移检查清单。

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

服务器声明 `command`（作为子进程启动，通过 stdio 通信）**或** `url`（一个 Streamable HTTP 端点，主动连接），二者选一。两者都在启动时连接一次，连接失败的服务器会被跳过并发出警告，而不会中止运行。HTTP 传输让集成可以完全位于本仓库之外——一个单独部署、单独版本化的服务，运行时正是借此支持专有或需授权的连接器，而无需随附它们。`token` 只适用于 HTTP 传输，在每个请求上作为 bearer 凭据发送；端点不需要时请不要设置。

连接时发现的工具以 `mcp:<server>/<tool>` 的名称进入注册表。如果服务器断开连接，调用会返回结构化的 `unavailable`。

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

配置 `builtin:web_search` 工具背后的引擎，每次调用可通过工具的 `engine` 参数选择（`tools.run("builtin:web_search", {query=..., engine="doubao"})`）；省略 `engine` 则使用 `search.default`。没有配置任何引擎时，该工具如实报告不可用（`{ok:false, unavailable:true}`）。每个引擎的 `base_url` 和 `timeout` 是可选的覆盖项。结果统一为 `{title, url, site, summary, snippet, content, published, score}`；`score` 是引擎原生的（doubao 是 0..1 的相关度，stackoverflow 是原始点赞数，github 是 star 数；youtube 没有）。doubao、ollama 和 youtube（以及有依据的引擎 google\_search/x\_search）需要 `api_key`；stackoverflow 和 github 不需要（`site` 字段可选择任意 StackExchange 网络站点，例如 `superuser` 或 `serverfault`）。缺少或无法解析密钥时，会**跳过该引擎并在启动时警告**，而不会导致启动失败；如果被跳过的引擎是 `search.default`，默认引擎会回退到唯一存活的引擎（或没有）。github 搜索仓库，并支持在查询中使用 GitHub 完整的限定符语法（`language:go stars:>100 pushed:>2024-01-01`）。youtube 通过 YouTube Data API v3 搜索视频（site = 频道标题，content = 描述，摘要前缀 `LIVE:` 表示直播）；共享的 `time_range` 映射到 `publishedAfter`/`publishedBefore`——注意 search.list 每次调用消耗 100 个配额单位（按默认项目配额约每天 100 次搜索），配额耗尽时会如实给出 `quotaExceeded` 错误。

### `legal_search`

```
legal_search:
  default: hklii                # 调用省略 `engine` 时使用的引擎
  engines:
    hklii: {}                   # 香港法律资讯中心——免费，无需密钥
    npc: {}                     # 国家法律法规数据库——免费，无需密钥
                                # （可选的 base_url / timeout 覆盖项）
```

配置两个工具背后的引擎，每次调用可通过 `engine` 参数选择（默认 `legal_search.default`）；没有配置引擎时两者都会如实报告不可用。两个引擎的应答方式不同，因此工具描述根据所配置的引擎拼装——例如 `abbr`/`year`/`num`/`lang` 参数只在存在 hklii 时提供，因为 npc 不读取其中任何一个。结果同理：所选引擎不填写的字段会被省略，而不是作为空字符串发送，因为空字符串会被理解为“未知”而不是“不适用”。

`builtin:legal_search` 以 `{title, path, url, court, date, type?, effective?, status?, neutral?, parallel?, action?, parties?, coram?}` 的形式返回命中结果，`total` 为全部匹配数。它的 `disable_fuzzy` 参数（默认 false）在不同引擎上含义不同——在 hklii 上是关键词与概念检索之分，在 npc 上是仅标题与全文检索之分。`builtin:legal_read` 以命中结果的 `path` 为键获取单个文档的完整纯文本，长文档可用 `start`/`max_chars` 分页。配置了多个引擎时，`path` 会被路由到签发它的引擎，因此无需重复搜索调用中的 `engine`：命中结果的 path 是一个只有其所属引擎才能读取的不透明令牌，要求调用者再次指明引擎，只会在运行时早已知道的事情上出错。显式指明 `engine` 仍然优先，且不会被质疑。

**hklii** 涵盖香港判例法和成文法。搜索命中结果带有中立引用和平行引用、法院、日期、当事人和审理法官。读取*仅限判决书*：`path` 形如 `/en/cases/hkcfi/2020/1808`，或显式给出 `abbr`/`year`/`num`，以及可选的 `lang`（`en`|`tc`|`sc`）。成文法命中结果——`court` 为 `Hong Kong Ordinances`、路径形如 `/en/legis/ord/201/` 的结果——在这里没有可读的全文；其路径会以明确的错误被拒绝，而不是返回错误的内容。

**npc** 涵盖中国国家法律、行政法规和司法解释。命中结果以 `court` 表示制定机关，以 `date` 表示公布日期，并添加 `type`（法律/行政法规/…）、`effective`（施行日期）和 `status`（有效/已修改/已废止/尚未生效）；npc 没有引用和当事人，因此这些字段不存在。读取只接受命中结果的 `path`——不透明的 `bbbs` 文档 id——别无其他：`abbr`、`year`、`num` 和 `lang` 在这里没有意义。返回的 `neutral` 就是同一个 id，而不是引用。文本通过内置的 doc/docx 解析器从返回的 Word 文档中提取。

### `browser`

```
browser:
  account_id: ${CLOUDFLARE_ACCOUNT_ID}   # Cloudflare 账户 id（不是密钥）
  api_token: ${CLOUDFLARE_API_TOKEN}     # 需要 "Browser Rendering - Edit" 权限
  base_url: https://api.cloudflare.com/client/v4   # 可选的端点覆盖
  timeout: 60s                           # 每个请求的时限（默认 60s）
```

配置 `builtin:browser`，它通过 Cloudflare **Browser Run**（原 Browser Rendering）的 Quick Actions 读取页面——这是一个真正的无头浏览器，因此在原始 HTTP 请求只能拿到空壳的地方，JavaScript 渲染的内容也能拿到。一个工具，每次调用指定 `action`：`markdown` 以 Markdown 返回页面，`content` 返回完整渲染后的 HTML，`links` 返回所有链接，`scrape` 返回匹配 `selectors`（CSS）的元素，`json` 返回由 `prompt`/`schema` 描述的结构化数据，`accessibility_tree` 返回无障碍树。`url` 或 `html` 二选一指定页面；`wait_until`（`load`|`domcontentloaded`|`networkidle0`|`networkidle2`）和 `wait_for_selector` 控制页面何时算作加载完成——单页应用正是靠 `networkidle` 这几个值才能正常工作。`json` 动作在 Cloudflare 端运行 AI 模型，按 Workers AI 用量计费。

长结果是有界的：`max_chars`（默认 20000，`0` 表示不限制）限制动作返回的文本，结果始终以 `chars` 报告真实长度，并在截断时设置 `truncated`，这样模型就能分辨短页面和被截断的页面；`links` 同样最多 500 条。没有配置账户时，该工具如实报告不可用（`{ok:false, unavailable:true}`），无法解析的 `api_token` 会**跳过该工具并在启动时警告**，而不会导致启动失败——与所有可选组件遵循的策略相同。只设置 `account_id` 和 `api_token` 中的一个是启动错误，因为 `account_id` 不是密钥，配置了一半的块是值得立即发现的错误。

有三个 Quick Actions 被有意不开放：`screenshot` 和 `pdf` 返回原始字节，目前还没有合适的去处（工具结果是整段进入模型上下文的 JSON，下游没有任何东西会截断它）；`crawl` 是异步的，返回一个需要轮询的作业 id。

### `net`——出站连接策略

```
net:
  http:
    allow_private: false     # 默认值；true 会停用地址防护
```

用一个设置同时管控**两条**出站 HTTP 路径——`builtin:fetch` 工具和 Lua 的 `http.request` 操作——这样部署就不会出现一条受防护、另一条不受防护的情况。

防护会拒绝连接到回环地址、RFC 1918、运营商级 NAT、链路本地地址（包括位于 `169.254.169.254` 的云元数据端点）、唯一本地地址、组播，以及保留 / 特殊用途地址段。它安装在**拨号器**中，而不是在解析 URL 时检查，这个位置正是关键所在：检查在 DNS 解析之后、建立连接之前立即运行，因此看到的是实际拨号的地址。解析时的检查会被 DNS 重绑定绕过——名称只被判断一次，等到建立连接时已经解析到别处——而且每一跳重定向都会建立自己的连接，因此每一跳天然都会被检查。

有两个值得了解的细节。*带区域标识*的地址（`http://[fc00::1%25en0]/`）会被直接拒绝，而且在任何其他检查之前：Go 的前缀匹配不会匹配带区域标识的地址，所以如果没有显式拒绝，这样的地址会漏过所有地址段规则，在防护毫无察觉的情况下到达唯一本地地址的主机。另外，v4 映射的 v6 地址（`::ffff:10.0.0.1`）会在检查前解除映射，因为它与其 v4 形式指向同一台主机。

**取舍：** 受防护的传输有意忽略 `HTTP_PROXY` / `HTTPS_PROXY`。在出口代理之后，拨号器连接的是*代理*，于是防护判断的是代理的地址，而目标根本不会被检查。只通过代理出站的部署会发现这些路径不使用代理。还要注意，`198.18.0.0/15` 作为基准测试地址段被拦截，而它正是 Clash/mihomo 和 sing-box 在 TUN 模式下默认的虚假 IP 池——在运行其中任一工具的开发机上，每个被代理的域名都会解析到这个地址段，从而被拒绝。

`allow_private: true` 会完全停用防护，并在启动时记录一条点明风险的 WARN，因此无法被悄悄开启。它是为确实需要调用内部服务的部署准备的；默认关闭，因为防护是遭受提示注入的智能体与主机自身网络之间唯一的屏障。防护只作用于这两条由智能体控制的路径——模型、搜索、法律和浏览器驱动使用管理员配置的 `base_url`，有意不在覆盖范围内，因此内部镜像仍可正常工作。

### `media`——blob 存储后端

```
media:
  backend: fs                 # fs（默认）| s3
  dir: ./data/media           # fs 根目录（默认：在持久化数据库旁边）
  s3:                         # backend: s3 时必填——适用于 AWS S3 和 MinIO
    bucket: my-media
    region: us-east-1
    endpoint: ~               # MinIO/S3 兼容服务时设置（path-style）；留空 = AWS（virtual-hosted）
    prefix: agentflow         # 对象键前缀
    access_key: ${S3_ACCESS_KEY}
    secret_key: ${S3_SECRET_KEY}
```

选择渠道接收的媒体字节存放在哪里。无论使用哪种后端，对象都按内容寻址（`media:<sha256>` 句柄），因此切换后端时循环和审计记录的形态都不会改变。`s3` 后端直接使用 SigV4（不依赖 SDK）：重复写入相同字节时会在 PUT 之前先 HEAD 去重，密钥永远不会出现在任何错误中。S3 凭据是密钥（见注册表）：无法解析的一对凭据会跳过该存储并在启动时警告（渠道随后在媒体停用的状态下运行），而不会导致启动失败。只有句柄会穿过 Lua 桥——字节一直留在存储中，直到提供商或渠道解析它们。

### `files`——用户范围的文件存储

```
files:
  backend: fs                 # fs（默认）| s3（AWS S3、MinIO、Cloudflare R2）
  dir: ./data/files           # fs 根目录（默认：在持久化数据库旁边）
  s3:                         # 字段和解析方式与 media.s3 相同
    bucket: my-files
    region: auto
    endpoint: https://<accountid>.r2.cloudflarestorage.com
    access_key: ${R2_ACCESS_KEY_ID}
    secret_key: ${R2_SECRET_ACCESS_KEY}
  max_file_bytes: 33554432    # 单个文件上限（默认 32 MiB）
  scratch_ttl: 24h            # 每个会话临时空间的生存期；0 = 永不过期
  gc_grace: 24h               # blob GC 宽限期；0 = 停用 GC
```

为 `files.*` Lua 操作（能力 `files`）提供支持：带有引擎原生快照版本控制的用户范围项目树，外加每个会话的临时空间。启动时的标记-清除会回收超过 `gc_grace` 的孤立 blob（已删除的文件、已被替换的内容）：存活集合包括树记录、提交树（因此仍可回滚）和临时空间引用的每个句柄——去重不受影响，而宽限期（相对于先写 blob、后写元数据的写入顺序）使清除无需加锁即可安全进行。

文件字节与媒体完全一样在 blob 存储中按内容寻址；快照元数据存放在运行时存储的 `files_meta` 表中（普通 sqlite，可检查）。S3 兼容端点需要真正的 SigV4 密钥对——bearer 令牌无法用于 S3。无法解析的一对凭据会停用该存储并在启动时警告（之后这些操作会如实失败），绝不会导致启动失败。

### `audit`——消息日志

```
audit:
  enabled: true               # 默认开启
  retention_days: 90          # 清理期限；0 = 永久保留
```

核心把每条消息记录到运行时 SQLite 存储中的 `message_journal` 表：入站消息在路由器处记录（标记稳定的消息 id 以及 `routed`/`dropped_queue` 状态），出站消息在每个会话的出站处记录（`delivered` / `failed` / `blocked_safety`，并关联到它所回复的入站 id）。每行包含渠道、聊天、发送者、智能体、会话、文本、附件描述符、文本和附件大小，以及来源信息。渠道的 `journal:` 设置会缩小记录范围：`metadata` 保留该行及其大小，但不含文本和附件；`off` 则不为该渠道写入任何行。这是一项信任边界功能：循环和渠道既不能绕过它，也不能伪造它——媒体只按句柄记录，使审计记录保持精简。清理在启动时以及每天按 `retention_days` 运行。

### `usage`——Token 账本

```
usage:
  events: false               # 逐次调用的明细行（默认关闭；每日汇总始终会写入）
  retention_days: 30          # 账本的清理期限；0 = 永久保留
  default_tokens_per_day: 0   # 每个用户的每日上限；0 = 不限制。用户资料自身的上限优先
```

每日汇总（`usage_daily`）为每一项配额和智能体预算决策提供依据，并保存在运行时存储中。配置了日志平面时，逐次调用的事件会写入日志平面。`GET /admin/api/usage` 和 Studio 的“用量”页面读取的是汇总数据。

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

覆盖项在启动时一次性写入注册表，时机是所有工具（内置、法律、shell、MCP）注册完成之后、任何智能体工具集开放之前。`params` 中指定了 schema 未声明的参数的条目会被跳过并发出警告，绝不会报错。策略字段按工具生效；`permission: forbidden` 会把该工具从所有开放的工具集中移除——这也是其中唯一真正起作用的字段。`cost_level`、`user_visible` 和 `autonomous` 会被接受、复制到工具上，但不会被任何东西读取；`tools.policy.write` 同样会被解析但不使用。设置其中任何一个时，引擎都会在启动时警告。

**作用于 Lua 声明的工具**：覆盖项也可以指定循环用 `tool.def` 声明的工具——见下文。这些条目不会在启动时写入（代码块加载之前该工具并不存在）；它们在循环构建工具列表时，根据实时的提示词注册表解析，因此编辑基于 `file:` 的提示词后，下一次 `tools.list()` 就会体现出来。拼错的*提示词键*对这两类工具来说仍然是启动错误。既未注册、也没有被任何已加载循环声明的工具名称无法在启动时发现，因此它会在启动时被记录一次，标为等待 Lua 声明（一行列出这些名称的信息日志，这样在任何智能体开始轮次之前就能看到拼写错误），之后在某个循环首次声明其工具却没有认领该名称时，对每个名称警告一次。

### `agents`

```
agents:
  bot:
    loop: ./loop.lua
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

**能力上限**：`plugins.allow_capabilities`（全局）⊇ `agents.<name>.capabilities`（按智能体）⊇ 派生规格授予的能力（子智能体）。你无法委托自己没有的能力。如果智能体省略 `capabilities`，它会得到默认值 `[llm.chat, memory, tools, agent.send, net.http, users]`——注意 `net.mail`、`shell.exec` 和 `session.state` *不是*默认值，必须显式声明。

**能力在运行时强制执行**，而不仅仅是在启动时校验。每项能力把守其背后的 Lua 操作：`llm.chat` → `llm.*`，`memory` → `store.*`，`tools` → `tools.*`，`shell.exec` → `shell.*`，`net.http` → `http.request` 和 `os.env`，`net.mail` → `mail.*`，`files` → `files.*`。缺少某项能力的智能体会被拒绝，并给出指明操作、智能体和所缺能力的消息——操作仍然可以寻址，并明确地失败，而不是消失成 *"attempt to call a nil value"*。`agent.send`、`agent.request` 和 `agent.spawn` 一直是这样强制执行的；其余的现在也与之一致。`runtime.*` 和 `credential.get` 有意不加把守——前者是每个智能体都需要的循环机制，后者已经由智能体自身的凭据列表限定了范围。启动时的警告会准确列出每个智能体和方案被扣留了哪些能力。只有在需要让依赖“每个智能体都拥有所有操作”的配置继续运行时，才设置 `plugins.enforce_capabilities: false`（默认 `true`）。

**`builtin:` 是工具的命名空间。** 过去有四套词汇都用它来拼写——循环 / 插件名称、工具名称、记忆提供商名称，以及内置记忆方案——而字符串本身并不能说明一个名称属于哪一套。现在每套都有自己的写法，因此这个前缀只有一个含义：

- **工具**保留它：`builtin:web_search`、`builtin:fetch`、`builtin:browser`、`builtin:legal_read`；
- **循环和插件**写作 `plugin:<name>`：`plugin:per_chat`、`plugin:recency`、`plugin:semantic`。这也彻底解决了“循环还是路径”的问题，因为路径永远不会以 `plugin:` 开头——而以前名为 `per_chat` 的文件是无法访问的；
- **记忆提供商**不带前缀：`sqlite`、`qdrant`、`redisvector`——`provider:` 键已经说明了它是哪类名称；
- **内置记忆方案**是 `conversational`，这个名称现在是保留的：同名的 `profiles.memory` 条目是启动错误，而不是一个永远无法运行的方案。

旧写法会按名称识别，而不是报告为未知，因此错误会告诉你该怎么写：`loop:` 写成 `builtin:per_chat` 会得到 `loops are spelled plugin: now`，provider 写成 `builtin:sqlite` 会得到 `providers are unprefixed now`。另外两种错误也会被报告——`loop:` 中写的是*工具*名称会导致启动失败，并提示它应属于哪个字段；`skills:` 条目中写的是循环名称会发出警告，因为否则这种错误是无声的（它匹配不到任何工具，被丢弃，智能体看起来只是没有工具）。完全匹配不到任何已注册工具的 `skills:` 条目*不会*被报告，因为循环可能合理地列出它自己用 `tool.def` 声明的工具。

**方案的 extras 与 credentials**：`extras` 是部署特定的只读方案数据，通过 `agent.config()` 提供给循环（`goal` 块会被提升为顶层的 `goal` 键；其余内容都放在 `extras` 下）。*恰好*是密钥引用的值——`${VAR}`、`${VAR:-default}` 或 `cred:<service>`——在这里永远不会被解析：在 configdir 路径下它们保持原样，呈现为不透明的标记（`{env="VAR"}` / `{cred="service"}`），因此不会有任何解析后的密钥到达循环（或派生出的子智能体）；只是嵌入了引用的字符串（`prefix-${VAR}`）是普通字面值，照常展开。`credentials` 是 `credential.get()` 的允许列表：智能体只能获取它在这里列出的已存储密钥；为空或缺失 = 无访问权限。这两个字段同样适用于派生方案，并传递到每个子智能体。

**常驻智能体**：会话通常在收到第一条消息时才惰性派生。设置了 `persistent: true` 的智能体会在运行时就绪后（渠道注册之后）收到一条合成消息 `{ type = "boot", from = "system:supervisor" }`，这样 cron 调度器和队列消费者无需外部流量即可启动。

**租户范围随委托传递**：智能体之间的跳转（`agent.send` / `agent.request` / `agent.reply`）会在消息由核心标记的来源信息上携带发送轮次的租户，接收方智能体以该租户的身份行事——因此为某个人工作的一组智能体在每一跳写入的都是这个人的文件、记忆和 shell 范围，而不是一个集群范围的范围。该标记取自发送者由引擎分配的身份，绝不取自载荷，因此循环无法指定另一个租户（`payload.user_uuid` 会被忽略，无论是谁写的）。这条链只能削弱：智能体要么拥有被赋予的租户，要么没有。`inherit_user: false` 让智能体不在跳转中继承租户——它随后在共享的 `service` 层中以无租户身份运行——而直接与它对话的人仍然以自己的范围访问它。派生方案上同样可用。

**预算**：`tokens_per_day` 把 LLM 调用包装在针对 Token 池的预留 / 提交 / 释放之中。默认情况下池每天重置；设置 `window`（例如 `24h`、`168h`）可改为滚动窗口计量，用量随提交记录过期而持续回落——不会在午夜骤然重置。会超出预算的调用会被拒绝（计入 `agentflow_budget_denied`）。

### `prompts`——部署级提示词注册表

```
prompts:
  assistant_system:
    file: ./prompts/assistant.md       # 相对于 configdir 解析
  distiller:
    inline: "You are a memory distiller..."   # 字面值（text: 是别名）
  kb_header:
    inline: "[Knowledge base — recalled facts with provenance]"
```

系统提示词、工具提示词、注入提示词和提炼文本都集中在一处，因此不会硬编码在 Lua 中，也不会在多个文件中重复。每个条目*只能*指定 `file:`、`inline:` 或 `text:`（`inline:` 的别名）中的一个；无法读取的 `file:` 会导致启动失败，方案或工具覆盖项引用了未定义的键也会导致启动失败——缺失的提示词绝不会降级为一个静默的空字符串。提示词文件在加载时读取（并在其变化时重新读取），相对于 configdir 解析（单文件配置则相对于配置文件本身解析）。解析后的文本以 `agent.config().prompts`（未声明提示词时为空表）的形式只读地提供给每个循环，键名与此相同。

**引用提示词**：智能体或派生方案上的 `instructions` 既接受文件路径（与以前一样），也接受 `{prompt: <key>}`，后者会把注册表中的文本作为该智能体的系统提示词。工具覆盖项的描述也接受同样的形式：除了已有的字面字符串，还可以写 `description: {prompt: <key>}`。基于 `file:` 的条目是*实时*的：编辑文件会把其文本重新发布到每个循环的 `agent.config().prompts`，并就地刷新来源于该键的所有智能体的系统提示词，无需重启会话（文件无法读取时会警告，并保留已发布的文本）。`prompts:` 块本身不是实时的——添加键、把某个键改指向另一个文件，或把条目在 `inline:` 和 `file:` 之间切换，仍然需要重启。

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

`plugins.dir` 还会**按名称覆盖内置项**：文件 `<dir>/<name>.lua`（例如 `./plugins/recency.lua`）会在所有地方替换同名的内嵌内置项——循环 / 路由引用（`loop: plugin:recency`，包括热重载的监视路径）以及加载到每个会话中的辅助代码块。覆盖文件会被监视：被覆盖的辅助代码块在每次会话启动时（首次启动以及每次重新加载后的重启）重新读取，编辑会重启所有运行中的会话，因此无需重启进程即可生效。未被覆盖的代码块是内嵌的，没有可编辑的文件。

### `triggers`——定时与事件任务

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

**`every:` 和 `cron:` 触发器由引擎执行**，而不是由智能体执行：到期时，运行时把一条普通消息放入目标的邮箱——

```
{ id = "cron:<trigger-name>:<unix>", type = "cron", from = "system:scheduler",
  payload = { ... }, ts = ..., provenance = { kind = "scheduler", principal = "trigger:<name>" } }
```

——放入会话 `<target>|<trigger-name>`。没有任何智能体需要为此持有能力，也没有循环需要定时器：调度是引擎的基础设施，因此部署可以去掉以前运行的用户空间调度器智能体。投递始终会被记录并计数（`agentflow_trigger_fires`），以 `info` 级别记录触发（`debug` 级别附带计算出的下一次触发时间）。

- `every:` 接受以 `ms`、`s`、`m`、`h` 或 `d` 为单位的时长，允许组合和小数（`30s`、`15m`、`6h`、`1d`、`1.5h`、`2d12h`）。首次触发在启动后一个间隔时（或在设置 `run_on_boot` 时立即触发）；之后的触发都以首次触发为锚点，因此不会漂移。最短间隔为 100ms。
- `cron:` 接受经典的 5 个字段（`minute hour day-of-month month day-of-week`），支持 `*`、`N`、`N-M`、`*/N`、`N-M/N` 以及逗号分隔的列表。星期字段接受 `0`–`7`（`0` 和 `7` 都表示星期日）。当日期*和*星期都受限时，匹配*任一*条件的日子都会触发（经典规则）。不支持名称（`mon`、`jan`），也没有秒字段。
- `cron:` 字段按 `runtime.timezone_offset_hours` 匹配（固定的 UTC 偏移，默认 0 = UTC）。它是偏移量而不是具名时区，因此计划永远不会因其他地区的夏令时而偏移——`30 9 * * 1-5` 配合 `8` 表示一年中每一天的 UTC+8 09:30。
- `target.profile` 指向一个已配置的智能体或派生方案。派生方案解析为 `spawn:<profile>` 地址——一个运行该方案的循环、模型和授权的顶层会话（没有父级，也不会相对父级削弱能力）。
- `event:` 触发器**不会**被调度：由路由器和循环自己进行匹配。`runtime.triggers()` 仍然返回完整的合并列表，形态不变，因此读取它的循环可以继续工作。
- 无法调度的触发器——表达式格式错误、间隔低于下限，或 `target.profile` 既不是智能体也不是派生方案——会被**记录并跳过**；其余触发器照常运行。一处配置拼写错误只会让你损失一个触发器和一行警告，绝不会让启动失败。
