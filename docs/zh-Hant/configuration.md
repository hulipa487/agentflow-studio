## 組態指南

空部署也是有效的：`agents:` 可以整段省略，此時執行環境會作為管理／Studio 控制平面啟動，方便從 Studio 建立第一個代理。不過頻道、觸發器和其他參照仍然必須指向已存在的代理。

AgentFlow 用單一 YAML 檔案（`agentflow.yaml`）設定。所有值都在啟動時驗證 —— 未知的鍵就是啟動錯誤，並給出檔案和行號，絕不會拖到執行時才爆出來。凡是接受字串的地方都支援環境變數插值（`${VAR}` 與 `${VAR:-default}`）。

### 密鑰：環境變數參照、憑證儲存區與如實降級

含密鑰的欄位（`models.*.api_key`、`search.engines.*.api_key`、`gateway.channels[].token`/`secret`、`memory.backends.*.config.url`/`password`/`api_key`、`media.s3.access_key`/`secret_key`、`browser.api_token`、`profiles.ssh.*.password`）接受三種寫法：

- 字面值；
- `${VAR}`（或 `${VAR:-default}`）—— 從行程環境解析，找不到就回退到憑證儲存區裡同名的那一筆；
- `cred:<service>` —— 只從憑證儲存區解析。

在 **configdir** 路徑下，這些參照保持惰性，等使用方建構時才解析；在單一檔案 `-config` 路徑下，整份文件在載入時按位元組展開（舊行為，未改）。解析順序是：環境變數 → 憑證儲存區 → 解析不出來。降級原則是一致的：*選用*元件（搜尋引擎、telegram/ghhook 頻道、記憶後端、S3 媒體儲存區）上的憑證解析不出來，就**跳過該元件並發一則點名憑證的警告** —— 絕不會導致啟動失敗。模型的 `api_key` *不是*選用的：模型仍保持設定狀態，第一次 LLM 呼叫會失敗，錯誤裡點名那個沒解析出來的憑證。`runtime.credentials.enabled=false`（或沒有憑證儲存區）會讓 `cred:` 參照解析不出來，同樣適用這套原則。

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
apis:         { ... }   # 外部 API 擴充（Goja 指令碼 / wazero Wasm）
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

YAML 定義*拓撲* —— 有什麼、怎麼接。行為放在 TypeScript/JS 外掛和 Wasm 外部 API 裡，參數放在 YAML 裡。這種分離意味著一個能用的聊天機器人只要約 12 行 YAML、零程式碼，同時整條管線的每一處仍可覆寫。

> **嚴格解析**：`KnownFields(true)` 意味著每個未知鍵都是啟動錯誤。不存在「被忽略」的拼字錯誤。

### 組態目錄（`-config <dir>`）

除了單一檔案，也可以給 `-config <dir>` 傳一個目錄來載入目錄配置；只放了一個 `agentflow.yaml` 的目錄（也就是 `agentflow init DIR` 寫出來的）會載入那個檔案。每個片段都依同樣嚴格的方式解碼，而驗證在合併*之後*進行，所以跨檔案參照的檢查方式和單一檔案完全一樣。組態來源 —— 檔案所在目錄，或目錄本身 —— 是所有相對路徑的解析基準（迴圈、指令、提示詞 `file:` 路徑、路由器的路由、持久性、sqlite 後端路徑、Shell 的 `key_file`、`plugins.dir`、`media.dir`）。單一檔案也一樣：`agentflow run -config deploy/agentflow.yaml` 會在那個檔案旁邊解析 `./loops/x.ts`，與 Shell 的工作目錄無關。

```
<dir>/
  system.yaml       # 必填：除 agents/channels 以外的頂層組態
  channels.yaml     # 選填：channels: [...]
  profiles/*.yaml   # 選填：每個檔案一個代理（依排序後的 glob）
  triggers/*.yaml   # 選填：triggers: [...]（依排序合併，後面的檔案優先）
```

`profiles/*.yaml` 是一份代理組態，外加 `name` 與 `spawn` 旗標：`spawn: false`（或不寫）歸到 `agents`，`spawn: true` 歸到 `profiles.agent`（一個衍生範本，包含它的 `extras` 與 `credentials` 允許清單 —— 衍生出的子工作階段兩者都看得到）。在衍生設定檔上寫只屬於代理的欄位（`persistent`、`singleton`、`taps`、`channels`、`lifecycle`）是啟動錯誤。觸發器的形狀是 `{name, event{channel, match} | cron | every, run_on_boot, target{profile}, payload}` —— `every:` 與 `cron:` 觸發器*由引擎*執行（見下文 `triggers`），合併後的清單透過 `runtime.triggers()` 作為資料提供給迴圈。目錄部署會輪詢重讀 `triggers/*.yaml`，所以改排程無需重新啟動即可生效。

## 完整組態參考

### `runtime` —— 執行個體層級調校

| 鍵 | 型別 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `vm.memory_limit` | string | — | 純 Go JS/TS VM（`goja`）每個狀態的軟記憶體上限（`b`/`kb`/`mb`/`gb`/`MiB`/`GiB`；留空 = 不設限）。限制超大文字與集合的配置，超限時表現為指令碼記憶體不足錯誤 |
| `scheduler.workers` | int | 8 | 操作池大小。這是唯一能設定它的地方 |
| `persistence` | string | sqlite://./data/agentflow.db | 訊息日誌、Token 帳冊及其逐次明細，以及引擎的鍵值列（檔案樹、工作階段狀態、路由狀態、Shell 控制代碼）存放的位置。排程器計時器*不*在這裡：計時服務在記憶體裡，所以重新啟動會忘掉所有計時器，直到迴圈重新註冊 |
| `topology` | string | single | 部署層級：`single`（單一行程）或 `regional`（一個區域內多個執行個體共用一個 PostgreSQL）。不設定即 `single`。層級決定其他哪些設定合法；啟動時會拒絕它兌現不了的設定。見[拓撲](#topology) |
| `zone` | string | "" | 區域內的可用區標籤（字母、數字與 `. _ -`，最多 32 個字元）。會出現在指標與 `/admin/api/state` 裡；它只是標籤，絕不參與路由決策 |
| `shutdown_grace` | duration | 區域 5s · 單一執行個體 0 | 關閉時先回報未就緒、繼續服務多久才關閉監聽器，好讓負載平衡器排空、讓進行中的輪次跑完。`0` 表示不等待 |
| `sessions.idle_unload` | duration | 30m | 工作階段閒置這麼久後卸載。它的 VM 區域變數隨之消失；`session.state.*` 仍在，下一則訊息會重建工作階段。工作階段 ID 不回收，租約釋放。`0` 關閉卸載 |
| `admin.cors_origins` | list | \[\] | 允許呼叫 `/admin/api/*` 的精確瀏覽器來源（自建前端或別的瀏覽器用戶端）。Studio 不需要它：桌面應用程式與網頁應用程式的本機代理都是同源呼叫該 API。留空表示只允許同源；`"*"` 會被拒絕。預檢在 Bearer 驗證之前應答 |
| `admin.socket` | string | "" | 增加一個僅本機的管理監聽：Unix 上是 unix socket 路徑（相對路徑依組態目錄解析），Windows 上是 `\.\pipe\agentflow-admin` 這類具名管道。仍需 Bearer 權杖 |
| `admin.tcp` | bool | true | 在 HTTP 監聽器（`gateway.listen`）上提供管理面，掛 `/admin/` 以及 `/healthz`、`/readyz`、`/metrics`。`false` 則只在 `admin.socket` 上提供；兩種傳輸至少要留一個。不存在 `admin.listen`：管理面沒有自己的連接埠，設定了會被拒絕 |
| `admin.operators` | map | {} | 具名管理權杖（`name: token`），每個的權限與 `ADMIN_TOKEN` 相同。請求在日誌裡歸屬到對應名字。值可以是 `${VAR}` 或 `cred:<service>` 參照，啟動時解析；解析不出來的會被跳過並給出警告 |
| `limits.sessions_per_tenant` | int | 0 | 在 supervisor 的衍生邊界（路由首觸與 `agent.spawn`）限制每個租戶的作用中工作階段數。0 表示不限 |
| `limits.ephemeral_per_tenant` | int | 16 | 限制每個使用者的並行單輪工作階段數（OpenAI 頻道的預設工作階段類型）。與 `sessions_per_tenant` 分開，所以無狀態流量擠不掉對話名額。它需要有使用者才能計數，因此沒有使用者的請求（`auth: key` 或 `none` 的 `openai` 頻道）不受它限制。0 表示不限 |
| `limits.egress_per_minute` | int | 0 | 限制每個租戶每分鐘的 `http.request` 與郵件操作次數（固定視窗）。0 表示不限。租戶之間的操作池公平性始終開啟，不可設定 |
| `studio.enabled` | bool | true | 提供 `/admin/api/studio/*`（建構器、遊樂場、追蹤）。沒有管理權杖時該 API 無實際作用；`false` 回傳 503 |
| `studio.workspace` | string | <config dir>/studio | 單一檔案部署把 Studio 的迴圈與提示詞寫在這裡。目錄部署則寫自己的 `loops/`、`prompts/` 與 `profiles/` |
| `studio.trace_events` | int | 2000 | 每個工作階段為追蹤時間軸保留的事件數（有界環狀緩衝，最舊的先丟） |
| `studio.trace_sessions` | int | 200 | 保留多少個工作階段的追蹤環；超過就淘汰最久未活動的 |
| `studio.trace_payloads` | bool | false | 在追蹤事件裡帶上訊息、回覆與結果文字。中繼資料（操作、模型、耗時、Token、錯誤）始終保留 |
| `studio.chat_timeout` | duration | 120s | 遊樂場一輪最多等多久，逾時則帶著部分回覆與 `timed_out` 回傳 |
| `identity.enabled` | bool | false | 為每個傳入生成穩定的使用者 UUID；把 `from` 改寫為 `user:<uuid>`，從而啟用 `session.push_user`。開啟會重新劃定現有狀態的範圍 —— 見表格下方的說明 |
| `identity.persistence` | string | <persistence dir>/identity.db | 身分登錄表的 SQLite 路徑或 PostgreSQL DSN（預設跟隨 `runtime.persistence`） |
| `identity.require_registration` | bool | true | 讓未連結的帳號留在共用 service 層（沒有個人範圍，沒有配額身分），直到它連結到使用者資料。`false` 則在首次接觸時自動認領一個資料 |
| `identity.unregistered` | string | allow | 沒有資料的傳送者怎麼處理：`allow` 繼續在共用層裡服務；`reject` 丟棄傳入並要求其註冊或連結 |
| `credentials.enabled` | bool | false | 加密的依租戶憑證儲存區，支撐 `auth={service=...}` |
| `credentials.persistence` | string | <persistence dir>/credentials.db | 憑證儲存區的 SQLite 路徑或 PostgreSQL DSN（預設跟隨 `runtime.persistence`） |
| `credentials.master_key_env` | string | CREDENTIALS\_MASTER\_KEY | 存放主金鑰的環境變數名稱（啟動時讀取，絕不從檔案讀） |
| `credentials.engine_wide` | string | allow | 閘門控管 `credential.get()` —— 把原始密鑰交給指令碼的那個操作。`allow`（或留空）允許代理讀取其 `credentials:` 清單點名的內容；`maintenance` 則限制為引擎觸發的輪次（啟動、排程器、觸發器），租戶輪次因此永遠看不到全域密鑰。依租戶的路徑（`http.request` 與 `tools.http` 上的 `auth = {service = ...}`）從不回傳值 |
| `users.enabled` | bool | false | 在閘道監聽器上提供公開的 `/v1/users` API（需要 `identity.enabled`） |
| `users.registration` | string | open | `open` 或 `invite`（一次性邀請碼來自 `POST /admin/api/users/invites`） |
| `users.link_ttl` | duration | 10m | 頻道連結碼的有效期 |
| `users.push_same_tenant` | bool | false | 讓 `session.push_user` 拒絕目前輪次租戶以外的目標。多租戶聊天產品請開啟 |
| `users.jit_provisioning` | bool | true | OIDC 首次登入驗證通過時建立資料；`false` 則拒絕未知主體，直到操作員手動開通 |
| `users.oidc.issuer` · `audience` · `jwks_url` | string | — | 身分提供者的驗證設定（見下文*身分提供者*） |
| `users.oidc.subject_claim` · `email_claim` · `name_claim` | string | sub · email · name | 承載身分與開通資料欄位的權杖宣告 |
| `cluster.session_ttl` | duration | 30s | 工作階段認領在不續約的情況下能存活多久 |
| `cluster.poll_interval` | duration | 250ms | 執行個體多久查一次等待它擁有的工作階段的訊息 —— 這就是訊息落到錯誤執行個體上時多出的延遲 |
| `region` | string | "" | 給本執行個體所服務的區域命名：一個指標標籤 `region`、`/admin/api/state` 裡的一個欄位，以及執行個體登錄表的一部分。區域是一個獨立的單元，而不是全域部署的分片 |
| `log_plane.persistence` | string | "" | 僅附加平面的目錄 —— 訊息日誌與逐次用量明細 —— 每個 UTC 日寫一個 JSONL 檔案。留空則兩者都留在執行環境儲存區裡 |
| `timezone_offset_hours` | float | 0 | cron 觸發器依哪個 UTC 偏移匹配（例如 `8`、`-5.5`）；`every:` 不受影響 |

> **開啟身分層會重新劃定一切的範圍**：`identity.enabled` 不是裝飾性開關。開啟後，每個傳入的 `from` 都會從頻道原生傳送者改寫為穩定的 `user:<identity>`。個人範圍要往後才來：一旦某個帳號連結到資料 —— 或在首次接觸時認領一個，那只在 `runtime.users.require_registration: false` 時發生 —— 引擎會在訊息由核心掌管的 `provenance` 上蓋 `user_uuid`（迴圈讀的就是這個欄位；它絕不會寫進 payload，因為委派跳轉會從指令碼重建 payload），使用者範圍的各個子系統就依這個戳來解析。檔案儲存區的範圍隨之從 `agent:<agent>` 變成 `user:<uuid>`，一輪的記憶從共用 service 層轉到互動層，依使用者的配額也有了可扣的帳戶。在連結之前，它完全不帶個人範圍：變的只有 `from`。迴圈裡任何以 `from` 為鍵、或自己拼出範圍的東西，都會跟著一起變。
>
>  **什麼都不會自動遷移，引擎也判斷不出什麼挪了位置。** 在舊標頭下寫的列 —— 檔案、記憶、迴圈自己分鍵的任何東西 —— 不會被認領，因為沒法知道昨天的 `agent:bot` 就屬於現在這個 `user:u_xxxx`。所以要嘛在有真實流量之前就開啟這一層，要嘛自己寫遷移：一個把舊標頭的列複製到新標頭的迴圈，在切換時跑一次。反過來也一樣 —— 關掉它，或解除某個帳號的連結，都會把個人範圍留在原地。

管理伺服器提供 `GET /healthz`、`GET /readyz`、`GET /metrics`（Prometheus 文字格式）、`GET /admin/sessions` 與 `/admin/credentials`。（工作階段清單以前是 `/v1/sessions`：那個前綴屬於公開的使用者 API，而這個私有監聽器並不提供它。）即時計數器包括 `agentflow_sessions_active`、`agentflow_children_spawned`/`died`、`agentflow_llm_calls`、`agentflow_llm_tokens`、`agentflow_budget_denied` 與 `agentflow_trigger_fires`。

### 拓撲

`runtime.topology` 決定部署形態，而形態決定哪些設定合法。它是一個設定而不是十幾個，是因為另一種做法會得到「配了一半、看著卻健康」的部署：區域叢集配本機 blob 目錄，每個執行個體各留一份資料，還不報錯。啟動會拒絕這類組合，`agentflow validate` 會在啟動前就拒絕。

#### `single`

預設值，也是這個設定出現之前寫的所有組態的層級。單一行程；以前合法的現在都合法：

| 後端 | `single` |
| --- | --- |
| 執行環境、身分、憑證、租約、收件匣 | SQLite 或 PostgreSQL |
| 記憶：`sqlite`、`postgres`、`pgvector`、`qdrant`、`redis`、`redisvector`、`volatile` | 全部允許 |
| Blob（`media`、`files`） | 本機目錄或 S3 |
| `runtime.log_plane.persistence` | 允許 |

同一儲存區上的第二個作用中行程會在啟動時被拒絕，並給出對端的 id 與最後活動時間。`-force-single` 可以強行啟動；只在崩潰之後用，或者等過 30 秒的存活視窗。

單一執行個體不安裝工作階段中樞：沒有第二個擁有者可以接管對話，所以既不寫 `session:` 租約列，也不寫共用收件匣列。`-force-single` 會保留中樞 —— 用它意味著操作員宣告兩個行程可能共用一個儲存區，那麼租約就是防止同一個對話跑兩遍的唯一手段。

#### `regional`

一個區域內跨可用區的多個執行個體，共用一個 PostgreSQL。啟動會拒絕：

| 設定 | 拒絕原因 |
| --- | --- |
| `runtime.persistence` 不是 PostgreSQL | 否則每個執行個體都會留自己的資料庫 |
| `runtime.identity.persistence` / `runtime.credentials.path` 指向 SQLite | 否則每個執行個體都會留自己的身分或憑證。取消設定以跟隨執行環境儲存區，或指向 PostgreSQL |
| `runtime.cluster.persistence` 指向不同目標 | 租約與收件匣總是在執行環境儲存區裡 |
| 任何 `memory.backends` 條目使用 `sqlite` 供應商 | 否則每個執行個體都會留自己的對話歷史 |
| `media.backend` 或 `files.backend` 不是 `s3` | 否則每個執行個體都會留自己的 blob |
| 設定了 `runtime.log_plane.persistence` | 本機目錄只屬於一個行程 |
| `ADMIN_TOKEN` 未設定且 `runtime.admin.operators` 為空 | 每次啟動產生的權杖對每個對端都不認識，所以一個都不產生（啟動時檢查，因為它是環境設定） |

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

Redis 是一個涵蓋整個區域的叢集（locality 為 `region`）；`volatile` 仍為執行個體本地。每個執行個體每 10 秒往 `instances` 表寫一列心跳，最後心跳在 30 秒內就算存活；`/admin/api/state` 會回報 `topology`、`zone` 與每個作用中的 `instances` 條目，並對 `config_epoch` 不一致的對端加上標記。Studio 的總覽頁顯示同樣的清單。

多區域部署就是每個區域一個獨立的 `regional` 單元 —— 沒有全域層。見[多區域](multi-region.md)。

`agentflow doctor -online`（區域組態的預設行為）檢查執行側：開啟執行環境儲存區、確認每個 pgvector 後端都有 `vector` 擴充、在每個 S3 儲存桶裡 put/get/delete 一個探測物件，並 ping 一下 Redis 與 Qdrant。每列都是 PASS/FAIL，附帶修復方法。

### 命令列

`agentflow help` 列出命令，`agentflow help <command>`（或 `<command> -h`）印出某個命令的參數。這份說明是從執行檔自己的參數定義產生的，所以永遠是最新參考。

`agentflow run [flags]`，或直接 `agentflow [flags]` 不帶命令，就是執行引擎：

| 參數 | 預設值 | 含義 |
|---|---|---|
| `-config PATH` | `agentflow.yaml`，然後 `config.yaml` | 一個檔案、一個組態目錄，或一個含有 `agentflow.yaml` 的目錄 |
| `-log-level` | `info` | `dev`、`debug`、`info`、`warn` 或 `error` |
| `-log-format` | `text` | `text` 或 `json` |
| `-log-file PATH` | | 同時把日誌附加到這個檔案 |
| `-force-single` | 關閉 | 即使儲存區上看起來還有別的執行個體在跑也啟動（僅 `single`；崩潰之後用） |

所有命令遵循同樣的約定：

- 參數可以放在位置參數之後（`agentflow cred set NAME -user u_1`），`-config` 也可以放在命令之前（`agentflow -config deploy/ validate`）。
- 命令作用的對像是位置參數：`init [DIR]`、`user export USER`、`user import BUNDLE.zip`、`user keygen [PREFIX]`、`migrate blobs DIR`、`cred set NAME`。
- `-out FILE` 寫進檔案；不給就輸出到 stdout。唯一的例外是 `support-bundle`，它預設寫一個帶時間戳的 zip。
- `-json` 在 `doctor`、`config show`、`cred get` 與 `cred list` 上輸出機器可讀結果。
- 命令不適用的參數是錯誤，絕不靜默忽略。
- `user export` 與 `user import` 找管理 API 的順序是 `-url` → `$AGENTFLOW_URL` → 組態裡的 `gateway.listen` → `http://127.0.0.1:8080`；權杖是 `-token` 或 `$ADMIN_TOKEN`。

工作池大小是 `runtime.scheduler.workers`（沒有對應參數）。已移除的參數（`-configdir`、`-workers`、`-log-json`、`-no-tui`、`-no-console`、`-no-webui`、`init -dir`、`user export -id`、`user keygen -out`、`migrate blobs -from`、`cred -format`）會被拒絕，並給出替代寫法。

### 憑證命令列

`agentflow cred set|get|list|delete <name>` 管理載入組態（`-config`）所指儲存區裡的全域憑證。`set` 從互動式提示或 stdin 讀值 —— 絕不從 argv（用管道：`printf %s "$TOKEN" | agentflow cred set github_token`）。主金鑰來自 `credentials.master_key_env` 指定的環境變數（預設 `CREDENTIALS_MASTER_KEY`）；沒設定就是一個明確的錯誤。`list` 只印名字，從不印值。這些記錄正是 `credential.get()` 與組態裡的 `cred:<service>` 參照所解析的對象。

### 遷移部署

從單一執行個體升到區域單元是一次資料搬遷，`agentflow migrate` 不需要轉換就能做：兩種後端說的是同一種 SQL 方言，blob 又是內容定址的。

```
agentflow migrate store -from sqlite://./data/agentflow.db -to postgres://...
agentflow migrate store -from <src> -to <dst> -tables files_meta -append
agentflow migrate blobs ./data/media -kind media -config agentflow.yaml
```

`migrate store` 複製執行環境各平面、身分登錄表與憑證儲存區。目標非空時會被拒絕，除非傳 `-append`；它會回報哪些表被複製、哪些不存在。憑證按密文複製：目標必須使用同一個主金鑰，否則每個密鑰都解不開。`migrate blobs` 走訪本機 media 或 files 目錄，把每個 blob PUT 進 `-kind` 指定的 S3 儲存桶；內容定址意味著重跑時先 HEAD 已存在的，只上傳缺的。兩個命令都可以中斷後重跑。

### 使用者 API

引擎**不帶任何使用者介面**。它在 HTTP 頻道使用的同一個公開監聽器上提供一套純 JSON 的使用者資料 API（`runtime.users.enabled`，需要 `runtime.identity.enabled`），前端因此可以住在自己的儲存庫裡、依自己的節奏部署。介面約定以 OpenAPI 形式發佈在 `openapi/users.yaml`。

- **註冊** —— `POST /v1/users/register` 建立資料，並*只回傳一次*它的 API 權杖。`runtime.users.registration: invite` 改為僅限邀請，邀請碼由操作員從管理 API 發放（`POST /admin/api/users/invites`）。
- **頻道連結** —— `POST /v1/users/me/links` 回傳一個一次性驗證碼；使用者*從要連結的那個帳號*傳送 `/link <code>`，引擎在那則訊息到達時完成連結。只有資料擁有者能看到驗證碼，只有帳號擁有者能從該帳號發出，所以單靠任何一半都連結不了什麼。帶驗證碼的訊息永遠不會進到迴圈。只有平台會驗證傳送者身分的頻道才能連結 —— 目前是 telegram；webhook 自稱的傳送者與 ghhook 固定的傳送者都不行。
- **自助** —— `/me`（資料與連結帳號）、`/me/usage`（今天、七天視窗與配額狀況）、`/me/projects`（呼叫方的檔案專案）、`/me/links/{id}` 解除連結，以及 `/me/token/rotate` 作廢外洩的權杖。
- **認證永遠是 Bearer 權杖** —— 要嘛是部署身分提供者發的存取權杖，要嘛是 `POST /register` 發的 `afu_…` API 權杖。沒有 cookie，也沒有伺服器端工作階段，所以別的站點沒有可重放的背景憑證，也沒有 CSRF 權杖要帶。
- **跨來源** —— 在操作員把某個來源列進 `runtime.users.cors_origins` 之前，CORS 是**關閉**的；只有列出的來源才會被應答，而 `Access-Control-Allow-Credentials` 是刻意永不送出的：請求帶的是 Bearer 權杖，不是 cookie，瀏覽器沒有可借給別的來源的背景權限。`GET /v1/users/config` 是公開的，告訴前端註冊是否開放、能提供哪些頻道，以及本單元服務哪個區域（`runtime.region`）。

**身分提供者。** 設定 `runtime.users.oidc.issuer`（再加上 `audience`，以及發現端點不可用時的 `jwks_url`），引擎就會驗證那裡簽發的權杖：先對簽發方 JWKS 驗章，再驗 `iss`/`aud`/`exp`，最後把 subject 宣告當身分 —— 以 `<issuer>|<sub>` 為鍵，兩個簽發方因此不會撞車。登入、密碼、MFA 與工作階段時長都放在那個提供者裡；引擎只負責驗證。首次登入驗證通過時，會用權杖的 `email`/`name` 宣告開通資料，除非 `jit_provisioning: false` —— 那樣未知主體會被拒絕，直到操作員手動開通。只接受非對稱演算法，且簽發方在啟動時必須可達。

**部署提示。** `POST /register` 依用戶端位址限流，所以反代後面的前端也得在那層限流 —— 否則所有呼叫方共用一個位址。

### 管理 API

執行環境本身不帶內嵌的瀏覽器介面或終端面板 —— 它在 `/admin/api/` 下提供一套無頭的 JSON 與 SSE 控制面，**AgentFlow Studio**（以及自動化指令碼）連的就是它。

**一個連接埠。** 管理面與頻道共用 `gateway.listen`：`/admin/api/*`、`/admin/sessions`、`/healthz`、`/readyz` 與 `/metrics`。`/admin/`、`/healthz`、`/readyz`、`/metrics`、`/health` 與 `/v1/users/` 下的頻道路徑會被拒絕。

**認證：** 每條管理 API 路由（以及 `/metrics` 與 `/admin/sessions`）都要 Bearer 權杖 —— 即使來自回送位址也一樣。設定 `ADMIN_TOKEN`（或具名 `runtime.admin.operators`）即可固定；否則會產生一個隨機的每次啟動權杖，只印一次到日誌。每次啟動的權杖對任何能讀日誌的人都不算秘密，所以在權杖是臨時產生的情況下，管理面**只應答本機用戶端**：來自另一台機器的請求，或帶了 `Forwarded`、`X-Forwarded-For`、`X-Real-IP` 的請求（所以同機上的反代不算本機），一律 `403`；`/healthz` 與 `/readyz` 則對負載平衡器保持開放。設定了憑證就會把管理面開放給遠端用戶端，只靠權杖把守，所以前面要加 TLS。區域單元必須設定一個：每次啟動的權杖別的執行個體不認識。密鑰全程只寫：列舉端點回傳中繼資料與指紋，從不回傳值；組態檢視會遮住敏感鍵，同時保留 `${VAR}` 預留位置。

**本機傳輸：** `runtime.admin.socket` 增加第二個僅本機的管理監聽 —— Unix 上是 unix domain socket，Windows 上是具名管道 —— 而 `runtime.admin.tcp: false` 把管理面從 HTTP 監聽器上拿掉。socket 僅擁有者可存取（0600；管道 DACL 授予擁有者、Administrators 與 SYSTEM），Bearer 權杖仍然必要，Studio 桌面應用程式接受 `unix:///path` 或 `npipe://name` 形式的 URL。

**路由。** AgentFlow Studio 做的每件事都由 `/admin/api/` 下的一條 JSON 或 SSE 路由支撐，操作員也可以直接寫指令碼呼叫。所有路由都要同一個 Bearer 權杖。

| 路由 | 作用 |
| --- | --- |
| `GET /admin/api/state` | 執行個體身分、執行時間、組態世代、作用中工作階段、代理與頻道。 |
| `GET /admin/api/config` | 原始組態文字、遮罩後的結構化檢視，以及檔案的 mtime。 |
| `POST /admin/api/config/validate` | 對提交的文字做與啟動等價的驗證（嚴格欄位、環境變數展開、交互參照）。 |
| `POST /admin/api/config/save` | 驗證、備份成 `.bak`、原子取代。檔案在你背後被改過就回傳 409。 |
| `GET /admin/api/models` | 執行中 ∪ 檔案的模型集；密鑰遮成末四位指紋。 |
| `PUT/DELETE /admin/api/models/{name}` | 把模型熱套用到*本行程*（見上文叢集限制）。 |
| `POST /admin/api/models/{name}/test` | 透過執行中的管理器發一次 1 Token 呼叫，並分類結果（auth / not\_found / unreachable / …）。 |
| `POST /admin/api/models/persist` · `/revert` | 把執行中的集合寫進檔案，或把檔案裡的集合重載進執行中的管理器。 |
| `GET /admin/api/metrics` | 取樣計數器序列（5 秒一次，約 2 小時），供迷你圖使用。 |
| `GET /admin/api/logs` | SSE 日誌尾端：先回放環狀緩衝，再即時推送。 |
| `GET /admin/api/users` · `/{id}` | 帶今日用量與配額狀況的使用者資料；單一資料連同最近呼叫、稽核尾端與專案。 |
| `POST /admin/api/users/{id}/limit` | 設定資料的每日 Token 限額（0 表示沿用部署預設值）。 |
| `PUT /admin/api/users/{id}/settings` | 資料的模型與指令層 —— 這是設定它們的唯一途徑，因為一個人用哪個模型是成本決策，不是自助選項。 |
| `POST /admin/api/users/{id}/unlink` | 解除一個頻道帳號（最後一個會被拒絕）。 |
| `POST /admin/api/users/invites` | 產生一次性註冊邀請。 |
| `GET /admin/api/agents` · `PUT/DELETE /admin/api/agents/{name}` | 在執行時依定義安裝、取代或移除代理（不寫檔案；Studio 的代理路由才是會持久化的版本）。 |
| `GET/POST /admin/api/groups` · `GET/DELETE /{id}` | 租戶群組：列舉、建立、帶成員檢視、解散。 |
| `POST /admin/api/groups/{id}/members` · `DELETE /{id}/members/{user}` · `PUT /{id}/master` | 新增或移除成員，或把群組交給新群主。 |
| `GET/POST /admin/api/pools` · `GET/PUT/DELETE /{id}/grants` | 具名共用記憶集區，以及決定誰能用它的授權（租戶 → 讀／寫）。儲存區用 `pool: <name>` 綁定到集區。 |
| `GET /admin/api/users/{id}/export` · `DELETE /admin/api/users/{id}` | 匯出為某個資料保存的一切（隱私匯出），或清除它。 |
| `GET /admin/api/users/{id}/bundle?public_key=…` | 全保真遷移封裝：資料、記憶列、檔案位元組、群組與憑證，封給呼叫方的 X25519 公開金鑰。目標單元用私鑰 `POST /admin/api/users/import`。見[多區域](multi-region.md)。 |
| `GET /admin/api/usage?day=YYYY-MM-DD` | 當天的依租戶 Token 彙總，含總計。 |
| `GET /admin/api/credentials/users` | 存有憑證的使用者分區。 |
| `GET/POST /admin/credentials` · `DELETE /admin/credentials/{service}?user=` | 憑證儲存區：列舉中繼資料、開通依租戶密鑰、撤銷一個。只寫 —— 值永不回傳。 |

瀏覽器主控台涵蓋上面那四個分頁。這張表裡的其他一切（使用者、邀請、配額、租戶、用量、日誌與原始組態）在 AgentFlow Studio 裡都有對應畫面，或者可以用 `curl` 指令碼化。

### Studio —— 代理建構器、遊樂場與追蹤

**AgentFlow Studio** 是一個用來試作代理的桌面與網頁應用程式：寫迴圈和指令、驗證、把定義即時套用、與之對話、觀察這一輪的每一步、把結果持久化進部署組態。它住在自己的儲存庫 `agentflow-studio` 裡 —— 沒有嵌進執行檔 —— 用和主控台相同的 Bearer 權杖與管理 API 通訊。

- **代理** —— 每個已設定的代理，帶模型、能力、作用中工作階段數，以及它只存在於執行環境還是已持久化。可從範本建立（echo、LLM 對話、工具迴圈、一次性、帶記憶的群組聊天、知識庫、單輪 API）、編輯或刪除。
- **建構器** —— 常見欄位的表單（模型、安全、記憶設定檔、能力、技能、歷史預算、常駐旗標、`can_contact`、憑證、extras）、TypeScript/JS 編輯器、指令編輯器，以及用於其餘一切的原始 YAML。*驗證* 會檢查 prelude 指令、編譯程式碼區塊，並跑與啟動相同的代理驗證；*套用* 把它裝進執行環境；*套用並儲存* 還會寫進組態（一個會記住選擇的拆分按鈕）。編輯器旁的停駐區要嘛是 **執行** 面板 —— 與正在編輯的代理對話並看追蹤 —— 要嘛是指令碼參考（F1）；Ctrl+Enter 會即時套用待處理的變更並送出面板裡的訊息，或重送上一則，所以下一輪跑的就是螢幕上看到的。
- **遊樂場** —— 與單一代理的對話串。每條對話串是一個獨立工作階段（`<agent>|studio:<thread>`），所以對話不會串。回覆經合成的 `studio` 頻道到達，也就是說它們走的是普通傳出路徑：安全過濾器、稽核日誌與傳遞都不變。選用的*以使用者身分* UUID 會蓋上租戶，從而讓個人記憶範圍、配額與集區授權生效；不帶它時輪次沒有租戶。
- **追蹤** —— 這一輪的即時時間軸：`turn.start`、每個 `op.start`/`op.end`（模型、工具、耗時、Token、工具呼叫）、安全丟棄、回覆、重載與迴圈錯誤。文字酬載預設關閉（`studio.trace_payloads`）；中繼資料始終捕捉。
- **頻道** —— 閘道持久化的頻道集：新增、編輯、驗證、刪除 webhook、Telegram、GitHub-webhook 與 bridge 條目；密鑰保持只寫。儲存後下次啟動時綁定。
- **模型** —— 執行中的模型登錄表：新增、編輯、用真實呼叫測試、刪除，然後把執行中的集合持久化到組態或還原成檔案內容。
- **記憶** —— 見[記憶](memory.md)指南。編輯 `memory.backends` 與 `profiles.memory`：後端依供應商呈現，只顯示該供應商讀取的鍵；設定檔則是儲存區清單（後端、表、保留期限、視窗、必要特性、範圍或共用集區）加喚回、寫入、嵌入與重新排序設定。後端 URL 與 API 金鑰只寫：畫面顯示 `${VAR}` 或 `cred:` 參照，或把密碼打碼後的 URL。驗證與啟動等價，仍在使用的設定檔或後端不能刪除。沒宣告後端的部署會把內建的 `main_db` 顯示在執行環境儲存區上；儲存第一個後端時會把它一併寫進檔案，用內建 `conversational` 設定檔的代理因此不會丟儲存區。代理在啟動時開啟儲存區，所以儲存要下次啟動才生效，新建的設定檔也要等那次重新啟動後才能在建構器裡選。
- **安全** —— 編輯 `profiles.safety` 過濾鏈（有序的內建項與 `classifier:<name>` 參照）與 `safety.classifiers` 政策。驗證與啟動等價；仍被參照的設定檔與分類器不能刪除。分派器是依代理建構的，所以儲存要下次啟動才生效。
- **憑證** —— 依分區瀏覽加密儲存區（全域或依使用者），建立、編輯、撤銷，並設定注入的要求標頭／配置。值只寫，只以末四位指紋顯示；編輯即時生效，因為儲存區在請求時才讀。
- **總覽** —— 落地頁：執行個體、區域、版本、執行時間與組態世代，作用中工作階段、代理、頻道，以及取樣計數器的迷你圖（`/admin/api/state`、`/admin/api/metrics`）。
- **使用者** —— 帶今日配額的使用者資料、連結帳號（可解除）、專案、最近呼叫與稽核尾端；設定每日限額以及依使用者的模型與指令層；發放邀請；匯出或清除資料（`/admin/api/users/*`）。
- **租戶** —— 群組（成員、群主移交、解散）與記憶集區（建立、依代理與租戶授權）（`/admin/api/groups`、`/admin/api/pools`）。
- **用量** —— 依租戶、代理或模型分組的每日 Token 彙總（`/admin/api/usage`）。
- **日誌** —— 帶等級與文字篩選的即時日誌尾端（`/admin/api/logs`）。
- **組態** —— 原始組態檔，帶與啟動等價的驗證和受 mtime 保護、會備份的儲存（`/admin/api/config*`）；組態目錄部署則透過結構化畫面與檔案本身來編輯。

`Ctrl+K` 會開啟一個涵蓋所有畫面、代理與動作的命令面板。

**語言。** Studio 介面提供英文、簡體中文（简体中文）與繁體中文（繁體中文）。**EN / 简体 / 繁體** 切換在每個畫面的命令列、登入頁，以及*設定 → 語言*下。切換立即生效，無需重新啟動或重新載入，並且會被記住（瀏覽器存在本機儲存，桌面應用程式存在其 webview 的本機儲存）；首次啟動跟隨作業系統或瀏覽器的語言，找不到就回退英文。這些指南跟著同一個開關走，尚未翻譯的指南顯示英文。組態鍵、識別碼、模型與設定檔名、YAML、指令碼，以及執行環境回傳的任何內容 —— 包括錯誤訊息 —— 都依執行環境的原樣顯示。

**持久化是組態原生的。** 目錄組態下，Studio 寫 `profiles/<name>.yaml`（標上 `# managed by agentflow studio`）、`loops/<name>.ts` 與 `prompts/<name>.md`。單一組態檔下，它取代 `agents.<name>` 鍵（保留註解與其他小節），並把迴圈／提示詞寫到 `studio.workspace`；定義裡存的是絕對路徑，所以持久化的檔案不依賴下一個行程怎麼啟動。兩種方式都是當下即時套用、下次啟動時從組態載入。不是 Studio 寫的設定檔會被拒絕，除非強制覆寫。

**執行它。** 用戶端是自己儲存庫裡的獨立 Go 模組：

```
cd ../agentflow-studio
make build        # or: CGO_ENABLED=1 CC="zig cc" CXX="zig c++" go build -o agentflow-studio.exe .
./agentflow-studio
```

輸入執行個體 URL（`http://127.0.0.1:8080`，即 `gateway.listen` 的位址）與管理權杖。作為原生用戶端它直接送出權杖，所以不需要 `runtime.admin.cors_origins` 條目。要從另一台機器連線，執行環境需要設定好的 `ADMIN_TOKEN`（每次啟動的權杖只應答本機用戶端）；前面加 TLS。那份允許清單仍留給管理 API 的*瀏覽器*用戶端，且只接受精確來源（`http://localhost:5173` 與 `http://127.0.0.1:5173` 是兩個不同來源）；`"*"` 會被拒絕。

| Studio 路由 | 作用 |
| --- | --- |
| `GET /admin/api/studio/catalog` | 模型、工具、能力、安全／記憶設定檔、提示詞鍵、prelude 版本與範本。 |
| `GET /admin/api/studio/agents` · `/{name}` | 代理清單；單一代理連同其 YAML、迴圈、指令與持久化檔案中繼資料。 |
| `POST /admin/api/studio/agents/validate` | 對提交的定義與迴圈做無副作用驗證。 |
| `PUT /admin/api/studio/agents/{name}` | 驗證、即時安裝（`apply`）、選用持久化（`persist`）；回傳實際套用了什麼以及任何持久化錯誤。 |
| `DELETE /admin/api/studio/agents/{name}` | 停止它的工作階段、移除它，並只移除 Studio 管理的檔案。`force` 可越過作用中工作階段與手寫設定檔。 |
| `POST /admin/api/studio/chat` | 遊樂場的一輪。串流 SSE（`trace`、`reply`，然後 `done` 或 `error`），或 `stream: false` 時回傳 JSON。一輪以 `done` 結束（完成、逾時或被安全攔截）；傳遞不出去的訊息或中途失敗的迴圈以 `error` 結束，此時 `stream: false` 回傳 `ok: false` 連同追蹤。根本起不了一輪的請求（未知代理、不是資料的 `act_as_user`）在任何串流開始前就以 4xx 拒絕。 |
| `GET /admin/api/studio/threads` · `POST /threads/{thread}/stop` | 已觀察的對話串，以及對某個作用中工作階段的停止請求。 |
| `GET /admin/api/studio/trace/{agent}/{thread}` | 緩衝的追蹤；加 `?stream=true` 走即時 SSE。 |
| `GET/POST /admin/api/studio/channels` · `PUT/DELETE /{index}` | 持久化的頻道集；驗證草稿、建立、取代或移除。儲存需重新啟動。 |
| `GET /admin/api/studio/safety` | 安全設定檔與分類器、它們的使用情況，以及編輯器可參照的內建過濾器與模型集合。 |
| `POST /admin/api/studio/safety/validate` | 對設定檔或分類器草稿做與啟動等價的試執行。 |
| `POST/PUT/DELETE /admin/api/studio/safety/profiles` · `/classifiers` | 建立、取代或移除設定檔或分類器。正在使用中的會被拒絕；寫入會驗證、留 `.bak`，並需重新啟動。 |
| `GET /admin/api/studio/memory` | 記憶後端（密鑰以參照或打碼 URL 呈現）與設定檔、它們的使用情況，以及編輯器可選的供應商、模型與外掛。 |
| `POST /admin/api/studio/memory/validate` | 對設定檔或後端草稿做與啟動等價的試執行。 |
| `POST/PUT/DELETE /admin/api/studio/memory/profiles` · `/backends` | 建立、取代或移除設定檔或後端。名稱不可變；空的 URL 或 API 金鑰保留已儲存的值；正在使用中的會被拒絕；寫入會驗證、留 `.bak`，並需重新啟動。 |
| `GET/POST/DELETE /admin/api/credentials` | 列舉某個分區的中繼資料（`?user=`；留空為全域）、建立或更新密鑰（空密鑰保留已儲存的值）、撤銷一個（`?service=&user=`）。從不回傳值。 |

**限制。** 遊樂場是執行個體本地的：合成頻道註冊在一個行程裡，所以叢集裡某個對話串的工作階段歸另一個執行個體擁有時，這裡就收不到回覆。Studio 不存聊天歷史 —— 代理的記憶才是事實來源 —— 追蹤環在記憶體裡，重新啟動即重置。衍生設定檔撰寫、附件、回放與評測不在第一版範圍內。

### 日誌

日誌用五個**累積**等級 —— 選中一個就印它及以上。用 `-log-level` 選擇（預設 `info`）：

| 等級 | 放什麼 | 例子 |
| --- | --- | --- |
| `ERR` | 執行環境退出或無法繼續 | 組態載入失敗、記憶後端開啟失敗、連接埠綁定失敗 |
| `WARN` | 降級但繼續 | 上游 429/502 + 重試、頻道傳遞錯誤、迴圈崩潰（重新啟動中）、重載編譯失敗（保留舊版） |
| `INFO` | 生命週期 | 執行環境啟動、頻道註冊、telegram webhook 設定、工作階段衍生／退出、熱重載被接受 |
| `DEBUG` | 每次互動 | LLM API 請求、記憶 put/get/query（redis 往返、pgvector 查詢）、收到 telegram 更新、webhook/ghhook 傳入、操作分派、imap 擷取 / smtp 寄送 |
| `DEV` | 臨時開發日誌 | 除錯時加的臨時探針 —— `llog.Dev(log, ...)`；絕不有意提交 |

同樣的記錄也餵給 AgentFlow Studio 的即時日誌尾端（`/admin/api/logs`，短標籤 `ERR/WRN/INF/DBG/DEV`）。

### `models` —— 具名的 LLM 供應商

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

`provider` 選擇請求／回應形狀，`base_url` 選擇主機。OpenAI 相容端點（Ollama、vLLM、LiteLLM）要在 `base_url` 裡帶上 `/v1`；`openai` 會附加 `/chat/completions`，`openai-responses` 會附加 `/responses`。Anthropic 相容端點把 `base_url` 設到主機根；執行環境會附加 `/v1/messages`。`gemini` 則把 `base_url` 指向版本前綴（預設 `https://generativelanguage.googleapis.com/v1beta`），執行環境會附加 `/interactions`，並在 `x-goog-api-key` 要求標頭裡送密鑰。Gemini Interactions API 是非串流的，也不接受 Token 上限，所以 `llm.stream` 只產出一個緩衝的增量，`max_tokens` 不會送出。`provider: rerank` 是一個說 `POST {base_url}/rerank` 的交叉編碼器端點（TEI、vLLM、Jina、Cohere），透過 `llm.rerank` / 記憶的 `rerank_model` 使用，而不是當對話模型。

`server_tools` 列出要在每個請求上啟用的供應商原生伺服器端工具（例如 gemini 的 `google_search`、openai-responses 的 `web_search`/`x_search`）。與透過 `opts.tools` 按次傳入的用戶端函式工具不同，這些在供應商內部執行；執行環境只注入原生的 `{type: <name>}` 條目，依你寫的名字轉發。值是與供應商相關的字串，執行環境因此保持通用。在 OpenAI 兩種形狀上，這個裸條目在 SDK 的工具聯集裡沒有對應變體（它的工具類型被釘在 `"function"`），所以用 `param.Override` 建構；在 `gemini` 上，`google_search` 會對應到原生工具，其他名字依原始條目透傳。

### `memory.backends` —— 後端執行個體

下面每個後端都是真實的供應商 —— 沒有樁。儲存後端承載 KV／前綴／全文／TTL；向量後端承載語意喚回。SQLite 會有意以明確錯誤拒絕向量查詢，而不是靜默回傳空結果。

| 供應商 | 類別 | 特性 |
| --- | --- | --- |
| `sqlite` | 儲存 | kv、prefix\_scan、text\_search、ttl、transaction、vector（對 BLOB 欄做暴力餘弦；無需擴充，受 `vector_scan_limit` 限制，預設 50000 列） |
| `redis` | 儲存 | kv、prefix\_scan、ttl |
| `postgres` | 儲存 | kv、prefix\_scan、text\_search、transaction —— 資料庫裝了 pgvector 擴充時再加 vector（啟動時偵測；同一張表） |
| `volatile` | 儲存 | kv、prefix\_scan、ttl —— 記憶體裡，不持久；受 `max_entries` 限制 |
| `pgvector` | 向量 | vector（餘弦 k-NN；嵌入維度透過 `dim` 設定，預設 1536）。作為別名／相容供應商保留；`postgres` 在擴充存在時已能提供向量 |
| `qdrant` | 向量 | vector（經 Qdrant 的 gRPC API 做餘弦 k-NN —— `url` 的連接埠按 REST 讀，所以 `:6333` 變成 gRPC 的 `:6334`；`dim` 預設 1536，`collection` 預設 `agentflow`，選用的 `api_key`） |
| `redisvector` | 向量 | kv、prefix\_scan、ttl、vector（需要 RediSearch 模組，例如 Redis Stack；餘弦 k-NN，`dim` 預設 1536） |

每個後端有一個 **locality**，由供應商與 `runtime.topology` 推導，並以 `agent.info().memory.stores[*].locality` 暴露給迴圈：

| 供應商 | `single` | `regional` |
| --- | --- | --- |
| `postgres`、`pgvector`、`qdrant` | `deployment` | `deployment` |
| `sqlite` | `deployment`（這唯一執行個體就是部署） | 拒絕 |
| `redis`、`redisvector` | `deployment` | `region`（每個單元一個叢集） |
| `volatile` | `instance` | `instance` |

在區域單元裡，持久對話儲存區 —— `dialogue`、`facts`，或 `recall: plugin:semantic` 下的 `semantic` —— 必須放在 `deployment` 或 `region` 後端上。把持久儲存區放在 `volatile` 上會被驗證拒絕（工作階段遷到別的執行個體就丟了歷史），除非儲存區設了 `locality_ok: true`。無論哪個層級，易失性記憶都必須有界：要嘛後端設了 `config.max_entries`（最舊的條目被淘汰），要嘛綁定到它的每個儲存區都設了 `retention` TTL。

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

三種向量儲存區都只能做向量查詢，和 `pgvector` 一樣：其他任何查詢種類都是明確錯誤，絕不靜默回傳空疊代器。`dim` 在儲存區首次建立時固定，必須和嵌入模型一致；之後要改需要手工遷移。`redisvector` 把值存在向量旁邊，所以也提供 kv、prefix\_scan 與 ttl；`redis` 不變，仍是純 KV 選項。

`qdrant` 說 gRPC，但它的 `url` 保持每個部署現有的形狀 —— REST 的。協定決定傳輸（`https` 帶 TLS 撥號），連接埠會被換算：顯式的 `6333`（Qdrant 的 REST 預設連接埠）變成 gRPC 的 `6334`；其他顯式連接埠依原樣撥號，所以 gRPC 代理也能用；不帶連接埠的 URL 得到 `6334`。之前經僅 REST 的代理存取 Qdrant 的部署，需要改指 gRPC 監聽器。點本身仍用同樣的確定性 ID 定址，所以既有的 `agentflow` 集合可以原樣讀寫。

完全沒有 `memory.backends` 段時，引擎會在執行環境儲存區上註冊 `main_db`：檔案是 `sqlite` 供應商加 `path`，DSN 是 `postgres` 供應商加 `url`。這正是內建 `memory: conversational` 設定檔在最小組態裡也能用的原因，也意味著單一執行個體部署不需要第二個資料庫就能有持久記憶。`postgres` 執行環境儲存區同理也不需要 `memory:` 段；裝了 pgvector 擴充時，它的資料庫就能提供向量。

`config.url`/`config.password`/`config.api_key` 的密鑰參照解析不出來的後端會**被跳過並給出警告**，而不是讓啟動失敗：綁定到它的儲存區會重新綁到一個還活著的後端上（優先選提供 `text_search` 的，這樣向量儲存區降級為全文搜尋；迴圈依綁定的特性做能力偵測），`requires` 下調到回退後端支援的項；沒有後端存活時則被丟棄。真正未知的後端名仍然是啟動錯誤。

### `profiles` —— 可重用組態

組態依種類分組放在 `profiles:` 下 —— `memory`、`safety`、`shell` 與 `agent` —— 代理依名字參照它們（`memory: chat`、`safety: strict`、`shell: python`）。

#### 記憶設定檔

[記憶](memory.md)是完整指南：後端、設定檔與儲存區怎麼拼在一起，迴圈怎麼寫入與喚回，隔離、配方，以及驗證訊息。這一節是參考。

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

`write` 會作為 `agent.info().memory.write` 傳給迴圈；`memory.write` 一律走路由表（`plugin:routing_table`，或覆寫了它的 `plugins/routing_table.ts`）。

用 `recall: plugin:semantic` 時，`memory.recall({ text = "...", k = 5 })` 會把查詢轉成嵌入、在向量儲存區上做餘弦 k-NN，並在設了 `rerank_model` 時重新排序。沒有文字的查詢退回到依時間喚回。

| 儲存區鍵 | 說明 |
| --- | --- |
| `backend` | 必填。參照一個後端執行個體名。 |
| `table` | 這個儲存區綁定的實體表（或集合）。 |
| `retention` | 以 `ms`、`s`、`m`、`h`、`d` 為單位的時長（允許複合與小數，例如 `2d12h`），或字面值 `forever`。**它是寫入 TTL，不是清掃：** 它會成為任何未明確指定 TTL 的 `store.put` 的預設 TTL，所以紀錄在*寫入*之後那個時長過期，而在這個設定存在之前寫入的列永遠不會過期。解析不出來的值是啟動錯誤。 |
| `window` | 每張實體表最多保留的紀錄數，由定期 GC 裁剪。適用於 KV 後端；**向量後端（pgvector、Qdrant、Redis）不實作裁剪**，所以在這些上面設 window 或 retention 只是宣告了一個沒人執行的邊界 —— 引擎會在啟動時警告，而表會無限成長。 |
| `requires` | 啟動時依供應商驗證的特性清單。 |
| `pool` | 把儲存區綁定到具名共用集區（經租戶管理 API 建立）。授權決定哪些租戶能讀寫；綁定集區的儲存區不設 `scope`，且集區在啟動時必須存在。`shared: true` 是啟動錯誤 —— 集區取代了它。 |
| `scope` | `agent` 讓私有儲存區重新加入每個代理一個集區，而不是引擎的依使用者鍵範圍。 |
| `collection` / `policy` | **已棄用 —— 無效果。** 儲存區由後端與表綁定，沒有實作任何依儲存區的原則開關。解析它只為讓舊組態仍能啟動；設定了任一項時引擎會在啟動時警告。 |

> **隔離變化**：記憶儲存區**預設依代理私有** —— 共用同一設定檔（或 `conversational`）的兩個代理不再共用實體表；每個綁定 `<agent>.<table>`（例如 `writer.dialogue`）。迴圈仍用設定檔的儲存區名字；前綴在綁定時加上。**刻意共用**：把儲存區綁定到具名集區（`pool: <name>`），再透過租戶管理 API 授權給租戶；`shared: true` 現在是啟動錯誤。依代理隔離之前寫的既有列仍留在舊的未加前綴表裡，需要明確遷移。

#### 安全設定檔

```
profiles:
  safety/default:
    filters: [source-attribution, signal-gate, steady-directive, support-offer, affect-guard]
```

`safety: none` 是明確退出。分派器由核心掌管，與迴圈處理常式無關地執行。

#### 以模型為後盾的分類器

`safety.classifiers` 定義由專用模型支撐的審核或分類原則。設定檔用 `classifier:<name>` 來選擇啟用：

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

模型只回傳標籤與分數；執行環境套用 `categories` 與 `threshold`，所以模型輸出無法自己決定動作。必須有一個 `safe: allow`（或其他 `allow` 標籤）。分類器模型必須用引擎金鑰（`key_source: config`，預設值）；`key_source: user` 的模型會在啟動時被拒絕，否則一個沒有金鑰的使用者就能關掉審核。呼叫模型時不帶工具，也不帶伺服器端工具。

失敗遵循明確原則而不是靜默放行：`on_error`（供應商錯誤、輸出無效、逾時）、`on_saturated`（並行上限滿）、`on_budget_exhausted`（專用審核預算用盡）。三者預設都是 `block`。設 `mode: shadow` 可以只記錄判定而不執行；模型呼叫仍會發生（延遲也仍在），只是動作被抑制。`sources` 使用正規化詞彙 `channel`、`agent`、`scheduler`、`system`（頻道驅動內部蓋章 `user`，計時器蓋章 `timer`）。

傳入攔截會跳過本輪：迴圈根本看不到訊息，設了 `refusal` 就送出它。傳出攔截會壓制回覆，設了 `fallback` 就送出它。拒絕與退回每個工作階段每 30 秒最多一次，免得被攔訊息的洪峰變成傳出洪峰。分類器花費記錄在合成的 `__moderation_<name>` 帳冊桶下，所以既不消耗代理的 `tokens_per_day` 預算，也不消耗使用者的 LLM 配額。沒有 `safety:` 的衍生設定檔繼承發起工作階段的過濾鏈；要在 `profiles.agent.*` 上設 `safety:` 才能給子工作階段不同的設定檔。

#### Shell 設定檔

Shell 設定檔設定代理的本機沙箱工作區。用 `agents.<name>.shell` 綁定（代理還需要 `shell.exec` 能力）。

```
profiles:
  shell:
    code:
      provider: sandbox          # 僅 sandbox（預設）
      environment: python        # environments 中的一個項目（必填）
      project: workspace         # 工作執行所在的檔案儲存區專案（預設 workspace）
      workdir: app               # 命令的起始位置，相對於專案
      timeout: 90s               # 工作的時間上限，受 runtime.sandbox.max_timeout 限制
      mem_limit: 512MiB          # 可以調低 runtime.sandbox.memory，但不能調高
      cpu_limit: 1.0
      env: { CI: "1" }
      ignore: ["node_modules/", "__pycache__/"]   # 由工作產生，從不儲存
      network: none              # 唯一的值：沙箱工作沒有網路
```

**`provider: sandbox`** 把每條命令作為一次性沙箱作業執行，用 [gVisor](https://gvisor.dev)（`provider: gvisor`，Linux 上的預設值）或 [microsandbox](https://github.com/superradcompany/microsandbox)（`provider: microsandbox`，Linux 上走 KVM 的硬體隔離微虛擬機、macOS 上走 Apple Silicon HVF、或 Windows 上走 WHP）：環境提供根工具鏈，專案以讀寫方式掛到 `/workspace`，命令寫進專案的內容在結束時存下。別的都活不過一條命令，所以沒什麼要追蹤的，任何執行個體都能跑下一條。`ignore` 用一個小的 `.gitignore` 子集：`node_modules/`（任意位置叫這個名字的目錄）、`*.pyc`（任意位置的名字）、`build/out`（從專案根算起的路徑）；結尾的斜線把樣式限定為目錄。被忽略的路徑既不儲存也不清理 —— 它們作為盡力而為的快取留在執行個體上。

專案可以用工作樹裡的 `.agentflow/shell.yaml` 定製它的代理設定檔。它可以設 `environment`（部署宣告過的）、`env`、`workdir`、`timeout` 與 `ignore`。它沒有用於映像、網路原則或資源限制的欄位，未知欄位是錯誤而不是靜默無操作。

`profiles.shell` 下的 `provider: docker` 與 `provider: ssh` 已移除（`provider: ssh` 移到了 `profiles.ssh`）。仍在用它們設定會在載入時失敗，並給出替代寫法。

#### SSH 設定檔

SSH 設定檔獨立於 `profiles.shell` 設定持續性遠端 SSH 目標。用 `agents.<name>.ssh` 綁定（代理還需要 `net.ssh` 能力）。

```
profiles:
  ssh:
    box:
      host: build.example.com:22
      user: dev
      key_file: /etc/agentflow/id_ed25519      # 或 password:（密鑰參照）
      workdir: /srv/work
```

SSH 設定檔每個工作階段保持一條到已存在機器的連線；這裡不負責開通它。連線活在撥號的那個執行個體上，隨工作階段關閉。一個代理可以同時綁定 `shell:` 與 `ssh:` —— 本機專案工具（`builtin:shell.exec`、`builtin:fs.*`）仍操作沙箱專案，而 `builtin:ssh.*` 操作遠端主機。

#### `environments` —— 宣告的工具鏈

環境是沙箱作業啟動時的根檔案系統與變數。環境由操作員宣告；Shell 設定檔選一個，專案也可以另選一個。

```
environments:
  python:
    image: docker.io/library/python:3.12-slim@sha256:<64 hex digits>
    env: { PYTHONUNBUFFERED: "1" }
  tools:
    rootfs: /srv/agentflow/envs/tools
```

`image` 與 `rootfs` 二選一：

- **`image`** —— 一個**依摘要釘住**的 OCI 映像。只寫標籤會被拒絕：標籤可能被指向不同內容，那樣兩個執行個體就會用同一個名字跑不同的工具鏈。在 `gvisor` 下，映像首次使用時擷取並解包到 `<runtime.sandbox.data_dir>/images/<digest>`（公開／匿名擷取的儲存庫，gzip 壓縮層）。在 `microsandbox` 下，`msb` 首次使用時原生擷取並快取 OCI 映像。兩種情況下都會先套用映像自帶的 `Env`，再疊加這裡的 `env`。
- **`rootfs`** —— 主機上的一個目錄，裡面已經是根檔案系統（例如匯出的容器，或 `debootstrap` 樹）。它需要有 `/bin/sh`。

因為作業沒有網路，命令需要的一切都必須已經在環境裡：套件是映像的一部分，而不是命令裝出來的。

#### `runtime.sandbox` —— Shell 作業怎麼跑

每個設定都有預設值；已經宣告了環境與 Shell 設定檔的部署一個都不用配。

| 鍵 | 預設值 | 含義 |
|---|---|---|
| `provider` | `gvisor` | 沙箱執行環境後端：`gvisor`（`runsc`，Linux）或 `microsandbox`（`msb`，Linux KVM / macOS Apple Silicon / Windows WHP） |
| `runsc` | `PATH` 上的 `runsc` | `provider: gvisor` 時的 gVisor 執行環境執行檔。把該發行版的 `gvisor-bin/` 目錄放在它旁邊 |
| `microsandbox` | `PATH` 上的 `msb` | `provider: microsandbox` 時的 microsandbox CLI 執行檔 |
| `data_dir` | `<data dir>/sandbox` | 工作區、解包後的映像與執行環境狀態。這是快取：裡面的一切都能從檔案儲存區與映像儲存庫重建 |
| `concurrency` | 4 | 本執行個體同時執行的作業數 |
| `per_user` | 2 | 其中同一使用者最多能佔幾個 |
| `queue_timeout` | 30s | 作業排隊等輪到自己多久後失敗 |
| `timeout` | 60s | 沒有其他設定時的作業時限 |
| `max_timeout` | 10m | 呼叫、專案或設定檔最多能要求的時限 |
| `background_max_timeout` | 2h | `shell.start` 作業的時限 |
| `output_limit` | 256KiB | stdout 與 stderr 各自的上限；超出部分丟棄並置 `truncated` |
| `tmp_limit` | 256MiB | 作業的 `/tmp` 與家目錄大小 |
| `memory` | 1GiB | 整個沙箱的記憶體上限。超限的作業被殺，呼叫拋錯 |
| `cpus` | 無 | CPU 上限，單位為核 |
| `pids` | 512 | 沙箱內的行程數（`gvisor`） |
| `write_back.max_files` | 10000 | 一個作業最多能存多少個改動過的檔案 |
| `write_back.max_bytes` | 1GiB | 一個作業能存的檔案總大小 |

**要求。** **`gvisor`** 只在 Linux 上跑（x86_64 與 ARM64，核心 5.6 或更新，需要非特權使用者命名空間；在沒有 `/dev/kvm` 的雲端主機／容器裡也能用）。**`microsandbox`** 透過 `msb` CLI 在 Linux（`/dev/kvm`）、macOS（Apple Silicon Hypervisor.framework）與 Windows（WHP）上跑（行程外 —— AgentFlow 仍是純 Go `CGO_ENABLED=0`）。設定的執行環境在主機上跑不起來時，引擎仍會啟動，每次 Shell 呼叫都以一句說明缺什麼的話失敗。

**記憶體與 CPU 限制。** 在 `gvisor` 下，主機有 systemd manager 時由核心透過 systemd scope 強制執行（否則作業不帶這兩項限制執行，而 PID、tmpfs 大小、時限與輸出上限仍然生效）。在 `microsandbox` 下，記憶體（`--memory`）與 CPU（`--cpus`）限制由微虛擬機 hypervisor 直接執行。`agentflow doctor` 會回報目前生效的供應商、它的版本、限制是否被強制執行，以及每個環境是否可用。

**一台主機，一個資料目錄。** 兩個引擎共用一個 `data_dir` 會在啟動時互清對方的作業狀態。

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

宣告了 `budget.tokens_per_day` 的衍生設定檔會得到一個**由它衍生出的每個子工作階段共用**的計量 LLM 集區，所以一個專案類型（經理 + 工人）從同一個預算裡花。設了 `window` 時，用量隨提交逐步超過視窗而持續釋放，而不是在日界處清零。

`extras` 與 `credentials` 會帶到每個子工作階段，所以衍生出的迴圈呼叫 `agent.config()` 時看到的是自己的設定檔選項與憑證允許清單 —— 和靜態代理看到的一樣。

### `gateway` —— 一個共用 HTTP 監聽器

所有 HTTP 頻道（webhook、ghhook、bridge、telegram webhook/auto、qq webhook）都把路徑掛到**一個共用 HTTP 伺服器**上 —— 沒有依頻道的 `listen`。監聽器在 `gateway.listen` 設一次（預設 `:8080`），這個伺服器還提供 `GET /health` → `200 {"ok":true}`，供探測端點與反代健康檢查使用。`gateway.public_url` 是外部可達的基礎位址（例如 `https://bot.example.com`），執行環境需要替你註冊 webhook 時用它。

| 鍵 | 型別 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `gateway.listen` | string | :8080 | 所有 HTTP 頻道共用的那一個 HTTP 監聽器 |
| `gateway.public_url` | string | — | 外部基礎 URL；telegram 的 `webhook`/`auto` 模式需要 |
| `gateway.channels` | list | — | 頻道定義（見下） |

#### 頻道類型

| 類型 | 說明 |
| --- | --- |
| `webhook` | POST → 路由 → 工作階段 → 回覆。**同步**（預設）：回應暫停，直到代理回覆或 `timeout`（預設 `55s`）→ 504。**非同步**（`async: true`）：立刻回傳 202 與工作 ID；回覆透過 `GET <path>result/<id>`（未完成時 202）收取，或 POST 到呼叫方給的 `callback_url` —— 非同步作業不會被同步逾時截斷，15 分鐘後過期。預設路徑 `/webhook/` |
| `ghhook` | GitHub webhook 入口：用 `secret` 驗證 `X-Hub-Signature-256` HMAC，立即回 200，再把事件作為代理類型的訊息轉發（payload `{type: project_event, source: github, event, data}`）。沒有回覆路徑。預設路徑 `/hooks/github/` |
| `bridge` | 面向任何其他聊天平台（Slack、Discord、Teams 等）的行程外配接器。配接器把事件 POST 到 `path`，帶 `X-Agentflow-Signature: sha256=<請求主體的 HMAC>`，密鑰是 `secret`；執行環境把回覆以 `{to, text, attachments}` POST 到 `deliver_url`，只用 `token` 作為 Bearer 認證（傳出傳遞不做 HMAC 簽章）。`timeout` 限制每次傳遞（預設 10s）。bridge 的傳送者記為「聲稱身分」，因此無法連結到資料 |
| `telegram`（`mode: polling`） | 預設。長輪詢 `getUpdates`，強制執行 `allow_users`。會先清掉 Telegram 側殘留的 webhook —— 有 webhook 註冊時 getUpdates 會 409（例如從 webhook 模式退回時） |
| `telegram`（`mode: webhook`） | 在 `path` 接收 POST 更新（預設 `/webhook/telegram/`）；需要 `gateway.public_url`。每次傳遞都必須帶 `secret_token` 要求標頭（否則 401/403，在解析之前檢查）；依頻道設定，缺失時每次啟動產生 |
| `telegram`（`mode: auto`） | 啟動時探測 `<public_url>/health`：可達就註冊 `setWebhook`，否則 `deleteWebhook` + 長輪詢。`setWebhook` 失敗時也會先清 webhook 再輪詢。需要 `gateway.public_url` |
| `qq`（`mode: websocket`） | 預設。騰訊官方 QQ 機器人 API v2（QQ 開放平台）。執行環境帶著應用存取權杖主動連到 QQ 閘道（權杖由 `app_id` + `secret` 換取，到期前刷新），訂閱 `scenes` 所需的 intents，送出心跳，掉線後恢復工作階段；無須對公網開放任何入口。因 intents 不被允許或機器人被封而斷開時，頻道會帶錯誤停止 |
| `qq`（`mode: webhook`） | 平台把事件 POST 到 `path`（預設 `/webhook/qq/<name>/`）；在主控台把 `<公網 https 位址><路徑>` 設為回呼位址（連接埠 80、443、8080 或 8443）。每個請求（包括 URL 驗證握手）都必須帶由 `secret` 衍生金鑰簽出的有效 Ed25519 簽章（否則 401/403，在解析之前檢查） |
| `openai` | 在 `<path>chat/completions` 提供 OpenAI Chat Completions API，在 `<path>models` 提供模型清單。`model` 是公開的 `agents` 之一，每個跑自己的 `loop:`；把代理指向 `./examples/loops/openai_passthrough.ts`，就能把用戶端的訊息、工具與取樣參數原樣轉發給它的模型。`auth` 可以是 `user_token`（依使用者 Bearer，需要 `runtime.identity.enabled`）、`key`（來自 `keys` 的共用金鑰）或 `none`（僅回送監聽器）。此類型的 `journal` 預設是 `metadata`，對話文字因此永不進入稽核日誌。見 [OpenAI 相容 API](openai-api.md) |

每個 HTTP 頻道的 `path` 必須以 `/` 結尾（子樹掛載）。UUID 風格的路徑片段是推薦的不可猜密鑰前綴。兩個頻道掛同一個路徑是啟動錯誤，`openai` 頻道的端點與另一個頻道的子樹重疊也是。Telegram 的 `token` 與 `secret_token`、ghhook 的 `secret`、qq 的 `secret`、bridge 的 `secret` 與 `token`、`openai` 頻道的 `keys` 都是密鑰（見登錄表）：單一檔案路徑上做環境變數插值，configdir 路徑上是惰性參照 —— 解析不出來的會跳過該頻道並給出警告，絕不導致啟動失敗。

**QQ 機器人**（`type: qq`）細節：

- **場景與 intents。** `c2c`（與機器人的私聊）與 `group`（QQ 群組裡的 @ 機器人）共用 `GROUP_AND_C2C_EVENT` intent；`guild`（頻道裡的 @）用 `PUBLIC_GUILD_MESSAGES`；`dm`（頻道私訊）用 `DIRECT_MESSAGE`，這個需要向平台申請開通，所以不在預設集合裡。
- **工作階段。** 每則訊息的回覆目標，以及對路由器而言的 `payload.chat_id`，是 `c2c:<user_openid>`、`group:<group_openid>`、`guild:<channel_id>` 或 `dm:<guild_id>`，所以一個群組或頻道就是一個對話，每個私聊各自獨立。傳送者是 `user:qq:<openid>`（群組裡是 `member_openid`，頻道裡是作者 id）；payload 還帶場景、訊息 ID、事件 ID、`union_openid` 以及平台送來的暱稱。QQ 會驗證傳送者身分，所以身分層把它們視為已驗證、可連結。
- **回覆。** 在平台的主動回覆視窗內（私聊 60 分鐘，群組與頻道 5 分鐘），回覆會參照最新的傳入訊息，帶 `msg_id` 與遞增的 `msg_seq`，私聊每則訊息最多 4 則回覆，群組 5 則。超出視窗，或平台答覆視窗已關閉時，回覆改為主動訊息送出，平台會限流，使用者也可以關掉。長文字按最多 1800 個字元分塊。
- **Markdown。** 開 `markdown: true` 時，私聊與群組裡的文字回覆以 markdown 訊息送出（`msg_type: 2`），在回覆視窗內仍是被動的；頻道與私訊保持純文字。Markdown 是平台依機器人開通的能力。若平台拒絕 markdown 訊息，同樣的文字會改以純文字重送，並暫停 markdown 十分鐘，所以機器人沒有這項能力時也不會丟內容。被分塊切斷的圍籬程式碼區塊會在一個塊末尾關閉，並在下一個塊開頭重新打開。
- **媒體。** 傳入附件依頻道的 `media` 原則下載；沒有文字的語音訊息使用平台的語音辨識文字。傳出時，QQ 只透過公開 URL 抓取媒體：頻道要嘛要求 `media.backend: s3`（短時效 SigV4 預先簽章 GET URL），要嘛要求 `media.temp_url.enabled: true` 加 `gateway.public_url`（HMAC-SHA256 臨時 HTTP URL）；兩者都沒設定時會在啟動時給出警告並跳過該頻道。blob 儲存區控制代碼與內嵌圖片（例如模型輸出的 WebP/PNG）會自動轉碼成 QQ 相容格式（JPEG/PNG，上限 10 MiB），並在簽 URL 之前快取為衍生變體（私聊與群組裡以富媒體上傳，頻道裡作為圖片連結送出）。
- **重複。** 平台可能重複傳遞同一則訊息（webhook 重試、恢復後的重放）；30 分鐘內重複的訊息 ID 會被丟棄。
- **主控台設定。** 從 q.qq.com 的機器人頁面複製 AppID 與 AppSecret。webhook 模式在那裡填回呼位址，並把執行環境的出口 IP 加進主控台要求的 IP 允許清單；websocket 模式只需要允許清單。機器人處於沙箱狀態時，只有在主控台設定過的沙箱群組、頻道與成員才能觸達它。

**稽核模式**依頻道設定：`journal: full`（除 `openai` 外每種類型的預設值）記錄訊息文字與附件描述元，`journal: metadata`（`openai` 的預設值）每則訊息記一列，含 ID、大小、使用者與耗時，**絕不含文字**，`journal: off` 對該頻道什麼都不記。見 [`audit`](#audit--message-journal)。

**傳入媒體**依頻道透過 `media:` 段選擇性開啟 —— 一組 MIME 樣式允許清單（`image/*`、`application/pdf`、`audio/*` 等）加 `max_bytes`（預設 8 MiB）。Telegram 下載檔案（getFile），webhook 接受 base64 `attachments`；位元組落到內容定址的 blob 儲存區裡（預設本機檔案系統，或經頂層 `media:` 組態用 S3/MinIO），訊息只帶很小的片段描述元。沒有 media 段時，附件被丟棄，只有文字／說明文字流過。

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

> **破壞性變更**：依頻道的 `listen:` 欄位已移除。仍帶著它的組態會以 `field listen not found in type config.Channel` 載入失敗。請把每個頻道的連接埠移到統一的 `gateway.listen`，給每個 HTTP 頻道一個以 `/` 結尾的 `path`，並為 telegram 的 webhook/auto 模式設定 `gateway.public_url`。見 README 的遷移清單。

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

伺服器宣告 `command`（作為子行程衍生，經 stdio 通訊）**或** `url`（一個 Streamable HTTP 端點，直接撥號）二選一。兩者都在啟動時連線一次，連不上的伺服器會被跳過並給出警告，而不是讓執行停下來。HTTP 傳輸正是讓整合能完全住在這個儲存庫之外的原因 —— 一個獨立部署、獨立版本化的服務，執行環境藉此支援自有或授權的連接器而無須隨附它們。`token` 只作用於 HTTP 傳輸，每次請求都作為 Bearer 憑證送出；不需要的端點留空即可。

連線時發現的工具以 `mcp:<server>/<tool>` 進入登錄表。伺服器中斷後，呼叫回傳結構化的 `unavailable`。

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

設定 `builtin:web_search` 工具背後的引擎，可依呼叫經工具的 `engine` 參數選擇（`tools.run("builtin:web_search", {query=..., engine="doubao"})`）；省略 `engine` 就用 `search.default`。沒有設定任何引擎時，工具如實回報不可用（`{ok:false, unavailable:true}`）。依引擎的 `base_url` 與 `timeout` 是選用覆寫。結果正規化為 `{title, url, site, summary, snippet, content, published, score}`；`score` 是引擎原生的（doubao 是 0..1 的相關度，stackoverflow 是原始贊同數，github 是星標數；youtube 沒有）。doubao、ollama 與 youtube（以及帶檢索依據的 google\_search/x\_search）需要 `api_key`；stackoverflow 與 github 不需要（`site` 欄位可選任意 StackExchange 站點，例如 `superuser` 或 `serverfault`）。密鑰缺失或解析不出來會**跳過該引擎並給出啟動警告**，而不是讓啟動失敗；如果被跳過的是 `search.default`，預設值退到唯一還活著的引擎（或沒有）。github 搜尋儲存庫，查詢裡支援 GitHub 的完整限定符語法（`language:go stars:>100 pushed:>2024-01-01`）。youtube 經 YouTube Data API v3 搜尋影片（site = 頻道標題，content = 描述，`LIVE:` 前綴的片段標記直播）；共用的 `time_range` 對應到 `publishedAfter`/`publishedBefore` —— 注意 search.list 每次呼叫消耗 100 個配額單位（預設專案配額下約 100 次搜尋／天），配額用完會如實報 `quotaExceeded` 錯誤。

### `apis`

```yaml
apis:
  hklii:
    dir: ./examples/apis/hklii       # 包含 manifest.yaml 的套件目錄
  npc:
    dir: ./examples/apis/npc         # Wasm 外部 API 套件
  billing:                           # 或不設 dir 直接內嵌宣告
    runtime: script                  # script (Goja TS/JS) | wasm (wazero)
    entry: ./apis/billing.ts
    timeout: 15s
    capabilities:
      http:
        allowed_hosts: ["api.stripe.com"]
        credentials: ["stripe"]
    actions:
      charge:
        description: "Charge a customer"
        expose_as_tool: true         # 註冊為工具 api:billing.charge（需 apis 能力）
```

將外部 API 套件註冊到執行環境的隔離擴充登錄表（在 `apis` 能力下於迴圈透過 `api.*` 呼叫）。每個套件每次呼叫都會在全新 Goja VM 中執行（`runtime: script`，支援 TypeScript/JS），或在 `wazero` 沙盒內的已編譯 `.wasm` 模組中執行（`runtime: wasm`，受 `timeout` 與 `memory_mb` 限制）；所有傳出 HTTP 均受 `capabilities.http.allowed_hosts` 與主機端注入的 `capabilities.http.credentials` 約束。

指定 `dir` 會載入 `<dir>/manifest.yaml`（並在套件目錄內解析 `entry`），或直接內嵌宣告 `runtime`、`entry` 與 `actions`。設為 `expose_as_tool: true` 的動作也會自動註冊為模型工具 `api:<pkg>.<action>`。取代原內建法律檢索工具的兩個外部 API 參考實作（TypeScript/Goja 版 `hklii` 與 Go/Wasm 版 `npc`）位於 `examples/apis/hklii` 與 `examples/apis/npc`。

### `browser`

```
browser:
  account_id: ${CLOUDFLARE_ACCOUNT_ID}   # Cloudflare 帳戶 ID（不是密鑰）
  api_token: ${CLOUDFLARE_API_TOKEN}     # 需要 "Browser Rendering - Edit" 權限
  base_url: https://api.cloudflare.com/client/v4   # 選填的端點覆寫
  timeout: 60s                           # 每個請求的時間上限（預設 60s）
```

設定 `builtin:browser`，它透過 Cloudflare **Browser Run**（原 Browser Rendering）的 Quick Actions 讀取頁面 —— 一個真正的無頭瀏覽器，所以 JavaScript 渲染的內容在那裡是存在的，而裸 HTTP 抓取只能拿到空殼。一個工具，依呼叫的 `action` 區分：`markdown` 回傳 Markdown 化的頁面，`content` 回傳完全渲染的 HTML，`links` 回傳所有連結，`scrape` 回傳符合 `selectors`（CSS）的元素，`json` 回傳由 `prompt`/`schema` 描述的結構化資料，`accessibility_tree` 回傳無障礙樹。`url` 或 `html` 任選其一指定頁面；`wait_until`（`load`|`domcontentloaded`|`networkidle0`|`networkidle2`）與 `wait_for_selector` 控制頁面何時算載入完成 —— `networkidle` 那幾個值正是單頁應用程式能正常工作的關鍵。`json` 動作在 Cloudflare 側跑 AI 模型，依 Workers AI 用量計費。

長結果有界：`max_chars`（預設 20000，`0` 表示不限）限制某個動作回傳的文字，結果總是回報真實長度 `chars`，被截斷時置 `truncated`，模型因此能分辨短頁面與被裁剪的頁面；`links` 同樣限制 500 條。沒設定帳號時工具如實回報不可用（`{ok:false, unavailable:true}`），而解析不出來的 `api_token` 會**跳過該工具並給出啟動警告**，而不是讓啟動失敗 —— 和其他選用元件遵循同一原則。只設 `account_id` 與 `api_token` 其中一個而不設另一個是啟動錯誤，因為 `account_id` 不是密鑰，配了一半的段是值得立刻抓出來的錯誤。

有三個 Quick Actions 刻意不公開：`screenshot` 與 `pdf` 回傳原始位元組，而它們目前還沒有正確的去處（工具結果是進模型上下文的 JSON，而且整塊進去，下游沒有任何地方會截斷它）；`crawl` 是非同步的，回傳一個必須輪詢的工作 ID。

### `net` —— 傳出連線原則

```
net:
  http:
    allow_private: false     # 預設；true 會停用位址防護
```

用一個設定管轄**兩條**傳出 HTTP 路徑 —— `builtin:fetch` 工具與指令碼的 `http.request` 操作 —— 所以部署不會出現一條有防護、另一條沒有的情況。

防護會拒絕連向回送、RFC 1918、電信級 NAT、連結本地（包括 `169.254.169.254` 的雲端中繼資料端點）、唯一本地、群播，以及保留／特殊用途網段。它裝在**撥號器**裡，而不是解析 URL 時檢查，這個位置正是關鍵：檢查在 DNS 解析之後、連線建立之前執行，所以它看到的是實際要撥的位址。解析時檢查會被 DNS 重綁定擊穿 —— 名字被判定一次，等連線建起來時已經解析到別處了 —— 而每個重新導向跳轉都各自建立連線，所以每一跳都天然被檢查到。

兩個值得知道的細節。帶 zone 的位址（`http://[fc00::1%25en0]/`）會最先被直接拒絕：Go 的前綴匹配匹配不到帶 zone 的位址，若不明確拒絕，這種位址會從每條網段規則底下漏過去，落到唯一本地主機上而防護毫不知情。而 v4 映射的 v6 位址（`::ffff:10.0.0.1`）會在檢查前被還原，因為它和它的 v4 形式指向同一台主機。

**取捨：** 有防護的傳輸層有意忽略 `HTTP_PROXY` / `HTTPS_PROXY`。在出口代理後面，撥號器連的是*代理*，防護就會去判斷代理的位址，目標根本沒被檢查過。只經代理出網的部署會發現這些路徑不走代理。另外注意 `198.18.0.0/15` 作為基準測試網段被封，而它正是 Clash/mihomo 與 sing-box 在 TUN 模式下的預設 fake-IP 池 —— 在跑了其中任何一個的開發機上，每個經代理的網域都會解析進去並被拒絕。

`allow_private: true` 會徹底關掉防護，並在啟動時發一條點名風險的 WARN，所以它不可能被悄悄打開。它是給那些確實要調內部服務的部署準備的；預設關閉，因為防護是提示詞注入的代理與主機自身網路之間唯一的東西。防護只涵蓋這兩條由代理控制的路徑 —— 模型、搜尋與瀏覽器驅動用的是管理員設定的 `base_url`，刻意不涵蓋，好讓內部鏡像源繼續工作。

### `media` —— blob 儲存後端

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

選擇集中式媒體平面，供從頻道注入的位元組、LLM 輸出（`reply.media`）、MCP 工具與指令碼邊界使用。不管後端是什麼，物件都是內容定址的（`media:<sha256>` 控制代碼），所以切換後端時迴圈與稽核日誌的形狀不變；只有控制代碼過指令碼橋，傳出匯（模型與頻道）再把原始位元組或簽章 URL 落地，並帶自動圖片轉碼與衍生變體快取。存取依引擎蓋章的作用域（`user:<uuid>` 或 `agent:<name>`）經執行環境儲存區裡的授權記錄執行，同時保留內容去重；刪除使用者會清掉他的全部授權。`s3` 後端只說原生 SigV4（不用 SDK）：相同位元組的重複寫入在 PUT 前用 HEAD 去重，密鑰絕不會出現在任何錯誤裡。開 `temp_url.enabled: true`（需要 `gateway.public_url`）時，執行環境在 `temp_url.path` 下提供短時效的 HMAC-SHA256 簽章 URL，讓 QQ 這類只吃 URL 的頻道在本機 `fs` 儲存區上也能工作，不必上 S3。

### `files` —— 使用者範圍的檔案儲存區

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

支撐 `files.*` 指令碼操作（能力 `files`）：帶引擎原生快照版本管理的使用者範圍專案樹，外加依工作階段的暫存檔。預設情況下，`files` 沒有明確設定 `backend`、`dir` 或 `s3` 段時，它會共用 `media` 的 blob 儲存區，一個統一的每小時標記清掃 GC 會保護 `files_meta` 參照與作用中媒體授權的聯集，超過 `gc_grace` 才回收；設定了 `files.backend`、`files.dir` 或 `files.s3` 則保留一個專用的獨立 blob 儲存區，同時仍讓 `media` 作為回退從它讀控制代碼。

檔案位元組和媒體一樣在 blob 儲存區裡內容定址；快照中繼資料在執行環境儲存區的 `files_meta` 表裡（原生 sqlite，可直接檢視）。S3 相容端點需要真正的 SigV4 金鑰對 —— Bearer 權杖說不了 S3。解析不出來的憑證對會在啟動時停用儲存區並給出警告（之後操作如實失敗），絕不導致啟動失敗。

### `audit` —— 訊息日誌

```
audit:
  enabled: true               # 預設開啟
  retention_days: 90          # 清除期限；0 = 永久保留
```

核心把每則訊息記進執行環境 SQLite 儲存區的 `message_journal` 表：傳入在路由器處（蓋上穩定的訊息 ID 與 `routed`/`dropped_queue` 狀態），傳出在每個工作階段的傳出處（`delivered` / `failed` / `blocked_safety`，並關聯到它所回覆的傳入 ID）。列裡帶頻道、聊天、傳送者、代理、工作階段、文字、附件描述元、文字與附件大小，以及來源資訊。頻道的 `journal:` 設定會收窄它：`metadata` 保留列與大小但不含文字和附件，`off` 對該頻道不寫任何列。這是一項信任邊界特性：迴圈與頻道既繞不過也偽造不了它 —— 媒體只按控制代碼記錄，稽核鏈因此保持精簡。清理在啟動時和每天依 `retention_days` 執行。

### `usage` —— Token 帳冊

```
usage:
  events: false               # 逐次呼叫的明細資料列（預設關閉；每日彙總一律寫入）
  retention_days: 30          # 帳本的清除期限；0 = 永久保留
  default_tokens_per_day: 0   # 每位使用者的每日上限；0 = 無限制。使用者資料本身的上限會覆寫它
```

每日彙總（`usage_daily`）餵養每一次配額與代理預算決策，並留在執行環境儲存區裡。設定了日誌平面時，逐次呼叫事件寫入日誌平面。`GET /admin/api/usage` 與 Studio 的用量畫面讀的是這份彙總。

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

覆寫在啟動時一次性地烤進登錄表，在所有工具都註冊完（內建、shell、MCP、APIs）之後、任何代理集合公開之前。`params` 條目點名一個 schema 沒有宣告的參數時會被跳過並給出警告，絕不報錯。原則欄位依工具生效；`permission: forbidden` 把工具從每個公開集合裡移除 —— 也是其中唯一真正生效的一個。`cost_level`、`user_visible` 與 `autonomous` 會被接受、複製到工具上，然後沒有任何地方讀取它們；`tools.policy.write` 同樣只解析不使用。設定了其中任何一個時，引擎會在啟動時警告。

**觸達指令碼宣告的工具**：覆寫也可以點名迴圈用 `tool.def` 宣告的工具 —— 見下文。那些條目不在啟動時烤入（程式碼區塊載入之前工具並不存在）；它們在迴圈建構清單時解析，對著活躍的提示詞登錄表，所以編輯一個基於 `file:` 的提示詞，會在下一次 `tools.list()` 時生效。拼錯的*提示詞鍵*對兩類工具都仍是啟動錯誤。一個既沒註冊、也沒被任何已載入迴圈宣告的工具名，無法在啟動時抓到，所以啟動時會記一列「保留給指令碼宣告」（列出這些名字，讓拼錯在任何代理跑起來之前就可見），之後在某個迴圈首次宣告工具卻沒認領它時，依名字警告一次。

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
    ssh: prod_box                       # 選填
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

**能力上限**：`plugins.allow_capabilities`（全域）⊇ `agents.<name>.capabilities`（依代理）⊇ 衍生時授予（子工作階段）。你無法委派自己沒有的能力。代理省略 `capabilities` 時得到預設值 `[llm.chat, memory, tools, agent.send, net.http, users]` —— 注意 `net.mail`、`shell.exec`、`net.ssh` 與 `session.state` *不是*預設值，必須明確宣告。

**能力在執行時強制執行**，而不只是在啟動時驗證。每種能力閘門控管其後的指令碼操作：`llm.chat` → `llm.*`，`memory` → `store.*`，`tools` → `tools.*`，`apis` → `api.*`，`shell.exec` → `shell.*`，`net.ssh` → `ssh.*`，`net.http` → `http.request` 與 `os.env`，`net.mail` → `mail.*`，`files` → `files.*`。省略了某個能力的代理會被拒絕，訊息點名操作、代理與缺少的能力 —— 操作仍然可達，只是明確失敗，而不是消失成 undefined 屬性錯誤。`agent.send`、`agent.request` 與 `agent.spawn` 一直就是這樣強制的；其餘現在也對齊了。`runtime.*` 與 `credential.get` 刻意不受閘門控管 —— 前者是每個代理都需要的迴圈機制，後者已經由代理自己的憑證清單劃定範圍。啟動警告會逐條列出從每個代理與設定檔收回了什麼。只有為了讓一個依賴「每個代理都有每個操作」的組態繼續跑，才把 `plugins.enforce_capabilities: false`（預設 `true`）。

**`builtin:` 是工具命名空間。** 以前有四種詞彙都用它來拼 —— 迴圈／外掛名、工具名、記憶供應商名與內建記憶設定檔 —— 而字串本身說不出一個名字屬於哪一種。現在各自有自己的寫法，前綴的含義因此變得唯一：

- **工具**保留它：`builtin:web_search`、`builtin:fetch`、`builtin:browser`；
- **迴圈與外掛**是 `plugin:<name>`：`plugin:per_chat`、`plugin:recency`、`plugin:semantic`。這也徹底解決了「迴圈還是路徑」的問題，因為路徑永遠不會以 `plugin:` 開頭 —— 而在以前，一個名為 `per_chat` 的檔案是無從觸達的；
- **記憶供應商**是裸名：`sqlite`、`qdrant`、`redisvector` —— `provider:` 鍵本身已經說明了這是什麼名字；
- **內建記憶設定檔**是 `conversational`，這個名字現在被保留：`profiles.memory` 裡同名條目是啟動錯誤，而不是一個永遠跑不起來的設定檔。

舊寫法會依名字被識別，而不是報成未知，所以錯誤會告訴你該寫什麼：`loop:` 寫 `builtin:per_chat` 會得到 `loops are spelled plugin: now`，供應商寫 `builtin:sqlite` 會得到 `providers are unprefixed now`。另兩種錯誤也會報 —— `loop:` 裡放了*工具*名會讓啟動失敗，並提示它該待在哪個欄位；`skills:` 裡放迴圈名會給出警告，因為那一種否則就是靜默的（匹配不到任何工具，被丟棄，代理看起來就沒有工具）。完全不匹配任何已註冊工具的 `skills:` 條目*不會*被報，因為迴圈完全可以列出它自己用 `tool.def` 宣告的工具。

**設定檔 extras 與憑證**：`extras` 是部署特有的、唯讀的設定檔資料，由 `agent.config()` 呈現給迴圈（`goal` 塊會提升為頂層 `goal` 鍵，其餘落在 `extras` 下）。*恰好*是密鑰參照的值 —— `${VAR}`、`${VAR:-default}` 或 `cred:<service>` —— 在這個面上永遠不會被解析：configdir 路徑下它們保持原樣，渲染成不透明標記（`{env="VAR"}` / `{cred="service"}`），所以解析後的密鑰到不了迴圈（也到不了衍生的子工作階段）；只是嵌入參照的字串（`prefix-${VAR}`）是普通字面值，照常展開。`credentials` 是 `credential.get()` 的允許清單：代理只能取它在這裡點名的已儲存密鑰；空或缺失 = 無權限。兩個欄位對衍生設定檔同樣生效，並到達每個子工作階段。

**常駐代理**：工作階段通常按需在第一則訊息時衍生。`persistent: true` 的代理在執行環境啟動後（頻道註冊之後）會收到一則合成的 `{ type = "boot", from = "system:supervisor" }` 訊息，所以 cron 排程器與佇列消費者無需外部流量就能起來。

**租戶範圍跟隨委派**：代理跳轉（`agent.send` / `agent.request` / `agent.reply`）會把傳送輪次的租戶帶在訊息由核心蓋章的來源資訊上，接收方以該租戶身分行事 —— 所以為一群人服務的代理叢集會在每一跳寫這個人的檔案、記憶與 Shell 範圍，而不是一個全叢集共享的範圍。這個戳取自傳送方由引擎賦予的身分，絕不取自 payload，所以迴圈無法點名另一個租戶（`payload.user_uuid` 無論誰寫的都被忽略）。鏈條只能衰減：代理拿到的是給它的那個租戶，或者沒有。`inherit_user: false` 讓代理退出跨跳繼承 —— 它在共用的 `service` 層裡無租戶執行 —— 而一個人直接跟它說話時，仍會在自己的範圍下觸達它。衍生設定檔上同樣可用。

**預算**：`tokens_per_day` 用 reserve/commit/release 把 LLM 呼叫包在一個 Token 集區外面。預設每天重置；設 `window`（例如 `24h`、`168h`）則依滾動視窗記帳，用量隨提交逐步超出視窗而持續釋放 —— 沒有午夜懸崖。會超預算的呼叫被拒絕（計入 `agentflow_budget_denied`）。

### `prompts` —— 部署層級提示詞登錄表

```
prompts:
  assistant_system:
    file: ./prompts/assistant.md       # 相對於 configdir 解析
  distiller:
    inline: "You are a memory distiller..."   # 字面值（text: 是別名）
  kb_header:
    inline: "[Knowledge base — recalled facts with provenance]"
```

一個放系統提示詞、工具提示詞、注入提示詞與蒸餾文字的地方，省得在指令碼裡硬編碼或在多個檔案裡重複。每個條目只能寫 `file:`、`inline:` 或 `text:`（`inline:` 的別名）其中之一；讀不到的 `file:` 會讓啟動失敗，設定檔或工具覆寫點名了未定義的鍵也會讓啟動失敗 —— 缺失的提示詞絕不會降級成靜默的空字串。提示詞檔案在載入時讀取（變更時重讀），相對 configdir 解析（單一檔案組態相對組態檔本身解析）。解析後的文字以唯讀方式呈現給每個迴圈的 `agent.config().prompts`（沒宣告提示詞時為空物件），依同樣的名字索引。

**參照提示詞**：代理或衍生設定檔上的 `instructions` 接受檔案路徑（和以前一樣）或 `{prompt: <key>}`，後者讓登錄表文字成為該代理的系統提示詞。工具覆寫接受同樣的形式作為描述：`description: {prompt: <key>}`，與現有的字面值字串並存。基於 `file:` 的條目是*即時的*：編輯檔案會把文字重新發佈到每個迴圈的 `agent.config().prompts`，並原地刷新任何源自該鍵的代理的系統提示詞，無需重新啟動工作階段（讀不到的檔案會警告並保留已發佈的文字）。`prompts:` 段本身不是即時的 —— 新增鍵、把某個鍵改指向別的檔案，或在 `inline:` 與 `file:` 之間切換，仍需重新啟動。

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
    - net.ssh
    - net.http
    - net.mail
    - channel.push
    - agent.spawn
    - agent.send
    - agent.request
  enforce_capabilities: true    # 預設；false 會恢復強制執行之前的行為
```

`plugins.dir` 也會**依名字覆寫內建實作**：檔案 `<dir>/<name>.ts`（或 `.js`，例如 `./plugins/recency.ts`）會在所有地方取代同名內建實作 —— 迴圈／路由參照（`loop: plugin:recency`，包括熱重載監聽路徑）以及載入每個工作階段的支援片段。覆寫檔案受監看：被覆寫的支援片段在每次工作階段啟動時重讀（第一次以及每次重載重新啟動），編輯它會重新啟動所有作用中工作階段，所以無需重新啟動行程即可生效。未被覆寫的片段是內嵌的，沒有檔案可改。

### `triggers` —— 排程與事件工作

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

**`every:` 與 `cron:` 觸發器由引擎執行**，不是由代理執行：到期時執行環境會把一則普通訊息入列到目標信箱 ——

```
{ id = "cron:<trigger-name>:<unix>", type = "cron", from = "system:scheduler",
  payload = { ... }, ts = ..., provenance = { kind = "scheduler", principal = "trigger:<name>" } }
```

—— 進入工作階段 `<target>|<trigger-name>`。沒有任何代理為此持有能力，也沒有迴圈需要計時器：排程是引擎基礎設施，所以部署可以刪掉它以前養著的使用者態排程代理。傳遞總會記入日誌並計數（`agentflow_trigger_fires`），並以 `info` 等級觸發（`debug` 帶下一次計算的觸發時刻）。

- `every:` 接受 `ms`、`s`、`m`、`h` 或 `d` 單位的時長，允許複合與小數（`30s`、`15m`、`6h`、`1d`、`1.5h`、`2d12h`）。首次觸發在啟動後一個間隔（或 `run_on_boot` 時立即）；之後以那一次為錨，所以不會漂移。最短間隔 100ms。
- `cron:` 用經典的 5 欄位（`minute hour day-of-month month day-of-week`），支援 `*`、`N`、`N-M`、`*/N`、`N-M/N` 與逗號分隔清單。星期接受 `0`–`7`（`0` 與 `7` 都是週日）。當 day-of-month *和* day-of-week 都受限時，符合*任一*即觸發（經典規則）。不支援名字（`mon`、`jan`），也沒有秒欄位。
- `cron:` 欄位依 `runtime.timezone_offset_hours` 匹配（固定 UTC 偏移，預設 0 = UTC）。它是偏移不是具名時區，所以排程不會因別的區域的日光節約時間而漂移 —— `30 9 * * 1-5` 配 `8` 就是全年每天 UTC+8 的 09:30。
- `target.profile` 點名一個已設定的代理或一個衍生設定檔。衍生設定檔解析為 `spawn:<profile>` 位址 —— 一個用該設定檔的迴圈、模型與授權執行的頂層工作階段（沒有上層，也沒有對上層的能力衰減）。
- `event:` 觸發器**不**參與排程：路由器與迴圈自己匹配它們。`runtime.triggers()` 仍回傳整個合併清單，形狀不變，所以讀它的迴圈照常工作。
- 排不上期的觸發器 —— 表示式格式錯誤、間隔低於下限、或 `target.profile` 既不是代理也不是衍生設定檔 —— 會**記日誌並跳過**，其餘的照常執行。一個組態筆誤只讓你丟一個觸發器和一列警告，絕不丟一次啟動。
