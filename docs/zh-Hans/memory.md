# 记忆

记忆是智能体在轮次之间保留的东西：最近的对话、被告知的事实、可供检索的文档。本页说明它是怎么搭起来的、如何在配置文件中或 Studio 里设置，以及循环怎么使用它。

一共三层：

| 层 | 位置 | 是什么 |
| --- | --- | --- |
| **后端** | `memory.backends.<name>` | 一个数据库：SQLite 文件、PostgreSQL 或 Redis 服务器、Qdrant 集群，或进程内存。 |
| **方案** | `profiles.memory.<name>` | 一组具名的逻辑 **存储区**，每个都是某个后端上的一张表，外加 `memory.recall` 如何在其中检索。 |
| **智能体** | `agents.<name>.memory` | 智能体用哪个方案：方案名、内置的 `conversational`，或内联方案。 |

多个智能体可以共用一个方案。每个智能体仍然有自己的表（见[隔离](#isolation)），所以共用方案共用的是形状，不是数据。

## 两分钟版

完全没有 memory 段的配置本来就能用：

```yaml
agents:
  assistant:
    loop: ./loops/chat.ts
    memory: conversational        # 内置方案
```

`conversational` 把对话存在 `dialogue` 表（30 天，最近 1000 条记录），把事实存在可检索的 `facts` 表（永久保留），后端叫 `main_db`。配置里没声明任何后端时，`main_db` 就是运行时存储本身（`runtime.persistence`），所以最小部署不需要第二个数据库。

在循环里：

```ts
memory.write({ type: "turn", id: msg.id, text: msg.text, reply: reply.text });
const recent = memory.recall({ kind: "all" });          // 来自 dialogue 的记录
```

智能体需要 `memory` 能力，它在默认集合里。

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

| 提供商 | 适合 | 读取的设置 |
| --- | --- | --- |
| `sqlite` | 单实例，无需服务器。键值、全文检索和暴力向量检索。 | `path`、`vector_scan_limit` |
| `postgres` | 共享服务器。键值和全文检索；装了 pgvector 也能做向量。 | `url`、`dim` |
| `pgvector` | PostgreSQL 向量检索（为兼容保留；`postgres` 已覆盖）。 | `url`、`dim` |
| `redis` | 快速共享键值，无检索。 | `url` |
| `redisvector` | Redis Stack：键值加向量检索。 | `url`、`dim` |
| `qdrant` | 大规模向量检索。 | `url`、`api_key`、`collection`、`dim`、`distance`、`timeout` |
| `volatile` | 进程内存：快，重启即丢，所有会话共享。 | `max_entries` |

- **密钥。** `url`、`password` 和 `api_key` 接受 `${VAR}`、`${VAR:-default}` 或 `cred:<service>`，密钥因此不会进文件。密钥解析不出来的后端会被跳过并给出警告，它的存储区转到还活着的后端上。
- **`dim`** 在向量表首次创建时固定，必须和嵌入模型一致（`text-embedding-3-small` 是 1536，很多开源模型是 768）。
- **`main_db`** 是内置方案的后端。配置里一旦声明了任何后端，就要把 `main_db` 也声明出来，否则用 `conversational` 的智能体会退回到 `./data/agentflow.db`。Studio 的“记忆”页面会替你写出来。

完整的提供商表、locality 规则和 Qdrant 传输细节见[配置](configuration.md)。

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

方案名为 1–64 个字母、数字、`_`、`.` 或 `-`。`conversational` 保留给内置方案。

### 存储区

存储区是循环使用的名字（`dialogue`、`facts`、`semantic`，或你自己起的），绑定到某个后端上的一张物理表。

| 键 | 含义 |
| --- | --- |
| `backend` | 必填。一个 `memory.backends` 名称。 |
| `table` | 物理表或集合。 |
| `retention` | 记录写入后存活多久：`30d`、`12h`、`2d12h` 或 `forever`。它是每次写入的默认 TTL，不是清扫，所以在设置它之前写入的行永远不会过期。 |
| `window` | 每张表最多保留多少条记录，由定期清扫裁剪。仅限键值后端；向量后端不裁剪。 |
| `requires` | 后端必须具备的特性：`kv`、`text_search`、`vector`。启动时检查，所以放错后端的存储区在启动时就失败，而不是等到第一次召回。 |
| `scope` | `user`（默认）按用户保留数据行；`agent` 在整个智能体的所有用户之间共用一份。 |
| `pool` | 改为把存储区绑定到“租户”里的具名共享池，由授权决定谁能读写。绑定池的存储区不设 `scope`。 |
| `locality_ok` | 在区域单元中，接受把持久存储区放在实例本地的后端上。 |

内置插件会找三个存储区名字：

- **`dialogue`** 存 `turn` 记录，也是 `memory.recall` 默认读的地方。
- **`facts`** 存 `fact` 记录。用 `kind = "text"` 和 `store = "facts"` 检索。
- **`semantic`** 是语义召回的检索目标；没有它就在 `dialogue` 里检索。

### 召回

`recall` 决定 `memory.recall` 分派到哪个处理器。

| 值 | `memory.recall` 的行为 |
| --- | --- |
| `plugin:recency`（默认） | 直接读存储区：最新记录、前缀扫描、全文检索或时间窗。 |
| `plugin:semantic` | 把查询文本转成嵌入，做向量检索，可选地重排。没有 `text` 的查询退回到按时间。 |

语义召回需要在方案上配三样东西：

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

- **`embed_model`** 在召回时把查询转成嵌入。它也会把 `memory.write` 送到支持向量的后端存储区的每条记录转成嵌入，所以写入和查询用的是同一个模型。
- **`rerank_model`** 可选。设置后，向量检索先取 `k × oversample` 个命中，再由交叉编码器排序；每条结果带上 `meta.rerank_score`。

### 写入

`write` 列出写入链。目前 `memory.write` 始终走路由表：随附的 `plugin:routing_table`，或者部署覆写了它时的 `plugins/routing_table.ts`。这个列表会作为 `agent.info().memory.write` 传给循环，循环可以读它，但它并不会切换链路。保持 `[plugin:routing_table]` 即可。

## 给智能体配记忆

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

- **能力。** 带记忆的智能体需要 `memory` 能力。它在默认集合里；如果你自己列了 `capabilities:`，记得把它写进去。
- **派生模板。** `profiles.agent.<name>.memory` 用同样的方式指定方案。
- **不要记忆。** 把 `memory:` 留空。此时 `memory.write` 无处可路由，召回返回空列表。

## 在循环里使用记忆

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

记录只会进方案里声明过的存储区：方案没有 `dialogue` 存储区的智能体写 `turn` 会被丢弃，而不是报错。

### 召回

`memory.recall` 返回 `{ key, value, meta }` 列表。

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

不带 `store` 时，按时间召回读 `dialogue`，所以要在 `facts` 上做全文检索就得指明存储区。时间边界是 RFC3339 字符串。

### 原始表

`store.put`、`store.get`、`store.query` 和 `store.delete` 按名字直接操作存储区，不经过路由也不做嵌入。详见[脚本循环](loops.md#memory)。

## 隔离

- **按智能体。** 每个智能体绑定自己的物理表（`<agent>.<table>`），所以共用同一方案的两个智能体永远读不到对方的行。
- **按用户。** 在同一智能体内，数据行默认按用户保留：一个人的轮次不会出现在另一个人的召回里。`scope: agent` 会关掉某个存储区的这一行为，用于所有人都该看到的知识。
- **有意共享。** `pool: <name>` 把存储区绑定到“租户”页面创建的池。授权决定哪些租户可以读写。

## 保留期、窗口与上限

- `retention` 是写入时打上的 TTL；之后再改只影响新行。
- `window` 在定期清扫时裁掉键值表中最旧的行。
- `volatile` 后端上的存储区必须有界：要么后端设 `max_entries`，要么存储区设 `retention`。否则校验会拒绝，因为所有会话共用的进程内存会一直涨到进程死掉。
- 在区域单元中，`dialogue`、`facts` 和（启用语义召回时的）`semantic` 必须放在共享后端上，不能用 SQLite 或 `volatile`，除非存储区设了 `locality_ok: true`。

## 在 Studio 里设置

**记忆**（在两个应用的“接入”下）不改 YAML 就能编辑 `memory.backends` 和 `profiles.memory`。

- **后端。** 选一个提供商，表单只显示该提供商读取的设置。URL 和 API 密钥只写不读：页面显示 `${VAR}`、`cred:` 引用，或把密码打码后的 URL；留空即保留已存储的值。
- **方案。** 每个存储区一行：名称、后端、表、保留期、窗口、它要求的特性，以及范围或池。存储区下面是召回、写入、嵌入与重排模型，以及过采样倍数。新方案以内置方案为副本开始。
- **校验** 按整份配置检查改动，和重启时一样，但不写任何东西。**保存** 会再校验一次，留一份 `.bak`，并且只写这两节。
- **删除** 在有东西引用时会被拒绝，页面会指出是哪个。
- **隐式的 `main_db`。** 没声明后端的配置会把 `main_db` 显示为内置。保存第一个后端时，`main_db` 也会一并写进文件，用 `conversational` 的智能体因此不会丢存储。
- 在智能体构建器里，**记忆方案**（挨着安全方案）提供 *无*、*conversational（内置）*、每个具名方案，以及该智能体已有的内联方案（按原样保留）。

智能体在 agentflow 启动时打开存储区，所以记忆的改动要重启后才生效。在“记忆”页面新建的方案也要等那次重启后才会出现在构建器的选择器里。

页面使用这些管理路由：

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/memory` | 后端（密钥以引用或打码 URL 呈现）、方案、各自被谁使用，以及编辑器可选的提供商、模型和插件。 |
| `POST /admin/api/studio/memory/validate` | 对方案或后端草稿做一次试运行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 创建一个、替换一个或删除一个。名称不可更改。 |

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

`scope: agent` 让群里的所有人共用一份历史。`agentflow-studio` 里的 `examples/qq-group-chat` 就是一个完整版本。

### 建在 SQLite 上的知识库，不额外加服务器

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

SQLite 用暴力方式检索向量，受 `vector_scan_limit` 限制（默认 50000 行）。超过这个量就把存储区迁到 `pgvector` 或 `qdrant`，循环不用改。见 `examples/telegram-rag`。

### 进程内存里的草稿空间

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

## 校验报错时

| 报错 | 要改什么 |
| --- | --- |
| `agent "x" uses memory but lacks memory capability` | 把 `memory` 加进智能体的 `capabilities`，或删掉这个列表以使用默认值。 |
| `agent "x" references unknown memory profile "p"` | 定义 `profiles.memory.p`，或修正名字。在 Studio 里建的方案要重启后智能体才能用。 |
| `store "s" references unknown backend "b"` | 定义 `memory.backends.b`。一旦声明了任何后端，`main_db` 也必须声明。 |
| `profiles.memory defines "conversational", which is reserved` | 换个名字。 |
| `invalid retention "soon"` | 用 `ms`、`s`、`m`、`h`、`d` 单位，或 `forever`。 |
| `... volatile backend ... grows without bound` | 在后端上设 `max_entries`，或在存储区上设 `retention`。 |
| `regional topology: ... instance-local` | 把存储区挪到共享后端；如果丢了也能接受，就设 `locality_ok: true`。 |
| `recall plugin:semantic needs an embed model`（Studio） | 在方案上选一个嵌入模型。 |
| `a postgres backend needs a URL`（Studio） | 填上 URL，或填指向它的 `${VAR}` / `cred:` 引用。 |
