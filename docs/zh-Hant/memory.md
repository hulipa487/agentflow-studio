# 記憶

記憶是代理在輪次之間保留的東西：最近的對話、被告知的事實、可供檢索的文件。本頁說明它是怎麼搭起來的、如何在組態檔裡或 Studio 中設定，以及迴圈怎麼使用它。

一共三層：

| 層 | 位置 | 是什麼 |
| --- | --- | --- |
| **後端** | `memory.backends.<name>` | 一個資料庫：SQLite 檔案、PostgreSQL 或 Redis 伺服器、Qdrant 叢集，或處理程序記憶體。 |
| **設定檔** | `profiles.memory.<name>` | 一組具名的邏輯 **儲存區**，每個都是某個後端上的一張表，外加 `memory.recall` 如何在其中檢索。 |
| **代理** | `agents.<name>.memory` | 代理用哪個設定檔：設定檔名、內建的 `conversational`，或內嵌設定檔。 |

多個代理可以共用一個設定檔。每個代理仍然有自己的表（見[隔離](#isolation)），所以共用設定檔共用的是形狀，不是資料。

## 兩分鐘版

完全沒有 memory 段的組態本來就能用：

```yaml
agents:
  assistant:
    loop: ./loops/chat.ts
    memory: conversational        # 內建設定檔
```

`conversational` 把對話存在 `dialogue` 表（30 天，最近 1000 筆紀錄），把事實存在可檢索的 `facts` 表（永久保留），後端叫 `main_db`。組態裡沒宣告任何後端時，`main_db` 就是執行環境儲存區本身（`runtime.persistence`），所以最小部署不需要第二個資料庫。

在迴圈裡：

```ts
memory.write({ type: "turn", id: msg.id, text: msg.text, reply: reply.text });
const recent = memory.recall({ kind: "all" });          // 來自 dialogue 的記錄
```

代理需要 `memory` 能力，它在預設集合裡。

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

| 供應商 | 適合 | 讀取的設定 |
| --- | --- | --- |
| `sqlite` | 單一執行個體，不需伺服器。鍵值、全文搜尋與暴力向量搜尋。 | `path`、`vector_scan_limit` |
| `postgres` | 共用伺服器。鍵值與全文搜尋；裝了 pgvector 也能做向量。 | `url`、`dim` |
| `pgvector` | PostgreSQL 向量搜尋（為相容保留；`postgres` 已涵蓋）。 | `url`、`dim` |
| `redis` | 快速共用鍵值，無搜尋。 | `url` |
| `redisvector` | Redis Stack：鍵值加向量搜尋。 | `url`、`dim` |
| `qdrant` | 大規模向量搜尋。 | `url`、`api_key`、`collection`、`dim`、`distance`、`timeout` |
| `volatile` | 處理程序記憶體：快，重新啟動即遺失，所有工作階段共用。 | `max_entries` |

- **密鑰。** `url`、`password` 與 `api_key` 接受 `${VAR}`、`${VAR:-default}` 或 `cred:<service>`，密鑰因此不會進檔案。密鑰解析不出來的後端會被跳過並給出警告，它的儲存區轉到還活著的後端上。
- **`dim`** 在向量表首次建立時固定，必須和嵌入模型一致（`text-embedding-3-small` 是 1536，很多開放模型是 768）。
- **`main_db`** 是內建設定檔的後端。組態裡一旦宣告了任何後端，就要把 `main_db` 也宣告出來，否則用 `conversational` 的代理會退回到 `./data/agentflow.db`。Studio 的「記憶」畫面會替你寫出來。

完整的供應商表、locality 規則與 Qdrant 傳輸細節見[組態](configuration.md)。

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

設定檔名為 1–64 個字母、數字、`_`、`.` 或 `-`。`conversational` 保留給內建設定檔。

### 儲存區

儲存區是迴圈使用的名字（`dialogue`、`facts`、`semantic`，或你自己取的），綁定到某個後端上的一張實體表。

| 鍵 | 含義 |
| --- | --- |
| `backend` | 必填。一個 `memory.backends` 名稱。 |
| `table` | 實體表或集合。 |
| `retention` | 紀錄寫入後存活多久：`30d`、`12h`、`2d12h` 或 `forever`。它是每次寫入的預設 TTL，不是清掃，所以在設定它之前寫入的列永遠不會過期。 |
| `window` | 每張表最多保留多少筆紀錄，由定期清掃裁剪。僅限鍵值後端；向量後端不裁剪。 |
| `requires` | 後端必須具備的特性：`kv`、`text_search`、`vector`。啟動時檢查，所以放錯後端的儲存區在啟動時就失敗，而不是等到第一次喚回。 |
| `scope` | `user`（預設）依使用者保留資料列；`agent` 在整個代理的所有使用者之間共用一份。 |
| `pool` | 改為把儲存區綁定到「租戶」裡的具名共用集區，由授權決定誰能讀寫。綁定集區的儲存區不設 `scope`。 |
| `locality_ok` | 在區域單元中，接受把持久儲存區放在執行個體本地的後端上。 |

內建外掛會找三個儲存區名字：

- **`dialogue`** 存 `turn` 紀錄，也是 `memory.recall` 預設讀的地方。
- **`facts`** 存 `fact` 紀錄。用 `kind = "text"` 與 `store = "facts"` 檢索。
- **`semantic`** 是語意喚回的檢索目標；沒有它就在 `dialogue` 裡檢索。

### 喚回

`recall` 決定 `memory.recall` 分派到哪個處理常式。

| 值 | `memory.recall` 的行為 |
| --- | --- |
| `plugin:recency`（預設） | 直接讀儲存區：最新紀錄、前綴掃描、全文搜尋或時間視窗。 |
| `plugin:semantic` | 把查詢文字轉成嵌入，做向量搜尋，可選地重新排序。沒有 `text` 的查詢退回到依時間。 |

語意喚回需要在設定檔上配三樣東西：

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

- **`embed_model`** 在喚回時把查詢轉成嵌入。它也會把 `memory.write` 送到支援向量的後端儲存區的每筆紀錄轉成嵌入，所以寫入與查詢用的是同一個模型。
- **`rerank_model`** 選用。設定後，向量搜尋先取 `k × oversample` 個命中，再由交叉編碼器排序；每筆結果帶上 `meta.rerank_score`。

### 寫入

`write` 列出寫入鏈。目前 `memory.write` 一律走路由表：隨附的 `plugin:routing_table`，或部署覆寫了它時的 `plugins/routing_table.ts`。這個清單會作為 `agent.info().memory.write` 傳給迴圈，迴圈可以讀它，但它並不會切換鏈路。保持 `[plugin:routing_table]` 即可。

## 為代理設定記憶

```yaml
agents:
  helper:
    loop: ./loops/helper.ts
    memory: group_chat            # 具名設定檔

  scratch:
    loop: ./loops/scratch.ts
    memory:                       # 內嵌設定檔，僅供這一個代理使用
      stores:
        dialogue: { backend: main_db, table: scratch, retention: 1h }
```

- **能力。** 帶記憶的代理需要 `memory` 能力。它在預設集合裡；如果你自己列了 `capabilities:`，記得把它寫進去。
- **衍生範本。** `profiles.agent.<name>.memory` 用同樣的方式指定設定檔。
- **不要記憶。** 把 `memory:` 留空。此時 `memory.write` 無處可路由，喚回傳回空清單。

## 在迴圈裡使用記憶

### 寫入

```ts
// 一個輪次：路由到 "dialogue"，鍵為 turn:<id>。
memory.write({ type: "turn", id: msg.id, text: msg.text, reply: reply.text });

// 一項事實：路由到 "facts"，以 key 作為鍵。
memory.write({ type: "fact", key: "name:" + msg.from, text: "Their name is Ada." });

// 明確指定目標：任何儲存區、任何鍵，以及選填的 ttl（秒）。
memory.write({ targets: [
  { store: "notes", key: "todo:" + msg.id, value: { text: msg.text }, ttl: 3600 },
] });
```

紀錄只會進設定檔裡宣告過的儲存區：設定檔沒有 `dialogue` 儲存區的代理寫 `turn` 會被丟棄，而不是報錯。

### 喚回

`memory.recall` 傳回 `{ key, value, meta }` 清單。

```ts
// plugin:recency
memory.recall({ kind: "all" });                                  // dialogue
memory.recall({ kind: "text", store: "facts", text: "name", k: 5 });
memory.recall({ kind: "prefix", store: "notes", prefix: "todo:" });
memory.recall({ kind: "time", store: "dialogue",
                from: "2026-11-01T00:00:00Z", to: "2026-11-02T00:00:00Z" });

// plugin:semantic
memory.recall({ text: msg.text, k: 5 });                         // 語意召回，否則讀取 dialogue
```

不帶 `store` 時，依時間喚回讀 `dialogue`，所以要在 `facts` 上做全文搜尋就得指明儲存區。時間邊界是 RFC3339 字串。

### 原始表

`store.put`、`store.get`、`store.query` 與 `store.delete` 依名字直接操作儲存區，不經過路由也不做嵌入。詳見[指令碼迴圈](loops.md#memory)。

## 隔離

- **依代理。** 每個代理綁定自己的實體表（`<agent>.<table>`），所以共用同一個設定檔的兩個代理永遠讀不到對方的列。
- **依使用者。** 在同一代理內，資料列預設依使用者保留：一個人的輪次不會出現在另一個人的喚回裡。`scope: agent` 會關掉某個儲存區的這項行為，用於所有人都該看到的知識。
- **刻意共用。** `pool: <name>` 把儲存區綁定到「租戶」畫面建立的集區。授權決定哪些租戶可以讀寫。

## 保留期限、視窗與上限

- `retention` 是寫入時打上的 TTL；之後再改只影響新列。
- `window` 在定期清掃時裁掉鍵值表中最舊的列。
- `volatile` 後端上的儲存區必須有界：要嘛後端設 `max_entries`，要嘛儲存區設 `retention`。否則驗證會拒絕，因為所有工作階段共用的處理程序記憶體會一直漲到行程死掉。
- 在區域單元中，`dialogue`、`facts` 與（啟用語意喚回時的）`semantic` 必須放在共用後端上，不能用 SQLite 或 `volatile`，除非儲存區設了 `locality_ok: true`。

## 在 Studio 裡設定

**記憶**（在兩個應用程式的「接入」下）不改 YAML 就能編輯 `memory.backends` 與 `profiles.memory`。

- **後端。** 選一個供應商，表單只顯示該供應商讀取的設定。URL 與 API 金鑰只寫不讀：畫面顯示 `${VAR}`、`cred:` 參照，或把密碼打碼後的 URL；留空即保留已儲存的值。
- **設定檔。** 每個儲存區一列：名稱、後端、表、保留期限、視窗、它要求的特性，以及範圍或集區。儲存區下面是喚回、寫入、嵌入與重新排序模型，以及過取樣倍數。新設定檔以內建設定檔為副本開始。
- **驗證** 依整份組態檢查改動，和重新啟動時一樣，但不寫任何東西。**儲存** 會再驗證一次，留一份 `.bak`，並且只寫這兩節。
- **刪除** 在有東西參照時會被拒絕，畫面會指出是哪一個。
- **隱式的 `main_db`。** 沒宣告後端的組態會把 `main_db` 顯示為內建。儲存第一個後端時，`main_db` 也會一併寫進檔案，用 `conversational` 的代理因此不會丟儲存區。
- 在代理建構器裡，**記憶設定檔**（挨著安全設定檔）提供 *無*、*conversational（內建）*、每個具名設定檔，以及該代理已有的內嵌設定檔（依原樣保留）。

代理在 agentflow 啟動時開啟儲存區，所以記憶的改動要重新啟動後才生效。在「記憶」畫面新建的設定檔也要等那次重新啟動後才會出現在建構器的選擇器裡。

畫面使用這些管理路由：

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/memory` | 後端（密鑰以參照或打碼 URL 呈現）、設定檔、各自被誰使用，以及編輯器可選的供應商、模型與外掛。 |
| `POST /admin/api/studio/memory/validate` | 對設定檔或後端草稿做一次試執行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 建立一個、取代一個或刪除一個。名稱不可變更。 |

## 配方

### 群組聊天：近期上下文加上記住的事實

```yaml
profiles:
  memory:
    group_chat:
      stores:
        dialogue: { backend: main_db, table: group_dialogue, retention: 7d, window: 500, requires: [kv], scope: agent }
        facts:    { backend: main_db, table: group_facts, retention: forever, requires: [kv, text_search], scope: agent }
      recall: plugin:recency
```

`scope: agent` 讓群組裡的所有人共用一份歷史。`agentflow-studio` 裡的 `examples/qq-group-chat` 就是一個完整版本。

### 建在 SQLite 上的知識庫，不額外加伺服器

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

SQLite 用暴力方式檢索向量，受 `vector_scan_limit` 限制（預設 50000 列）。超過這個量就把儲存區遷到 `pgvector` 或 `qdrant`，迴圈不用改。見 `examples/telegram-rag`。

### 處理程序記憶體裡的暫存空間

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

## 驗證報錯時

| 報錯 | 要改什麼 |
| --- | --- |
| `agent "x" uses memory but lacks memory capability` | 把 `memory` 加進代理的 `capabilities`，或刪掉這個清單以使用預設值。 |
| `agent "x" references unknown memory profile "p"` | 定義 `profiles.memory.p`，或修正名字。在 Studio 裡建的設定檔要重新啟動後代理才能用。 |
| `store "s" references unknown backend "b"` | 定義 `memory.backends.b`。一旦宣告了任何後端，`main_db` 也必須宣告。 |
| `profiles.memory defines "conversational", which is reserved` | 換個名字。 |
| `invalid retention "soon"` | 用 `ms`、`s`、`m`、`h`、`d` 單位，或 `forever`。 |
| `... volatile backend ... grows without bound` | 在後端上設 `max_entries`，或在儲存區上設 `retention`。 |
| `regional topology: ... instance-local` | 把儲存區挪到共用後端；如果丟了也能接受，就設 `locality_ok: true`。 |
| `recall plugin:semantic needs an embed model`（Studio） | 在設定檔上選一個嵌入模型。 |
| `a postgres backend needs a URL`（Studio） | 填上 URL，或填指向它的 `${VAR}` / `cred:` 參照。 |
