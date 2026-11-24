# 记忆

记忆是智能体在轮次之间保留下来的东西：最近的对话、别人告诉它的事实、它可以搜索的文档。本页说明这些部分如何组合在一起，如何在配置文件或 Studio 中设置，以及循环如何使用它。

记忆分为三层：

| 层 | 位置 | 是什么 |
| --- | --- | --- |
| **后端** | `memory.backends.<name>` | 一个数据库：SQLite 文件、PostgreSQL 或 Redis 服务器、Qdrant 集群，或进程内存。 |
| **方案** | `profiles.memory.<name>` | 一组具名的逻辑**存储区**，每个存储区是某个后端上的一张表，外加 `memory.recall` 如何搜索它们。 |
| **智能体** | `agents.<name>.memory` | 智能体使用哪个方案：方案名称、内置的 `conversational`，或内联方案。 |

多个智能体可以共用一个方案。每个智能体仍然拥有自己的表（见[隔离](#isolation)），所以共用方案共享的是结构，而不是数据。

## 两分钟速览

完全没有 memory 部分的配置本身就能用：

```yaml
agents:
  assistant:
    loop: ./loops/chat.ts
    memory: conversational        # 内置方案
```

`conversational` 把对话存放在 `dialogue` 表中（保留 30 天，最多最近 1000 条记录），把事实存放在可搜索的 `facts` 表中（永久保留），两者都位于名为 `main_db` 的后端上。如果配置没有声明任何后端，`main_db` 就是运行时存储本身（`runtime.persistence`），因此最小部署不需要第二个数据库。

在循环中：

```ts
memory.write({ type: "turn", id: msg.id, text: msg.text, reply: reply.text });
const recent = memory.recall({ kind: "all" });          // 来自 dialogue 的记录
```

智能体需要 `memory` 能力，它默认已开启。

## 后端

```yaml
memory:
  backends:
    main_db:
      provider: sqlite
      config: { path: ./data/agentflow.db }
    vectors:
      provider: pgvector
      config: { url: "${PG_URL}", dim: 1536 }
```

| 提供商 | 适用场景 | 读取的设置 |
| --- | --- | --- |
| `sqlite` | 单实例，无需服务器。键值、全文和暴力向量搜索。 | `path`, `vector_scan_limit` |
| `postgres` | 共享服务器。键值和全文；安装 pgvector 后也支持向量。 | `url`, `dim` |
| `pgvector` | PostgreSQL 向量搜索（为兼容而保留；`postgres` 已涵盖）。 | `url`, `dim` |
| `redis` | 快速的共享键值，不支持搜索。 | `url` |
| `redisvector` | Redis Stack：键值加向量搜索。 | `url`, `dim` |
| `qdrant` | 大规模向量搜索。 | `url`, `api_key`, `collection`, `dim`, `distance`, `timeout` |
| `volatile` | 进程内存：速度快，重启即丢失，所有会话共享。 | `max_entries` |

- **密钥。** `url`、`password` 和 `api_key` 接受 `${VAR}`、`${VAR:-default}` 或 `cred:<service>`，这样密钥就不会写进文件。密钥无法解析的后端会被跳过并给出警告，它的存储区会转移到仍可用的后端上。
- **`dim`** 在首次创建向量表时就固定下来。它必须与嵌入模型一致（`text-embedding-3-small` 为 1536，许多开源模型为 768）。
- **`main_db`** 是内置方案使用的后端。一旦配置声明了任何后端，也要同时声明 `main_db`，否则使用 `conversational` 的智能体会退回到 `./data/agentflow.db`。Studio 的记忆界面会替你写出它。

完整的提供商表、本地性规则和 Qdrant 传输细节见[配置](configuration.md)。

## 方案

```yaml
profiles:
  memory:
    group_chat:
      stores:
        dialogue: { backend: main_db, table: group_dialogue, retention: 7d, window: 500, requires: [kv] }
        facts:    { backend: main_db, table: group_facts, retention: forever, requires: [kv, text_search] }
      write: [plugin:routing_table]
      recall: plugin:recency
```

方案名称由 1–64 个字母、数字、`_`、`.` 或 `-` 组成。`conversational` 保留给内置方案。

### 存储区

存储区是循环使用的名称（`dialogue`、`facts`、`semantic` 或你自己起的名字），绑定到某个后端上的一张物理表。

| 键 | 含义 |
| --- | --- |
| `backend` | 必填。一个 `memory.backends` 名称。 |
| `table` | 物理表或集合。 |
| `retention` | 记录写入后保留多久：`30d`、`12h`、`2d12h` 或 `forever`。它是每次写入的默认 TTL，而不是定期清理，所以在设置它之前写入的行永远不会过期。 |
| `window` | 每张表最多保留的记录数，由定期清理裁剪。仅适用于键值后端；向量后端不做裁剪。 |
| `requires` | 后端必须具备的功能：`kv`、`text_search`、`vector`。在启动时检查，因此放错后端的存储区会在启动时失败，而不是等到第一次召回。 |
| `scope` | `user`（默认）按用户分别保存行；`agent` 让该智能体的所有用户共享一组行。 |
| `pool` | 改为把存储区绑定到“租户”中的某个具名共享池。由授权决定谁可以读写它。绑定到池的存储区不能设置 `scope`。 |
| `locality_ok` | 在区域单元中，接受位于实例本地后端上的持久存储区。 |

内置插件会查找三个存储区名称：

- **`dialogue`** 接收 `turn` 记录，也是 `memory.recall` 默认读取的存储区。
- **`facts`** 接收 `fact` 记录。用 `kind = "text"` 和 `store = "facts"` 搜索它。
- **`semantic`** 是语义召回搜索的存储区；没有它时则搜索 `dialogue`。

### 召回

`recall` 选择 `memory.recall` 分派到哪个处理器。

| 值 | `memory.recall` 的行为 |
| --- | --- |
| `plugin:recency`（默认） | 直接读取存储区：最新的记录、前缀扫描、全文搜索或时间窗口。 |
| `plugin:semantic` | 为查询文本生成嵌入，执行向量搜索，并可选地重排。没有 `text` 的查询会退回到按时间读取。 |

语义召回需要在方案上设置三项：

```yaml
    knowledge:
      stores:
        semantic: { backend: vectors, table: passages, retention: forever, requires: [vector] }
        dialogue: { backend: main_db, table: kb_dialogue, retention: 30d, window: 200 }
      recall: plugin:semantic
      embed_model: embed          # 一个能生成嵌入的 models: 条目
      rerank_model: rerank        # 可选：一个 provider: rerank 的 models: 条目
      oversample: 4               # 配合重排器：取回 k*4 条结果，再重排到 k 条
```

- **`embed_model`** 在召回时为查询生成嵌入。对于 `memory.write` 发往支持向量的后端上的存储区的每条记录，它也会生成嵌入，因此写入和查询使用同一个模型。
- **`rerank_model`** 是可选的。设置后，向量搜索会取回 `k × oversample` 条结果，再由交叉编码器排序；每条结果都带有 `meta.rerank_score`。

### 写入

`write` 列出写入链。目前 `memory.write` 总是运行路由表：随附的 `plugin:routing_table`，或者当你的部署覆盖了它时运行 `plugins/routing_table.ts`。这个列表本身会以 `agent.info().memory.write` 的形式传给循环，循环可以读取它，但它并不会切换写入链。请保持为 `[plugin:routing_table]`。

## 为智能体提供记忆

```yaml
agents:
  helper:
    loop: ./loops/helper.ts
    memory: group_chat            # 一个具名方案

  scratch:
    loop: ./loops/scratch.ts
    memory:                       # 内联方案，仅供这一个智能体使用
      stores:
        dialogue: { backend: main_db, table: scratch, retention: 1h }
```

- **能力。** 带记忆的智能体需要 `memory` 能力。它在默认集合中；如果你自己列出了 `capabilities:`，记得把它包含进去。
- **派生模板。** `profiles.agent.<name>.memory` 以同样的方式指定方案。
- **不需要记忆。** 不写 `memory:` 即可。这时 `memory.write` 无处可路由，召回会返回空列表。

## 在循环中使用记忆

### 写入

```ts
// 一轮对话：路由到 "dialogue"，键为 turn:<id>。
memory.write({ type: "turn", id: msg.id, text: msg.text, reply: reply.text });

// 一条事实：路由到 "facts"，以 key 为键。
memory.write({ type: "fact", key: "name:" + msg.from, text: "Their name is Ada." });

// 显式目标：任意存储区、任意键，可选的 ttl（秒）。
memory.write({ targets: [
  { store: "notes", key: "todo:" + msg.id, value: { text: msg.text }, ttl: 3600 },
] });
```

记录只会写入方案中声明的存储区：如果智能体的方案没有 `dialogue` 存储区，它写入的 `turn` 会被丢弃，而不会报错。

### 召回

`memory.recall` 返回一个 `{ key, value, meta }` 列表。

```ts
// plugin:recency
memory.recall({ kind: "all" });                                  // dialogue
memory.recall({ kind: "text", store: "facts", text: "name", k: 5 });
memory.recall({ kind: "prefix", store: "notes", prefix: "todo:" });
memory.recall({ kind: "time", store: "dialogue",
                from: "2026-11-01T00:00:00Z", to: "2026-11-02T00:00:00Z" });

// plugin:semantic
memory.recall({ text: msg.text, k: 5 });                         // 语义召回，否则读 dialogue
```

不指定 `store` 时，按时间读取的是 `dialogue`，所以要在 `facts` 上做文本搜索，就得写明存储区。时间范围使用 RFC3339 字符串。

### 原始表

`store.put`、`store.get`、`store.query` 和 `store.delete` 按名称直接操作存储区，不经过路由，也不生成嵌入。详见 [脚本循环](loops.md#memory)。

## 隔离

- **按智能体。** 每个智能体绑定自己的物理表（`<agent>.<table>`），所以使用同一方案的两个智能体永远不会读到彼此的行。
- **按用户。** 在一个智能体内部，行默认按用户分别保存：一个人的对话轮次不会出现在另一个人的召回结果中。对某个存储区设置 `scope: agent` 可以关闭这一点，用于所有用户都应看到的知识。
- **有意共享。** `pool: <name>` 把存储区绑定到在“租户”界面创建的池。由授权决定哪些租户可以读写它。

## 保留期、窗口和上限

- `retention` 是写入时打上的 TTL；之后修改它只影响新写入的行。
- `window` 在定期清理时裁剪键值表中最旧的行。
- 位于 `volatile` 后端上的存储区必须有上限：要么后端设置 `max_entries`，要么存储区设置 `retention`。否则校验会拒绝它，因为所有会话共享的进程内存会一直增长，直到进程崩溃。
- 在区域单元中，`dialogue`、`facts` 以及（使用语义召回时的）`semantic` 必须位于共享后端上，不能是 SQLite 或 `volatile`，除非该存储区设置了 `locality_ok: true`。

## 在 Studio 中设置

**记忆**（两个应用中都位于“连接”分组下）无需接触 YAML 就能编辑 `memory.backends` 和 `profiles.memory`。

- **后端。** 选择一个提供商，表单就只显示该提供商读取的设置。URL 和 API 密钥只能写入：界面显示的是 `${VAR}` 或 `cred:` 引用，或者隐去密码的 URL；字段留空则保留已存储的值。
- **方案。** 每个存储区占一行：名称、后端、表、保留期、窗口、所需功能，以及它的范围或池。存储区下方是召回、写入、嵌入模型和重排模型，以及过采样倍数。新方案一开始是内置方案的副本。
- **校验** 会像重启时一样，针对整个配置检查这次修改，但不写入任何内容。**保存** 会再校验一次，保留一份 `.bak`，并且只写入这两个部分。
- **删除** 在条目仍被使用时会被拒绝；界面会指出是谁在使用。
- **隐式的 `main_db`。** 没有后端的配置会把 `main_db` 显示为内置。保存你的第一个后端时也会把 `main_db` 写入文件，这样使用 `conversational` 的智能体仍能保留它们的存储区。

在智能体构建器中，**记忆方案**（位于安全方案旁边）提供以下选项：*无*、*conversational（内置）*、每个具名方案，以及智能体已有的内联方案（会按原样保留）。

智能体在 agentflow 启动时打开它们的存储区，所以记忆相关的修改要在重启后才生效。在记忆界面创建的方案，也要在重启之后才会出现在构建器的选择列表中。

该界面使用以下管理路由：

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/memory` | 后端（密钥以引用或隐去密码的 URL 显示）、方案、各自的使用方，以及编辑器可提供的提供商、模型和插件。 |
| `POST /admin/api/studio/memory/validate` | 对方案或后端草稿做试运行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 创建、替换或移除一项。名称不可更改。 |

## 配方

### 群聊：近期上下文加上记住的事实

```yaml
profiles:
  memory:
    group_chat:
      stores:
        dialogue: { backend: main_db, table: group_dialogue, retention: 7d, window: 500, requires: [kv], scope: agent }
        facts:    { backend: main_db, table: group_facts, retention: forever, requires: [kv, text_search], scope: agent }
      recall: plugin:recency
```

`scope: agent` 让群里的每个人共享同一份历史。`agentflow-studio` 仓库中的 `examples/qq-group-chat` 场景是一个完整的版本。

### 基于 SQLite 的知识库，无需额外服务器

```yaml
models:
  embed: { provider: openai, model: text-embedding-3-small, api_key: "${OPENAI_API_KEY}" }

profiles:
  memory:
    knowledge:
      stores:
        semantic: { backend: main_db, table: passages, retention: forever, requires: [vector], scope: agent }
      recall: plugin:semantic
      embed_model: embed
```

SQLite 以暴力方式搜索向量，上限由 `vector_scan_limit` 控制（默认 50,000 行）。超过这个规模时，把存储区迁移到 `pgvector` 或 `qdrant`；循环无需改动。参见 `examples/telegram-rag`。

### 进程内存中的临时空间

```yaml
memory:
  backends:
    main_db: { provider: sqlite, config: { path: ./data/agentflow.db } }
    cache:   { provider: volatile, config: { max_entries: 10000 } }

profiles:
  memory:
    with_scratch:
      stores:
        dialogue: { backend: main_db, table: dialogue, retention: 30d }
        scratch:  { backend: cache, table: scratch, retention: 1h }
```

## 校验不通过时

| 消息 | 需要修改什么 |
| --- | --- |
| `agent "x" uses memory but lacks memory capability` | 把 `memory` 加入智能体的 `capabilities`，或者删掉这个列表以使用默认值。 |
| `agent "x" references unknown memory profile "p"` | 定义 `profiles.memory.p`，或者改正名称。在 Studio 中创建的方案需要重启后智能体才能使用。 |
| `store "s" references unknown backend "b"` | 定义 `memory.backends.b`。一旦声明了任何后端，也必须声明 `main_db`。 |
| `profiles.memory defines "conversational", which is reserved` | 换一个名称。 |
| `invalid retention "soon"` | 使用 `ms`、`s`、`m`、`h`、`d` 单位或 `forever`。 |
| `... volatile backend ... grows without bound` | 在后端上设置 `max_entries`，或在存储区上设置 `retention`。 |
| `regional topology: ... instance-local` | 把存储区迁移到共享后端；如果可以接受丢失它，则设置 `locality_ok: true`。 |
| `recall plugin:semantic needs an embed model`（Studio） | 在方案上选择一个嵌入模型。 |
| `a postgres backend needs a URL`（Studio） | 填写 URL，或填写指向它的 `${VAR}` / `cred:` 引用。 |
