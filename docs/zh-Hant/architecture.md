## 架構

一個請求會依序穿過一個不深的堆疊。頻道驅動程式把平台傳來的位元組轉成 `session.Message`。選用的身分層判斷是誰傳送的，路由器的指令碼處理常式再選出工作階段鍵。監督者（supervisor）找到或建立工作階段 actor，由它在 Goja 虛擬機器中執行一個 TypeScript/JS 迴圈。迴圈透過非同步橋接呼叫以能力為後盾的操作（以及已設定的 Goja 或 `wazero` Wasm 外部 API），驅動程式再連到外部系統。安全過濾器、稽核記錄與來源資訊都由核心掌管。它們就位在這條路徑上，所以迴圈無法移除或偽造。

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

### 功能流程：使用者的一個輪次

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

何時以及如何呼叫工具，由迴圈決定。沒有 `on_tool_call` 掛鉤：迴圈從 `llm.chat` 讀取 `reply.tool_calls`，再自行呼叫 `tools.run`（參見 `examples/loops/tool_loop.ts`）。若 `tools.run` 回應 `needs_confirm`，指令碼執行會暫停，由 actor 詢問使用者，之後再繼續執行。

---

### 多代理協作

靜態代理（各自有自己的模型與指令）在 `can_contact` ACL 的約束下，透過 `agent.send`/`agent.request` 互相協作；臨時子代理則從 `profiles.agent` 範本衍生，並縮減其能力。參考用的協調器（主代理 + 專家 + 專案經理 + 工作者）是建立在這些基本元件上的常見模式；它不在這個執行環境中，而是放在另一個產品儲存庫：

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

`session.push`（能力 `channel.push`）是主動傳出：背景代理不需要有正在處理的傳入訊息，就能聯絡使用者。`session.exit` 會乾淨地結束一個衍生的工作階段（父代理會收到 `agent.died`）。協調器的串接方式與各角色的提示詞隨參考產品儲存庫提供，不在這個執行環境中。

### 各層職責

| 層 | 負責 | 說明 |
| --- | --- | --- |
| channels | 傳輸 | 把平台的原始位元組正規化成共用的 `session.Message`，並把回覆送回去。`studio` 是為遊樂場註冊的虛擬頻道。 |
| identity | 身分 | 選用。把頻道帳號解析成 `user:<uuid>`、消耗 `/link` 代碼，並在 `runtime.users.unregistered: reject` 時拒絕未註冊的傳送者。 |
| router | 路由 | 先將傳入訊息寫入稽核記錄，再於每個行程一個的 Goja 服務虛擬機器中執行 `gateway.route` 指令碼處理常式，算出工作階段鍵（也可以提議一個群組身分）。可熱重新載入。 |
| supervisor | 權限 | 工作階段的查找與衍生、蓋上來源資訊、驗證群組成員資格、每個租戶的工作階段上限、代理之間轉送時的 `can_contact`、spawn/request/reply，以及透過工作階段中樞在叢集內交接。 |
| actor | 執行 | 每個工作階段一個 goroutine + 一個 Goja 虛擬機器。驅動迴圈，在 `__af_op` 上暫停，取得結果後繼續，在 `session.inbox()` 執行傳入安全檢查，並在每次傳送時執行傳出安全檢查。 |
| capabilities | 操作 | 面向指令碼的 API（`llm`、`memory`、`store`、`tools`、`api`、`shell`、`http`、`mail`、`files`、`agent`、`scheduler`、`session.state`、`user`、`credential`），全部建立在單一的非同步橋接基本操作上。每個代理只會取得其 `capabilities` 授予的處理常式。 |
| drivers | I/O | 連到實際系統：LLM 供應商、儲存與向量後端、shell 供應商、MCP 伺服器、`wazero` Wasm 外部 API 模組、搜尋引擎、瀏覽器、郵件、S3。 |
| safety | 防護 | 由核心掌管的過濾鏈（`profiles.safety`、以模型為基礎的 `safety.classifiers`、`safety.script_filters`），不論迴圈怎麼寫，都會在 actor 內於傳入與傳出時執行。無法從迴圈指令碼移除。 |
| admin plane | 操作人員 | `/healthz`、`/readyz`、`/metrics`，以及 Studio 與瀏覽器主控台使用的 `/admin/api/*` JSON API，透過 TCP 和／或本機通訊端或具名管道提供。 |

## 部署：兩種層級

引擎可以單一行程執行，也可以作為區域單元執行。層級是明確設定的（`runtime.topology`），由它決定哪些後端有效。若某項設定是該層級無法滿足的，啟動時會直接拒絕，而不是接受它，卻默默在每個執行個體各存一份資料。未設定這個鍵即為 `single`，所以在這項設定出現之前寫的每份組態，都能照常啟動。

### `single`：單一執行個體

`runtime.persistence: sqlite://./data/agentflow.db`（或完全不設定）會把稽核記錄、檔案儲存區的中繼資料、Token 帳本、身分登錄、憑證儲存區、租約表、工作階段收件匣與 shell 控制代碼記錄，各自放在執行環境資料庫旁的本機檔案中。不需要安裝任何東西，也不必透過網路連到任何地方，而引擎對單一執行個體所做的每項保證都成立：稽核記錄完整、配額依據重新啟動後仍保留的帳本強制執行，排程的工作也會觸發。

支援兩種形態：

- **單一資料庫模式**：所有東西都用 SQLite（包括向量，透過 SQLite 的向量支援），或使用搭配 pgvector 的 PostgreSQL。
- **分離模式**：對話狀態使用 SQLite 或 PostgreSQL，向量則使用 Qdrant 或 redisvector。

Blob 可以放在本機目錄或 S3。Redis 與 volatile 記憶後端本質上就是執行個體本地的（執行個體就是整個部署）。可以使用日誌平面目錄，它就是一個行程的日誌。搭配自行架設的模型（Ollama、vLLM、在 OpenAI 相容端點後方的 llama.cpp），這個層級完全不需要任何第三方服務。

`single` 部署會拒絕在同一個儲存區上再執行第二個行程。引擎維護一份執行個體登錄：每個行程在執行環境儲存區中有一列心跳記錄，第二個行程會在啟動時停止，並顯示對方的 ID、主機與最後出現時間。這也涵蓋對方已當機的情況：等待 30 秒的存活視窗過去，或以 `-force-single` 啟動一次。

### `regional`：同一區域內的多個執行個體

將 `runtime.persistence` 指向 PostgreSQL DSN，並設定層級：

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require

media: { backend: s3, s3: { bucket: agentflow-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-files, region: eu-west-1 } }
```

層級的要求由 `agentflow validate` 強制執行，所以每個執行個體都會在同一個時間點拒絕同樣的錯誤設定：

| 設定 | Regional 要求 | 原因 |
| --- | --- | --- |
| `runtime.persistence` | PostgreSQL | 一個所有執行個體都看得到的儲存區。 |
| `runtime.identity.persistence`、`runtime.credentials.path` | 不設定（沿用執行環境儲存區），或 PostgreSQL。SQLite 會被拒絕 | 共用的身分登錄與憑證儲存區；兩個執行個體必須把同一個帳號解析成同一位使用者。 |
| `runtime.cluster.persistence` | 已淘汰。不設定，或與 `runtime.persistence` 相同的目標 | 租約與工作階段收件匣現在一律放在執行環境儲存區；在區域單元中設定不同的目標會被拒絕。 |
| `memory.backends` | 不可使用 `sqlite` 供應商 | 否則每個執行個體都會保有自己的對話紀錄。請使用 `postgres`、`pgvector`、`qdrant`、`redis` 或 `redisvector`。 |
| `media.backend`、`files.backend` | `s3` | 否則每個執行個體都會把自己的 blob 放在本機目錄。 |
| `runtime.log_plane.persistence` | 不設定 | 日誌平面目錄只屬於一個行程；單元的稽核記錄必須共用。 |
| `ADMIN_TOKEN` 或 `runtime.admin.operators` | 至少設定一項 | 每次啟動時產生的管理權杖，其他對等執行個體都不知道，所以啟動時會拒絕產生。具名的操作人員可以完全取代共用權杖。 |

每個執行個體都在同一個執行環境儲存區中註冊，每 10 秒送出一次心跳，只要最後一次心跳在 30 秒內，就視為執行中。`/admin/api/state` 會回報 `topology`、`zone` 與每一列執行中的 `instances`，並標出 `config_epoch` 不同的對等執行個體；Studio 的總覽也會顯示同一份清單。心跳只是存活標記，不是鎖：誰擁有哪些工作，仍以租約表為準。

### 多區域

沒有全域層級。多區域部署就是每個區域各一個獨立的 `regional` 單元，各單元在執行時不共用任何東西。前端的應用程式將每位使用者固定到一個主區域，並把註冊、邀請、憑證設定與流量都送往那裡；每個區域都是獨立的故障範圍，熱路徑上沒有跨區域延遲，資料也留在所屬區域。共用的聊天整合（一個 Telegram 機器人只有一個接收端）屬於這道前門，由它透過 `bridge` 頻道轉送到使用者所在的區域。管理與計費按區域分開進行，災難復原則是提升一台待命的 PostgreSQL，用來建立替代的單元。完整的模式，包括橋接、按區域計費與災難復原手冊，請見 [多區域](multi-region.md)。

目標由一個 scheme 與一個位址組成：伺服器用 `postgres://…` 或 `postgresql://…`，本機檔案用 `sqlite://<path>` 或直接寫路徑，日誌平面則是一個目錄：`./data/log` 或 `file://./data/log`。每個目標都在啟動時於同一處解析，所以引擎沒有對應後端的 scheme 會成為啟動錯誤，並列出它支援的 scheme：打錯字不會悄悄變成一個以它命名的目錄。

### 日誌平面

引擎的狀態有三種形態，而形態決定了它該放在哪裡：

- **在熱路徑上讀取以做出決定**：每次呼叫前配額檢查與預算更新要讀的 Token 彙總、檔案儲存區的中繼資料、工作階段狀態、路由狀態、shell 控制代碼。這些留在交易式儲存區，加總精確，且所有執行個體立即可見。
- **持續寫入、很少讀取、依時間過期**：訊息日誌與每次呼叫的用量明細。這就是日誌平面，也是部署可以放到別處的部分：`runtime.log_plane.persistence: ./data/log` 會把兩者都寫成以換行分隔的 JSON，每個 UTC 日一個檔案，並以刪除整日檔案的方式清理。

儲存區之上的一切都不變：操作人員讀到的稽核記錄，以及設定檔頁面上的每次呼叫明細，都來自平面所指向的位置；而 `usage_daily`（這份彙總不能搬走）讓每項配額與預算決策保持精確。在把它指向目錄之前，有三項限制值得了解。機器當機可能遺失最後寫入的記錄，因為它們沒有經過 fsync（稽核記錄在兩個呼叫點本來就是盡力而為，所以這並未削弱任何既有的承諾）。這個目錄只屬於一個行程。而且放在本機檔案的稽核記錄是*每個執行個體各一份*，恰好把稽核記錄的用途拆得支離破碎：區域單元會把稽核記錄放在共用儲存區，驗證器也會拒絕在那裡使用日誌平面目錄。保留期限仍按平面分別設定：`audit.retention_days` 清理稽核記錄（不論它放在哪裡），`usage.retention_days` 清理明細，兩者都會回報刪除了什麼。

### 多主動部署的保證

| 保證 | 機制 | 值得了解的限制 |
| --- | --- | --- |
| 一段對話只在一個地方執行 | 一個 `session:<agent\|key>` 租約；取不到租約的執行個體會把訊息排入佇列，而不是直接投遞。對等執行個體取得租約後，舊擁有者的 actor 會停止，計時器的觸發也像其他訊息一樣經由中樞路由 | 持有者被強制終止後，租約會再保留一個 TTL（預設 30 秒）；正常關機則會立即釋放。工作階段閒置時，遺失租約會在 TTL 的三分之一內被偵測到；若有訊息在等待，則立即偵測 |
| 每則訊息都會送達其工作階段 | 共用收件匣，由擁有者取出，依排入時間排序 | 至少一次：在投遞與確認之間當機，會重新投遞。同一執行個體以相同訊息 ID 重新張貼，只算一列 |
| 接手時會一併帶走積壓訊息 | 尚未投遞的資料列仍可被認領，由新擁有者取出。已投遞的資料列一小時後清除，收件匣不會無限成長 | 進行中的輪次會遺失並重新投遞，所以迴圈可能看到同一則訊息兩次 |
| 常駐代理在其執行個體失效後仍能存續 | 每半個工作階段 TTL 執行一次容錯移轉檢查，向租約表查詢每個常駐代理的 `boot` 工作階段是否有擁有者；若沒有，就由某個執行個體認領並啟動 | 啟動輪次會在新擁有者上執行一次。常駐代理的對話存放在共用記憶後端，所以會接續下去，而不是從空白開始 |
| 容錯移轉後工作階段保有它的 shell | 沒有需要保留的東西：shell 命令是在共用檔案儲存區中某個專案上執行的一次性沙箱工作，所以下一個命令會在目前擁有該工作階段的執行個體上執行 | 在失效的執行個體上執行中的工作會遺失，連同尚未寫回的內容；背景工作會被回報為失敗，而不會重新執行。SSH 控制代碼不會轉移：迴圈要重新衍生一個 |
| 排程觸發器只觸發一次 | 一個 `trigger:<name>` 租約，以觸發器本身的間隔認領 | 持有者被終止後，觸發器會停滯，直到認領過期（一個間隔再加上緩衝） |
| 例行維護在每個單元只執行一次 | `files-gc`、`files-scratch`、`journal-prune`、`usage-prune`、`rows-sweep` 租約，各自持有該工作的間隔再加上餘裕 | 執行時間超過租約的清理工作，仍可能與另一個執行個體的重疊 |
| 正常關機時會排空 | 就緒狀態先變為 false，經過 `runtime.shutdown_grace`（regional 為 5 秒，single 為 0）後才關閉監聽器；中樞在結束時釋放它的工作階段 | 當機會跳過排空；對等執行個體改為在租約 TTL 到期時接手 |
| 預算以總量計算 | 代理的每日預算讀取帳本，而不是自己的計數器（最多每 15 秒更新一次） | 超支的上限是整個叢集在一個更新間隔內的花費，與執行個體數量無關。滾動視窗預算仍按執行個體分別計算 |
| 每位使用者的配額成立 | 共用帳本，加上進行中的預留額度：在區域單元中共用，在 `single` 中只存在於行程內 | 區域預留額度每次模型呼叫要多一筆交易；每位使用者的諮詢鎖讓兩個執行個體的預留成為同一個決定 |

### 部署必須遵守的不變條件

- **路由處理常式把狀態放在儲存區，而不是指令碼全域變數中。** 路由器在每個行程是一個 Goja 虛擬機器，由接收到傳入訊息的那個執行個體執行；所以在模組全域變數中記住任何東西的處理常式，會讓每個執行個體各有不同的答案，重新啟動後也會遺忘。應該改用 `router.state.*`：持久、整個部署共用、在每個執行個體上值都相同，而且能撐過容錯移轉。
- **明確設定 `ADMIN_TOKEN`，或把每個執行個體都設為具名操作人員。** 在區域單元中，每次啟動時產生的權杖會被拒絕，而不會被產生，因為每個主控台都會拒絕其他主控台的權杖。
- **訊息 ID 在每個執行個體內唯一，而不是在整個叢集內唯一。** 頻道驅動程式常自行為訊息編號（`wh-1`）。收件匣會以接受該訊息的執行個體來限定它們，所以不會遺失任何東西；但只用 ID 在稽核記錄中查詢，可能會比對到兩個執行個體的資料列。查詢時請把 ID 當成每個執行個體各自的鍵。
- **比較組態紀元。** `/admin/api/state` 會回報 `config_epoch`，也就是組態片段原始寫法的短雜湊，並標出紀元不同的執行中執行個體。兩個執行個體不一致，表示其中一個執行的是不同的部署；輪替密鑰不會改變它（它雜湊的是寫下的內容，而不是解析後的內容）。
- **在主控台編輯組態只會影響一個執行個體。** 模型分頁會把變更熱套用到提供該主控台的行程；重新載入監看器涵蓋迴圈、指令與提示詞檔案，但不涵蓋模型登錄。請先永久儲存到組態檔，再滾動更新叢集。
- **租約與收件匣放在執行環境儲存區。** 它們必須共用同一個目標：租約在一處、收件匣在另一處，會把工作階段路由給一個永遠看不到其訊息的擁有者。`runtime.cluster.persistence` 正是因此被淘汰。
- **每個執行個體都需要沙箱才能執行 shell 工作。** 工作在擁有該工作階段的執行個體上執行，所以每一個都需要所設定的沙箱執行環境（`gvisor` 需要 `runsc` 與非特權使用者命名空間；`microsandbox` 需要 `msb` 與虛擬化存取權限）及其環境映像。專案與背景工作記錄存放在共用儲存區；執行個體上的工作區目錄只是它們的快取。
- **每個專案在每個執行個體上一次只執行一個工作，而不是在每個單元上。** 當同一位使用者的兩個工作階段落在不同執行個體上時，兩個執行個體可以同時對同一個專案執行命令。兩者不會互相覆寫：雙方都改過的路徑會保留先儲存的版本，較晚的工作則把它回報為衝突。
