## 架構

一個請求沿著一條不長的鏈路往下走。頻道驅動把平台位元組變成 `session.Message`；選用的身分層判定傳送者是誰；路由器的指令碼處理常式算出工作階段鍵。supervisor 找到或建立工作階段 actor，它在一個 Goja VM 裡跑一段 TypeScript/JS 迴圈。迴圈透過非同步橋呼叫由能力支撐的操作（以及設定好的 Goja 或 `wazero` Wasm 外部 API），驅動再去連外部系統。安全過濾器、稽核日誌與來源標記由核心掌管，它們就長在這條鏈路上，迴圈既刪不掉也偽造不了。

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

### 功能流程：一輪使用者請求

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

迴圈自己決定何時、如何呼叫工具。沒有 `on_tool_call` 掛鉤：迴圈從 `llm.chat` 的 `reply.tool_calls` 裡讀，然後自己呼叫 `tools.run`（見 `examples/loops/tool_loop.ts`）。`tools.run` 遇到 `needs_confirm` 會把指令碼執行暫停，等 actor 向使用者確認後再恢復。

---

### 多代理編排

靜態代理（各自帶模型與指令）透過 `agent.send`/`agent.request` 在 `can_contact` ACL 下協作；臨時子代理從 `profiles.agent` 範本衍生，能力逐層收窄。參考實作的編排器（主控 + 專家 + 專案經理 + 工人）就是建在這些原語上的常見形態 —— 它不在本執行環境裡，而是一個獨立的產品儲存庫：

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

`session.push`（能力 `channel.push`）是主動傳出 —— 背景代理不必有正在等待的傳入訊息也能觸達使用者。`session.exit` 乾淨地結束一個衍生工作階段（上層工作階段會收到 `agent.died`）。編排器的接線與各角色提示詞隨參考產品儲存庫提供，不在本執行環境中。

### 各層職責

| 層 | 負責 | 說明 |
| --- | --- | --- |
| channels | 傳輸 | 把平台原始位元組轉成共用的 `session.Message`，再把回覆送回去。`studio` 是為遊樂場註冊的合成頻道。 |
| identity | 身分 | 選用。把頻道帳號解析成 `user:<uuid>`，消費 `/link` 碼；當 `runtime.users.unregistered: reject` 時拒絕未註冊的傳送者。 |
| router | 路由 | 先記錄傳入，再在每個行程一個 Goja 服務 VM 裡執行 `gateway.route` 指令碼處理常式，算出工作階段鍵（可選地提議群組身分）。可熱重載。 |
| supervisor | 權威 | 工作階段查找與衍生、來源標記、群組成員驗證、依租戶的工作階段上限、代理跳轉時的 `can_contact`、衍生／請求／回覆，以及透過工作階段中樞完成的叢集交接。 |
| actor | 執行 | 每個工作階段一個 goroutine 加一個 Goja VM。驅動迴圈，在 `__af_op` 上暫停、拿到結果後恢復，在 `session.inbox()` 處跑安全傳入，在每次傳送時跑安全傳出。 |
| capabilities | 操作 | 面向指令碼的 API（`llm`、`memory`、`store`、`tools`、`api`、`shell`、`http`、`mail`、`files`、`agent`、`scheduler`、`session.state`、`user`、`credential`），都架在同一個非同步橋原語上。每個代理只拿到 `capabilities` 授予的處理常式。 |
| drivers | I/O | 連到真實系統：LLM 供應商、儲存與向量後端、Shell 提供者、MCP 伺服器、`wazero` Wasm 外部 API 模組、搜尋引擎、瀏覽器、郵件、S3。 |
| safety | 防護 | 由核心掌管的過濾鏈（`profiles.safety`、以模型為後盾的 `safety.classifiers`、`safety.script_filters`），在 actor 內部的傳入與傳出處執行，與迴圈怎麼寫無關。迴圈指令碼卸不掉它。 |
| admin plane | 維運 | `/healthz`、`/readyz`、`/metrics`，以及供 Studio 與瀏覽器主控台使用的 `/admin/api/*` JSON API，走 TCP 和／或本機通訊端、具名管道。 |

## 部署：兩個層級

引擎不是以單一行程執行，就是作為一個區域單元執行。層級是明確的 —— `runtime.topology` —— 由它決定哪些後端合法。啟動時會拒絕該層級無法兌現的設定，而不是先接受、再悄悄留一份執行個體本地的資料副本。未設定時按 `single` 處理，所以在這個設定出現之前寫的組態都能原樣啟動。

### `single` —— 單一執行個體

`runtime.persistence: sqlite://./data/agentflow.db`（或者乾脆不寫）就把稽核日誌、檔案儲存中繼資料、Token 帳冊、身分登錄表、憑證儲存區、租約表、工作階段收件匣與 Shell 控制代碼記錄都放在執行環境資料庫旁的本機檔案裡。不必裝東西，不必走網路，引擎對單一執行個體承諾的每一條都成立：稽核鏈完整、配額由重新啟動後依然存在的帳冊強制執行、排程工作照常觸發。

支援兩種形態：

- **單一資料庫模式** —— 全部用 SQLite（向量也在裡面，來自 SQLite 的向量支援），或用 PostgreSQL 加 pgvector。
- **拆分模式** —— 對話狀態用 SQLite 或 PostgreSQL，向量另用 Qdrant 或 redisvector。

blob 可以是本機目錄或 S3。Redis 與易失性記憶後端按定義就是執行個體本地的（執行個體就是部署）。允許使用日誌平面目錄，它恰好只屬於一個行程。配上自架模型（Ollama、vLLM、llama.cpp 掛 OpenAI 相容端點），這個層級完全不需要第三方服務。

`single` 部署會拒絕同一個儲存區上的第二個作用中行程。引擎維護一份執行個體登錄表 —— 執行環境儲存區裡每個行程一列心跳 —— 第二個行程啟動時就會帶著對端的 id、主機與最後活動時間停下。對端崩潰的情況也一樣：等過 30 秒的存活視窗，或用 `-force-single` 啟動一次。

### `regional` —— 一個區域內的多個執行個體

把 `runtime.persistence` 指向 PostgreSQL DSN，並設定層級：

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.internal:5432/agentflow?sslmode=require

media: { backend: s3, s3: { bucket: agentflow-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-files, region: eu-west-1 } }
```

這一層級的硬性要求由 `agentflow validate` 強制執行，所以每個執行個體都會在同一點拒絕同樣的錯誤設定：

| 設定 | 區域部署的要求 | 原因 |
| --- | --- | --- |
| `runtime.persistence` | PostgreSQL | 需要一個所有執行個體都看得見的儲存區。 |
| `runtime.identity.persistence`、`runtime.credentials.path` | 不設定（跟隨執行環境儲存區），或使用 PostgreSQL。SQLite 會被拒絕 | 共用的身分登錄表與憑證儲存區；兩個執行個體必須把同一個帳號解析成同一個使用者。 |
| `runtime.cluster.persistence` | 已棄用。不設定，或與 `runtime.persistence` 指向同一目標 | 租約與工作階段收件匣現在總是在執行環境儲存區中；區域單元裡指向不同目標會被拒絕。 |
| `memory.backends` | 不能用 `sqlite` 供應商 | 否則每個執行個體都會留一份自己的對話歷史。請用 `postgres`、`pgvector`、`qdrant`、`redis` 或 `redisvector`。 |
| `media.backend`、`files.backend` | `s3` | 否則每個執行個體都會在本機目錄留一份自己的 blob。 |
| `runtime.log_plane.persistence` | 不設定 | 日誌平面目錄只屬於一個行程，單元的稽核日誌必須共用。 |
| `ADMIN_TOKEN` 或 `runtime.admin.operators` | 至少設定一個 | 每次啟動產生的權杖對對等執行個體一無所知，所以啟動會拒絕產生。具名操作員可以完全取代共用權杖。 |

每個執行個體都登錄到同一個執行環境儲存區，每 10 秒一次心跳，最後心跳在 30 秒內就算存活。`/admin/api/state` 會回報 `topology`、`zone` 與每個作用中的 `instances` 列，並對 `config_epoch` 不一致的對端加上標記；Studio 的總覽頁顯示同樣的清單。心跳只是存活標記，不是鎖 —— 誰擁有工作仍然由租約表說了算。

### 多區域

沒有全域層。多區域部署就是每個區域一個獨立的 `regional` 單元，單元之間執行時什麼都不共用。前面的應用把每個使用者固定到一個歸屬區域，註冊、邀請、憑證設定與流量都發往那裡；每個區域是獨立的故障域，熱路徑上沒有跨區域延遲，資料也留在本區域。共用的聊天整合（一個 Telegram 機器人只有一個接收方）屬於那個入口服務，由它透過 `bridge` 頻道轉發到使用者所在區域。管理與計費依區域扇出，災難復原則是把備用 PostgreSQL 提升起來，起一個替代單元。完整做法 —— 包括 bridge、依區域計費與災難復原手冊 —— 見[多區域](multi-region.md)。

目標是「協定 + 位址」：伺服器用 `postgres://…` 或 `postgresql://…`，本機檔案用 `sqlite://<path>` 或裸路徑，日誌平面則用目錄 —— `./data/log` 或 `file://./data/log`。所有目標都在啟動時於同一處解析，所以引擎沒有後端的協定會直接報啟動錯誤，並列出它支援的協定：拼錯也不會悄悄變成一個同名的目錄。

### 日誌平面

引擎的狀態有三種形態，形態決定它該放哪裡：

- **熱路徑上需要讀來做決策** —— 配額檢查與預算刷新在每次呼叫前都要讀的 Token 彙總、檔案儲存中繼資料、工作階段狀態、路由狀態、Shell 控制代碼。這些留在交易式儲存區裡，求和精確，並且對所有執行個體立即可見。
- **寫得勤、讀得少、依時間過期** —— 訊息日誌與逐次呼叫的用量明細。這就是日誌平面，也是部署可以換個地方放的部分：`runtime.log_plane.persistence: ./data/log` 把兩者都寫成換行分隔的 JSON，每個 UTC 日一個檔案，並以整天刪除來清理。
- **儲存區之上的東西一律不變**：操作員查看的稽核鏈與資料頁上的逐次明細，都來自平面所指的位置；而 `usage_daily` —— 這份不能挪走的彙總 —— 讓每一次配額與預算決策都保持精確。在把它指向目錄之前，有三條限制值得知道。機器崩潰可能丟掉最後寫入的記錄，因為它們沒有 fsync（訊息日誌在兩個呼叫點本來就是盡力而為，所以這並沒有削弱任何已承諾的東西）。這個目錄只屬於一個行程。而放在本機檔案上的訊息日誌是*依執行個體*的，這恰好把稽核鏈本該彙總的東西拆散了 —— 區域單元把訊息日誌留在共用儲存區裡，驗證器也會拒絕在那裡用日誌平面目錄。保留期限仍依平面分別設定：`audit.retention_days` 清理訊息日誌（無論它在哪兒），`usage.retention_days` 清理用量明細，各自回報刪了什麼。

### 多活保證了什麼

| 保證 | 機制 | 值得知道的限制 |
| --- | --- | --- |
| 一個對話只在一處執行 | 一個 `session:<agent\|key>` 租約；拿不到租約的執行個體會把訊息排隊，而不是傳遞。對等執行個體拿到租約後，舊擁有者的 actor 會停止，計時器觸發也像其他訊息一樣經工作階段中樞路由 | 被硬殺的持有者，其租約在 TTL（預設 30s）記憶體活；正常關閉則立即釋放。工作階段閒置時，租約遺失最多在 TTL 三分之一內被發現；有訊息在等則立即發現 |
| 每則訊息都能到達它的工作階段 | 共用收件匣，由擁有者排空，依入列時間排序 | 至少一次：傳遞與確認之間崩潰會重送。同一執行個體重送同一則訊息 ID 只算一列 |
| 接管會帶走積壓 | 未傳遞的列仍可認領，新擁有者會排空它們。已傳遞的列一小時後清掃，收件匣不會無限成長 | 進行中的一輪會遺失並重新傳遞，所以迴圈可能看到同一則訊息兩次 |
| 常駐代理能撐過執行個體故障 | 每半個工作階段 TTL 跑一次故障轉移檢查，向租約表詢問每個常駐代理的 `boot` 工作階段歸誰所有；無人擁有時，由某個執行個體認領並拉起 | 啟動輪只在新擁有者上跑一次。常駐代理的對話存在共用記憶後端裡，所以是接著聊而不是從空白開始 |
| 工作階段在故障轉移後仍保有它的 Shell | 沒什麼要保的：Shell 命令是共用檔案儲存區裡某個專案上的一次性沙箱作業，下一條命令在哪台執行個體上跑都行 | 在故障執行個體上執行的作業會丟，連同尚未寫回的內容；背景作業隨後會報為失敗而不是重跑。SSH 控制代碼不會遷移：迴圈需要重新衍生一個 |
| 排程觸發器只觸發一次 | 一個 `trigger:<name>` 租約，依觸發器自身的間隔認領 | 被殺的持有者會讓觸發器卡住，直到認領過期（一個間隔再加點餘量） |
| 清理工作每個單元只跑一次 | `files-gc`、`files-scratch`、`journal-prune`、`usage-prune`、`rows-sweep` 各自的租約，持有時間為作業自身間隔加餘量 | 跑超時的清掃仍可能和另一個執行個體重疊 |
| 正常關機會排空 | 就緒探針先轉 false，再過 `runtime.shutdown_grace`（區域 5s，單一執行個體 0）才關閉監聽器；工作階段中樞結束時釋放工作階段 | 崩潰不排空；對等執行個體改在租約 TTL 時接管 |
| 預算在總體上成立 | 每日代理預算讀帳冊而不是自己的計數器（最多每 15s 重新整理一次） | 超支幅度取決於整個叢集在一個刷新間隔內的花費，與執行個體數無關。滾動視窗預算仍是依執行個體的 |
| 依使用者配額成立 | 共用帳冊，加上進行中的預佔 —— 區域單元中共用，`single` 中行程本地 | 區域部署裡每次模型呼叫要一次交易做預佔；真正讓兩個執行個體的預留變成同一個決策的，是依使用者建議鎖 |

### 部署必須守住的幾條不變量

- **路由處理常式的狀態放儲存區裡，別放指令碼全域。** 路由器是每行程一個 Goja VM，哪個執行個體收到傳入就由它執行 —— 所以在模組全域裡記東西的處理常式，會讓每個執行個體給出各自的答案，重新啟動還會忘光。該放 `router.state.*`：持久、全部署一致、每個執行個體同一個值，而且撐得過故障轉移。
- **要嘛明確設定 `ADMIN_TOKEN`，要嘛給每個執行個體都取操作員名。** 區域單元裡會拒絕產生每次啟動的權杖，因為各主控台都會拒絕別人的權杖。
- **訊息 ID 依執行個體唯一，不是依叢集唯一。** 頻道驅動常給自己的訊息編號（`wh-1`）。收件匣會用接收它的執行個體來限定，所以不會丟東西 —— 但只依 ID 在訊息日誌裡做稽核查詢，可能匹配到兩個執行個體的列。查詢時把 ID 當作依執行個體的鍵。
- **比一比組態世代。** `/admin/api/state` 會回報 `config_epoch` —— 依寫入原樣算出的組態片段短雜湊 —— 並標記世代不一致的作用中執行個體。兩個執行個體不一致，表示有一個跑的是別的部署；輪換密鑰不會改變它（雜湊的是寫下的內容，不是解析後的值）。
- **主控台的組態修改只會到達一個執行個體。** 模型分頁熱套用到服務該主控台的行程；重載監看器只涵蓋迴圈、指令與提示詞檔案，不涵蓋模型登錄表。要儲存到組態檔，再滾動更新整個叢集。
- **租約與收件匣都在執行環境儲存區裡。** 它們必須共用一個目標：租約在一處、收件匣在另一處，會把工作階段路由給一個永遠看不到訊息的所有者。`runtime.cluster.persistence` 正因此被棄用。
- **每個執行個體都需要沙箱才能跑 Shell 作業。** 作業在擁有該工作階段的執行個體上執行，所以每個執行個體都要配好沙箱執行環境（`gvisor` 用 `runsc` 加非特權使用者命名空間，`microsandbox` 用可存取 hypervisor 的 `msb`）及其環境映像。專案與背景作業記錄在共用儲存區裡；執行個體上的工作目錄只是它們的快取。
- **每個專案在每個執行個體上同時只跑一個作業，不是每個單元一個。** 同一使用者的兩個工作階段落在不同執行個體時，兩個執行個體可能同時在同一個專案上跑命令。兩邊不會互相覆蓋：兩邊都改過的路徑保留先存的版本，後完成的作業會把它報為衝突。
