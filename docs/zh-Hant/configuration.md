## 組態指南

空的部署也是有效的：`agents:` 可以完全省略，執行環境會以管理／Studio 控制平面的形式啟動，讓你從 Studio 建立第一個代理。不過頻道、觸發器及其他參照仍必須指向已存在的代理。

AgentFlow 透過單一 YAML 檔案（`agentflow.yaml`）設定。每個值都會在啟動時驗證：未知的鍵會成為附帶檔名與行號的啟動錯誤，絕不會在執行時才冒出意外。凡是接受字串的地方，都支援環境變數插值（`${VAR}` 與 `${VAR:-default}`）。

### 密鑰：環境變數參照、憑證儲存區，以及誠實降級

帶有密鑰的欄位（`models.*.api_key`、`search.engines.*.api_key`、`gateway.channels[].token`/`secret`、`memory.backends.*.config.url`/`password`/`api_key`、`media.s3.access_key`/`secret_key`、`browser.api_token`、`profiles.shell.*.password`）接受三種寫法：

- 字面值；
- `${VAR}`（或 `${VAR:-default}`）：從行程環境解析，並以憑證儲存區中同名的項目作為後備；
- `cred:<service>`：只從憑證儲存區解析。

在 **configdir** 路徑上，這些值會保持為延遲參照，在建構使用它們的元件時才解析；在單一檔案的 `-config` 路徑上，整份文件會在載入時逐位元組展開（舊有行為，維持不變）。解析順序為 環境變數 → 憑證儲存區 → 無法解析。降級原則一致：*選用*元件（搜尋引擎、telegram／ghhook 頻道、記憶後端、S3 媒體儲存區）上若有無法解析的憑證，**會略過該元件，並發出指名該憑證的警告**，絕不會導致啟動失敗。模型的 `api_key` *不*屬於選用：模型仍會保持設定，但第一次 LLM 呼叫會失敗，並給出指名未解析憑證的明確錯誤。`runtime.credentials.enabled=false`（或沒有儲存區）會使 `cred:` 參照無法解析，並套用同一原則。

### 檔案骨架

```
version: "1"

runtime:      { ... }   # 整個執行個體：persistence、admin、cluster、log_plane、
                        #   identity、credentials、users、region、timezone_offset_hours
models:       { ... }   # 具名的 LLM 供應商
memory:       { ... }   # 整個執行個體共用的後端執行個體
profiles:     { ... }   # 可重複使用的組態：memory/、safety/、shell/、agent/
gateway:      { ... }   # 一個共用的監聽器及其頻道
mcp:          { ... }   # MCP 伺服器
tools:        { ... }   # 全域工具原則
search:       { ... }   # 網頁搜尋引擎
legal_search: { ... }   # HKLII / NPC
browser:      { ... }   # Cloudflare Browser Run
net:          { ... }   # 對外連線原則
media:        { ... }   # blob 儲存區後端：fs（預設）| s3
files:        { ... }   # 使用者範圍的檔案儲存區：fs（預設）| s3，暫存 TTL
audit:        { ... }   # 訊息日誌：enabled、retention_days
usage:        { ... }   # 每位使用者的 Token 帳本：配額、事件、保留期限
triggers:     [ ... ]   # 由引擎觸發的排程／事件任務
prompts:      { ... }   # 整個部署共用的提示詞登錄表
agents:       { ... }   # 代理本身
plugins:      { ... }   # 外掛目錄、能力上限
```

### 設計理念

YAML 定義的是*拓撲*：有哪些東西、彼此如何連接。行為寫在 TypeScript/JS 外掛與 Wasm 外部 API 中，參數寫在 YAML 中。這樣的分工意味著一個可運作的聊天機器人只需約 12 行 YAML、零程式碼，同時管線中的每個環節仍可被覆寫。

> **嚴格解析**：`KnownFields(true)` 表示每個未知的鍵都是啟動錯誤，不存在被「忽略」的拼字錯誤。

### 組態目錄（`-config <dir>`）

除了單一檔案，你也可以把一個目錄傳給 `-config <dir>`，以載入目錄式配置；若目錄中只有一個 `agentflow.yaml`（即 `agentflow init DIR` 產生的內容），則載入該檔案。每個片段都以同樣嚴格的驗證解碼，且驗證在合併*之後*執行，因此跨檔案的參照會與單一檔案時完全一樣地檢查。組態來源（檔案所在的目錄，或目錄本身）是所有相對路徑的解析基準（迴圈、指令、提示詞的 `file:` 路徑、路由器路由、persistence、sqlite 後端路徑、shell 的 `key_file`、`plugins.dir`、`media.dir`）。單一檔案也是如此：`agentflow run -config deploy/agentflow.yaml` 會在該檔案旁解析 `./loops/x.ts`，與 shell 的工作目錄無關。

```
<dir>/
  system.yaml       # 必填：除 agents/channels 以外的頂層組態
  channels.yaml     # 選填：channels: [...]
  profiles/*.yaml   # 選填：每個檔案一個代理（依排序後的 glob）
  triggers/*.yaml   # 選填：triggers: [...]（依排序合併，後面的檔案優先）
```

`profiles/*.yaml` 檔案是一份代理組態，再加上 `name` 與 `spawn` 旗標：`spawn: false`（或未設定）會將它歸入 `agents`，`spawn: true` 則歸入 `profiles.agent`（一個衍生範本，包含其 `extras` 與 `credentials` 允許清單，衍生出的子代理兩者都看得到）。在衍生設定檔上使用僅限代理的欄位（`persistent`、`singleton`、`taps`、`channels`、`lifecycle`）會造成啟動錯誤。觸發器的形式為 `{name, event{channel, match} | cron | every, run_on_boot, target{profile}, payload}`：`every:` 與 `cron:` 觸發器*由引擎*執行（見下文的 `triggers`），而合併後的清單會透過 `runtime.triggers()` 以資料形式提供給迴圈。目錄式部署會定期輪詢並重新讀取 `triggers/*.yaml`，因此修改排程不需重新啟動即可生效。

## 完整組態參考

### `runtime`：整個執行個體的調校

| 鍵 | 型別 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `vm.memory_limit` | string | — | 純 Go JS/TS 虛擬機器（`goja`）的每狀態軟記憶體上限（`b`/`kb`/`mb`/`gb`/`MiB`/`GiB`；留空表示不設上限）。限制超大字串與集合配置，超限時以指令碼記憶體不足錯誤呈現 |
| `scheduler.workers` | int | 8 | 操作池大小。這是唯一設定它的地方 |
| `persistence` | string | sqlite://./data/agentflow.db | 訊息日誌、Token 帳本及其逐次呼叫明細，以及引擎的鍵值資料列（檔案樹、工作階段狀態、路由狀態、shell 控制代碼）存放的位置。排程器的計時器*不*在這裡：計時器服務位於記憶體中，因此重新啟動後所有計時器都會遺失，直到其迴圈重新註冊 |
| `topology` | string | single | 部署層級：`single`（單一行程）或 `regional`（同一區域內多個執行個體共用一個 PostgreSQL）。未設定即為 `single`。層級決定了其他哪些設定有效；無法滿足的設定會在啟動時被拒絕。見[拓撲](#topology) |
| `zone` | string | "" | 區域內的可用區標籤（字母、數字與 `. _ -`，最多 32 個字元）。會出現在指標與 `/admin/api/state` 中；它只是標籤，從不影響路由決策 |
| `shutdown_grace` | duration | 5s regional · 0 single | 關機時先回報未就緒並繼續服務多久才關閉監聽器，讓負載平衡器得以排空連線、進行中的輪次得以完成。`0` 會停用等待 |
| `sessions.idle_unload` | duration | 30m | 對話閒置這麼久之後即卸載。其 VM 區域變數會消失；`session.state.*` 會保留，下一則訊息會重新建立工作階段。工作階段 ID 不會作廢，其租約會被釋放。`0` 會停用卸載 |
| `admin.cors_origins` | list | \[\] | 允許呼叫 `/admin/api/*` 的確切瀏覽器來源（自訂前端或其他瀏覽器用戶端）。Studio 不需要此設定：桌面應用程式與網頁應用程式的本機代理伺服器都以同源方式呼叫 API。留空會讓 API 只允許同源；`"*"` 會被拒絕。預檢請求會在 Bearer 檢查之前回應 |
| `admin.socket` | string | "" | 新增一個僅限本機的管理監聽器：在 Unix 上為 Unix 通訊端路徑（相對路徑以組態目錄為基準解析），在 Windows 上為具名管道，例如 `\\.\pipe\agentflow-admin`。仍然需要 Bearer 權杖 |
| `admin.tcp` | bool | true | 在 HTTP 監聽器（`gateway.listen`）上提供管理平面，路徑為 `/admin/`，以及 `/healthz`、`/readyz` 與 `/metrics`。設為 `false` 時只在 `admin.socket` 上提供；至少必須保留一種傳輸方式。沒有 `admin.listen`：管理平面沒有自己的連接埠，設定它的組態會被拒絕 |
| `admin.operators` | map | {} | 具名的管理權杖（`name: token`），每個都具有與 `ADMIN_TOKEN` 相同的權限。記錄中會以該名稱標示請求來源。值可以是 `${VAR}` 或 `cred:<service>` 參照，於啟動時解析；無法解析的項目會被略過並發出警告 |
| `limits.sessions_per_tenant` | int | 0 | 在監督者的衍生邊界（路由後的首次接觸與 `agent.spawn`）限制每個租戶的執行中工作階段數。0 表示停用 |
| `limits.ephemeral_per_tenant` | int | 16 | 限制每位使用者同時進行的單輪工作階段數（OpenAI 頻道預設的工作階段類型）。它與 `sessions_per_tenant` 分開，因此無狀態流量不會占用對話名額。它需要有可計數的使用者，因此沒有使用者的請求（`auth: key` 或 `none` 的 `openai` 頻道）不受它限制。0 表示停用 |
| `limits.egress_per_minute` | int | 0 | 限制每個租戶每分鐘的 `http.request` 與郵件操作次數（固定視窗）。0 表示停用。租戶之間的操作池公平性一律開啟，且無法設定 |
| `studio.enabled` | bool | true | 提供 `/admin/api/studio/*`（建構器、遊樂場、追蹤）。沒有管理權杖時 API 不起作用；設為 `false` 時回應 503 |
| `studio.workspace` | string | <config dir>/studio | 單一檔案部署寫入 Studio 迴圈與提示詞的位置。目錄式部署則改寫入自己的 `loops/`、`prompts/` 與 `profiles/` |
| `studio.trace_events` | int | 2000 | 每個工作階段為追蹤時間軸保留的事件數（有上限的環狀緩衝區，最舊的先捨棄） |
| `studio.trace_sessions` | int | 200 | 保留的工作階段追蹤環數量；超過時淘汰最久未活動者 |
| `studio.trace_payloads` | bool | false | 在追蹤事件中包含訊息、回覆與結果文字。中繼資料（操作、模型、耗時、Token、錯誤）一律保留 |
| `studio.chat_timeout` | duration | 120s | 一個遊樂場輪次最多可等待多久，逾時則帶著部分回覆與 `timed_out` 返回 |
| `identity.enabled` | bool | false | 為每則傳入訊息產生穩定的使用者 UUID；將 `from` 改寫為 `user:<uuid>`，以啟用 `session.push_user`。開啟它會重新劃分既有狀態的範圍，見表格下方的說明 |
| `identity.persistence` | string | <persistence dir>/identity.db | 身分登錄表的 SQLite 路徑或 PostgreSQL DSN（預設沿用 `runtime.persistence`） |
| `identity.require_registration` | bool | true | 讓未連結的帳號停留在共用服務層（沒有個人範圍、沒有配額身分），直到它連結到使用者資料為止。`false` 會在首次接觸時自動認領一份使用者資料 |
| `identity.unregistered` | string | allow | 沒有使用者資料的傳送者如何處理：`allow` 繼續在共用層為其服務；`reject` 捨棄傳入訊息，並請對方註冊或連結 |
| `credentials.enabled` | bool | false | 加密的每租戶憑證儲存區，支撐 `auth={service=...}` |
| `credentials.persistence` | string | <persistence dir>/credentials.db | 憑證儲存區的 SQLite 路徑或 PostgreSQL DSN（預設沿用 `runtime.persistence`） |
| `credentials.master_key_env` | string | CREDENTIALS\_MASTER\_KEY | 存放主金鑰的環境變數（於啟動時讀取，絕不從檔案讀取） |
| `credentials.engine_wide` | string | allow | 控管 `credential.get()`，也就是把原始密鑰回傳給指令碼的操作。`allow`（或留空）讓代理讀取其 `credentials:` 清單中列出的項目；`maintenance` 將其限制於引擎觸發的輪次（啟動、排程器、觸發器），使租戶的輪次永遠看不到引擎全域的密鑰。每租戶路徑，也就是 `http.request` 與 `tools.http` 上的 `auth = {service = ...}`，從不回傳該值 |
| `users.enabled` | bool | false | 在閘道監聽器上提供公開的 `/v1/users` API（需要 `identity.enabled`） |
| `users.registration` | string | open | `open` 或 `invite`（來自 `POST /admin/api/users/invites` 的一次性邀請碼） |
| `users.link_ttl` | duration | 10m | 頻道連結碼的有效時間 |
| `users.push_same_tenant` | bool | false | 讓 `session.push_user` 拒絕當前輪次租戶以外的目標。適用於多租戶聊天產品 |
| `users.jit_provisioning` | bool | true | 在首次通過驗證的 OIDC 登入時建立使用者資料；`false` 會拒絕未知的主體，直到操作人員為其開通 |
| `users.oidc.issuer` · `audience` · `jwks_url` | string | — | 身分識別提供者驗證（見下文的*身分識別提供者*） |
| `users.oidc.subject_claim` · `email_claim` · `name_claim` | string | sub · email · name | 攜帶身分以及開通之使用者資料欄位的權杖宣告 |
| `cluster.session_ttl` | duration | 30s | 工作階段的認領在未續約的情況下能維持多久 |
| `cluster.poll_interval` | duration | 250ms | 執行個體多久查看一次是否有訊息在等待它所擁有的工作階段，也就是訊息落在錯誤執行個體時增加的延遲 |
| `region` | string | "" | 此執行個體所服務的區域名稱：指標標籤 `region`、`/admin/api/state` 中的一個欄位，也是執行個體登錄表的一部分。一個區域是一個獨立單元，而非全球部署的分片 |
| `log_plane.persistence` | string | "" | 僅附加平面（訊息日誌與逐次呼叫用量明細）所在的目錄，每個 UTC 日寫成一個 JSONL 檔案。留空則兩者都保留在執行環境儲存區中 |
| `timezone_offset_hours` | float | 0 | 比對 cron 觸發器時使用的 UTC 時差（例如 `8`、`-5.5`）；`every:` 不受影響 |

> **開啟身分層會重新劃分所有範圍**：`identity.enabled` 不是表面上的開關。開啟後，每則傳入訊息的 `from` 都會從頻道原生的傳送者改寫為穩定的 `user:<identity>`。個人範圍是之後才出現的：當帳號連結到使用者資料（或在首次接觸時認領一份，而這只在 `runtime.users.require_registration: false` 時才會發生），引擎就會在訊息由核心擁有的 `provenance` 上蓋上 `user_uuid`（迴圈讀取的就是這個欄位；它絕不寫入酬載，因為委派跳轉時酬載會由指令碼重新建構），使用者範圍的子系統即以此為準進行解析。接著檔案儲存區的範圍從 `agent:<agent>` 移到 `user:<uuid>`，一個輪次的記憶從共用服務層移到互動層，每位使用者的配額也有了可以計費的帳戶。在帳號完成連結之前，它完全沒有個人範圍：改變的只有 `from`。迴圈以 `from` 作為鍵的任何資料，或自行組成範圍的任何資料，都會跟著移動。
>
>  **沒有任何資料會遷移，而且引擎無法得知什麼移動了。** 以舊的開頭寫入的資料列（檔案、記憶，以及迴圈自行以鍵存放的任何東西）不會被接收，因為沒有辦法知道昨天的 `agent:bot` 屬於現在的 `user:u_xxxx` 這個人。所以請在真實流量進來之前開啟身分層，或自己撰寫接收邏輯：寫一個迴圈，在切換時執行一次，把舊開頭的資料列複製到新開頭。反過來也一樣：關閉身分層，或解除帳號連結，都會把個人範圍留在原處。

管理伺服器提供 `GET /healthz`、`GET /readyz`、`GET /metrics`（Prometheus 文字格式）、`GET /admin/sessions` 與 `/admin/credentials`。（工作階段清單以前是 `/v1/sessions`：該前綴屬於公開的使用者 API，而這個私有監聽器並不提供它。）即時計數器包括 `agentflow_sessions_active`、`agentflow_children_spawned`/`died`、`agentflow_llm_calls`、`agentflow_llm_tokens`、`agentflow_budget_denied` 與 `agentflow_trigger_fires`。

### 拓撲

`runtime.topology` 決定部署的形態，而形態決定哪些設定有效。之所以只用一個設定而不是十幾個，是因為另一種做法會產生設定不完整卻看似健康的部署：一個使用本機 blob 目錄的區域叢集，會讓每個執行個體各自擁有一份資料，卻不出現任何錯誤。啟動時會拒絕這類組合，`agentflow validate` 則會在啟動前就拒絕。

#### `single`

預設值，也涵蓋此設定出現之前寫成的所有組態。單一行程；以前有效的設定仍然有效：

| 後端 | `single` |
| --- | --- |
| 執行環境、身分、憑證、租約、收件匣 | SQLite 或 PostgreSQL |
| 記憶：`sqlite`、`postgres`、`pgvector`、`qdrant`、`redis`、`redisvector`、`volatile` | 全部允許 |
| Blob（`media`、`files`） | 本機目錄或 S3 |
| `runtime.log_plane.persistence` | 允許 |

同一儲存區上若有第二個執行中的行程，啟動時會被拒絕，並列出對方的 ID 與最後出現時間。`-force-single` 會照樣啟動；只在當機後使用，或等待 30 秒的存活視窗過去。

單一執行個體不會安裝工作階段中樞：沒有第二個擁有者可以移交對話，因此不會寫入 `session:` 租約資料列，也不會寫入共用收件匣資料列。`-force-single` 會保留中樞：使用它的操作人員等於表示可能有兩個行程共用儲存區，此時租約就是防止同一段對話被執行兩次的唯一機制。

#### `regional`

同一區域內跨可用區的多個執行個體，共用一個 PostgreSQL。啟動時會拒絕：

| 設定 | 拒絕原因 |
| --- | --- |
| `runtime.persistence` 不是 PostgreSQL | 每個執行個體都會保有自己的資料庫 |
| `runtime.identity.persistence` / `runtime.credentials.path` 設為 SQLite | 每個執行個體都會保有自己的身分或憑證。取消設定以跟隨執行環境儲存區，或將它們指向 PostgreSQL |
| `runtime.cluster.persistence` 指向不同目標 | 租約與收件匣一律存放在執行環境儲存區 |
| 任何 provider 為 `sqlite` 的 `memory.backends` 項目 | 每個執行個體都會保有自己的對話歷史 |
| `media.backend` 或 `files.backend` 不是 `s3` | 每個執行個體都會保有自己的 blob |
| 設定了 `runtime.log_plane.persistence` | 本機目錄只屬於一個行程 |
| 未設定 `ADMIN_TOKEN` 且 `runtime.admin.operators` 為空 | 每次啟動產生的權杖其他對等執行個體都不知道，因此不會產生（因為這是環境設定，所以在啟動時檢查） |

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

Redis 是一個橫跨整個區域的叢集（locality 為 `region`）；`volatile` 則維持在執行個體本機。每個執行個體每 10 秒在 `instances` 資料表寫入一筆心跳資料列，只要最後一次心跳在 30 秒之內就視為存活；`/admin/api/state` 會回報 `topology`、`zone` 以及每個存活的 `instances` 項目，並標示 `config_epoch` 不一致的對等執行個體。Studio 的總覽也會顯示相同的清單。

多區域部署就是每個區域各一個獨立的 `regional` 單元，沒有全球層級。見[多區域](multi-region.md)。

`agentflow doctor -online`（區域組態的預設）會檢查執行中的一端：它會開啟執行環境儲存區、確認每個 pgvector 後端上都有 `vector` 擴充功能、在每個 S3 儲存貯體中放入／讀取／刪除一個探測物件，並 ping Redis 與 Qdrant。每一行都是 PASS/FAIL 並附上修正方式。

### 命令列

`agentflow help` 會列出所有命令，`agentflow help <command>`（或 `<command> -h`）則會印出某個命令的旗標。這份說明是從二進位檔本身的旗標定義產生的，因此它是永遠保持最新的參考。

`agentflow run [flags]`，或不帶命令的 `agentflow [flags]`，會執行引擎：

| 旗標 | 預設值 | 意義 |
|---|---|---|
| `-config PATH` | `agentflow.yaml`，其次 `config.yaml` | 一個檔案、一個組態目錄，或一個含有 `agentflow.yaml` 的目錄 |
| `-log-level` | `info` | `dev`、`debug`、`info`、`warn` 或 `error` |
| `-log-format` | `text` | `text` 或 `json` |
| `-log-file PATH` | | 同時將日誌附加到此檔案 |
| `-force-single` | 關閉 | 即使儲存區上似乎有另一個執行個體存活也照樣啟動（僅限 `single`；用於當機後） |

所有命令都遵循相同的慣例：

- 旗標可以放在位置參數之後（`agentflow cred set NAME -user u_1`），
  而 `-config` 可以放在命令之前（`agentflow -config deploy/ validate`）。
- 命令作用的對象是位置參數：`init [DIR]`、
  `user export USER`、`user import BUNDLE.zip`、`user keygen [PREFIX]`、
  `migrate blobs DIR`、`cred set NAME`。
- `-out FILE` 會寫入檔案；沒有它時輸出到 stdout。唯一的
  例外是 `support-bundle`，它預設會寫出一個帶時間戳記的 zip。
- `-json` 會在 `doctor`、`config show`、`cred get`
  與 `cred list` 上印出機器可讀的輸出。
- 不適用於某命令的旗標會被視為錯誤，絕不會被默默忽略。
- `user export` 與 `user import` 依序從 `-url`、
  `$AGENTFLOW_URL`、組態的 `gateway.listen`、
  `http://127.0.0.1:8080` 尋找管理 API；權杖則是 `-token` 或 `$ADMIN_TOKEN`。

工作者池的大小是 `runtime.scheduler.workers`（沒有對應的旗標）。
已移除的旗標（`-configdir`、`-workers`、`-log-json`、`-no-tui`、`-no-console`、`-no-webui`、`init -dir`、
`user export -id`、`user keygen -out`、`migrate blobs -from`、`cred -format`）
會被拒絕，並附上指出替代方式的訊息。

### 憑證 CLI

`agentflow cred set|get|list|delete <name>` 管理由已載入組態（`-config`）指定之儲存區中的引擎全域憑證。`set` 從互動式提示或 stdin 讀取值，絕不從 argv 讀取（請用管線：`printf %s "$TOKEN" | agentflow cred set github_token`）。主金鑰來自 `credentials.master_key_env` 指定的環境變數（預設為 `CREDENTIALS_MASTER_KEY`）；未設定金鑰會得到明確的錯誤。`list` 只印出名稱，絕不印出值。`credential.get()` 與 `cred:<service>` 組態參照解析的就是這些記錄。

### 遷移部署

從單一執行個體變更為區域單元屬於資料搬移，
`agentflow migrate` 不需轉換即可完成：兩種後端使用相同的
SQL 方言，而 blob 以內容定址。

```
agentflow migrate store -from sqlite://./data/agentflow.db -to postgres://...
agentflow migrate store -from <src> -to <dst> -tables files_meta -append
agentflow migrate blobs ./data/media -kind media -config agentflow.yaml
```

`migrate store` 會複製執行環境的各個平面、身分登錄表與
憑證儲存區。除非傳入 `-append`，否則它會拒絕非空的目的地，
並回報哪些資料表已複製、哪些不存在。憑證以密文
複製：目的地必須使用相同的主金鑰，否則每個
密鑰都會解密失敗。`migrate blobs` 會走訪本機的 media 或 files 目錄，
將每個 blob PUT 到 `-kind` 指定的 S3 儲存貯體；內容定址
意味著重新執行時會對已存在的物件發出 HEAD，只上傳缺少的部分。
這兩個命令都可以安全地中斷後重新執行。

### 使用者 API

引擎**不附帶任何使用者介面**。它改為在 HTTP 頻道所用的同一個公開監聽器上提供純 JSON 的使用者資料 API（`runtime.users.enabled`，需要 `runtime.identity.enabled`），讓前端可以放在自己的儲存庫中，按自己的節奏部署。合約以 OpenAPI 格式發布於 `openapi/users.yaml`。

- **註冊**：`POST /v1/users/register` 會建立使用者資料，並*僅此一次*回傳其 API 權杖。`runtime.users.registration: invite` 會改為僅限邀請，邀請碼由操作人員透過管理 API（`POST /admin/api/users/invites`）發出。
- **頻道連結**：`POST /v1/users/me/links` 會回傳一次性代碼；使用者*從要連結的帳號*傳送 `/link <code>`，引擎在該訊息抵達時完成連結。只有使用者資料的擁有者看得到代碼，也只有帳號的擁有者能從該帳號傳送訊息，因此任何一半單獨都無法連結任何東西。帶有代碼的訊息永遠不會到達迴圈。只有平台會驗證傳送者身分的頻道才能連結（目前是 telegram）；webhook 自行宣告的傳送者以及 ghhook 固定的傳送者都無法連結。
- **自助服務**：`/me`（使用者資料與帳號）、`/me/usage`（今日、七天視窗以及配額狀態）、`/me/projects`（呼叫者的檔案專案）、用於解除連結的 `/me/links/{id}`，以及用於作廢外洩權杖的 `/me/token/rotate`。
- **認證一律使用 Bearer 權杖**：可以是部署之身分識別提供者核發的存取權杖，也可以是來自 `POST /register` 的 `afu_…` API 權杖。沒有 Cookie，也沒有伺服器端工作階段，因此沒有可讓其他網站重送的隱含憑據，也不需要攜帶 CSRF 權杖。
- **跨來源**：CORS 在**操作人員於 `runtime.users.cors_origins` 列出來源之前一律關閉**；只有列出的來源才會得到回應，而且刻意從不傳送 `Access-Control-Allow-Credentials`：請求攜帶的是 Bearer 權杖而非 Cookie，因此瀏覽器沒有可借給其他來源的隱含權限。`GET /v1/users/config` 是公開的，會告訴前端是否開放註冊、可以提供哪些頻道，以及此單元服務哪個區域（`runtime.region`）。

**身分識別提供者。** 設定 `runtime.users.oidc.issuer`（加上 `audience`，若無法使用探索機制則再加上 `jwks_url`），引擎就會驗證該處核發的權杖：先以簽發者的 JWKS 驗證簽章，再檢查 `iss`/`aud`/`exp`，然後以主體宣告作為身分，並以 `<issuer>|<sub>` 為鍵，使兩個簽發者不會衝突。登入、密碼、MFA 與工作階段有效期都由該提供者負責；引擎只負責驗證。首次通過驗證的登入會依權杖的 `email`/`name` 宣告開通使用者資料，除非設定了 `jit_provisioning: false`，此時未知的主體會被拒絕，直到操作人員為其開通。只接受非對稱演算法，且啟動時必須能連上簽發者。

**部署注意事項。** `POST /register` 會依用戶端位址限制速率，因此位於反向代理伺服器之後的前端也必須在那裡限制速率，否則所有呼叫者都會共用同一個位址。

### 管理 API

執行環境本身不內嵌瀏覽器介面或終端機儀表板——它在 `/admin/api/` 下提供無頭的 JSON 與 SSE 控制平面，供 **AgentFlow Studio**（以及自動化指令碼）連線。

**單一連接埠。** 管理平面與頻道共用 `gateway.listen`：`/admin/api/*`、`/admin/sessions`、`/healthz`、`/readyz` 與 `/metrics`。位於 `/admin/`、`/healthz`、`/readyz`、`/metrics`、`/health` 與 `/v1/users/` 之下的頻道路徑都會被拒絕。

**認證：** 每個管理 API 路由（以及 `/metrics` 與 `/admin/sessions`）都需要 Bearer 權杖，即使來自回送位址也一樣。設定 `ADMIN_TOKEN`（或指定 `runtime.admin.operators`）即可固定它；否則每次啟動會產生一個隨機權杖，並在日誌中印出一次。對於任何能讀取該日誌的人來說，每次啟動產生的權杖都不算秘密，因此在使用這種權杖時，管理平面**只回應本機用戶端**：來自其他機器的請求，或帶有 `Forwarded`、`X-Forwarded-For` 或 `X-Real-IP` 的請求（因此同一主機上的反向代理伺服器不算本機），都會得到 `403`；`/healthz` 與 `/readyz` 仍對負載平衡器開放。設定好的憑證會讓管理平面對遠端用戶端開放，僅以權杖保護，因此請在前面加上 TLS。區域單元必須設定一個：每次啟動產生的權杖其他執行個體並不知道。密鑰從頭到尾都僅可寫入：清單端點只回傳中繼資料與指紋，絕不回傳值，組態檢視也會遮罩敏感鍵，同時保留 `${VAR}` 預留位置。

**本機傳輸方式：** `runtime.admin.socket` 會新增第二個僅限本機的管理監聽器（在 Unix 上是 Unix 網域通訊端，在 Windows 上是具名管道），而 `runtime.admin.tcp: false` 會把管理平面從 HTTP 監聽器上移除。通訊端僅限擁有者存取（0600；管道的 DACL 授予擁有者、Administrators 與 SYSTEM），仍然需要 Bearer 權杖，而 Studio 桌面應用程式接受 `unix:///path` 或 `npipe://name` 網址。

**路由。** AgentFlow Studio 所做的一切都由 `/admin/api/` 之下的 JSON 或 SSE 路由支撐，因此操作人員也可以直接以指令碼呼叫。它們全都需要同一個 Bearer 權杖。

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/state` | 執行個體識別、執行時間、組態紀元、執行中的工作階段、代理與頻道。 |
| `GET /admin/api/config` | 原始組態文字、遮罩後的結構化檢視，以及檔案的 mtime。 |
| `POST /admin/api/config/validate` | 對送出的文字進行與啟動等效的驗證（嚴格欄位、環境變數展開、交叉參照）。 |
| `POST /admin/api/config/save` | 驗證、備份為 `.bak`，再以原子方式取代。若檔案在你操作期間被修改，則以 409 拒絕。 |
| `GET /admin/api/models` | 執行中 ∪ 檔案中的模型集合；金鑰遮罩為末四碼指紋。 |
| `PUT/DELETE /admin/api/models/{name}` | 將模型熱套用到*此行程*（見上文的叢集限制）。 |
| `POST /admin/api/models/{name}/test` | 透過執行中的管理器發出一次 1 Token 的呼叫，並將結果分類（auth / not\_found / unreachable / …）。 |
| `POST /admin/api/models/persist` · `/revert` | 將執行中的集合寫入檔案，或將檔案中的集合重新載入執行中的管理器。 |
| `GET /admin/api/metrics` | 供走勢圖使用的取樣計數器序列（約 2 小時，每 5 秒一次）。 |
| `GET /admin/api/logs` | SSE 日誌尾端：先重播環狀緩衝區，再即時輸出。 |
| `GET /admin/api/users` · `/{id}` | 附有今日用量與配額狀態的使用者資料；單一使用者資料及其最近呼叫、稽核記錄尾端與專案。 |
| `POST /admin/api/users/{id}/limit` | 設定使用者資料的每日 Token 上限（0 表示沿用部署預設值）。 |
| `PUT /admin/api/users/{id}/settings` | 使用者資料的模型與指令層。這是設定它們的唯一方式，因為一個人使用哪個模型是成本決策，而非自助服務的選項。 |
| `POST /admin/api/users/{id}/unlink` | 解除一個頻道帳號的連結（最後一個會被拒絕）。 |
| `POST /admin/api/users/invites` | 產生一次性註冊邀請。 |
| `GET /admin/api/agents` · `PUT/DELETE /admin/api/agents/{name}` | 在執行時依定義安裝、取代或移除代理（不寫入任何檔案；Studio 的代理路由才是會永久儲存的版本）。 |
| `GET/POST /admin/api/groups` · `GET/DELETE /{id}` | 租戶群組：列出、建立、檢視成員、解散。 |
| `POST /admin/api/groups/{id}/members` · `DELETE /{id}/members/{user}` · `PUT /{id}/master` | 新增或移除成員，或將群組移交給新的群主。 |
| `GET/POST /admin/api/pools` · `GET/PUT/DELETE /{id}/grants` | 具名的共用記憶池，以及決定誰可以使用它們的授權（租戶 → 讀取／寫入）。儲存區以 `pool: <name>` 繫結到記憶池。 |
| `GET /admin/api/users/{id}/export` · `DELETE /admin/api/users/{id}` | 匯出為某份使用者資料保存的所有內容（隱私匯出），或將其清除。 |
| `GET /admin/api/users/{id}/bundle?public_key=…` | 完整保真的遷移套件：使用者資料、記憶資料列、檔案位元組、群組與憑證，以呼叫者的 X25519 金鑰密封。`POST /admin/api/users/import` 在目的地單元以私密金鑰匯入它。見[多區域](multi-region.md)。 |
| `GET /admin/api/usage?day=YYYY-MM-DD` | 當日每個租戶的 Token 彙總，附總計。 |
| `GET /admin/api/credentials/users` | 存有憑證的使用者分區。 |
| `GET/POST /admin/credentials` · `DELETE /admin/credentials/{service}?user=` | 憑證儲存區：列出中繼資料、開通每租戶金鑰、撤銷金鑰。僅可寫入，絕不回傳值。 |

瀏覽器主控台涵蓋上述四個分頁。此表中的其他所有功能（使用者、邀請、配額、租戶、用量、日誌以及原始組態）在 AgentFlow Studio 中都有對應畫面，或可以用 `curl` 以指令碼操作。

### Studio：代理建構器、遊樂場與追蹤

**AgentFlow Studio** 是用於製作代理原型的桌面與網頁應用程式：撰寫迴圈及其指令、驗證、即時套用定義、與它對話、觀察輪次的每一步，並把結果永久儲存到部署組態中。它位於自己的儲存庫 `agentflow-studio`（並非內嵌在二進位檔中），並以與主控台相同的 Bearer 權杖和管理 API 通訊。

- **代理**：列出每個已設定的代理，以及其模型、能力、執行中的工作階段數，以及它只存在於執行環境中還是已永久儲存。可從範本建立（echo、LLM 對話、工具迴圈、單次執行、附記憶的群組聊天、知識庫、單輪 API），也可以編輯或刪除。
- **建構器**：常用欄位的表單（模型、安全、記憶設定檔、能力、技能、歷史預算、常駐旗標、`can_contact`、憑證、額外欄位）、TypeScript/JS 編輯器、指令編輯器，以及處理其他所有內容的原始 YAML。*驗證*會檢查 prelude 指示詞、編譯程式碼區塊，並執行與啟動相同的代理驗證；*套用*會將它安裝到執行環境中；*套用並儲存*還會把它寫入組態（一個會記住選擇的分割按鈕）。編輯器旁的停駐區可放置**執行**面板（與正在編輯的代理對話並查看追蹤），或指令碼參考（F1）；Ctrl+Enter 會即時套用待處理的編輯並傳送面板中的訊息，或重複上一則，讓下一輪執行的就是畫面上的內容。
- **遊樂場**：針對單一代理的分對話聊天。每個對話都是獨立的工作階段（`<agent>|studio:<thread>`），因此對話不會混在一起。回覆經由合成的 `studio` 頻道送達，這表示它們走的是一般的傳出路徑：安全過濾器、稽核記錄與傳遞都不變。選填的*以使用者身分* UUID 會蓋上租戶，使個人記憶範圍、配額與記憶池授權得以套用；沒有它時，輪次不帶租戶。
- **追蹤**：輪次的即時時間軸：`turn.start`、每個 `op.start`/`op.end`（模型、工具、耗時、Token、工具呼叫）、安全攔截、回覆、重新載入與迴圈錯誤。文字酬載預設關閉（`studio.trace_payloads`）；中繼資料一律會擷取。
- **頻道**：閘道已永久儲存的頻道集合：新增、編輯、驗證與刪除 webhook、Telegram、GitHub webhook 與 bridge 項目；密鑰保持僅可寫入。儲存的變更在下次啟動時生效。
- **模型**：執行中的模型登錄表：新增、編輯、以真實呼叫測試、刪除，然後將執行中的集合永久儲存到組態，或還原為檔案中的內容。
- **記憶**：見[記憶](memory.md)指南。編輯 `memory.backends` 與 `profiles.memory`：後端依供應商區分，只顯示該供應商會讀取的鍵；設定檔則是儲存區清單（後端、資料表、保留期限、視窗、所需功能、範圍或共用池），加上召回、寫入、嵌入與重新排序設定。後端網址與 API 金鑰僅可寫入：畫面上顯示的是 `${VAR}` 或 `cred:` 參照，或遮罩了密碼的網址。驗證與啟動等效，仍在使用中的設定檔或後端無法刪除。未宣告任何後端的部署會顯示位於執行環境儲存區上的內建 `main_db`；儲存第一個後端時會將它寫入檔案，讓使用內建 `conversational` 設定檔的代理保有其儲存區。代理在啟動時開啟儲存區，因此儲存的變更在下次啟動時生效，而新的設定檔要在重新啟動之後才能在建構器中選用。
- **安全**：編輯 `profiles.safety` 過濾鏈（有序的內建過濾器與 `classifier:<name>` 參照）以及 `safety.classifiers` 原則。驗證與啟動等效；仍被參照的設定檔與分類器無法刪除。分派器是依代理建立的，因此儲存的變更在下次啟動時生效。
- **憑證**：依分區（引擎全域或依使用者）瀏覽加密儲存區，可建立、編輯、撤銷，並設定注入的標頭／認證方案。值僅可寫入，且只以末四碼指紋顯示；由於儲存區是在請求時讀取的，編輯會即時生效。
- **總覽**：登入後的首頁：執行個體、區域、版本、執行時間與組態紀元、執行中的工作階段、代理、頻道，以及取樣計數器的走勢圖（`/admin/api/state`、`/admin/api/metrics`）。
- **使用者**：附有今日配額的使用者資料、已連結的帳號（可解除連結）、專案、最近呼叫與稽核記錄尾端；設定每日上限以及每位使用者的模型與指令層；發出邀請；匯出或清除使用者資料（`/admin/api/users/*`）。
- **租戶**：群組（成員、移交群主、解散）與記憶池（建立、依代理與租戶授權）（`/admin/api/groups`、`/admin/api/pools`）。
- **用量**：依租戶、代理或模型分組的每日 Token 彙總（`/admin/api/usage`）。
- **日誌**：可依等級與文字篩選的即時日誌尾端（`/admin/api/logs`）。
- **組態**：原始組態檔，附有與啟動等效的驗證，以及以 mtime 保護、事先備份的儲存（`/admin/api/config*`）；組態目錄式部署則改由結構化畫面與檔案本身編輯。

`Ctrl+K` 會開啟命令面板，涵蓋所有畫面、代理與動作。

**語言。** Studio 的介面提供英文、簡體中文（简体中文）與繁體中文（繁體中文）。**EN / 简体 / 繁體** 切換鈕位於每個畫面的命令列、登入畫面，以及*設定 → 語言*中。變更會立即生效，不需重新啟動或重新載入，並會被記住（存在瀏覽器的本機儲存空間中，桌面應用程式則存在其 webview 的本機儲存空間中）；首次啟動時會跟隨作業系統或瀏覽器的語言，否則退回英文。這些指南也跟隨同一個切換鈕，尚未翻譯的指南則顯示英文版。組態鍵、識別碼、模型與設定檔名稱、YAML、指令碼，以及執行環境回傳的任何內容（包括錯誤訊息），都會完全照執行環境寫出的樣子顯示。

**永久儲存以組態為本。** 使用目錄式組態時，Studio 會寫入 `profiles/<name>.yaml`（標記為 `# managed by agentflow studio`）、`loops/<name>.ts` 與 `prompts/<name>.md`。使用單一組態檔時，它會取代 `agents.<name>` 鍵（保留註解與其他區段），並將迴圈／提示詞寫在 `studio.workspace` 之下；定義中存放的是絕對路徑，因此永久儲存的檔案不受下一個行程的啟動方式影響。無論哪種方式，變更都會立即即時套用，並在下次啟動時從組態載入。不是由 Studio 寫入的設定檔會被拒絕，除非強制執行。

**執行方式。** 用戶端是位於自己儲存庫中的獨立 Go 模組：

```
cd ../agentflow-studio
make build        # or: CGO_ENABLED=1 CC="zig cc" CXX="zig c++" go build -o agentflow-studio.exe .
./agentflow-studio
```

輸入執行個體網址（`http://127.0.0.1:8080`，即 `gateway.listen` 位址）與管理權杖。作為原生用戶端，它會直接傳送權杖，因此不需要 `runtime.admin.cors_origins` 項目。若要從其他機器連線，執行環境需要已設定的 `ADMIN_TOKEN`（每次啟動產生的權杖只回應本機用戶端）；並請在前面加上 TLS。該允許清單是保留給管理 API 的*瀏覽器*用戶端使用的，而且只接受確切的來源（`http://localhost:5173` 與 `http://127.0.0.1:5173` 是不同的來源）；`"*"` 會被拒絕。

| Studio 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/catalog` | 模型、工具、能力、安全／記憶設定檔、提示詞鍵、prelude 版本與範本。 |
| `GET /admin/api/studio/agents` · `/{name}` | 代理清單；單一代理及其 YAML、迴圈、指令與永久儲存檔案的中繼資料。 |
| `POST /admin/api/studio/agents/validate` | 對送出的定義與迴圈進行無副作用的驗證。 |
| `PUT /admin/api/studio/agents/{name}` | 驗證、即時安裝（`apply`），並可選擇永久儲存（`persist`）；回傳套用了什麼以及任何永久儲存錯誤。 |
| `DELETE /admin/api/studio/agents/{name}` | 停止其工作階段、移除它，並只移除由 Studio 管理的檔案。`force` 會無視執行中的工作階段與手寫的設定檔。 |
| `POST /admin/api/studio/chat` | 一個遊樂場輪次。以 SSE 串流（`trace`、`reply`，然後是 `done` 或 `error`），或在 `stream: false` 時回傳 JSON。輪次以 `done` 結束（已完成、逾時或被安全機制拒絕）；無法傳遞的訊息或在輪次中途失敗的迴圈則改以 `error` 結束，此時 `stream: false` 會回應 `ok: false` 並附上追蹤。永遠無法開始輪次的請求（未知的代理、不是使用者資料的 `act_as_user`）會在任何串流開始之前以 4xx 拒絕。 |
| `GET /admin/api/studio/threads` · `POST /threads/{thread}/stop` | 觀察到的對話，以及對某個執行中工作階段的停止請求。 |
| `GET /admin/api/studio/trace/{agent}/{thread}` | 緩衝的追蹤；加上 `?stream=true` 可取得即時 SSE。 |
| `GET/POST /admin/api/studio/channels` · `PUT/DELETE /{index}` | 已永久儲存的頻道集合；驗證草稿、建立、取代或移除一個頻道。儲存後需重新啟動。 |
| `GET /admin/api/studio/safety` | 安全設定檔與分類器、其使用情況，以及編輯器可參照的內建過濾器與模型集合。 |
| `POST /admin/api/studio/safety/validate` | 對設定檔或分類器草稿進行與啟動等效的試執行。 |
| `POST/PUT/DELETE /admin/api/studio/safety/profiles` · `/classifiers` | 建立、取代或移除設定檔或分類器。使用中的項目會被拒絕；寫入時會驗證、保留 `.bak`，並需重新啟動。 |
| `GET /admin/api/studio/memory` | 記憶後端（密鑰以參照或遮罩後的網址呈現）與設定檔、其使用情況，以及編輯器可提供的供應商、模型與外掛。 |
| `POST /admin/api/studio/memory/validate` | 對設定檔或後端草稿進行與啟動等效的試執行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 建立、取代或移除設定檔或後端。名稱不可變更；空的網址或 API 金鑰會保留已儲存的值；使用中的項目會被拒絕；寫入時會驗證、保留 `.bak`，並需重新啟動。 |
| `GET/POST/DELETE /admin/api/credentials` | 列出單一分區的中繼資料（`?user=`；留空為引擎全域）、建立或更新密鑰（空的密鑰會保留已儲存的值）、撤銷密鑰（`?service=&user=`）。絕不回傳值。 |

**限制。** 遊樂場僅限執行個體本機：合成頻道只註冊在一個行程中，因此在叢集中，若某個對話的工作階段由另一個執行個體擁有，這裡就擷取不到回覆。Studio 不儲存聊天記錄（代理的記憶才是唯一可信來源），而追蹤環位於記憶體中，重新啟動後就會重設。衍生設定檔的撰寫、附件、重播與評估不在第一版的範圍內。

### 日誌

日誌使用五個**累加**等級：選擇其中一個，就會印出該等級及其以上的所有等級。以 `-log-level` 選擇（預設 `info`）：

| 等級 | 記錄內容 | 範例 |
| --- | --- | --- |
| `ERR` | 執行環境結束或無法繼續 | 組態載入失敗、記憶後端開啟失敗、連接埠繫結失敗 |
| `WARN` | 功能降級但仍繼續 | 上游 429/502 並重試、頻道傳遞錯誤、迴圈當機（重新啟動中）、重新載入時編譯失敗（保留舊版） |
| `INFO` | 生命週期 | 執行環境啟動、頻道已註冊、已設定 telegram webhook、工作階段衍生／結束、熱重新載入已接受 |
| `DEBUG` | 每一次互動 | LLM API 請求、記憶 put/get/query（redis 往返、pgvector 查詢）、收到 telegram 更新、webhook/ghhook 傳入、操作分派、imap 擷取／smtp 傳送 |
| `DEV` | 暫時性的開發日誌 | 偵錯時加入的臨時檢測程式碼：`llog.Dev(log, ...)`；絕不刻意提交 |

同樣的記錄也會送往 AgentFlow Studio 的即時日誌檢視（`/admin/api/logs`，簡短標籤 `ERR/WRN/INF/DBG/DEV`）。

### `models`：具名的 LLM 供應商

```
models:
  default:
    provider: anthropic        # anthropic | openai | openai-responses | gemini | rerank
    model: claude-sonnet-5
    api_key: ${ANTHROPIC_API_KEY}
    base_url: ~                # 覆寫端點主機
    timeout: 30s
    retry: 2
    max_tokens: 4096
    thinking: medium         # off | low | medium | high | xhigh | max；留空 = 供應商預設
    server_tools: []           # 供應商原生的伺服器端工具
```

`provider` 選擇請求／回應的格式，`base_url` 選擇主機。對於相容 OpenAI 的端點（Ollama、vLLM、LiteLLM），請在 `base_url` 中包含 `/v1`；`openai` 會附加 `/chat/completions`，`openai-responses` 會附加 `/responses`。對於相容 Anthropic 的端點，請將 `base_url` 設為主機根目錄；執行環境會附加 `/v1/messages`。對於 `gemini`，請將 `base_url` 指向版本前綴（預設 `https://generativelanguage.googleapis.com/v1beta`）；執行環境會附加 `/interactions`，並在 `x-goog-api-key` 標頭中傳送金鑰。Gemini Interactions API 不支援串流，也不接受 Token 上限，因此 `llm.stream` 只會產生一個緩衝後的差量，且不會傳送 `max_tokens`。`provider: rerank` 是使用 `POST {base_url}/rerank` 的交叉編碼器端點（TEI、vLLM、Jina、Cohere），透過 `llm.rerank`／記憶的 `rerank_model` 使用，而不是當作聊天模型。

`server_tools` 列出要在每個請求上啟用的供應商原生伺服器端工具（例如 gemini 上的 `google_search`，openai-responses 上的 `web_search`/`x_search`）。與每次呼叫透過 `opts.tools` 傳入的用戶端函式工具不同，這些工具在供應商內部執行；執行環境只會注入原生的 `{type: <name>}` 項目，並以你寫的名稱轉送。值是各供應商特定的字串，讓執行環境保持通用。在 OpenAI 的格式中，這種純項目在 SDK 的工具聯集中沒有對應的變體（其工具型別固定為 `"function"`），因此是以 `param.Override` 建構的；在 `gemini` 上，`google_search` 對應到原生工具，其他名稱則以原始項目直接傳遞。

### `memory.backends`：後端執行個體

以下每個後端都是真正的供應商實作，沒有空殼。儲存型後端提供 KV／前綴／文字／TTL；向量型後端提供語意召回。SQLite 刻意以明確的錯誤拒絕向量查詢，而不是默默回傳空結果。

| 供應商 | 類型 | 功能 |
| --- | --- | --- |
| `sqlite` | 儲存 | kv、prefix\_scan、text\_search、ttl、transaction、vector（對 BLOB 欄進行暴力餘弦比對；不需擴充功能，受 `vector_scan_limit` 限制，預設 50000 列） |
| `redis` | 儲存 | kv、prefix\_scan、ttl |
| `postgres` | 儲存 | kv、prefix\_scan、text\_search、transaction；若資料庫有 pgvector 擴充功能，還支援 vector（於啟動時偵測；同一資料表） |
| `volatile` | 儲存 | kv、prefix\_scan、ttl：位於記憶體中、不永久保存；受 `max_entries` 限制 |
| `pgvector` | 向量 | vector（餘弦 k-NN；嵌入維度可透過 `dim` 設定，預設 1536）。保留作為別名／相容用的供應商；現在只要有擴充功能，`postgres` 就能提供向量 |
| `qdrant` | 向量 | vector（透過 Qdrant 的 gRPC API 進行餘弦 k-NN：`url` 的連接埠被視為 REST 連接埠，因此 `:6333` 會變成 gRPC 的 `:6334`；`dim` 預設 1536，`collection` 預設 `agentflow`，`api_key` 選填） |
| `redisvector` | 向量 | kv、prefix\_scan、ttl、vector（需要 RediSearch 模組，例如 Redis Stack；餘弦 k-NN，`dim` 預設 1536） |

每個後端都有一個 **locality**，由其供應商與 `runtime.topology` 推導而來，
並以 `agent.info().memory.stores[*].locality` 提供給迴圈：

| 供應商 | `single` | `regional` |
| --- | --- | --- |
| `postgres`、`pgvector`、`qdrant` | `deployment` | `deployment` |
| `sqlite` | `deployment`（唯一的執行個體就是整個部署） | 拒絕 |
| `redis`、`redisvector` | `deployment` | `region`（每個單元一個叢集） |
| `volatile` | `instance` | `instance` |

在區域單元中，持久的對話儲存區（`dialogue`、`facts`，或在
`recall: plugin:semantic` 下的 `semantic`）必須位於 `deployment` 或
`region` 後端上。驗證會拒絕位於 `volatile` 上的持久儲存區（移到另一個執行個體的工作階段
會遺失其歷史），除非該儲存區設定了
`locality_ok: true`。在每個層級中，volatile 記憶也都有上限：要麼
後端設定 `config.max_entries`（淘汰最舊的項目），要麼每個
繫結到它的儲存區都設定 `retention` TTL。

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
    vec:                                # 任何一個向量儲存區都能服務 semantic 設定檔
      provider: pgvector
      config: { url: "postgres://user:pw@host/db", dim: 1536 }
    # provider: qdrant          # gRPC；url 保持 REST 格式，:6333 會改連 gRPC 的 :6334
    # config: { url: "http://localhost:6333", dim: 1536, collection: agentflow, api_key: "${QDRANT_KEY}" }
    # provider: redisvector      # Redis Stack（RediSearch），不是一般的 Redis
    # config: { url: "redis://localhost:6379", dim: 1536 }
```

這三個向量儲存區與 `pgvector` 一樣只接受向量查詢：其他任何查詢類型都會得到明確的錯誤，絕不會是默默為空的迭代器。`dim` 在儲存區首次建立時就固定，因此必須與嵌入模型相符；之後要變更需要手動遷移。`redisvector` 會把值存放在向量旁邊，因此也提供 kv、prefix\_scan 與 ttl；`redis` 維持不變，仍是單純 KV 的選項。

`qdrant` 使用 gRPC，但其 `url` 保持每個部署原本就有的格式，也就是 REST 格式。協定決定傳輸方式（`https` 會以 TLS 連線），連接埠則會轉換：明確的 `6333`（Qdrant 的 REST 預設值）會變成 gRPC 的 `6334`；其他明確的連接埠則照原樣連線，因此 gRPC 代理伺服器仍可運作；沒有連接埠的網址則使用 `6334`。透過僅支援 REST 的代理伺服器連到 Qdrant 的部署，必須改為指向 gRPC 監聽器。資料點本身仍以與先前相同的確定性 ID 定址，因此既有的 `agentflow` 集合可以照常讀寫。

若完全沒有 `memory.backends` 區塊，引擎會在執行環境儲存區上註冊 `main_db`：檔案使用帶 `path` 的 `sqlite` 供應商，DSN 則使用帶 `url` 的 `postgres` 供應商。這就是內建的 `memory: conversational` 設定檔能在最精簡組態中運作的原因，也意味著單一執行個體的部署不需要第二個資料庫就能擁有持久記憶。因此，使用 `postgres` 執行環境儲存區時也不需要 `memory:` 區段；只要安裝了 pgvector 擴充功能，其資料庫就能提供向量。

若後端的 `config.url`/`config.password`/`config.api_key` 密鑰參照無法解析，該後端會**被略過並發出警告**，而不會導致啟動失敗：繫結到它的儲存區會重新繫結到仍存在的後端（優先選擇供應商提供 `text_search` 的後端，讓向量儲存區降級為全文搜尋；迴圈可從繫結的功能進行偵測），並將 `requires` 降低為後備後端所提供的功能；若沒有任何後端存在，則捨棄這些儲存區。真正未知的後端名稱仍然是啟動錯誤。

### `profiles`：可重複使用的組態

設定檔依類型分組於 `profiles:` 之下（`memory`、`safety`、`shell` 與 `agent`），代理以名稱參照它們（`memory: chat`、`safety: strict`、`shell: python`）。

#### 記憶設定檔

[記憶](memory.md)是完整的指南：後端、設定檔與儲存區如何搭配、迴圈如何寫入與召回、隔離、實用範例，以及驗證訊息。本節則是參考資料。

```yaml
profiles:
  memory:
    chat:                                 # "conversational" 保留給內建設定檔
      stores:
        dialogue: { backend: main_db, table: dialogue, window: 1000, retention: 30d }
        facts: { backend: main_db, table: facts, requires: [kv, text_search] }
      write: [plugin:routing_table]
      recall: plugin:recency

    semantic:                             # 嵌入 + 重新排序召回
      stores:
        semantic: { backend: vec, table: semantic, requires: [vector] }
      write: [plugin:routing_table]
      recall: plugin:semantic
      embed_model: embed                  # models: 中的項目；寫入向量儲存區時自動嵌入
      rerank_model: rerank                # 選填的 provider:"rerank" 項目
      oversample: 4                       # 擷取 k*4 筆向量結果，再重新排序縮減為 k 筆
```

`write` 以 `agent.info().memory.write` 傳給迴圈；`memory.write` 一律執行路由表（`plugin:routing_table`，或覆蓋它的 `plugins/routing_table.ts`）。

使用 `recall: plugin:semantic` 時，`memory.recall({ text = "...", k = 5 })` 會嵌入查詢、在向量儲存區上執行餘弦 k-NN，並在設定了 `rerank_model` 時重新排序。沒有文字的查詢會退回依時間先後召回。

| 儲存區鍵 | 說明 |
| --- | --- |
| `backend` | 必填。參照一個後端執行個體名稱。 |
| `table` | 此儲存區繫結的實體資料表（或集合）。 |
| `retention` | 以 `ms`、`s`、`m`、`h`、`d` 為單位的時間長度（允許組合與小數，例如 `2d12h`），或字面值 `forever`。**它是寫入時的 TTL，不是定期清掃：** 它會成為任何未明確指定 TTL 之 `store.put` 的預設 TTL，因此記錄會在*寫入*後經過這段時間才過期，而在此設定出現之前寫入的資料列永遠不會過期。無法解析的值是啟動錯誤。 |
| `window` | 每個實體資料表保留的最大記錄數，由定期的 GC 修剪。適用於 KV 後端；**向量後端（pgvector、Qdrant、Redis）不實作任何修剪**，因此在它們上面設定的視窗或保留期限只是一個沒有人強制執行的宣告上限：引擎會在啟動時發出警告，而資料表會無限制地成長。 |
| `requires` | 功能清單，於啟動時對照供應商進行驗證。 |
| `pool` | 將儲存區繫結到具名的共用池（透過租戶管理 API 建立）。授權決定哪些租戶可以讀取或寫入它；繫結記憶池的儲存區不接受 `scope`，且記憶池必須在啟動時已存在。`shared: true` 是啟動錯誤，記憶池已取代了它。 |
| `scope` | `agent` 讓私有儲存區改回每個代理一個池，而不是引擎的每使用者鍵範圍。 |
| `collection` / `policy` | **已淘汰，沒有作用。** 儲存區依後端與資料表繫結，也沒有實作任何每儲存區的原則開關。仍會解析，讓既有組態可以啟動；只要設定了其中任何一個，引擎就會在啟動時發出警告。 |

> **隔離方式變更**：記憶儲存區**預設為每個代理私有**：使用同一設定檔（或 `conversational`）的兩個代理不再共用實體資料表；各自繫結 `<agent>.<table>`（例如 `writer.dialogue`）。迴圈繼續使用設定檔中的資料表名稱；前綴是在繫結時套用的。**刻意共用：** 將儲存區繫結到具名記憶池（`pool: <name>`），並透過租戶管理 API 授予租戶存取權；`shared: true` 現在是啟動錯誤。在改為每代理隔離之前寫入的既有資料列，仍留在舊的無前綴資料表中，需要明確遷移。

#### 安全設定檔

```
profiles:
  safety/default:
    filters: [source-attribution, signal-gate, steady-directive, support-offer, affect-guard]
```

`safety: none` 是明確的退出選項。分派器由核心擁有，無論迴圈處理常式為何都會執行。

#### 以模型為基礎的分類器

`safety.classifiers` 定義以專用模型為基礎的審核或分類原則。設定檔以 `classifier:<name>` 選用：

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
      prompt: safety/moderation          # prompts: 登錄表的鍵；預設為內嵌版本
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
      refusal: ""                        # 選填的傳入拒絕回覆文字
      fallback: ""                       # 選填的傳出替代回覆文字

profiles:
  safety:
    strict:
      filters: [source-attribution, classifier:moderation, affect-guard]
```

模型只回傳標籤與分數；由執行環境套用 `categories` 與 `threshold`，因此模型輸出無法自行選擇動作。`safe: allow`（或其他 `allow` 標籤）為必填。分類器模型必須使用引擎金鑰（`key_source: config`，即預設值）；`key_source: user` 的模型會在啟動時被拒絕，否則沒有金鑰的使用者就可能停用審核。呼叫模型時不帶任何工具，也不帶伺服器端工具。

失敗時依明確的原則處理，而不是默默放行：`on_error`（供應商錯誤、無效輸出、逾時）、`on_saturated`（並行上限已滿）與 `on_budget_exhausted`（專用審核預算用盡）。三者的預設值都是 `block`。設定 `mode: shadow` 可記錄判定結果而不強制執行；模型呼叫仍會發生（延遲也仍然存在），只是動作被抑制。`sources` 使用正規化的詞彙 `channel`、`agent`、`scheduler`、`system`（頻道驅動程式在內部標記為 `user`，計時器標記為 `timer`）。

傳入攔截會略過該輪次：迴圈永遠看不到這則訊息，若有設定則傳送 `refusal`。傳出攔截會抑制回覆，若有設定則傳送 `fallback`。拒絕回覆與替代回覆的速率限制為每個工作階段每 30 秒一則，因此大量被攔截的訊息不會變成大量對外訊息。分類器的花費記錄在合成的 `__moderation_<name>` 帳本分區之下，因此不會消耗代理的 `tokens_per_day` 預算或使用者的 LLM 配額。沒有 `safety:` 的衍生設定檔會繼承衍生它之工作階段的過濾鏈；在 `profiles.agent.*` 上設定 `safety:` 可讓子代理使用不同的設定檔。

#### Shell 設定檔

Shell 設定檔說明代理的 shell 是什麼。以 `agents.<name>.shell` 繫結（代理還需要 `shell.exec` 能力）。

```
profiles:
  shell:
    code:
      provider: sandbox          # sandbox（預設）| ssh
      environment: python        # environments 中的一個項目（必填）
      project: workspace         # 工作執行所在的檔案儲存區專案（預設 workspace）
      workdir: app               # 命令的起始位置，相對於專案
      timeout: 90s               # 工作的時間上限，受 runtime.sandbox.max_timeout 限制
      mem_limit: 512MiB          # 可以調低 runtime.sandbox.memory，但不能調高
      cpu_limit: 1.0
      env: { CI: "1" }
      ignore: ["node_modules/", "__pycache__/"]   # 由工作產生，從不儲存
      network: none              # 唯一的值：沙箱工作沒有網路
    box:
      provider: ssh              # 連線到一台真實的遠端機器
      host: build.example.com:22
      user: dev
      key_file: /etc/agentflow/id_ed25519      # 或 password:（密鑰參照）
      workdir: /srv/work
```

**`provider: sandbox`** 會把每個命令當作一次性的沙箱工作執行，可選 [gVisor](https://gvisor.dev)（`provider: gvisor`，預設，Linux）或 [microsandbox](https://github.com/superradcompany/microsandbox)（`provider: microsandbox`，透過 KVM / macOS Apple Silicon HVF / Windows WHP 提供的硬體隔離微型虛擬機器）：環境提供工具鏈根目錄，專案以可讀寫方式掛載於 `/workspace`，命令寫入專案的內容會在結束時儲存。命令結束後其他東西都不會留存，因此沒有需要追蹤的狀態，任何執行個體都能執行下一個命令。`ignore` 使用 `.gitignore` 的一小部分語法：`node_modules/`（任何位置的同名目錄）、`*.pyc`（任何位置的名稱）、`build/out`（從專案根目錄起算的路徑）；結尾的斜線會將樣式限定為目錄。被忽略的路徑既不儲存也不清理：它們會留在執行個體上，作為盡力而為的快取。

專案可以在其工作樹中放一個 `.agentflow/shell.yaml` 檔案，自訂其代理的設定檔。它可以設定 `environment`（部署所宣告的其中之一）、`env`、`workdir`、`timeout` 與 `ignore`。它沒有映像檔、網路原則或資源限制的欄位，未知的欄位會是錯誤，而不是默默不起作用。

**`provider: ssh`** 會為每個工作階段保持一條連到既有機器的連線；這裡不會佈建任何機器。連線存在於撥出它的執行個體中，並隨工作階段關閉。

`provider: docker` 已移除。仍在使用它的組態會在載入時失敗，並附上指出替代方式的訊息。

#### `environments`：宣告的工具鏈

環境是沙箱工作啟動時使用的根檔案系統與變數。環境由操作人員宣告；shell 設定檔選擇其中一個，專案也可以選擇另一個。

```
environments:
  python:
    image: docker.io/library/python:3.12-slim@sha256:<64 hex digits>
    env: { PYTHONUNBUFFERED: "1" }
  tools:
    rootfs: /srv/agentflow/envs/tools
```

以下兩者必須恰好設定一個：

- **`image`**：**以摘要值固定**的 OCI 映像檔。只有標籤會被拒絕：標籤可能被移到不同的內容上，這樣兩個執行個體就會以同一名稱執行不同的工具鏈。在 `gvisor` 下，映像檔會在首次使用時拉取並解包到 `<runtime.sandbox.data_dir>/images/<digest>`（公開／匿名拉取、gzip 壓縮圖層）。在 `microsandbox` 下，`msb` 會於首次使用時直接原生拉取並快取 OCI 映像檔。兩者都會套用映像檔本身的 `Env`，再以這裡的 `env` 覆蓋。
- **`rootfs`**：主機上已存有根檔案系統的目錄（例如匯出的容器，或 `debootstrap` 樹）。它需要有 `/bin/sh`。

由於工作沒有網路，命令所需的一切都必須已經在環境中：套件是映像檔的一部分，而不是由命令安裝的。

#### `runtime.sandbox`：shell 工作的執行方式

每個設定都有預設值；宣告了環境與 shell 設定檔的部署完全不需要這些設定。

| 鍵 | 預設值 | 意義 |
|---|---|---|
| `provider` | `gvisor` | 沙箱執行環境後端：`gvisor`（`runsc`，Linux）或 `microsandbox`（`msb`，Linux KVM / macOS Apple Silicon / Windows WHP） |
| `runsc` | `PATH` 上的 `runsc` | `provider: gvisor` 時的 gVisor 執行環境二進位檔。請將發行版的 `gvisor-bin/` 目錄放在它旁邊 |
| `microsandbox` | `PATH` 上的 `msb` | `provider: microsandbox` 時的 microsandbox CLI 二進位檔 |
| `data_dir` | `<data dir>/sandbox` | 工作區、解開的映像檔以及執行環境自身狀態。這是快取：其中的一切都能從檔案儲存區與登錄伺服器重建 |
| `concurrency` | 4 | 此執行個體上同時執行的工作數 |
| `per_user` | 2 | 其中單一使用者最多可占用幾個 |
| `queue_timeout` | 30s | 工作在失敗之前等待輪到它的時間 |
| `timeout` | 60s | 沒有其他設定時工作的時間上限 |
| `max_timeout` | 10m | 單次呼叫、專案或設定檔最多可要求的時間 |
| `background_max_timeout` | 2h | `shell.start` 工作的上限 |
| `output_limit` | 256KiB | stdout 與 stderr 各自的上限；超出部分會被捨棄並設定 `truncated` |
| `tmp_limit` | 256MiB | 工作的 `/tmp` 與主目錄大小 |
| `memory` | 1GiB | 整個沙箱的記憶體上限。超出的工作會被終止，呼叫會擲回錯誤 |
| `cpus` | 無 | 以核心數表示的 CPU 上限 |
| `pids` | 512 | 沙箱內的行程數（`gvisor`） |
| `write_back.max_files` | 10000 | 單一工作可儲存的變更檔案數 |
| `write_back.max_bytes` | 1GiB | 單一工作可儲存的檔案總大小 |

**需求。** **`gvisor`** 只能在 Linux 上執行（x86_64 與 ARM64，核心 5.6 或更新版本，需啟用非特權使用者命名空間；無需 `/dev/kvm`，可在一般雲端 VM 與容器中運作）。**`microsandbox`** 透過外部 `msb` CLI（跨行程執行，AgentFlow 維持純 Go `CGO_ENABLED=0`）支援 Linux（`/dev/kvm`）、macOS（Apple Silicon Hypervisor.framework）與 Windows（WHP）。當所設定的執行環境無法在主機上運作時，引擎仍會啟動，而每個 shell 呼叫都會失敗並說明缺少什麼。

**記憶體與 CPU 上限。** 在 `gvisor` 下，當主機上有 systemd 管理員時由核心透過 systemd scope 強制執行（否則工作會在沒有這兩項上限的情況下執行，行程數、tmpfs、時間與輸出上限仍適用）。在 `microsandbox` 下，記憶體（`--memory`）與 CPU（`--cpus`）上限由微型虛擬機器管理程式直接強制執行。`agentflow doctor` 會回報目前後端、版本、上限是否強制執行，以及每個環境是否可用。

**一台主機，一個資料目錄。** 兩個共用同一 `data_dir` 的引擎會在啟動時清除彼此的工作狀態。

#### 代理設定檔（可衍生）

```
profiles:
  agent/coder:
    model: default
    loop: ./loop.ts
    skills: [builtin:fs.read, builtin:fs.write]
    capabilities: [llm.chat, tools]
    can_contact: []
    credentials: [deploy_token]   # 子代理的 credential.get 允許清單
    extras:                       # 在子代理中透過 agent.config() 唯讀取得
      workflow: release-train
      goal: { type: autonomous, max_turns: 12 }
    ephemeral:
      max_lifetime: 2h
      idle_ttl: 15m
      max_turns: 200
    budget:
      tokens_per_day: 100000
      window: 168h            # 選填的滾動視窗；省略則每日重設
```

宣告了 `budget.tokens_per_day` 的衍生設定檔會得到一個計量的 LLM 池，**由從它衍生的所有子代理共用**，因此一種專案類型（管理者 + 工作者）會從同一份預算中支用。設定 `window` 後，用量會隨著提交記錄超出視窗而持續回補，而不是在日界時重設。

`extras` 與 `credentials` 會沿用到每個子代理，因此呼叫 `agent.config()` 的衍生迴圈可以看到自己的設定檔選項與憑證允許清單，形式與靜態代理看到的相同。

### `gateway`：一個共用的 HTTP 監聽器

所有 HTTP 頻道（webhook、ghhook、bridge、telegram webhook/auto、qq webhook）都把路徑掛載在**同一個共用的 HTTP 伺服器**上，沒有每個頻道各自的 `listen`。監聽器只在 `gateway.listen` 設定一次（預設 `:8080`），伺服器也會提供 `GET /health` → `200 {"ok":true}`，供探測與反向代理伺服器健康檢查使用。`gateway.public_url` 是可從外部存取的基底網址（例如 `https://bot.example.com`），在需要執行環境替你註冊 webhook 時使用。

| 鍵 | 型別 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `gateway.listen` | string | :8080 | 所有 HTTP 頻道共用的單一 HTTP 監聽器 |
| `gateway.public_url` | string | — | 外部基底網址；telegram `webhook`/`auto` 必填 |
| `gateway.channels` | list | — | 頻道定義（見下文） |

#### 頻道類型

| 類型 | 說明 |
| --- | --- |
| `webhook` | POST → 路由器 → 工作階段 → 回覆。**同步**（預設）：回應會等到代理回覆，或在 `timeout`（預設 `55s`）到期時回傳 504。**非同步**（`async: true`）：立即回傳 202 與工作 ID；回覆透過 `GET <path>result/<id>` 取得（尚未完成時回傳 202），或 POST 到呼叫者提供的 `callback_url`；非同步工作永遠不會被同步逾時中斷，並在 15 分鐘後過期。預設路徑 `/webhook/` |
| `ghhook` | GitHub webhook 接收：以 `secret` 驗證 `X-Hub-Signature-256` HMAC，立即回應 200，並將事件以代理類型的訊息轉送（酬載 `{type: project_event, source: github, event, data}`）。沒有回覆路徑。預設路徑 `/hooks/github/` |
| `bridge` | 用於其他任何聊天平台（Slack、Discord、Teams…）的行程外配接器。配接器將事件 POST 到 `path`，並附上以 `secret` 為金鑰的 `X-Agentflow-Signature: sha256=<HMAC of the body>`；執行環境將回覆以 `{to, text, attachments}` POST 到 `deliver_url`，僅以 `token` 作為 Bearer 進行認證（對外傳遞不做 HMAC 簽章）。`timeout` 限制每次傳遞的時間（預設 10s）。Bridge 的傳送者記錄為自行宣稱的身分，因此無法連結到使用者資料 |
| `telegram`（`mode: polling`） | 預設。長輪詢 `getUpdates`，並強制執行 `allow_users`。會先清除 Telegram 端任何過時的 webhook：只要有 webhook 註冊中，getUpdates 就會回傳 409（例如從 webhook 模式切換回來時） |
| `telegram`（`mode: webhook`） | 在 `path`（預設 `/webhook/telegram/`）接收以 POST 傳來的更新；需要 `gateway.public_url`。每次傳遞都必須帶有 `secret_token` 標頭（否則回傳 401/403，在解析之前檢查）；可依頻道設定，未設定時每次啟動自動產生 |
| `telegram`（`mode: auto`） | 啟動時探測 `<public_url>/health`：可連線時註冊 `setWebhook`，否則 `deleteWebhook` 並改用長輪詢。`setWebhook` 失敗時也會先清除 webhook 再開始輪詢。需要 `gateway.public_url` |
| `qq`（`mode: websocket`） | 預設。騰訊官方的 QQ Bot API v2（QQ 開放平台）。執行環境以應用程式存取權杖（由 `app_id` + `secret` 取得，並在到期前更新）主動連線到 QQ 閘道，訂閱其 `scenes` 對應的 intents、傳送心跳，並在連線中斷後恢復工作階段；不需要任何可從網際網路存取的位址。若因 intents 不被允許或機器人遭封鎖而關閉連線，頻道會帶著錯誤停止 |
| `qq`（`mode: webhook`） | 平台將事件 POST 到 `path`（預設 `/webhook/qq/<name>/`）；請在主控台中將 `<public https url><path>` 設為回呼網址（連接埠 80、443、8080 或 8443）。每個請求（包括網址驗證交握）都必須帶有以 `secret` 衍生之金鑰所做的有效 Ed25519 簽章（否則回傳 401/403，在解析之前檢查） |
| `openai` | 在 `<path>chat/completions` 提供 OpenAI Chat Completions API，並在 `<path>models` 提供模型清單。`model` 指向公開的 `agents` 之一，每個代理執行自己的 `loop:`；若要把用戶端的訊息、工具與取樣參數原封不動轉送給模型，請將代理指向 `./examples/loops/openai_passthrough.ts`。`auth` 為 `user_token`（每位使用者的 Bearer，需要 `runtime.identity.enabled`）、`key`（來自 `keys` 的共用金鑰）或 `none`（僅限回送位址監聽器）。此類型的 `journal` 預設為 `metadata`，因此對話文字永遠不會進入稽核記錄。見[相容 OpenAI 的 API](openai-api.md) |

每個 HTTP 頻道的 `path` 都必須以 `/` 結尾（子樹掛載）。建議使用類似 UUID 的路徑區段，作為難以猜測的秘密前綴。兩個頻道掛載同一路徑是啟動錯誤，端點與其他頻道子樹重疊的 `openai` 頻道也一樣。Telegram 的 `token` 與 `secret_token`、ghhook 的 `secret`、qq 的 `secret`、bridge 的 `secret` 與 `token`，以及 `openai` 頻道的 `keys` 都是密鑰（見登錄表）：在單一檔案路徑上以環境變數插值，在 configdir 路徑上則是延遲參照；無法解析的密鑰會略過該頻道並發出警告，絕不會導致啟動失敗。

**QQ Bot**（`type: qq`）細節：

- **場景與 intents。** `c2c`（與機器人私聊）與 `group`（在 QQ 群聊中 @ 機器人）共用 `GROUP_AND_C2C_EVENT` intent；`guild`（在 QQ 頻道的子頻道中 @ 機器人）使用 `PUBLIC_GUILD_MESSAGES`；`dm`（QQ 頻道私訊）使用 `DIRECT_MESSAGE`，此權限需向平台申請才會授予，因此不在預設集合中。
- **工作階段。** 每則訊息的回覆目標，以及供路由器使用的 `payload.chat_id`，是 `c2c:<user_openid>`、`group:<group_openid>`、`guild:<channel_id>` 或 `dm:<guild_id>`，因此一個群聊或子頻道是一段對話，每個私聊則各自是一段對話。傳送者是 `user:qq:<openid>`（在群聊中為 `member_openid`，在 QQ 頻道中為作者 ID）；只要平台有提供，酬載也會帶有場景、訊息 ID、事件 ID、`union_openid` 與暱稱。QQ 會驗證傳送者，因此身分層會將其視為已驗證且可連結。
- **回覆。** 在平台的被動回覆視窗內（私聊 60 分鐘，群聊與 QQ 頻道 5 分鐘），回覆會以 `msg_id` 與遞增的 `msg_seq` 引用最新的傳入訊息，每則訊息在私聊中最多 4 則回覆，在群聊中最多 5 則。超過之後，或平台回應視窗已關閉時，回覆會改以主動訊息送出，而主動訊息會受到平台的速率限制，使用者也可以將其關閉。長文字會切成每段最多 1800 個字元。
- **Markdown。** 設定 `markdown: true` 後，私聊與群聊中的文字回覆會以 markdown 訊息（`msg_type: 2`）送出，在回覆視窗內仍屬被動回覆；QQ 頻道的子頻道與私訊則維持純文字。Markdown 是平台依機器人授予的能力。若平台拒絕某則 markdown 訊息，同樣的文字會以純文字重新送出，且 markdown 暫停 10 分鐘，因此在機器人缺少此能力時不會遺失任何內容。被切段打斷的程式碼區塊，會在一段的結尾關閉，並在下一段的開頭重新開啟。
- **媒體。** 傳入的附件會依頻道的 `media` 原則下載；沒有文字的語音訊息會使用平台的語音辨識文字。對外傳送時，QQ 僅透過公開網址擷取媒體：此頻道要求設定 `media.backend: s3`（短期 SigV4 預簽章 GET 網址）或 `media.temp_url.enabled: true` 搭配 `gateway.public_url`（HMAC-SHA256 臨時 HTTP 網址），兩者皆未設定時啟動階段會發出警告並略過該頻道。blob 儲存區控制代碼與內嵌圖片（例如模型輸出的 WebP/PNG）在簽署網址前會自動轉檔為 QQ 相容格式（JPEG/PNG，上限 10 MiB）並快取為衍生變體（在私聊與群聊中以富媒體上傳，在 QQ 頻道中以圖片連結送出）。
- **重複訊息。** 平台可能會多次傳遞同一則訊息（webhook 重試、恢復連線後的重播）；30 分鐘內重複出現的訊息 ID 會被捨棄。
- **主控台設定。** 從 q.qq.com 上機器人的頁面複製 AppID 與 AppSecret。使用 webhook 模式時，在該處輸入回呼網址，並將執行環境的對外 IP 加入主控台要求的 IP 允許清單；使用 websocket 模式時只需設定允許清單。機器人處於沙箱階段時，只有你在主控台中設定的沙箱群聊、QQ 頻道與成員能與它互動。

**稽核模式**依頻道設定：`journal: full`（除 `openai` 以外所有類型的預設值）會記錄訊息文字與附件描述；`journal: metadata`（`openai` 的預設值）每則訊息記錄一列，包含 ID、大小、使用者與時間，**絕不包含文字**；`journal: off` 則不為該頻道記錄任何內容。見 [`audit`](#audit--message-journal)。

**傳入媒體**需依頻道以 `media:` 區塊選擇啟用：一份 MIME 樣式允許清單（`image/*`、`application/pdf`、`audio/*`…）加上 `max_bytes`（預設 8 MiB）。Telegram 會下載檔案（getFile），webhook 則接受 base64 的 `attachments`；位元組會存入以內容定址的 blob 儲存區（預設為本機檔案系統，或透過頂層的 `media:` 組態使用 S3/MinIO），訊息只攜帶很小的部分描述。沒有 media 區塊時，附件會被捨棄，只有文字／說明文字會傳遞。

```
gateway:
  listen: ":8080"               # 單一共用監聽器（預設 :8080）
  public_url: ""                # 外部基底網址，例如 https://bot.example.com
  channels:
    - type: webhook
      path: /webhook/dev/9f2c.../   # 子樹；UUID 區段 = 秘密前綴
      agent: bot
#     timeout: 120s                 # 同步回覆的等待時間（預設 55s）
#     async: true                   # 或改為送出後輪詢：202 + 工作 ID，然後
                                    # GET result/ 或 callback_url
#     media:                        # 選填，媒體接收預設關閉
#       max_bytes: 8388608
#       allow: [image/*, application/pdf]
    - type: ghhook
      path: /webhook/github/9f2c.../
      secret: ${GITHUB_WEBHOOK_SECRET}   # HMAC 金鑰；留空則略過驗證（僅限開發）
      agent: bot
    - type: telegram
      token: ${TG_TOKEN}
#     secret_token: ${TG_WEBHOOK_SECRET}  # webhook 認證；留空 = 每次啟動自動產生
      mode: auto                 # polling（預設）| webhook | auto
      allow_users: [123456789]
      agent: bot
      media:
        max_bytes: 8388608
        allow: [image/*, application/pdf]   # 照片與文件會送給視覺模型
    - type: bridge
      name: teams
      path: /bridge/teams/
      agent: bot
      deliver_url: https://bridge.example.com/deliver
      secret: ${BRIDGE_SECRET}       # 傳入事件的 HMAC 金鑰（必填）
      token: ${BRIDGE_TOKEN}         # 選填，對外傳遞時使用的 Bearer
#     timeout: 10s                   # 每次傳遞（預設 10s）
    - type: qq
      name: qq
      agent: bot
      app_id: "102xxxxxx"            # 機器人的 AppID（QQ 開放平台主控台）
      secret: ${QQ_BOT_SECRET}       # AppSecret
      mode: websocket                # websocket（預設）| webhook
#     path: /webhook/qq/qq/          # 僅限 webhook 模式（此為預設值）
#     scenes: [c2c, group, guild]    # c2c | group | guild | dm（預設 c2c、group、guild）
#     api_base: https://api.bot.qq.com   # 預設值
#     markdown: true                 # 私聊與群聊中以 markdown 回覆
    - type: openai
      name: api
      path: /v1/                     # 提供 <path>chat/completions 與 <path>models
      agents: [bot]                  # 以模型 ID 的形式公開
      auth: key                      # user_token | key | none（僅限回送位址）
      keys: ["${COMPAT_API_KEY}"]    # auth: key
#     timeout: 600s                  # 整個請求的時間上限（預設 600s）
#     max_request_bytes: 33554432    # POST 本文上限（預設 32 MiB）
#     journal: metadata              # full | metadata | off；此類型預設為 metadata
#     expose_reasoning: false        # 以 reasoning_content 傳送模型的推理內容
```

> **破壞性變更**：每個頻道的 `listen:` 欄位已移除。仍帶有此欄位的組態會載入失敗，並出現 `field listen not found in type config.Channel`。請將每個頻道的連接埠移到單一的 `gateway.listen`，為每個 HTTP 頻道設定以 `/` 結尾的 `path`，並為 telegram webhook/auto 設定 `gateway.public_url`。見 README 中的遷移檢查清單。

### `mcp.servers`

```
mcp:
  servers:
    filesystem:                  # stdio：由執行環境啟動伺服器
      command: npx
      args: ["-y", "@mcp/server-filesystem", "/data"]
    affiliate:                   # Streamable HTTP：執行環境連線到已在執行的伺服器
      url: https://mcp.internal.example/mcp
      token: ${MCP_AFFILIATE_TOKEN}   # 以 Authorization: Bearer 傳送
```

伺服器宣告的**要麼**是 `command`（以子行程啟動，透過 stdio 溝通），要麼是 `url`（一個 Streamable HTTP 端點，主動連線）。兩者都在啟動時連線一次，無法連線的伺服器會被略過並發出警告，而不會中止執行。HTTP 傳輸方式讓整合可以完全存在於本儲存庫之外：一個獨立部署、獨立版本管理的服務，執行環境正是以此支援專有或需授權的連接器，而不必隨附它們。`token` 只適用於 HTTP 傳輸方式，會在每個請求中以 Bearer 憑證傳送；若端點不需要，請不要設定。

連線時探索到的工具會以 `mcp:<server>/<tool>` 進入登錄表。若伺服器中斷連線，呼叫會回傳結構化的 `unavailable`。

### `search`

```
search:
  default: ollama               # 呼叫未指定 `engine` 時使用的引擎
  engines:
    doubao:
      api_key: ${DOUBAO_API_KEY}   # 火山引擎網頁搜尋 Custom API
    ollama:
      api_key: ${OLLAMA_API_KEY}   # ollama.com 託管的網頁搜尋
    stackoverflow:
      site: stackoverflow          # 選填；任何 StackExchange 網站（預設 stackoverflow）
      # api_key 選填：匿名配額為每天 300 次請求；免費的應用程式金鑰可提高到 10k
    github:
      # 儲存庫搜尋：api_key 選填（權杖可將每分鐘 10 次請求提高到每分鐘 30 次）
    youtube:
      api_key: ${YOUTUBE_API_KEY}  # YouTube Data API v3：必填；配額約每天 100 次搜尋
```

設定 `builtin:web_search` 工具背後的引擎，可透過工具的 `engine` 參數在每次呼叫時選擇（`tools.run("builtin:web_search", {query=..., engine="doubao"})`）；省略 `engine` 時使用 `search.default`。若未設定任何引擎，工具會誠實回報不可用（`{ok:false, unavailable:true}`）。每個引擎的 `base_url` 與 `timeout` 都是選填的覆寫值。結果會正規化為 `{title, url, site, summary, snippet, content, published, score}`；`score` 是各引擎原生的分數（doubao 是 0..1 的相關度，stackoverflow 是原始的贊成票數，github 是星號數；youtube 沒有分數）。doubao、ollama 與 youtube（以及具依據的引擎 google\_search/x\_search）需要 `api_key`；stackoverflow 與 github 不需要（`site` 欄位可選擇任何 StackExchange 網路站點，例如 `superuser` 或 `serverfault`）。缺少或無法解析的金鑰會**略過該引擎並在啟動時發出警告**，而不會導致啟動失敗；若被略過的引擎是 `search.default`，預設值會退回唯一剩下的引擎（或沒有）。github 搜尋的是儲存庫，並支援查詢中 GitHub 完整的限定詞語法（`language:go stars:>100 pushed:>2024-01-01`）。youtube 透過 YouTube Data API v3 搜尋影片（site = 頻道標題，content = 說明，摘要前綴 `LIVE:` 表示直播）；共用的 `time_range` 會對應到 `publishedAfter`/`publishedBefore`。請注意 search.list 每次呼叫消耗 100 個配額單位（以預設專案配額計約每天 100 次搜尋），配額用盡時會誠實地回報 `quotaExceeded` 錯誤。

### `legal_search`

```
legal_search:
  default: hklii                # 呼叫未指定 `engine` 時使用的引擎
  engines:
    hklii: {}                   # 香港法律資訊研究中心：免費，不需金鑰
    npc: {}                     # 國家法律法規資料庫：免費，不需金鑰
                                # （可選擇覆寫 base_url / timeout）
```

設定兩個工具背後的引擎，可透過 `engine` 參數在每次呼叫時選擇（預設 `legal_search.default`）；未設定任何引擎時，兩者都會誠實回報不可用。兩個引擎的回答方式不同，因此工具說明是依已設定的引擎組合而成的：例如 `abbr`/`year`/`num`/`lang` 參數只在有 hklii 時才提供，因為 npc 一個都不讀。結果也是同樣的道理：所選引擎不會填寫的欄位會被省略，而不是以空字串送出，因為空字串會被理解為「未知」，而不是「不適用」。

`builtin:legal_search` 以 `{title, path, url, court, date, type?, effective?, status?, neutral?, parallel?, action?, parties?, coram?}` 回傳搜尋結果，`total` 則是完整的符合數量。其 `disable_fuzzy` 參數（預設 false）在每個引擎上的意義不同：在 hklii 上是關鍵字與概念搜尋之分，在 npc 上是僅標題與全文搜尋之分。`builtin:legal_read` 以搜尋結果的 `path` 為鍵，擷取一份文件的完整純文字，並以 `start`/`max_chars` 對長篇內容分頁。設定了多個引擎時，`path` 會被路由到發出它的引擎，因此不必重複搜尋呼叫時的 `engine`：搜尋結果的路徑是只有其所屬引擎才能讀取的不透明權杖，要求呼叫者再次指名該引擎，只是讓它有機會在執行環境早已知道的事情上出錯。明確指定 `engine` 仍然優先，且永遠不會被質疑。

**hklii** 涵蓋香港判例法與法例。搜尋結果帶有中立引註與平行引註、法院、日期、當事人與審理法官。閱讀*僅限案件判決*：`path` 為 `/en/cases/hkcfi/2020/1808`，或明確提供 `abbr`/`year`/`num`，並可選擇提供 `lang`（`en`|`tc`|`sc`）。法例搜尋結果（其 `court` 為 `Hong Kong Ordinances`，路徑類似 `/en/legis/ord/201/`）在這裡沒有可讀的全文；其路徑會以明確的錯誤拒絕，而不是回傳錯誤的內容。

**npc** 涵蓋中華人民共和國的國家法律、行政法規與司法解釋。搜尋結果以 `court` 表示制定機關，以 `date` 表示公布日期，並增加 `type`（法律/行政法规/…）、`effective`（施行日期）與 `status`（有效/已修改/已废止/尚未生效）；npc 沒有引註也沒有當事人，因此這些欄位不存在。閱讀只需要搜尋結果的 `path`（不透明的 `bbbs` 文件 ID），不接受其他參數：`abbr`、`year`、`num` 與 `lang` 在這裡沒有意義。回傳的 `neutral` 就是同一個 ID，而不是引註。文字是透過內建的 doc/docx 剖析器，從回傳的 Word 文件中擷取的。

### `browser`

```
browser:
  account_id: ${CLOUDFLARE_ACCOUNT_ID}   # Cloudflare 帳戶 ID（不是密鑰）
  api_token: ${CLOUDFLARE_API_TOKEN}     # 需要 "Browser Rendering - Edit" 權限
  base_url: https://api.cloudflare.com/client/v4   # 選填的端點覆寫
  timeout: 60s                           # 每個請求的時間上限（預設 60s）
```

設定 `builtin:browser`，它透過 Cloudflare **Browser Run**（前身為 Browser Rendering）的 Quick Actions 讀取頁面：這是真正的無頭瀏覽器，因此在原始 HTTP 擷取只會得到空殼的地方，也能取得由 JavaScript 轉譯的內容。單一工具，每次呼叫指定 `action`：`markdown` 以 Markdown 回傳頁面，`content` 回傳完整轉譯後的 HTML，`links` 回傳所有連結，`scrape` 回傳符合 `selectors`（CSS）的元素，`json` 回傳由 `prompt`/`schema` 描述的結構化資料，`accessibility_tree` 回傳無障礙樹。`url` 或 `html` 擇一指定頁面；`wait_until`（`load`|`domcontentloaded`|`networkidle0`|`networkidle2`）與 `wait_for_selector` 控制頁面何時算是載入完成，`networkidle` 系列的值正是讓單頁應用程式得以運作的關鍵。`json` 動作會在 Cloudflare 端執行 AI 模型，並以 Workers AI 用量計費。

長結果有上限：`max_chars`（預設 20000，`0` 表示不設上限）限制動作回傳的文字量，結果一律以 `chars` 回報真實長度，並在截斷時設定 `truncated`，讓模型能分辨是頁面本來就短還是被截斷了；`links` 同樣限制在 500 筆。未設定帳戶時，工具會誠實回報不可用（`{ok:false, unavailable:true}`），而無法解析的 `api_token` 會**略過該工具並在啟動時發出警告**，而不會導致啟動失敗，這與每個選用元件遵循的原則相同。只設定 `account_id` 與 `api_token` 其中之一是啟動錯誤，因為 `account_id` 不是密鑰，而設定一半的區塊是值得立即抓出來的錯誤。

有三個 Quick Actions 刻意不公開：`screenshot` 與 `pdf` 回傳原始位元組，目前還沒有合適的去處（工具結果是會整個進入模型上下文的 JSON，下游沒有任何東西會截斷它）；而 `crawl` 是非同步的，回傳的工作 ID 必須輪詢。

### `net`：對外連線原則

```
net:
  http:
    allow_private: false     # 預設；true 會停用位址防護
```

以單一設定同時控管**兩條**對外 HTTP 路徑：`builtin:fetch` 工具與指令碼的 `http.request` 操作，因此部署不會落得一條有防護、另一條沒有的情況。

防護會拒絕連線到回送位址、RFC 1918、電信級 NAT、鏈路本機位址（包括位於 `169.254.169.254` 的雲端中繼資料端點）、唯一本機位址、多播，以及保留／特殊用途的位址範圍。它安裝在**撥號器**中，而不是在剖析網址時檢查，而這個位置正是重點所在：檢查在 DNS 解析之後、建立連線之前立即執行，因此看到的是實際要撥接的位址。在剖析時檢查會被 DNS 重新繫結攻擊破解：名稱只被判斷一次，等到建立連線時已解析到別處；而且每一次重新導向都會開啟自己的連線，因此依其設計，每一跳都會被檢查。

有兩個值得注意的細節。*帶區域*的位址（`http://[fc00::1%25en0]/`）會在任何其他檢查之前直接被拒絕：Go 的前綴比對不會比對帶有區域的位址，因此若沒有明確的拒絕，這樣的位址會穿過每一條範圍規則，在防護毫不知情的情況下抵達唯一本機位址的主機。而 v4 對應的 v6 位址（`::ffff:10.0.0.1`）會在檢查前先解除對應，因為它與其 v4 形式指的是同一台主機。

**取捨：** 有防護的傳輸方式會刻意忽略 `HTTP_PROXY` / `HTTPS_PROXY`。在對外代理伺服器之後，撥號器連線的是*代理伺服器*，因此防護判斷的會是代理伺服器的位址，而目標根本不會被檢查。只透過代理伺服器對外連線的部署會發現這些路徑不使用它。另請注意，`198.18.0.0/15` 作為基準測試範圍會被封鎖，而它正是 Clash/mihomo 與 sing-box 在 TUN 模式下預設的假 IP 池：在執行其中任一者的開發機上，每個經由代理伺服器的網域都會解析到這個範圍內，因而被拒絕。

`allow_private: true` 會完全停用防護，並在啟動時記錄一則說明風險的 WARN，因此無法被默默開啟。它是為確實需要呼叫內部服務的部署而存在的；預設關閉，因為防護是遭提示詞注入的代理與主機自身網路之間唯一的屏障。防護只涵蓋這兩條由代理控制的路徑：模型、搜尋、法律與瀏覽器驅動程式使用的是管理員設定的 `base_url`，刻意不在涵蓋範圍內，因此內部鏡像仍能正常運作。

### `media`：blob 儲存區後端

```
media:
  backend: fs                 # fs（預設）| s3
  dir: ./data/media           # fs 根目錄（預設：persistence 資料庫旁）
  retention: 168h             # 依作用域的授權保留時間（預設 7 天；0 = 永久）
  derived_ttl: 24h            # 轉檔衍生變體快取 TTL（預設 24 小時）
  temp_url:                   # 選填的 HMAC-SHA256 臨時 HTTP 端點（適用於 fs + QQ）
    enabled: false
    path: /media/t/           # 掛載於 gateway.listen 的路由前綴（必須以 "/" 開頭與結尾）
    ttl: 10m                  # 簽章網址有效期限（預設 10 分鐘）
    secret: ${MEDIA_TEMP_URL_SECRET}  # 單節點選填（每次啟動隨機產生）；多執行個體叢集必填
  s3:                         # backend: s3 時必填；適用於 AWS S3 與 MinIO
    bucket: my-media
    region: us-east-1
    endpoint: ~               # MinIO／相容 S3 時設定（路徑樣式）；留空 = AWS（虛擬主機樣式）
    public_endpoint: ~        # 選填：當 endpoint 為內網位址時，供預簽章 GET 連結使用的公開基底 URL
    prefix: agentflow         # 物件鍵前綴
    access_key: ${S3_ACCESS_KEY}
    secret_key: ${S3_SECRET_KEY}
```

選擇統一的媒體儲存平面，集中存放來自頻道、LLM 輸出（`reply.media`）、MCP 工具與指令碼邊界的媒體位元組。無論後端為何，物件都以內容定址（`media:<sha256>` 控制代碼），因此切換時迴圈與稽核記錄的格式都不會改變；只有控制代碼會跨越指令碼橋接，傳出端（模型與頻道）會依需求具現化為位元組或簽章網址，並自動進行圖片轉檔與衍生變體快取。存取權限透過執行環境儲存區中的授權紀錄，依引擎蓋章的作用域（`user:<uuid>` 或 `agent:<name>`）強制隔離，同時保留內容去重；刪除使用者時會一併清除其所有授權。`s3` 後端使用純 SigV4（不依賴 SDK）：相同位元組的重複寫入會在 PUT 之前以 HEAD 去重，而密鑰永遠不會出現在任何錯誤中。啟用 `temp_url.enabled: true`（需設定 `gateway.public_url`）後，執行環境會在 `temp_url.path` 下提供短期 HMAC-SHA256 簽章網址，讓 QQ 等僅支援網址的頻道在本機 `fs` 儲存區下不需 S3 也能運作。

### `files`：使用者範圍的檔案儲存區

```
files:
  backend: fs                 # fs（預設）| s3（AWS S3、MinIO、Cloudflare R2）
  dir: ./data/files           # fs 根目錄（選填；省略時與 media 共用 blob 儲存區）
  s3:                         # 欄位與解析方式同 media.s3（選填獨立儲存貯體）
    bucket: my-files
    region: auto
    endpoint: https://<accountid>.r2.cloudflarestorage.com
    access_key: ${R2_ACCESS_KEY_ID}
    secret_key: ${R2_SECRET_ACCESS_KEY}
  max_file_bytes: 33554432    # 每個檔案的上限（預設 32 MiB）
  scratch_ttl: 24h            # 每個工作階段暫存區的存活時間；0 = 永不過期
  gc_grace: 24h               # blob GC 寬限期；0 = 停用 GC
```

支撐 `files.*` 指令碼操作（能力 `files`）：使用者範圍的專案樹，具備引擎原生的快照版本控制，另加每個工作階段的暫存區。預設情況下，當 `files` 未明確設定 `backend`、`dir` 或 `s3` 區塊時，會與 `media` 共用同一個 blob 儲存區，並由一次統一的每小時標記清除 GC 保護 `files_meta` 參照與有效媒體授權的聯集（超過 `gc_grace` 後才回收）；若明確設定了 `files.backend`、`files.dir` 或 `files.s3`，則維持獨立的 blob 儲存區，同時仍允許 `media` 將其作為控制代碼讀取備援來源。

檔案位元組與媒體一樣，在 blob 儲存區中以內容定址；快照中繼資料存放在執行環境儲存區的 `files_meta` 資料表中（一般的 sqlite，可直接檢視）。相容 S3 的端點需要真正的 SigV4 金鑰組，Bearer 權杖無法使用 S3 協定。無法解析的憑證組會停用該儲存區並在啟動時發出警告（隨後各操作會誠實地失敗），絕不會導致啟動失敗。

### `audit`：訊息日誌

```
audit:
  enabled: true               # 預設開啟
  retention_days: 90          # 清除期限；0 = 永久保留
```

核心會把每則訊息記錄到執行環境 SQLite 儲存區中的 `message_journal` 資料表：傳入訊息在路由器記錄（蓋上穩定的訊息 ID 與 `routed`/`dropped_queue` 狀態），傳出訊息在每個工作階段的傳出端記錄（`delivered` / `failed` / `blocked_safety`，並關聯到它所回覆的傳入訊息 ID）。資料列包含頻道、聊天、傳送者、代理、工作階段、文字、附件描述、文字與附件大小，以及來源資訊。頻道的 `journal:` 設定可縮小記錄範圍：`metadata` 保留資料列及其大小，但不含文字或附件；`off` 則不為該頻道寫入任何資料列。這是一項信任邊界功能：迴圈與頻道既無法繞過它，也無法偽造它；媒體只以控制代碼記錄，讓稽核記錄保持精簡。清除作業會在啟動時以及每天依 `retention_days` 執行。

### `usage`：Token 帳本

```
usage:
  events: false               # 逐次呼叫的明細資料列（預設關閉；每日彙總一律寫入）
  retention_days: 30          # 帳本的清除期限；0 = 永久保留
  default_tokens_per_day: 0   # 每位使用者的每日上限；0 = 無限制。使用者資料本身的上限會覆寫它
```

每日彙總（`usage_daily`）是所有配額與代理預算決策的依據，並保留在執行環境儲存區中。若有設定日誌平面，逐次呼叫事件會寫到那裡。`GET /admin/api/usage` 與 Studio 的用量畫面讀取的都是彙總資料。

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
        description: "Search the deployment's runbooks."   # 逐字取代，
                                                           # 或使用 prompts: 中的 {prompt: }
        params:                            # 淺層合併到 parameters.properties
          query: { description: "What to look up, in the user's words" }
        needs_confirm: false               # 原則欄位：needs_confirm、permission
        autonomous: true                   #   （cost_level / user_visible / autonomous
                                           #    會被解析但沒有任何東西讀取，啟動時會警告）
```

覆寫會在啟動時一次寫入登錄表，時機是在每個工具都註冊完成之後（內建、法律、shell、MCP），且在任何代理工具集合公開之前。指向結構描述未宣告之參數的 `params` 項目會被略過並發出警告，絕不會是錯誤。原則欄位依工具套用；`permission: forbidden` 會把該工具從每個公開的集合中移除，而且是其中唯一有作用的欄位。`cost_level`、`user_visible` 與 `autonomous` 會被接受並複製到工具上，但沒有任何東西會參考它們；`tools.policy.write` 同樣會被解析但不被使用。只要設定了其中任何一個，引擎就會在啟動時發出警告。

**涵蓋指令碼宣告的工具**：覆寫也可以指向迴圈以 `tool.def` 宣告的工具，見下文。這些項目不會在啟動時寫入（工具在程式碼區塊載入之前並不存在）；它們會在迴圈建立其清單時，對照即時的提示詞登錄表進行解析，因此編輯以 `file:` 為來源的提示詞，會在下一次 `tools.list()` 時出現。拼錯的*提示詞鍵*對任一種工具而言仍是啟動錯誤。既未註冊、也未被任何已載入迴圈宣告的工具名稱，無法在啟動時抓出，因此會在啟動時記錄一次，標示為保留給指令碼宣告（一行列出這些名稱的資訊訊息，讓拼字錯誤在任何代理執行輪次之前就能被看見），之後在某個迴圈首次宣告其工具卻沒有認領該名稱時，每個名稱各警告一次。

### `agents`

```
agents:
  bot:
    loop: ./loop.ts
    model: default
    instructions: ./prompts/bot.md     # 或 {prompt: assistant_system}，見 prompts:
    capabilities: [llm.chat, memory, tools, agent.send, shell.exec, net.mail]
    skills: [builtin:web_search, builtin:fs.read]
    memory: conversational     # 預設集，或內嵌定義
    safety: default
    shell: sandboxed                    # 選填
    can_contact: [worker]               # ACL
    budget:
      tokens_per_day: 500000
      window: 24h                       # 選填的滾動視窗；省略則每日重設
    channels: [webhook]
    persistent: true                    # 選填：常駐代理（見下文）
    credentials: [deploy_token]         # 選填：credential.get 允許清單
    inherit_user: false                 # 選填：即使被委派也以不帶租戶的方式執行
    extras:                             # 選填：透過 agent.config() 唯讀取得
      goal: { type: autonomous, success_signal: "PR merged", max_turns: 40 }
      workflow: release-train
```

**能力上限**：`plugins.allow_capabilities`（全域）⊇ `agents.<name>.capabilities`（每個代理）⊇ 衍生規格授予的能力（子代理）。你無法委派自己沒有的東西。若代理省略 `capabilities`，會得到預設值 `[llm.chat, memory, tools, agent.send, net.http, users]`；請注意 `net.mail`、`shell.exec` 與 `session.state` *不是*預設值，必須明確宣告。

**能力在執行時強制執行**，而不只是在啟動時驗證。每項能力控管其背後的指令碼操作：`llm.chat` → `llm.*`、`memory` → `store.*`、`tools` → `tools.*`、`apis` → `api.*`、`shell.exec` → `shell.*`、`net.http` → `http.request` 與 `os.env`、`net.mail` → `mail.*`、`files` → `files.*`。省略某項能力的代理會被拒絕，並附上指出該操作、代理與所缺能力的訊息：操作仍可被定址，並會清楚地失敗，而不是消失成未定義屬性錯誤。`agent.send`、`agent.request` 與 `agent.spawn` 一直都以這種方式強制執行；其餘的現在也一致了。`runtime.*` 與 `credential.get` 刻意不受控管：前者是每個代理都需要的迴圈機制，後者已經由代理自己的憑證清單限定範圍。啟動警告會確切列出每個代理與設定檔被扣留了哪些能力。只有在需要讓某個依賴「每個代理都擁有所有操作」的組態繼續運作時，才設定 `plugins.enforce_capabilities: false`（預設 `true`）。

**`builtin:` 是工具的命名空間。** 過去有四套詞彙都使用這個前綴（迴圈／外掛名稱、工具名稱、記憶供應商名稱，以及內建記憶設定檔），而字串本身完全看不出某個名稱屬於哪一套。現在每一套都有自己的寫法，因此這個前綴只代表一件事：

- **工具**保留它：`builtin:web_search`、`builtin:fetch`、`builtin:browser`、`builtin:legal_read`；
- **迴圈與外掛**寫成 `plugin:<name>`：`plugin:per_chat`、`plugin:recency`、`plugin:semantic`。這也一併解決了迴圈與路徑之間的混淆，因為路徑永遠不會以 `plugin:` 開頭，而以前名為 `per_chat` 的檔案是無法存取的；
- **記憶供應商**不加前綴：`sqlite`、`qdrant`、`redisvector`，`provider:` 鍵本身已說明了這是哪一種名稱；
- **內建記憶設定檔**是 `conversational`，而這個名稱現在是保留的：同名的 `profiles.memory` 項目是啟動錯誤，而不是一個永遠無法執行的設定檔。

舊的寫法會依名稱被辨識出來，而不是回報為未知，因此錯誤訊息會告訴你該怎麼寫：`builtin:per_chat` 的 `loop:` 會得到 `loops are spelled plugin: now`，`builtin:sqlite` 的供應商則會得到 `providers are unprefixed now`。另外兩種錯誤也會被回報：`loop:` 中寫的是*工具*名稱時，啟動會失敗，並提示它應該放在哪個欄位；`skills:` 項目中寫的是迴圈名稱時會發出警告，因為否則它不會有任何提示（它不符合任何工具、會被捨棄，而代理看起來就只是沒有工具）。完全不符合任何已註冊工具的 `skills:` 項目*不會*被回報，因為迴圈可以合理地列出它自己以 `tool.def` 宣告的工具。

**設定檔的額外欄位與憑證**：`extras` 是部署特定、唯讀的設定檔資料，透過 `agent.config()` 提供給迴圈（`goal` 區塊會被提升為頂層的 `goal` 鍵；其他一切都放在 `extras` 之下）。*完全*等於密鑰參照的值（`${VAR}`、`${VAR:-default}` 或 `cred:<service>`）在這個介面上永遠不會被解析：在 configdir 路徑上，它們保持原始形式，並呈現為不透明標記（`{env="VAR"}` / `{cred="service"}`），因此任何已解析的密鑰都不會到達迴圈（或衍生的子代理）；僅僅內嵌了參照的字串（`prefix-${VAR}`）則是一般的字面值，會照常展開。`credentials` 是 `credential.get()` 的允許清單：代理只能擷取它在這裡列出的已儲存密鑰；留空或缺少 = 沒有存取權。這兩個欄位也適用於衍生設定檔，並會傳到每個子代理。

**常駐代理**：工作階段通常在收到第一則訊息時才延遲衍生。設定了 `persistent: true` 的代理會在執行環境就緒後（頻道註冊之後）收到一則合成的 `{ type = "boot", from = "system:supervisor" }` 訊息，因此 cron 排程器與佇列取用者不需要外部流量就能啟動。

**租戶範圍隨委派傳遞**：代理跳轉（`agent.send` / `agent.request` / `agent.reply`）會在訊息由核心蓋章的來源資訊上攜帶傳送端輪次的租戶，接收端代理即以該租戶的身分行事，因此為同一個人工作的代理叢集，在每一跳都寫入那個人的檔案、記憶與 shell 範圍，而不是一個叢集共用的範圍。這個戳記取自傳送者由引擎指派的身分，絕不取自酬載，因此迴圈無法指名其他租戶（`payload.user_uuid` 會被忽略，無論是誰寫的）。這條鏈只能削弱：代理擁有的要麼是被給予的租戶，要麼沒有。`inherit_user: false` 讓代理不在跳轉間繼承租戶，此時它會在共用的 `service` 層中以不帶租戶的方式執行；而直接與它對話的人，仍會以自己的範圍與它互動。衍生設定檔也可以使用。

**預算**：`tokens_per_day` 會把 LLM 呼叫包在針對 Token 池的保留／提交／釋放流程中。預設情況下，池會每日重設；設定 `window`（例如 `24h`、`168h`）即可改為滾動視窗計算，用量會隨著提交記錄逐漸過期而持續回補，沒有午夜驟變。會超出預算的呼叫會被拒絕（計入 `agentflow_budget_denied`）。

### `prompts`：整個部署共用的提示詞登錄表

```
prompts:
  assistant_system:
    file: ./prompts/assistant.md       # 相對於 configdir 解析
  distiller:
    inline: "You are a memory distiller..."   # 字面值（text: 是別名）
  kb_header:
    inline: "[Knowledge base — recalled facts with provenance]"
```

系統提示詞、工具提示詞、注入提示詞與蒸餾文字都集中在一個地方，不必寫死在指令碼中，也不必在多個檔案間重複。每個項目都*恰好*指定 `file:`、`inline:` 或 `text:`（`inline:` 的別名）其中之一；無法讀取的 `file:` 會導致啟動失敗，而設定檔或工具覆寫若指向未定義的鍵，也會導致啟動失敗：缺少的提示詞絕不會降級為默默為空的字串。提示詞檔案在載入時讀取（並在變更時重新讀取），相對於 configdir 解析（單一檔案組態則相對於組態檔本身解析）。解析後的文字會以 `agent.config().prompts`（未宣告任何提示詞時為空物件）唯讀提供給每個迴圈，鍵名相同。

**參照提示詞**：代理或衍生設定檔上的 `instructions` 接受檔案路徑（與以前一樣），或 `{prompt: <key>}`，後者會讓登錄表中的文字成為該代理的系統提示詞。工具覆寫的說明也接受同樣的形式：除了既有的字面字串之外，還可以寫 `description: {prompt: <key>}`。以 `file:` 為來源的項目是*即時*的：編輯檔案會把其文字重新發布到每個迴圈的 `agent.config().prompts`，並就地更新所有以該鍵為來源之代理的系統提示詞，不需重新啟動工作階段（無法讀取的檔案會發出警告，並保留已發布的文字）。`prompts:` 區塊本身不是即時的：新增鍵、把鍵改指向其他檔案，或在 `inline:` 與 `file:` 之間切換項目，仍然需要重新啟動。

### `plugins`

```
plugins:
  dir: ./plugins
  allow_capabilities:           # 全域上限：任何東西都不能超過
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
  enforce_capabilities: true    # 預設；false 會恢復強制執行之前的行為
```

`plugins.dir` 也會**依名稱覆蓋內建項目**：檔案 `<dir>/<name>.ts`（或 `.js`，例如 `./plugins/recency.ts`）會在所有地方取代同名的內嵌內建項目，包括迴圈／路由參照（`loop: plugin:recency`，含熱重新載入的監看路徑），以及載入到每個工作階段中的支援程式碼區塊。覆蓋檔案會受到監看：被覆蓋的支援程式碼區塊會在每次工作階段啟動時（第一次以及每次重新載入重啟時）重新讀取，而編輯會重新啟動每個執行中的工作階段，因此不需重新啟動行程即可生效。未被覆蓋的程式碼區塊是內嵌的，沒有可供編輯的檔案。

### `triggers`：排程與事件任務

```
triggers:                       # 頂層，或組態目錄中的 triggers/*.yaml
  - name: morning-digest
    cron: "30 9 * * 1-5"        # kind = cron  (5 個欄位)
    every: 15m                  # kind = every (ms, s, m, h, d)
    event: { channel: webhook, match: urgent }   # kind = event
    run_on_boot: false          # 啟動時觸發一次，之後依排程執行
    target: { profile: pm }     # 已設定的代理，或衍生設定檔
    payload: { topic: news }    # 原封不動複製到觸發的訊息中
```

**`every:` 與 `cron:` 觸發器由引擎執行**，而不是由代理執行：到期時，執行環境會把一則一般訊息放入目標的信箱，

```
{ id = "cron:<trigger-name>:<unix>", type = "cron", from = "system:scheduler",
  payload = { ... }, ts = ..., provenance = { kind = "scheduler", principal = "trigger:<name>" } }
```

並送入工作階段 `<target>|<trigger-name>`。沒有任何代理為此持有能力，也沒有任何迴圈需要計時器：排程是引擎的基礎設施，因此部署可以移除過去用來執行的使用者空間排程器代理。傳遞一律會記入日誌並計數（`agentflow_trigger_fires`），並以 `info` 等級觸發（`debug` 會帶有下一次計算出的發生時間）。

- `every:` 接受以 `ms`、`s`、`m`、`h` 或 `d` 為單位的時間長度，允許組合與小數（`30s`、`15m`、`6h`、`1d`、`1.5h`、`2d12h`）。第一次觸發在啟動後經過一個間隔時（或使用 `run_on_boot` 時立即觸發）；之後的觸發都以第一次的時間點為基準，因此不會漂移。最短間隔為 100ms。
- `cron:` 接受經典的 5 個欄位（`minute hour day-of-month month day-of-week`），支援 `*`、`N`、`N-M`、`*/N`、`N-M/N` 以及以逗號分隔的清單。星期欄位接受 `0`–`7`（`0` 與 `7` 都是星期日）。當日期欄位*與*星期欄位都有限制時，符合*任一者*的日子都會觸發（經典規則）。不支援名稱（`mon`、`jan`），也沒有秒欄位。
- `cron:` 欄位以 `runtime.timezone_offset_hours`（固定的 UTC 時差，預設 0 = UTC）比對。它是時差，而不是具名時區，因此排程永遠不會因為其他地區的夏令時間而偏移：在 `8` 下，`30 9 * * 1-5` 表示 UTC+8 的 09:30，一年到頭皆然。
- `target.profile` 指向已設定的代理或衍生設定檔。衍生設定檔會解析為 `spawn:<profile>` 位址：一個執行該設定檔之迴圈、模型與授權的頂層工作階段（沒有父代理，也不會依父代理削弱能力）。
- `event:` 觸發器**不會**被排程：由路由器與迴圈自行比對。`runtime.triggers()` 仍會回傳完整的合併清單，格式不變，因此讀取它的迴圈仍能正常運作。
- 無法排程的觸發器（格式錯誤的運算式、低於下限的間隔，或既非代理也非衍生設定檔的 `target.profile`）會**被記錄並略過**；其餘的照常執行。組態中的一個拼字錯誤只會讓你損失一個觸發器與一行警告，絕不會讓啟動失敗。
