# 記憶

記憶是代理在各輪次之間保留下來的東西：近期的對話、被告知的事實、可搜尋的文件。本頁說明這些部分如何組合在一起、如何在組態檔或 Studio 中設定，以及迴圈如何使用記憶。

共分三層：

| 層 | 位置 | 是什麼 |
| --- | --- | --- |
| **後端** | `memory.backends.<name>` | 一個資料庫：SQLite 檔案、PostgreSQL 或 Redis 伺服器、Qdrant 叢集，或行程記憶體。 |
| **設定檔** | `profiles.memory.<name>` | 一組具名的邏輯**儲存區**，每個儲存區是某個後端上的一張資料表，另外定義 `memory.recall` 如何搜尋它們。 |
| **代理** | `agents.<name>.memory` | 代理使用哪個設定檔：設定檔名稱、內建的 `conversational`，或內嵌設定檔。 |

多個代理可以共用一個設定檔。每個代理仍有自己的資料表（參見[隔離](#isolation)），所以共用設定檔共用的是結構，而不是資料。

## 兩分鐘速覽

完全沒有記憶區段的組態就已經能運作：

```yaml
agents:
  assistant:
    loop: ./loops/chat.lua
    memory: conversational        # 內建設定檔
```

`conversational` 把對話存放在 `dialogue` 資料表（保留 30 天、最新 1000 筆記錄），把事實存放在可搜尋的 `facts` 資料表（永久保留），兩者都位於名為 `main_db` 的後端。組態沒有宣告任何後端時，`main_db` 就是執行環境儲存區本身（`runtime.persistence`），所以最小的部署不需要第二個資料庫。

在迴圈中：

```lua
memory.write({ type = "turn", id = msg.id, text = msg.text, reply = reply.text })
local recent = memory.recall({ kind = "all" })          -- 來自 dialogue 的記錄
```

代理需要 `memory` 能力，這項能力預設就已啟用。

## 後端

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

| 供應商 | 適用情境 | 讀取的設定 |
| --- | --- | --- |
| `sqlite` | 單一執行個體，不需要伺服器。鍵值、全文與暴力向量搜尋。 | `path`、`vector_scan_limit` |
| `postgres` | 共用伺服器。鍵值與全文；安裝 pgvector 後也支援向量。 | `url`、`dim` |
| `pgvector` | PostgreSQL 向量搜尋（為相容性而保留；`postgres` 已涵蓋）。 | `url`、`dim` |
| `redis` | 快速的共用鍵值，不支援搜尋。 | `url` |
| `redisvector` | Redis Stack：鍵值加向量搜尋。 | `url`、`dim` |
| `qdrant` | 大規模向量搜尋。 | `url`、`api_key`、`collection`、`dim`、`distance`、`timeout` |
| `volatile` | 行程記憶體：速度快、重新啟動即遺失，所有工作階段共用。 | `max_entries` |

- **密鑰。** `url`、`password` 與 `api_key` 接受 `${VAR}`、`${VAR:-default}` 或 `cred:<service>`，讓密鑰不必寫在檔案裡。密鑰無法解析的後端會被略過並發出警告，它的儲存區則移到仍可用的後端。
- **`dim`** 在向量資料表第一次建立時就固定下來，必須與嵌入模型相符（`text-embedding-3-small` 為 1536，許多開放模型為 768）。
- **`main_db`** 是內建設定檔的後端。組態一旦宣告了任何後端，也請一併宣告 `main_db`，否則使用 `conversational` 的代理會退回 `./data/agentflow.db`。Studio 的記憶畫面會替你寫出它。

完整的供應商表、位置規則與 Qdrant 傳輸細節，請見[組態](configuration.md)。

## 設定檔

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

設定檔名稱由 1–64 個字母、數字、`_`、`.` 或 `-` 組成。`conversational` 保留給內建設定檔。

### 儲存區

儲存區是迴圈使用的名稱（`dialogue`、`facts`、`semantic` 或你自訂的名稱），綁定到某個後端上的實體資料表。

| 鍵 | 意義 |
| --- | --- |
| `backend` | 必填。`memory.backends` 中的名稱。 |
| `table` | 實體資料表或集合。 |
| `retention` | 記錄寫入後保留多久：`30d`、`12h`、`2d12h` 或 `forever`。它是每次寫入的預設 TTL，而不是定期清理，所以在設定之前寫入的資料列永遠不會過期。 |
| `window` | 每張資料表最多保留的記錄數，由定期清理修剪。僅限鍵值後端；向量後端不會修剪。 |
| `requires` | 後端必須具備的功能：`kv`、`text_search`、`vector`。在啟動時檢查，所以放錯後端的儲存區會在那時就失敗，而不是等到第一次召回。 |
| `scope` | `user`（預設）依使用者分別保存資料列；`agent` 讓代理的所有使用者共用同一組資料。 |
| `pool` | 改為將儲存區綁定到租戶中某個具名的共用池。由授權決定誰能讀寫。綁定共用池的儲存區不接受 `scope`。 |
| `locality_ok` | 在區域單元中，允許把持久儲存區放在執行個體本地的後端上。 |

內建外掛會尋找三個儲存區名稱：

- **`dialogue`** 接收 `turn` 記錄，也是 `memory.recall` 預設讀取的儲存區。
- **`facts`** 接收 `fact` 記錄。以 `kind = "text"` 與 `store = "facts"` 搜尋它。
- **`semantic`** 是語意召回搜尋的對象；沒有它時則搜尋 `dialogue`。

### 召回

`recall` 決定 `memory.recall` 要分派給哪個處理常式。

| 值 | `memory.recall` 的行為 |
| --- | --- |
| `plugin:recency`（預設） | 直接讀取儲存區：最新的記錄、前置詞掃描、全文搜尋或時間區間。 |
| `plugin:semantic` | 嵌入查詢文字、執行向量搜尋，並可選擇重新排序。沒有 `text` 的查詢會退回依時間新近程度讀取。 |

語意召回需要在設定檔上設定三樣東西：

```yaml
    knowledge:
      stores:
        semantic: { backend: vectors, table: passages, retention: forever, requires: [vector] }
        dialogue: { backend: main_db, table: kb_dialogue, retention: 30d, window: 200 }
      recall: plugin:semantic
      embed_model: embed          # 一個能產生嵌入的 models: 項目
      rerank_model: rerank        # 選填：一個 provider: rerank 的 models: 項目
      oversample: 4               # 搭配重新排序器：取回 k*4 筆結果，再重新排序縮減為 k 筆
```

- **`embed_model`** 在召回時嵌入查詢。對於後端能存放向量的儲存區，它也會嵌入 `memory.write` 送往該儲存區的每筆記錄，所以寫入與查詢使用同一個模型。
- **`rerank_model`** 為選填。設定後，向量搜尋會取回 `k × oversample` 筆結果，由交叉編碼器排序；每筆結果都帶有 `meta.rerank_score`。

### 寫入

`write` 列出寫入鏈。目前 `memory.write` 一律執行路由表：隨附的 `plugin:routing_table`，或在你的部署覆蓋它時使用 `plugins/routing_table.lua`。這份清單本身會以 `agent.info().memory.write` 傳給迴圈，讓迴圈可以讀取，但它不會切換寫入鏈。請保留為 `[plugin:routing_table]`。

## 讓代理擁有記憶

```yaml
agents:
  helper:
    loop: ./loops/helper.lua
    memory: group_chat            # 具名設定檔

  scratch:
    loop: ./loops/scratch.lua
    memory:                       # 內嵌設定檔，僅供這一個代理使用
      stores:
        dialogue: { backend: main_db, table: scratch, retention: 1h }
```

- **能力。** 具有記憶的代理需要 `memory` 能力。它在預設能力組中；如果你自行列出 `capabilities:`，請將它包含在內。
- **衍生範本。** `profiles.agent.<name>.memory` 以相同方式指定設定檔。
- **不使用記憶。** 省略 `memory:`。此時 `memory.write` 無處可路由，召回則回傳空清單。

## 在迴圈中使用記憶

### 寫入

```lua
-- 一個輪次：路由到 "dialogue"，鍵為 turn:<id>。
memory.write({ type = "turn", id = msg.id, text = msg.text, reply = reply.text })

-- 一項事實：路由到 "facts"，以 key 作為鍵。
memory.write({ type = "fact", key = "name:" .. msg.from, text = "Their name is Ada." })

-- 明確指定目標：任何儲存區、任何鍵，以及選填的 ttl（秒）。
memory.write({ targets = {
  { store = "notes", key = "todo:" .. msg.id, value = { text = msg.text }, ttl = 3600 },
} })
```

記錄只會送到設定檔宣告的儲存區：設定檔中沒有 `dialogue` 儲存區的代理寫入 `turn`，會被直接捨棄，而不是視為錯誤。

### 召回

`memory.recall` 回傳 `{ key, value, meta }` 的清單。

```lua
-- plugin:recency
memory.recall({ kind = "all" })                                  -- dialogue
memory.recall({ kind = "text", store = "facts", text = "name", k = 5 })
memory.recall({ kind = "prefix", store = "notes", prefix = "todo:" })
memory.recall({ kind = "time", store = "dialogue",
                from = "2026-11-01T00:00:00Z", to = "2026-11-02T00:00:00Z" })

-- plugin:semantic
memory.recall({ text = msg.text, k = 5 })                        -- 語意召回，否則讀取 dialogue
```

沒有指定 `store` 時，recency 會讀取 `dialogue`，所以要在 `facts` 上做全文搜尋時，請指定儲存區。時間邊界是 RFC3339 字串。

### 原始資料表

`store.put`、`store.get`、`store.query` 與 `store.delete` 依名稱操作儲存區，不經過路由也不產生嵌入。說明請見 [Lua 迴圈](loops.md#memory)。

## 隔離

- **依代理。** 每個代理綁定自己的實體資料表（`<agent>.<table>`），所以使用同一個設定檔的兩個代理永遠不會讀到對方的資料列。
- **依使用者。** 在同一個代理內，資料列預設依使用者分別保存：一個人的輪次不會出現在另一個人的召回結果中。`scope: agent` 會為某個儲存區關閉這項隔離，適用於每位使用者都應該看到的知識。
- **刻意共用。** `pool: <name>` 把儲存區綁定到在租戶畫面建立的記憶池。由授權決定哪些租戶可以讀寫。

## 保留期限、視窗與上限

- `retention` 是寫入時蓋上的 TTL；之後修改只會影響新的資料列。
- `window` 在定期清理時修剪鍵值資料表中最舊的資料列。
- 位於 `volatile` 後端的儲存區必須有上限：後端設定 `max_entries`，或儲存區設定 `retention`。否則驗證會拒絕它，因為所有工作階段共用的行程記憶體會一直成長，直到行程終止。
- 在區域單元中，`dialogue`、`facts` 以及（使用語意召回時的）`semantic` 必須位於共用後端，不能是 SQLite 或 `volatile`，除非該儲存區設定了 `locality_ok: true`。

## 在 Studio 中設定

**記憶**（在兩個應用程式中都位於「連接」之下）讓你不必碰 YAML 就能編輯 `memory.backends` 與 `profiles.memory`。

- **後端。** 選擇供應商後，表單只會顯示該供應商讀取的設定。URL 與 API 金鑰僅可寫入：畫面只顯示 `${VAR}` 或 `cred:` 參照，或已遮罩密碼的 URL；欄位留空則保留已儲存的值。
- **設定檔。** 每個儲存區占一列：名稱、後端、資料表、保留期限、視窗、所需的功能，以及它的範圍或記憶池。儲存區下方是召回、寫入、嵌入與重新排序模型，以及過取樣倍數。新的設定檔一開始是內建設定檔的副本。
- **驗證** 會依整份組態檢查變更，方式與重新啟動時相同，但不寫入任何東西。**儲存** 會再驗證一次、保留一份 `.bak`，並且只寫入這兩個區段。
- 有東西正在使用某個項目時，**刪除** 會被拒絕；畫面會指出是誰在使用。
- **隱含的 `main_db`。** 沒有任何後端的組態會把 `main_db` 顯示為內建。儲存第一個後端時，也會把 `main_db` 一併寫入檔案，讓使用 `conversational` 的代理保有它們的儲存區。

在代理建構器中，**記憶設定檔**（位於安全設定檔旁邊）提供 *無*、*conversational（內建）*、每個具名設定檔，以及代理已有的內嵌設定檔（會依原樣保留）。

代理會在 agentflow 啟動時開啟它們的儲存區，所以記憶的變更要在重新啟動後才會生效。在記憶畫面建立的設定檔，也要在重新啟動後才會出現在建構器的選擇器中。

這個畫面使用下列管理路由：

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/memory` | 後端（密鑰以參照或遮罩後的 URL 呈現）、設定檔、各自的使用者，以及編輯器可提供的供應商、模型與外掛。 |
| `POST /admin/api/studio/memory/validate` | 對設定檔或後端草稿試執行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 建立、取代或移除一個項目。名稱不可變更。 |

## 範例

### 群聊：近期上下文加上記住的事實

```yaml
profiles:
  memory:
    group_chat:
      stores:
        dialogue: { backend: main_db, table: group_dialogue, retention: 7d, window: 500, requires: [kv], scope: agent }
        facts:    { backend: main_db, table: group_facts, retention: forever, requires: [kv, text_search], scope: agent }
      recall: plugin:recency
```

`scope: agent` 讓群組中的每個人共用同一份紀錄。`agentflow-studio` 儲存庫中的 `examples/qq-group-chat` 情境是完整版本。

### 以 SQLite 建立知識庫，不需額外伺服器

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

SQLite 以暴力方式搜尋向量，上限由 `vector_scan_limit` 決定（預設 50,000 列）。超過這個數量時，請把儲存區移到 `pgvector` 或 `qdrant`；迴圈不需要修改。參見 `examples/telegram-rag`。

### 在行程記憶體中的暫存空間

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

## 驗證不通過時

| 訊息 | 要修改什麼 |
| --- | --- |
| `agent "x" uses memory but lacks memory capability` | 在代理的 `capabilities` 中加入 `memory`，或移除這份清單以使用預設值。 |
| `agent "x" references unknown memory profile "p"` | 定義 `profiles.memory.p`，或修正名稱。在 Studio 建立的設定檔需要重新啟動後，代理才能使用。 |
| `store "s" references unknown backend "b"` | 定義 `memory.backends.b`。一旦宣告了任何後端，也必須宣告 `main_db`。 |
| `profiles.memory defines "conversational", which is reserved` | 改用其他名稱。 |
| `invalid retention "soon"` | 使用 `ms`、`s`、`m`、`h`、`d` 單位或 `forever`。 |
| `... volatile backend ... grows without bound` | 在後端設定 `max_entries`，或在儲存區設定 `retention`。 |
| `regional topology: ... instance-local` | 把儲存區移到共用後端；如果可以接受遺失，則設定 `locality_ok: true`。 |
| `recall plugin:semantic needs an embed model`（Studio） | 在設定檔上選擇嵌入模型。 |
| `a postgres backend needs a URL`（Studio） | 填入 URL，或指向它的 `${VAR}` / `cred:` 參照。 |
