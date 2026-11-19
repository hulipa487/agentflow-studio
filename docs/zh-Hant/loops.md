# 撰寫 Luau 迴圈

以 Luau 撰寫 AgentFlow 工作階段迴圈的實用指南：執行模型、完整的 Lua API、實作模式，以及容易犯的錯誤。

這些模式的可執行版本位於 [`examples/loops/`](https://github.com/hulipa487/agentflow/tree/main/examples/loops)。

內建的 Luau 型別定義位於 [luau/af.d.luau](luau/af.d.luau)；可從本頁開啟，在編輯器中檢視 API 介面。

## 目錄

1. [最小的迴圈](#1-the-smallest-loop)
2. [執行模型](#2-execution-model)
3. [訊息](#3-messages)
4. [狀態與持久性](#4-state-and-persistence)
5. [能力](#5-capabilities)
6. [API 參考](#6-api-reference)
7. [模式](#7-patterns)
8. [韌性](#8-resilience)
9. [提示詞組裝與 Token 預算](#9-prompt-assembly-and-token-budgets)
10. [測試與偵錯](#10-testing-and-debugging)
11. [常見陷阱](#11-gotchas)
12. [速查表](#12-quick-reference-table)
13. [支援程式碼區塊、遮蔽與路由處理常式](#13-support-chunks-shadows-and-route-handlers)

---

## 1. 最小的迴圈

工作階段迴圈是一個定義了全域函式 `loop()` 的 Luau 程式碼區塊。執行環境為每個工作階段載入一個全新的 Lua 狀態並呼叫 `loop()`；這個函式會在該工作階段的整個生命週期內持續執行。

```lua
-- af-prelude-version: 1
function loop()
  while true do
    local msg = session.inbox()      -- 停駐直到下一則訊息到來
    session.send("echo: " .. (msg.text or ""))
  end
end
```

讓代理指向這個檔案：

```yaml
agents:
  echo:
    loop: ./examples/loops/echo.lua
```

啟動執行環境並傳送一則訊息（範例組態有一個 Webhook 頻道）：

```bash
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/local/ \
     -H 'Content-Type: application/json' \
     -d '{"from":"alice","text":"hello"}'
# echo: hello
```

### 唯一必要的一行

每個程式碼區塊——迴圈、支援程式碼區塊或路由處理常式——都必須在開頭的註解區塊中宣告 prelude API 版本：

```lua
-- af-prelude-version: 1
```

沒有這個指示詞或版本不同的程式碼區塊，會在**執行之前**就被拒絕。沒有相容過渡期：prelude 與使用它的 Lua 一起演進。這個版本在原始碼中是 `vm.PreludeVersion`，並由 `GET /admin/api/studio/catalog` 顯示；目前的值是 **1**。

### 執行前先驗證

Studio 應用程式（`/admin/api/studio/*`）驗證迴圈的方式是檢查指示詞並編譯程式碼區塊，然後即時套用。`POST
/admin/api/studio/agents/validate` 是同樣的檢查，但沒有副作用。Go 也提供單純的編譯檢查（`vm.CompileCheck`）。

編譯檢查只能證明語法，不能證明行為：參照了不存在的全域變數、呼叫了停用的能力，或在執行時做錯事的迴圈，會在第一輪失敗，並出現在 Studio 的追蹤中。

---

## 2. 執行模型

每個工作階段都是一個 **actor**：

- 一個 goroutine、一個信箱、一個 Luau 狀態、一個迴圈協程；
- 訊息依序逐一處理；
- 迴圈永遠不會與自己並行執行。

唯一提供給 Lua 的原生基本操作是 `__af_op(json_request)`。它會讓出協程；Go 執行該請求（便宜的操作直接內嵌執行，會阻塞的操作則交給工作者集區），然後以 `(json_response, ok)` 恢復協程。prelude 把它包裝成 `op(req)`，並在 `ok` 為 false 時把回應當作 Lua 錯誤拋出。由於每個阻塞呼叫都會讓出執行，停駐中的工作階段不佔用任何作業系統執行緒。

```lua
-- 概念上，prelude 替你做的事：
local resp, ok = __af_op(json.encode(req))
if not ok then error(resp, 0) end
return json.decode(resp)
```

### 停駐點與重新啟動

`session.inbox()` 就是停駐點。協程停駐期間，工作階段處於閒置狀態，成本很低。不需要用 `time.sleep` 輪詢迴圈——永遠不要寫 `while true do ... time.sleep(0.1) ... end` 來找工作；信箱就是工作佇列。

`loop()` 周圍會發生的事：

| 事件 | 結果 |
|---|---|
| `session.inbox()` | 停駐直到訊息到來 |
| `session.send` / `push` / `llm.chat` / `tools.run` / `store.*` / `shell.*` / `http.*` / `mail.*` / `agent.*` | 讓出執行；帶著結果恢復 |
| `log.*`、`agent.info`、`agent.config`、`json.*` | 內嵌執行；不讓出 |
| `loop()` 傳回 | 迴圈會立即**重新啟動**（迴圈結束不等於工作階段結束） |
| Lua 錯誤逃出 `loop()` | 迴圈當機，約 1 秒後重新啟動，無限重複 |
| `session.exit()` | 工作階段結束；協程永遠不再恢復；計時器與請求會被回收 |

有兩個值得牢記的結果：

- **要結束工作階段，請呼叫 `session.exit()`。** 從 `loop()` 傳回只會讓它重新啟動。
- **把可能失敗的工作包在 `pcall` 中。** 未攔截的供應商錯誤會造成當機迴圈，而不只是單一輪失敗。

**Lua 區域變數是快取，不是儲存空間。** 任何一輪都可能從全新的 Lua 狀態開始：熱重新載入、容錯移轉、租約遺失或閒置卸載（`runtime.sessions.idle_unload`，預設 30m）都會讓迴圈從頭重新開始。正確的迴圈在輪次之間需要記住的任何東西，都應放在 `session.state.*`、記憶或儲存區中——今天的重新啟動後還在的區域變數，下次不一定還在。把游標放在區域變數中的迴圈，只在對話閒置未滿 30 分鐘之前是正確的。

### 熱重新載入

編輯迴圈檔案後，工作階段的協程會在下一個停駐點從全新的 Lua 狀態重新開始。指令與以檔案為來源的提示詞會就地重新載入；路由處理常式會在兩則訊息之間重建；被遮蔽的支援程式碼區塊會重新啟動所有執行中的工作階段。無法編譯的新版本會讓執行中的版本保持不變。每個工作階段就是一段對話：一次編輯可能重新啟動所有執行個體上該工作階段的副本，而重新啟動會遺失所有不在 `session.state`、記憶、儲存區或檔案中的 Lua 狀態。

### 一次一個操作

讓操作呼叫留在迴圈協程上。不要在巢狀的 `coroutine.create` 協程中呼叫 `session.inbox`、`llm.chat`、`store.*` 或任何其他會讓出執行的操作：actor 驅動的是迴圈協程，橋接層預期讓出執行來自那裡。一般的 Lua 協程用於純運算沒有問題；任何跨越橋接層的操作都應放在主迴圈中。

---

## 3. 訊息

`session.inbox()` 會傳回類似這樣的表格：

```lua
{
  id = "msg-...",                 -- 穩定的訊息 ID（稽核依它關聯）
  type = "user",                  -- user | agent | timer | cron | boot | system
  from = "webhook:alice",         -- 由核心標記的寄件者
  text = "hello",                 -- 頻道文字；可能為空
  channel = "webhook",            -- 來源頻道（timer/cron/boot 沒有）
  reply_to = "...",               -- 回覆時使用的頻道專屬收件者
  ts = 1730000000,                -- Unix 秒數
  payload = { ... },              -- 操作專屬資料（見下文）
  attachments = { ... },          -- 媒體部分描述元（若有）
  provenance = {                  -- 由核心標記；迴圈永遠無法控制
    kind = "channel",             -- channel | agent | timer | scheduler | system | studio
                                  -- timer = session.scheduler.* 的觸發，範圍限於
                                  -- 排定它的租戶；scheduler = 引擎觸發器，
                                  -- 也是唯一的維護用計時器來源
    principal = "webhook:alice",
    parent = "...",               -- 上層工作階段 ID（代理訊息）
    request_id = "...",           -- 關聯 ID（agent.request / agent.reply）
    user_uuid = "u_...",          -- 這一輪代表的租戶，或 nil
    membership_uuid = "...",      -- 群組情境（若已驗證）
  },
}
```

### 訊息類型

| `type` | 來源 | `payload` |
|---|---|---|
| `user` | 頻道或 Studio 遊樂場 | 依頻道而定（`chat_id`、`text`…） |
| `agent` | `agent.send` / `agent.request` / `agent.spawn` 流量 | 寄件者的 payload 表格 |
| `timer` | `scheduler.every/after/cron` | `{ timer_id = "..." }` |
| `shell.result` | `shell.start` 工作結束 | 該工作：`{ id, state, exit_code, stdout, stderr, changed, ... }` |
| `cron` | 引擎的 `every:` / `cron:` 觸發器 | 原封不動複製觸發器的 `payload:` |
| `boot` | 監督程式，針對 `persistent: true` 代理 | 無 |
| `system` | 引擎生命週期訊息（例如 `agent.died`） | 依事件而定 |

`timer`、`shell.result`、`cron` 與 `boot` 輪次**沒有頻道**。`session.send` 只會回覆目前作用中的傳入訊息，否則會以 `no active message to reply
to` 失敗；有實際收件者時，請使用 `session.push(channel, reply_to, text)`（能力 `channel.push`）或 `session.push_user(uuid, text)`，或者把狀態記錄下來留給下一個使用者輪次。

### 身分來自來源資訊

從 `msg.provenance` 讀取身分，永遠不要從 `msg.payload` 讀取：

- `user_uuid` 是這一輪代表的租戶。沒有任何東西標記訊息時它**不存在**，訊息不屬於任何租戶時它**為空**。
- 當路由器提出了群組、且引擎驗證了寄件者的成員資格時，會設定 `membership_uuid`。
- 委派轉送會帶著寄件者的租戶往下傳；由引擎標記，而 `payload.user_uuid` 不論是誰寫的都會被忽略。

迴圈無法藉由在 payload 中放入 UUID 來擴大租戶範圍。

### 附件

頻道接收了媒體時，訊息會帶有小型描述元：

```lua
{
  type = "image", mime = "image/png",
  handle = "media:<sha256>", name = "photo.png",
}
```

把它們原封不動轉交給 `llm.chat`（`parts = msg.attachments`）或 `session.send`（`{ attachments = msg.attachments }`）。原始位元組永遠不會進入 Lua；執行環境會在供應商／頻道邊界解析控制代碼。

### 傳送

```lua
session.send(text)                              -- 回覆目前作用中的訊息
session.send(text, { attachments = parts })     -- 附帶媒體
session.push(channel, reply_to, text)           -- 主動傳出；能力 channel.push
session.push_user(uuid, text)                   -- 依身分 uuid；能力 channel.push
```

`session.send` 與 `session.push` 會經過由核心擁有的安全傳出過濾鏈與稽核訊息日誌；迴圈無法略過其中任何一個。`session.push_user` 需要身分層，以及不帶前綴的 UUID（prelude 會加上 `user:`）。

---

## 4. 狀態與持久性

狀態可以存放在四個地方。挑選能撐過你需要撐過之事件的最小一個。

| 機制 | 範圍 | 熱重新載入後保留 | 重新啟動後保留 | 多執行個體 | 用途 |
|---|---|---|---|---|---|
| Lua 區域變數 | 這個協程 | **否** | 否 | 否 | 單一輪次內的暫存 |
| `session.state` | 一個工作階段 | 是 | 是（執行環境儲存區） | 是，跟隨工作階段 | 游標、計數器、計時器 ID |
| 記憶設定檔（`memory.*`） | 一個代理（依使用者／鍵範圍） | 是 | 是 | 是 | 對話歷史、事實 |
| `store.*` | 具名資料表，可設範圍 | 是 | 是 | 是 | 原始鍵值或向量資料 |
| `files.*` | 一個使用者／代理的專案樹 | 是 | 是 | 是 | 文件與快照 |

### `session.state`

單一工作階段專用的小型持久鍵值儲存區。需要 `session.state` 能力（不在預設集合中）。

```lua
session.state.set(key, value)   -- value 可為任何可 JSON 編碼的 Lua 值
session.state.get(key)          -- 從未設定時為 nil
session.state.delete(key)
session.state.list()            -- { key = value }
```

限制：鍵最長 128 個字元，由字母、數字與 `. _ - :` 組成；值最大 64 KiB；每個工作階段 256 個鍵。這裡適合存放計時器 ID、游標或步驟計數——不適合存放對話歷史。

### 記憶

`memory.write` 會讓記錄經過設定檔的寫入鏈（預設為 `plugin:routing_table`），送進宣告的儲存區。`memory.recall` 會分派給設定檔的召回處理常式（預設為 `plugin:recency`，或 `plugin:semantic`）。

```lua
-- 一筆輪次記錄：預設的寫入鏈會把它送到 "dialogue" 儲存區。
memory.write({
  type = "turn",
  id = msg.id,
  text = msg.text,
  reply = reply.text,
})

-- 一筆事實：送到 "facts" 儲存區。
memory.write({ type = "fact", key = "name:" .. msg.from, value = { text = "..." } })

-- 明確指定目標時會略過路由表。
memory.write({ targets = { { store = "dialogue", key = "custom:" .. msg.id, value = "..." } } })
```

`memory.recall` 傳回記錄的**清單**，每筆為 `{ key, value, meta }`——而不是 `{ records }` 包裝。查詢結構取決於處理常式：

| 處理常式 | 查詢 |
|---|---|
| `plugin:recency`（預設） | `{ store?, kind = "all" \| "prefix" \| "text" \| "time", prefix?, text?, k?, from?, to? }` |
| `plugin:semantic` | `{ text, store?, k? }`（設定檔需要 `embed_model`） |

沒有指定 `store` 時，recency 會讀取 `dialogue`，所以要在 `facts` 上做文字搜尋時，請指名儲存區（`{ kind = "text", store = "facts", text = "...", k = 5 }`）。時間範圍是 `kind = "time"` 加上 RFC3339 格式的 `from` 與 `to`。召回會傳回處理常式傳回的任何內容；recency 與 semantic 都傳回記錄清單。[記憶](memory.md)指南說明了設定檔、儲存區與隔離。

### 原始儲存區

`store.*` 是較低階的資料表 API。它受 `memory` 能力控管，範圍由引擎決定（私有儲存區依使用者為資料列加上鍵，外加服務層與升級前的資料列；綁定記憶池的儲存區則由租戶授權管理）。

```lua
store.put("table", "key", value, { ttl = 3600, vector = { ... } })
store.get("table", "key")                 -- { value, found }
store.query("table", { kind = "all" })    -- { records = { { key, value, meta }, ... } }
store.delete("table", "key")
```

查詢種類：`all`、`prefix`（`prefix = "..."`）、`text`（`text = "...", k = n`）、`vector`（`vector = {...}, k = n`）、`time_range`（`from`、`to` 為 RFC3339）。後端必須支援該種類：宣告了 `requires: ["vector"]`（或 `["text_search"]`）的儲存區，若後端不支援，會在啟動時失敗。把向量傳給不具 `vector` 功能的後端會得到明確的錯誤，絕不會默默傳回空結果。

### 檔案

以使用者為範圍的檔案儲存區（`files` 能力）存放專案樹，並具備引擎原生的快照版本控管。位元組留在引擎端；`put` 接受文字或 base64，`read` 傳回中繼資料與控制代碼。

```lua
files.put("notes", "inbox/1.md", msg.text)
files.list("notes", "inbox/")
files.commit("notes", { ref = "main", message = "note " .. msg.id })
files.checkout("notes", "main")
```

暫存區依工作階段區分，並有 TTL：

```lua
files.scratch.put("page.html", body)
files.scratch.read("page.html")
files.scratch.list()
```

---

## 5. 能力

能力在執行時控管 Lua 可用的介面。被拒絕的操作會拋出清楚的錯誤，指出操作、代理與缺少的能力——而不會變成 `attempt to call a nil value`。

| 能力 | 控管 |
|---|---|
| `llm.chat` | `llm.chat`、`llm.stream`、`llm.embed`、`llm.rerank` |
| `memory` | `memory.*`、`store.*` |
| `tools` | `tools.list`、`tools.run`（宣告的工具遵循可見性規則） |
| `agent.send` | `agent.send`（`agent.reply` 依請求擁有權授權，`agent.list` 以聯絡人為範圍且不受控管） |
| `agent.request` | `agent.request` |
| `agent.spawn` | `agent.spawn` |
| `net.http` | `http.request`、`http.get`、`http.post`、`os.env` |
| `net.mail` | `mail.*` |
| `shell.exec` | `shell.*` |
| `files` | `files.*` |
| `session.state` | `session.state.*` |
| `users` | `user.current`、`user.usage`、`user.has_credential`、`user.get`、`user.list` |
| `channel.push` | `session.push`、`session.push_user` |

代理省略 `capabilities` 時的預設值：`llm.chat`、`memory`、`tools`、`agent.send`、`net.http`、`users`。`net.mail`、`shell.exec`、`files`、`session.state` 與 `channel.push` 須明確宣告。

不受此表控管的有：

- `scheduler.*`——計時器是引擎遞送的訊息，不是權限；
- `runtime.triggers()`——唯讀的部署資料；
- `credential.get(name)`——不受控管，但依代理設有允許清單（`credentials: [...]`）；每次存取都會記錄並計數。

`plugins.allow_capabilities` 是部署層級的上限：代理宣告的能力不能超過它允許的範圍，衍生的子代理取得上層與範本的交集，而 `plugins.enforce_capabilities: false` 會為舊組態恢復「只宣告、不強制」的行為。不要依賴它。

---

## 6. API 參考

以下所有簽章都是 prelude 的。失敗時會拋出 Lua 錯誤；在失敗不應讓迴圈當機的地方使用 `pcall`。

### 6.1 `session`

```lua
session.inbox() -> Message                  -- 停駐
session.send(text, opts?)                   -- 回覆目前作用中的訊息
session.send(text, { attachments = parts })
session.push(channel, reply_to, text, opts?) -- 能力：channel.push
session.push_user(uuid, text)                -- 能力：channel.push；身分層
session.exit()                               -- 結束工作階段（永不恢復）
session.state.set(key, value)                -- 能力：session.state
session.state.get(key) -> value | nil
session.state.delete(key)
session.state.list() -> { key = value }
```

### 6.2 日誌與時間

```lua
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
time.sleep(seconds)       -- 讓出執行；停駐期間工作階段處於閒置
os.time() -> Unix seconds -- UTC；不接受參數
os.date([format,] t?) -> date table | string
os.env(name) -> string    -- 引擎的環境變數；未設定時為 ""
```

`os.date` 預設傳回表格：`{ year, month, day, hour, min, sec, wday,
yday, isdst }`，`wday` 1 = 星期日，全部為 UTC。要取得文字，請傳入 strftime 字串：`os.date("%Y-%m-%d %H:%M:%S", t)`。支援的指示詞：`%Y %y %m %d %e %H %I
%M %S %p %A %a %B %b %j %w %u %Z %%`。沒有本地時區；開頭的 `!` 可接受，但不起作用。相對地，Cron 觸發器依 `runtime.timezone_offset_hours` 比對。

### 6.3 `agent`

```lua
agent.info() -> {
  name, session_id, address, model, instructions,
  history_budget,
  memory = { stores = { [name] = { backend, table, features, locality } },
             recall, embed_model, rerank_model, oversample },
  shell, skills, capabilities,
  agent_model, instructions_append, ...          -- 依使用者的覆寫
}
agent.config() -> {
  name, model, instructions_path | instructions_prompt,
  prompts = { key = "text" },
  goal = { type, success_signal, max_turns, on_goal_met }?,
  extras = { ... }
}
agent.send(address, payload)                    -- 能力：agent.send
agent.request(address, payload, timeout_s?) -> Message  -- 能力：agent.request
agent.reply(request_id, payload)                -- 依請求擁有者授權
agent.list() -> [ { session_id, agent, parent_id } ]   -- 以聯絡人為範圍
agent.spawn(profile, spec?) -> { address, session_id, agent } -- 能力：agent.spawn
```

- `agent.info().instructions` 是套用任何依使用者的層之後的系統提示詞；若迴圈想依覆寫內容分支，`agent_model` 與 `instructions_append` 會公開這些覆寫片段。
- `agent.info().memory.stores[name].locality` 為 `deployment`、`region` 或 `instance`：表示儲存區資料的涵蓋範圍。持久的對話資料（`dialogue`/`facts` 儲存區）應放在 `deployment` 或 `region` 儲存區；`instance` 儲存區會隨行程一起消失，因此只是快取，不是歷史。單一執行個體的部署可以刻意使用它，區域單元則必須在持久儲存區上設定 `locality_ok: true` 才能這樣做。
- `agent.config()` 是以資料形式呈現的部署設定檔。密鑰參照會呈現為不透明的標記 `{env="VAR"}` / `{cred="service"}`；解析後的值永遠不會跨越橋接層。
- 位址：已設定的靜態代理用 `agent:<name>`，執行中的工作階段用 `session:<id>`（衍生的子代理也是——使用 `spawn_result.address`）。`send`/`request` 不接受 `:new`。
- `agent.request` 逾時即取消；逾時單位是**秒**。
- `agent.reply` 必須使用核心標記之來源資訊中的請求 ID：`msg.provenance.request_id`。永遠不要從 `msg.payload` 組出它。
- `agent.spawn` 需要 `profiles.agent` 範本。子代理的能力是上層 ∩ 範本，其聯絡人是上層的聯絡人 ∩ 範本的聯絡人。為了相容性仍接受 `spec`，但這個版本**不會套用**它；請改為設定範本。

### 6.4 `scheduler`

```lua
scheduler.every(interval_s) -> { id }
scheduler.after(delay_s) -> { id }
scheduler.cron(expr) -> { id }
scheduler.cancel(id)
```

計時器會把 `Message{ type = "timer", payload = { timer_id = id } }` 遞送到同一個工作階段。它們存在記憶體中，且以工作階段為範圍：重新啟動會忘記它們，而熱重新載入會重新啟動迴圈**但不會**清除計時器，所以盲目地重新註冊會產生重複的計時器。請先取消記住的 ID（見 [`scheduled.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/scheduled.lua)）。

`scheduler.cron` 只支援子集：`*/N * * * *` 以及像 `30 9 * * *` 這樣的固定形式；日、月與星期必須是 `*`，而且 `*/N` 分鐘不能與固定的小時搭配。需要更複雜的排程時，請使用 `every`/`after`（或引擎觸發器）。

### 6.5 `llm`

```lua
llm.chat(messages, opts?) -> {
  text, thinking, thinking_blocks, usage, tool_calls
}
llm.stream(messages, opts?) -> next_delta, close
llm.embed(inputs, opts?) -> { vectors, usage }
llm.rerank(query, documents, opts?) -> { results = [ { index, score } ] }
```

`messages` 是由 `{ role, content }` 輪次組成的陣列；多模態輪次則用 `{ role, parts = {...} }`。工具歷史在 `role = "tool"` 輪次上使用 `tool_calls`、`tool_call_id` 與 `tool_result`/`content`。在開啟思考的情況下延續 Anthropic 工具迴圈時，請在 assistant 輪次上重播 `thinking_blocks`。

`llm.chat` 的 `opts`：

```lua
{
  model = "default",          -- models: 中的一個項目；"" = 代理預設值
  temperature = 0.7,
  max_tokens = 4096,
  thinking = "off"|"low"|"medium"|"high"|"xhigh"|"max",
  tools = tools.list(),       -- 模型可見的工具規格
  tool_choice = "auto"|"none",
}
```

在供應商有回報時，`usage` 為 `{ input, output, cached, cache_write, reasoning }`。

**串流。** 直接呼叫 `next_delta`；泛型 `for` 會被拒絕，因為迭代器會跨橋接層讓出執行。迭代器在完成時會自行關閉；若提早結束，請呼叫 `close()`。它只傳回文字增量——用量與工具呼叫請使用 `llm.chat`。請見 [`stream.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/stream.lua)。

**嵌入。** `llm.embed` 接受單一字串、字串陣列，或字串與 part 表格（`{type="image", handle=..., url=...}`）混合的陣列；`merged = true` 會把所有內容合併成一個向量（Jina 內容群組）。`llm.rerank` 傳回指向 `documents` 的 0 起算索引。

### 6.6 `memory` 與 `store`

```lua
memory.write(record) -> true
memory.recall(query, opts?) -> [ { key, value, meta } ]

store.put(table, key, value, opts?)   -- opts: { ttl = seconds, vector = {...} }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records = [ { key, value, meta } ] }
store.delete(table, key)
store.scopes(table) -> [ "user:<uuid>", ... ]   -- 僅限維護來源
```

記憶記錄與查詢的結構請見[記憶](#memory)。`store.scopes` 只能從引擎觸發的維護來源（`system` / `scheduler` 輪次）存取；頻道或代理轉送的輪次會被拒絕。夜間彙整程式就是靠它列舉使用者範圍，而不必讀取某個使用者的輪次。

### 6.7 `tools` 與 `tool.def`

```lua
tools.list() -> [ { type = "function", ["function"] = { name, description, parameters } } ]
tools.run(name, args, opts?) -> result
-- opts: { confirmed = true, confirm_id = "..." }
```

在程式碼區塊的最上層、`loop()` 之前，以 Lua 宣告模型可見的工具：

```lua
tool.def({
  name = "lua:shout",
  description = "Uppercase the text and return it.",
  params = {
    type = "object",
    properties = { text = { type = "string" } },
    required = { "text" },
  },
  handler = function(args)
    return { ok = true, shouted = string.upper(args.text or "") }
  end,
})
```

宣告的工具會併入 `tools.list()`，並在 `tools.run` 中分派；處理常式拋出錯誤時會傳回 `{ ok = false, error = ... }`，而不會終止迴圈。可見性完全比照以 Go 註冊的工具，遵循代理的 `skills` 與 `tools.policy.default`——隱藏的宣告工具不會列出，也無法分派。Go 的原則（`forbidden`、`needs_confirm`）不控管迴圈自己宣告的處理常式。

若某個工具（Go 工具，或透過覆寫宣告的工具）設有 `needs_confirm: true`，執行環境會攔截第一次 `tools.run`、詢問使用者，再以 `confirmed = true` 重新呼叫；迴圈只會看到最終結果。等待確認的時間有上限（十分鐘）。

### 6.8 `shell`

```lua
shell.run(command, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out,
                                 truncated, changed, deleted, conflicts, skipped }
shell.start(command, opts?) -> { id, state = "running" }   -- 結果會送到信箱
shell.status(job_id) / shell.cancel(job_id)

-- opts: { project, environment, workdir, env, timeout }   -- timeout 單位為秒
```

每次呼叫是在一個沙箱中、針對檔案儲存區中的一個專案執行一個指令。指令結束時，它在專案中寫入的內容會被儲存；其餘一切都不會保留到下一次呼叫，而下一次呼叫可能在任何執行個體上執行。非零的結束代碼是一種結果；無法執行的工作則會拋出錯誤。`shell.spawn`、`shell.exec`、`shell.write` 與 `shell.destroy` 仍然存在：用於連到遠端機器的 SSH 連線（`provider = "ssh"`），以及作為舊迴圈的相容層。需要 `shell.exec`。完整規則請見 Loop API 參考。

### 6.9 `http` 與 `os`

```lua
http.request(opts) -> {
  ok, status, headers, body,
  truncated?, saved?, save_error?
}
http.get(url, opts?)
http.post(url, body, opts?)

-- opts: {
--   method, url, headers, body, json, query, timeout,
--   auth = { service = "..." },      -- 儲存的憑證，由 Go 解析
--   save_to = "name",                -- 本文也會存入檔案暫存區
-- }
```

任何 HTTP 狀態都是成功的請求結果（`ok` 表示請求已完成；`status` 表示伺服器的回應）。本文上限為 10 MiB（被截斷時會設定 `truncated`）；`save_to` 讓大型本文不進入 Lua，並傳回 `saved = { name, handle, size }`。私有、回送、鏈路本機與保留位址的目的地會在撥號器中被拒絕。需要 `net.http`（它也控管 `os.env`）。

`builtin:fetch` **工具**是另一個功能更豐富的介面：它另外支援 `follow`/`max_redirects`、`user_agent`、`max_bytes` 與 `insecure`（關閉 TLS 驗證，一律記錄並計數）。要讓模型呼叫時使用工具；迴圈本身需要發出請求時使用 `http.request`。

### 6.10 `mail`

```lua
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
-- opts: { host, port = 993, user, mailbox = "INBOX", unseen = false,
--         limit = 10, auth = { service = "..." } }
-- messages: [ { seq, uid, flags, subject, from, to, date, body } ]

mail.smtp_send(opts) -> { ok, sent }
-- opts: { host, port = 465, user, from, to = { ... }, subject,
--         text_body, auth = { service = "..." } }
```

密碼會在呼叫時從憑證儲存區解析，永遠不會跨越橋接層。`mail.*` 不在 HTTP 私有位址防護的涵蓋範圍內。需要 `net.mail`。

### 6.11 `files`

```lua
files.put(project, path, text | { data = base64, handle = "media:...", mime = "..." }, opts?) -> { ok, entry }
files.read(project, path) -> { ok, entry }
files.list(project, prefix?) -> { ok, entries }
files.delete(project, path) -> { ok = true }
files.projects() -> { ok, projects }
files.commit(project, { ref = "main", message = "..." }?) -> { ok, commit }
files.checkout(project, ref | commit_id) -> { ok, manifest }

files.scratch.put(name, text | { data = base64, mime = "..." }) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

範圍由引擎依輪次的身分解析；迴圈無法指名其他使用者的專案。一次 commit 是完整樹狀結構的快照，帶有上層指標與具名 ref——版本控管是引擎原生的，不是 git 伺服器。需要 `files`。

### 6.12 `user`

```lua
user.current() -> { ok, registered, id, display_name, identities }
user.usage() -> { ok, input, output, cached, reasoning, calls, failed,
                  billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { ok, service, present }
user.get(id) -> profile          -- 僅限維護來源
user.list() -> [ profile ]       -- 僅限維護來源
```

關於這一輪所服務之人的唯讀資訊。`has_credential` 只回答金鑰是否存在，永遠不會給出它的值。`get`/`list` 僅限於引擎觸發的維護輪次。

### 6.13 `runtime` 與 `credential`

```lua
runtime.triggers() -> {
  ok,
  triggers = [ {
    name, kind, run_on_boot,
    target = { profile }, payload,
    event = { channel, match } | cron | every,
  } ]
}

credential.get(name) -> { ok, name, value }
```

`runtime.triggers()` 是啟動時建立快照的合併觸發器清單，迴圈與路由處理常式都能使用。`credential.get` 只對代理 `credentials:` 允許清單中的名稱成功；每次存取都會記錄並計數，而且這個值絕不應傳送給使用者。

### 6.14 `json`

```lua
json.encode(value) -> string
json.decode(text) -> value
```

跨越橋接層的一切都會經過 JSON 編碼。因此：

- **空表格會編碼為 `{}`**，而不是 `[]`；要傳送空陣列，請省略該欄位或改用其他訊號；
- 陣列必須是連續的 1..n 序列；混合或非數字的鍵會使它變成物件；
- 函式、userdata 與循環參照的表格無法跨越；
- `nil` 欄位會消失；`NaN`/`±Inf` 編碼為 `null`；
- `json.decode` 需要字串，遇到格式錯誤的輸入會拋出錯誤。

---

## 7. 模式

以下每個模式在 `examples/loops/` 中都有可執行的檔案。若要看結合這些模式的完整部署——具備記憶的 QQ 群組聊天機器人、Telegram RAG 機器人、處理 GitHub pull request 的程式碼代理、chat/completions 端點——請見 [`examples/`](https://github.com/hulipa487/agentflow/tree/main/examples) 中的情境。

### 7.1 請求／回應

[`echo.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/echo.lua)——最小的迴圈。

```lua
function loop()
  while true do
    local msg = session.inbox()
    session.send("echo: " .. (msg.text or ""))
  end
end
```

### 7.2 具備記憶的對話代理

[`chat.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/chat.lua) 會召回最近的輪次、組出提示詞、依歷史預算修剪、回覆，並記錄這一輪。

```lua
local info = agent.info()
local messages = {
  { role = "system", content = info.instructions or "You are a helpful assistant." },
}
local hits = memory.recall({ store = "dialogue", kind = "text", text = msg.text, k = 5 })
for _, hit in ipairs(hits) do
  local v = hit.value
  if type(v) == "table" then
    if v.text then table.insert(messages, { role = "user", content = v.text }) end
    if v.reply then table.insert(messages, { role = "assistant", content = v.reply }) end
  end
end
table.insert(messages, { role = "user", content = msg.text or "" })
messages = token_budget(messages, info.history_budget or 6000)
local reply = llm.chat(messages)
session.send(reply.text or "")
memory.write({ type = "turn", id = msg.id, text = msg.text, reply = reply.text })
```

`token_budget` 是載入到每個工作階段的支援程式碼區塊；它保留系統訊息與最新的訊息，並捨棄最舊的歷史，直到估計值符合預算。工具交換——要求使用工具的 assistant 輪次，以及回應它的結果——會作為一個整體保留或捨棄，因此修剪後的歷史一定是供應商能接受的。

### 7.3 串流

[`stream.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/stream.lua)——直接使用迭代器，提早結束時呼叫 `close()`。請記得：只有文字增量；泛型 `for` 無法使用。

### 7.4 多回合工具迴圈

[`tool_loop.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/tool_loop.lua)——完整的流程：

1. 以 `tools = tools.list()` 傳送使用者輪次；
2. 若回覆含有 `tool_calls`，附加 **assistant** 輪次（包含其工具呼叫與思考區塊）；
3. 執行每個呼叫，並附加帶有 `tool_call_id` 的 `role = "tool"` 輪次；
4. 再問一次——並持續循環，因為模型可能會要求另一個工具；
5. 在得到最終文字回覆或達到回合上限時停止。

迴圈若執行了工具卻從不把結果送回，或從不再問一次，模型看起來就會像是忽略了自己的工具。

### 7.5 以 Lua 宣告工具

在程式碼區塊的最上層使用 `tool.def`（見 API 參考）。處理常式在迴圈協程上執行，可以呼叫任何操作；拋出的錯誤會變成 `{ ok = false, error = ... }`。

### 7.6 計時器與週期性工作

[`scheduled.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/scheduled.lua) 以冪等方式註冊計時器，並依 `payload.timer_id` 加以區分。計時器輪次沒有頻道；不要在其中呼叫 `session.send`。

### 7.7 常駐代理

[`daemon.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/daemon.lua) 處理 `persistent: true` 代理在啟動時收到的合成 `boot` 訊息，接著服務使用者訊息與自己的計時器觸發。常駐代理是一個工作階段；在叢集中恰好由一個執行個體擁有它。

### 7.8 多代理委派

[`delegate.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/delegate.lua) 從 `profiles.agent` 範本衍生出子代理，並等待請求／回覆：

```lua
local child = agent.spawn("researcher")
local answer = agent.request(child.address, { question = msg.text }, 120)
```

在工作者端，以核心標記的 ID 回答請求：

```lua
if msg.type == "agent" then
  local rid = msg.provenance and msg.provenance.request_id
  if rid then agent.reply(rid, { answer = "..." }) end
elseif msg.type == "user" then
  session.send("...")
end
```

靜態的對等代理可以用 `agent:<name>` 定址（受 `can_contact` 限制）。租戶會隨著轉送傳遞；子代理會寫入同一個使用者的範圍，除非它以 `inherit_user: false` 選擇不繼承。

### 7.9 檔案與快照

[`files.lua`](https://github.com/hulipa487/agentflow/blob/main/examples/loops/files.lua) 寫入一則筆記、列出專案，並提交一份快照。使用 `files.checkout` 還原樹狀結構，或用 `shell.run("make", { project = "..." })` 在工作樹上執行指令。

### 7.10 搭配儲存憑證的 HTTP

```lua
local r = http.request({
  url = "https://api.example.com/v1/items",
  headers = { ["Accept"] = "application/json" },
  auth = { service = "example_api" },   -- 由 Go 解析；永遠不會進入 Lua
  query = { limit = 10 },
  timeout = 20,
})
if r.status == 200 then
  local data = json.decode(r.body)
  session.send("found " .. tostring(#(data.items or {})) .. " items")
else
  session.send("upstream said " .. tostring(r.status))
end
```

若本文很大，請加上 `save_to = "response.json"`，再用 `files.scratch.read` 讀取。目的地都受位址防護，除非部署以 `net.http.allow_private` 選擇停用。

### 7.11 郵件

```lua
local inbox = mail.imap_fetch({
  host = "imap.example.com", user = "bot@example.com",
  mailbox = "INBOX", unseen = true, limit = 5,
  auth = { service = "mail_password" },
})
```

### 7.12 維護迴圈

來源為 `system`/`scheduler` 的輪次可以列舉範圍與使用者資料：

```lua
for _, scope in ipairs(store.scopes("dialogue")) do
  -- scope 的形式為 "user:<uuid>"；以明確指定範圍的鍵讀寫
end
```

預期用途是引擎觸發的工作（夜間壓縮、彙整程式）。頻道輪次會被拒絕，因此一個使用者永遠無法列舉另一個使用者的範圍。

---

## 8. 韌性

### 在每一輪攔截失敗

```lua
local ok, err = pcall(handler, msg)
if not ok then
  log.error("turn failed: " .. tostring(err))
  session.send("Sorry — that turn failed.")
end
```

未攔截的錯誤會讓迴圈當機，並每秒重新啟動一次；對使用者顯示錯誤，幾乎總是比當機迴圈好。

### 以退避方式重試

```lua
local function with_retry(fn, attempts)
  for i = 1, attempts do
    local ok, res = pcall(fn)
    if ok then return true, res end
    if i < attempts then time.sleep(0.5 * i) end
  end
  return false, "retries exhausted"
end
```

`time.sleep` 會讓出執行；它不會阻塞執行緒。

### 冪等性

工作階段可能在一輪進行到一半時重新啟動（當機、熱重新載入、執行個體容錯移轉）。如果某一輪會產生外部副作用，請在執行前或執行後於 `session.state` 中記錄標記，並在進入時檢查它。重新啟動前後並不保證恰好執行一次。

### 為每個外部呼叫設限

在 `http.request` 上設定 `timeout`；使用 `agent.request` 的逾時；限制工具回合數；用 `token_budget` 限制歷史長度。可能永遠等待的迴圈，終究會永遠佔住一個工作階段。

---

## 9. 提示詞組裝與 Token 預算

可靠的提示詞結構：

```lua
local info = agent.info()
local messages = {
  { role = "system", content = info.instructions or "" },
}
-- 召回的歷史（最舊 → 最新）
-- 目前的使用者輪次
messages = token_budget(messages, info.history_budget or 6000)
local reply = llm.chat(messages, { model = info.model })
```

- `info.instructions` 已包含任何依使用者的指令層。
- `token_budget` 以 `ceil(chars / 4)` 估算 Token，保留系統訊息與最新的訊息，並捨棄最舊的歷史直到符合預算。估算涵蓋訊息的文字、重播的思考區塊，以及它所發出之工具呼叫的引數。
- **工具交換永遠不會被拆開。** 帶有 `tool_calls` 的 assistant 輪次與其後的 `role = "tool"` 訊息會一起保留或捨棄。在工具迴圈的各回合之間，最新的訊息是工具結果，因此即使目前這次交換本身就超出預算，也會整個保留。
- **大型工具結果有多重就佔多少預算。** 它和其他訊息一樣計算，所以一個巨大的本文可能擠掉真正的歷史，而引擎中沒有任何東西會截斷工具結果：請限制工具輸出（`builtin:fetch` 有 `max_bytes`，預設 1 MiB），並在迴圈中摘要大型結果。
- `token_budget` 只修剪，不修復。如果歷史中已經有沒有對應呼叫的工具結果，會原樣傳遞。
- 預算在呼叫之前強制執行：宣告的 `budget.tokens_per_day` 是每個代理的硬性額度池；每個使用者的配額則從帳本中扣除。
- 思考 Token 會在 `usage.reasoning` 中回報；它們會計入預算。

---

## 10. 測試與偵錯

### 在 Studio 中

1. 啟動執行環境並連線 Studio（`runtime.studio.enabled`，預設開啟）。
2. 在建構器中編輯迴圈並按下 **驗證**——它會檢查 prelude 指示詞、編譯程式碼區塊，並驗證代理定義。
3. 按下 **套用**，然後開啟遊樂場。每一輪都會串流一份追蹤：`turn.start`、`op.start`/`op.end`（模型、工具、耗時、Token）、安全判定、回覆與迴圈錯誤。
4. 設定 `runtime.studio.trace_payloads: true`，即可在追蹤中擷取提示詞與回覆文字（預設關閉）。

### 從終端機

```bash
./agentflow run -config agentflow.yaml -log-level debug -no-tui
```

`debug` 會記錄每一次操作分派、LLM 請求、記憶操作與工具呼叫。迴圈中的 `log.info` 會出現在同一個串流中。網頁主控台的日誌追蹤（SSE）也是同一個來源。

### 熱重新載入

儲存迴圈檔案；執行中的工作階段會在下一個停駐點從全新的狀態重新開始。如果新檔案無法編譯，或宣告了錯誤的 prelude 版本，執行中的版本會保持不變。`session.state`、記憶、儲存區與檔案在重新啟動後仍會保留；Lua 區域變數則不會。

### 範例迴圈

`examples/loops/` 下的每個檔案都由 `TestExampleLoopsCompile` 檢查——必須宣告目前的 prelude 版本並能編譯。把它們當作起點，然後讓代理的 `loop:` 指向你的副本。

---

## 11. 常見陷阱

- **沒有指示詞就不會執行。** 開頭註解區塊中的 `-- af-prelude-version: N` 是必要的。版本不符會直接被拒絕，而不只是警告。
- **從 `loop()` 傳回會讓它重新啟動。** 要結束工作階段，請呼叫 `session.exit()`。
- **未攔截的錯誤就是當機迴圈。** 請把每一輪包在 `pcall` 中。
- **`session.send` 需要作用中的傳入訊息。** 在 `timer`、`cron` 與 `boot` 輪次中它會失敗；請使用 `push`/`push_user` 或保存狀態。
- **計時器會在熱重新載入後重複。** 重新註冊前，請先取消記住的 ID；計時器存在記憶體中，重新啟動後就會遺失。
- **`llm.stream`：不能用泛型 `for`。** 直接呼叫 `next_delta()`，提早結束時呼叫 `close()`。它只產生文字。
- **工具迴圈需要第二回合。** 附加 assistant 的工具呼叫輪次與每個工具結果，然後再問一次。`token_budget` 不計算工具結果。
- **`agent.spawn(profile, spec)` 會忽略 `spec`。** 請設定衍生範本。
- **`agent.request` 的逾時單位是秒**，而 `agent.reply` 需要的是 `msg.provenance.request_id`，不是 payload 欄位。
- **身分來自來源資訊。** `payload.user_uuid` 會被忽略；迴圈無法擴大租戶範圍。
- **`memory.recall` 傳回清單**，而 `store.query` 傳回 `{ records }`。
- **範圍在引擎端強制執行。** `store.scopes` 與 `user.get/list` 僅限維護用途；頻道輪次會被拒絕。
- **`os` 不是真正的 Lua os。** 只有 `os.env`、`os.time` 與 `os.date`，全部為 UTC。沒有 `io`、沒有 `debug`，也沒有 socket 函式庫。
- **空表格會編碼為 `{}`。** 陣列必須連續；函式與循環參照的表格無法跨越橋接層。
- **能力是強制的，不是建議。** 被扣留的操作會拋出指名該能力的拒絕錯誤；`files`、`net.mail`、`shell.exec`、`session.state` 與 `channel.push` 須明確宣告。
- **沒有掛鉤。** 引擎只分派 `safety.ingress`、`safety.egress` 與 `gateway.route`；其餘都是由 prelude 或你的迴圈呼叫的命名慣例（`memory_routing_table`、`memory_recall_handler`、`token_budget`…）。
- **`scheduler.cron` 只支援子集。** 需要依日／月／星期的排程時，請使用 `every`/`after` 或引擎觸發器。
- **`http.request` 會在撥號器中拒絕私有目的地**（包括重新導向的每一跳），除非部署設定了 `net.http.allow_private`。

---

## 12. 速查表

| 操作 | 能力 | 是否阻塞 | 說明 |
|---|---|---|---|
| `session.inbox` | — | 停駐 | 信箱迴圈 |
| `session.send` | — | 是 | 回覆目前作用中的訊息；安全 + 訊息日誌 |
| `session.push` / `push_user` | `channel.push` | 是 | 主動傳出 |
| `session.exit` | — | 否 | 結束工作階段 |
| `session.state.*` | `session.state` | 是 | 每個工作階段的持久鍵值 |
| `log.*` | — | 內嵌 | 日誌等級 |
| `time.sleep` | — | 讓出執行 | 閒置，不阻塞 |
| `agent.info` / `config` | — | 內嵌 | 身分與設定檔 |
| `agent.send` | `agent.send` | 是 | `can_contact` |
| `agent.reply` / `agent.list` | — | 是／內嵌 | 請求擁有權／以聯絡人為範圍 |
| `agent.request` | `agent.request` | 是 | 秒 |
| `agent.spawn` | `agent.spawn` | 是 | 衍生範本 |
| `scheduler.*` | — | 是 | 工作階段計時器 |
| `llm.chat` / `stream` / `embed` / `rerank` | `llm.chat` | 是 | 計量／受預算限制 |
| `memory.*` / `store.*` | `memory` | 是 | 有範圍限制 |
| `tools.list` / `run` | `tools` | 是 | 確認攔截 |
| `shell.*` | `shell.exec` | 是 | 一次性沙箱工作；SSH 控制代碼 |
| `http.*` / `os.env` | `net.http` | 是 | 位址防護 |
| `mail.*` | `net.mail` | 是 | 無位址防護 |
| `files.*` | `files` | 是 | 以使用者為範圍 |
| `user.*` | `users` | 是 | get/list 僅限維護用途 |
| `runtime.triggers` | — | 內嵌 | 部署資料 |
| `credential.get` | 允許清單 | 是 | 會記錄；絕不回顯 |
| `json.*` | — | 內嵌 | 橋接層編碼 |

---

## 13. 支援程式碼區塊、遮蔽與路由處理常式

### 支援程式碼區塊

有五個內建項目會在迴圈**之前**載入並執行於每個工作階段狀態中：`token_budget`、`routing_table`、`recency`、`semantic`、`fact_extractor`。它們定義了你的迴圈可以呼叫的全域變數：

| 程式碼區塊 | 全域變數 | 使用方 |
|---|---|---|
| `plugin:token_budget` | `token_budget(messages, budget)` | 你的迴圈 |
| `plugin:routing_table` | `memory_routing_table(record)` | `memory.write` |
| `plugin:recency` | `memory_recall_handler(query, opts)` | `memory.recall` |
| `plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | `memory.recall` |
| `plugin:fact_extractor` | `fact_extractor_turn(turn)` | 你的迴圈，在一輪結束後 |

要替換其中一個，請在 `plugins.dir` 中放入同名檔案（例如 `plugins/recency.lua`）。它會在整個部署中遮蔽內建項目；遮蔽檔會受到監看，並在每個工作階段啟動時載入，因此編輯後不必重新啟動行程即可生效。遮蔽檔同樣受版本控管。

### 路由處理常式

`gateway.route`（預設 `plugin:per_chat`）是另一種介面：每個行程一個 Luau 狀態，位於任何工作階段之外。它的 `loop()` 接收 `{ message = Message, agent = "..." }` 項目，並決定訊息要送往何處：

```lua
function loop()
  while true do
    local item = session.inbox()
    local msg = item.message
    af.op({
      type = "deliver",
      agent = item.agent,
      key = (msg.channel or "?") .. ":" .. (msg.from or "?"),
      message = msg,
    })
  end
end
```

`deliver` 接受 `{ agent, key, message, identity_group?, kind? }`。沒有 `kind` 時，訊息會送到 `key` 對應的對話工作階段；該工作階段在第一則訊息時建立並持續保留。`kind = "ephemeral"` 會給訊息一個專屬的工作階段，只服務恰好一輪就消失：它不取得租約、不寫入信箱資料列，其 ID 也永遠不會被註銷，因此適合無狀態的流量。請給它一個每個請求都唯一的鍵；內建處理常式就是這樣處理 [OpenAI 相容頻道](openai-api.md)的：

```lua
if msg.payload and msg.payload.api == "openai.chat" then
  local conv = msg.payload.conversation_id
  if conv ~= nil and conv ~= "" then
    af.op({ type = "deliver", agent = item.agent, message = msg,
            key = "conv:" .. msg.channel .. ":" .. tostring(conv) })
  else
    af.op({ type = "deliver", agent = item.agent, message = msg, kind = "ephemeral",
            key = "req:" .. tostring(msg.payload.request_id) })
  end
end
```

未知的 `kind` 會被拒絕，訊息也不會遞送；暫時性的遞送不能帶有 `identity_group`。在使用共用金鑰的頻道上，若像上面聊天頻道的範例那樣以 `msg.from` 作為這類請求的鍵，就是錯誤的：所有呼叫端的寄件者都相同，所以它們會共用同一個工作階段。處理常式取得訊息後，若在要求下一則訊息之前沒有遞送它，就等於把它捨棄了；正在等待該請求的頻道（OpenAI 相容頻道）會立即回應 503。

路由處理常式只能呼叫 `inbox`、`deliver`、`router.state.*`、`runtime.triggers` 與 `log`；其他呼叫都會依名稱被拒絕。它無法傳送頻道回覆或呼叫模型——它只負責路由，工作由工作階段迴圈完成。`router.state` 是持久的、涵蓋整個部署的狀態（每個鍵預設 TTL 為 30 天），用來存放例如某個聊天的路由模式。預設的 `per_chat` 處理常式也實作了 `/group <uuid>` / `/personal` 切換，由引擎驗證所提出的成員資格；如果你的產品需要不同的原則，可以整個替換處理常式，但引擎端的驗證不是可選的。

---

## 延伸閱讀

- [完整組態參考](configuration.md)——代理、記憶、能力、觸發器、MCP、頻道。
- [Lua 外掛開發](loops-reference.md)——內嵌文件的 API 表格與多模態矩陣。
- [`examples/loops/`](https://github.com/hulipa487/agentflow/tree/main/examples/loops)——上述所有可執行的模式。
- [`agentflow.yaml`](https://github.com/hulipa487/agentflow/blob/main/agentflow.yaml)——範例部署（儲存庫以 gitignore 排除此檔，因為它是本機執行個體的組態；受版本追蹤的範例片段位於 `examples/` 下）。
