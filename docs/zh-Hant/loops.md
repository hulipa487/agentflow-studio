# 撰寫指令碼迴圈

一份用 TypeScript / JavaScript 寫 AgentFlow 工作階段迴圈的實用指南：執行模型、完整的指令碼 API、實戰模式，以及那些容易踩的坑。

這些模式的可執行版本都在 [`examples/loops/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/loops)。

內建的 TypeScript 型別宣告在 [ts/af.d.ts](ts/af.d.ts)；從本頁開啟就能在編輯器裡檢視 API 全貌。

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
11. [坑點](#11-gotchas)
12. [速查表](#12-quick-reference-table)
13. [支援片段、覆寫與路由處理常式](#13-support-chunks-shadows-and-route-handlers)

---

## 1. 最小的迴圈

工作階段迴圈是一個定義了全域或匯出函式 `loop()` 的 TypeScript / JavaScript 模組。執行環境為每個工作階段新建一個隔離的 Goja VM 狀態並呼叫 `loop()`；這個函式會在該工作階段的整個生命週期內持續執行。

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();      // 停駐直到下一則訊息到來
    session.send("echo: " + (msg.text || ""));
  }
}
```

讓代理指向這個檔案：

```yaml
agents:
  echo:
    loop: ./examples/loops/echo.ts
```

啟動執行環境並給它送一則訊息（範例組態帶一個 webhook 頻道）：

```bash
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/local/ \
     -H 'Content-Type: application/json' \
     -d '{"from":"alice","text":"hello"}'
# echo: hello
```

### 唯一必要的一行

每一段指令碼區塊 —— 迴圈、支援片段、路由處理常式都一樣 —— 都必須在開頭的註解區塊裡宣告 prelude API 版本：

```ts
// af-prelude-version: 2
```

沒有這個指令、或版本不對的程式碼區塊，會在**執行之前**就被拒絕。沒有相容視窗：prelude 和使用它的指令碼是一起升級的。版本號在原始碼裡是 `vm.PreludeVersion`（指令碼可見為 `af.version`），也可以從 `GET /admin/api/studio/catalog` 查到；目前值是 **2**。

### 先驗證再執行

Studio 應用程式（`/admin/api/studio/*`）驗證迴圈的方式是：檢查指令、剝掉 TypeScript 註記、編譯程式碼區塊，然後即時套用。`POST /admin/api/studio/agents/validate` 做同樣的檢查，但沒有副作用。Go 裡也可以直接做一次編譯檢查（`vm.CompileCheck`）。

編譯檢查只能證明語法沒問題，證明不了行為：引用了不存在的全域、呼叫了沒啟用的能力，或邏輯寫錯，都會在第一輪執行時報錯，並在 Studio 的追蹤裡顯示出來。

---

## 2. 執行模型

每個工作階段都是一個 **actor**：

- 一個 actor goroutine、一個信箱、一個隔離的 Goja VM 狀態加一個 worker goroutine；
- 訊息依序逐則處理；
- 迴圈永遠不會和自己並行。

暴露給 JS 的核心原生原語是 `__af_op(json_request)`。指令碼呼叫某個操作時，VM 的 worker goroutine 掛在一個 Go channel 上；Go 執行請求（快的操作內聯執行，阻塞的放到工作池），然後帶著 `[json_response, ok]` 恢復 VM。prelude 把它包成 `op(req)`，並在 `ok` 為 false 時拋 `Error`。因為暫停的工作階段 goroutine 是在 Go channel 上等，一個暫停的工作階段不佔用任何作業系統執行緒。

```ts
// 概念上，prelude 替你做的事：
const [resp, ok] = __af_op(json.encode(req));
if (!ok) throw new Error(String(resp));
return json.decode(resp);
```

### 暫停點與重新啟動

`session.inbox()` 就是暫停點。工作階段 VM 暫停等待下一則信箱訊息時，工作階段處於閒置、開銷極低。不需要用 `time.sleep` 輪詢 —— 永遠不要寫 `while (true) { ... time.sleep(0.1); ... }` 去找活幹；信箱本身就是工作佇列。

`loop()` 周圍會發生什麼：

| 事件 | 結果 |
|---|---|
| `session.inbox()` | 暫停，直到有訊息到達 |
| `session.send` / `push` / `llm.chat` / `tools.run` / `api.*` / `store.*` / `shell.*` / `http.*` / `mail.*` / `agent.*` | 跨 Go 橋讓出，拿到結果後恢復 |
| `log.*`、`agent.info`、`agent.config`、`json.*` | 快速的宿主呼叫，不需要卸載到工作池 |
| `loop()` 回傳 | 迴圈**立刻重新啟動**（跑完的迴圈不等於退出的工作階段） |
| 未捕捉的 JS 錯誤逃出 `loop()` | 迴圈崩潰，約 1 秒後重新啟動，如此反覆 |
| `session.exit()` | 工作階段結束；VM 被銷毀；計時器與請求被回收 |

有兩點值得記牢：

- **要結束工作階段，呼叫 `session.exit()`。** 從 `loop()` 回傳只會重新啟動它。
- **把可能失敗的工作包進 `try { ... } catch (err) { ... }`。** 未捕捉的供應商錯誤會變成崩潰迴圈，而不是一次失敗的輪次。

**JS 記憶體變數只是快取，不是儲存。** 一輪開始時 VM 狀態可能是全新的：熱重載、故障轉移、租約遺失，或閒置卸載（`runtime.sessions.idle_unload`，預設 30m）都會讓迴圈從頭重跑。正確的迴圈需要跨輪記住的東西，都該放在 `session.state.*`、記憶或儲存區裡 —— 今天撐過重新啟動的模組變數，下次未必。把游標放在 JS 變數裡的迴圈，只有在對話連續 30 分鐘沒訊息之前才是對的。

### 熱重載

改動迴圈檔案，會在它下一個暫停點用全新狀態重新啟動工作階段 VM。指令與以檔案為基礎的提示詞原地重新載入；路由處理常式在訊息之間重建；被覆寫的支援片段會重新啟動所有作用中工作階段。新版本編譯不過的話，正在跑的版本原樣留著。每個工作階段就是一個對話：一次編輯可能重新啟動每個執行個體上該工作階段的副本，而重新啟動會丟掉所有不在 `session.state`、記憶、儲存區或檔案裡的 JS 記憶體狀態。

### 一次一個操作

每個工作階段跑一個單執行緒的 Goja 執行環境，由工作階段 actor 一次驅動一個操作（沒有 `setTimeout` / `setInterval` 事件迴圈）。輔助函式與 `tool.def` 的處理常式被呼叫時就在同一個 VM goroutine 上同步執行，可以依序呼叫宿主操作，做完後正常交回控制權。

---

## 3. 訊息

`session.inbox()` 回傳這樣的物件：

```ts
{
  id: "msg-...",                 // 穩定的訊息 ID（稽核依它關聯）
  type: "user",                  // user | agent | timer | cron | boot | system
  from: "webhook:alice",         // 由核心標記的寄件者
  text: "hello",                 // 頻道文字；可能為空
  channel: "webhook",            // 來源頻道（timer/cron/boot 沒有）
  reply_to: "...",               // 回覆時使用的頻道專屬收件者
  ts: 1730000000,                // Unix 秒數
  payload: { ... },              // 操作專屬資料（見下文）
  attachments: [ ... ],          // 媒體部分描述元（若有）
  provenance: {                  // 由核心標記；迴圈永遠無法控制
    kind: "channel",             // channel | agent | timer | scheduler | system | studio
                                 // timer = session.scheduler.* 的觸發，範圍限於
                                 // 排定它的租戶；scheduler = 引擎觸發器，
                                 // 也是唯一的維護用計時器來源
    principal: "webhook:alice",
    parent: "...",               // 上層工作階段 ID（代理訊息）
    request_id: "...",           // 關聯 ID（agent.request / agent.reply）
    user_uuid: "u_...",          // 這一輪代表的租戶，或 null/undefined
    membership_uuid: "...",      // 群組情境（若已驗證）
  },
}
```

### 訊息類型

| `type` | 來自哪裡 | `payload` |
|---|---|---|
| `user` | 某個頻道，或 Studio 遊樂場 | 頻道相關（`chat_id`、`text` 等） |
| `agent` | `agent.send` / `agent.request` / `agent.spawn` 流量 | 傳送方的 payload 物件 |
| `timer` | `scheduler.every/after/cron` | `{ timer_id: "..." }` |
| `shell.result` | 一個 `shell.start` 作業結束 | 該作業：`{ id, state, exit_code, stdout, stderr, changed, ... }` |
| `cron` | 引擎的 `every:` / `cron:` 觸發器 | 觸發器的 `payload:` 原樣複製 |
| `boot` | supervisor 為 `persistent: true` 的代理發出 | 無 |
| `system` | 引擎生命週期訊息（例如 `agent.died`） | 事件相關 |

`timer`、`shell.result`、`cron` 與 `boot` 輪次**沒有頻道**。`session.send` 只回覆作用中的傳入訊息，否則會以 `no active message to reply to` 失敗；有真實接收方時用 `session.push(channel, reply_to, text)`（能力 `channel.push`）或 `session.push_user(uuid, text)`，否則就把狀態留到下一個使用者輪次。

### 身分來自來源資訊

身分從 `msg.provenance` 讀，絕不從 `msg.payload` 讀：

- `user_uuid` 是本輪所服務的一方。沒有任何東西給訊息蓋章時**不存在**，訊息不屬於任何租戶時**為空**。
- `membership_uuid` 在路由器提議了群組、且引擎驗證了傳送者成員資格時設定。
- 委派跳轉會帶上傳送方的租戶；由引擎蓋章，`payload.user_uuid` 無論誰寫的都被忽略。

迴圈沒法靠往 payload 裡塞 UUID 來擴大租戶範圍。

### 附件

頻道吃進媒體時，訊息會帶一些小的描述元：

```ts
{
  type: "image", mime: "image/png",
  handle: "media:<sha256>", name: "photo.png",
}
```

把它們原樣轉進 `llm.chat`（`parts: msg.attachments`）或 `session.send`（`{ attachments: msg.attachments }`）。原始位元組永遠不進 JS VM；執行環境在供應商／頻道邊界解析控制代碼。

### 傳送

```ts
session.send(text);                              // 回覆目前作用中的訊息
session.send(text, { attachments: parts });      // 附帶媒體
session.push(channel, reply_to, text);           // 主動傳出；能力 channel.push
session.push_user(uuid, text);                   // 依身分 uuid；能力 channel.push
```

`session.send` 與 `session.push` 都會經過由核心掌管的傳出安全鏈與稽核日誌；迴圈繞不過其中任何一個。`session.push_user` 需要身分層和一個裸 UUID（prelude 會補上 `user:`）。

---

## 4. 狀態與持久性

狀態有四個地方可以放。挑能撐過你需要的那個事件的最小一個。

| 機制 | 作用域 | 撐得過熱重載 | 撐得過重新啟動 | 多執行個體 | 適用 |
|---|---|---|---|---|---|
| JS 區域變數 | 這個 VM 狀態 | **否** | 否 | 否 | 一輪之內的暫存資料 |
| `session.state` | 一個工作階段 | 是 | 是（執行環境儲存區） | 是，跟著工作階段走 | 游標、計數器、計時器 ID |
| 記憶設定檔（`memory.*`） | 一個代理（依使用者／鍵範圍） | 是 | 是 | 是 | 對話歷史、事實 |
| `store.*` | 一張具名表，帶作用域 | 是 | 是 | 是 | 原始鍵值或向量資料 |
| `files.*` | 一個使用者／代理專案樹 | 是 | 是 | 是 | 文件與快照 |

### `session.state`

一個工作階段專屬的小型持久鍵值儲存。需要 `session.state` 能力（不在預設集合裡）。

```ts
session.state.set(key, value);   // value 可為任何可 JSON 編碼的 JS 值
session.state.get(key);          // 從未設定時為 null/undefined
session.state.delete(key);
session.state.list();            // { [key]: value }
```

限制：鍵最長 128 個字元，可含字母、數字與 `. _ - :`；值上限 64 KiB；每工作階段 256 個鍵。它適合放計時器 ID、游標或步數 —— 不適合放對話歷史。

### 記憶

`memory.write` 把紀錄依設定檔的寫入鏈（預設 `plugin:routing_table`）送進已宣告的儲存區。`memory.recall` 分派到設定檔的喚回處理常式（預設 `plugin:recency`，或 `plugin:semantic`）。

```ts
// 一筆輪次記錄：預設的寫入鏈會把它送到 "dialogue" 儲存區。
memory.write({
  type: "turn",
  id: msg.id,
  text: msg.text,
  reply: reply.text,
});

// 一筆事實：送到 "facts" 儲存區。
memory.write({ type: "fact", key: "name:" + msg.from, value: { text: "..." } });

// 明確指定目標時會略過路由表。
memory.write({ targets: [{ store: "dialogue", key: "custom:" + msg.id, value: "..." }] });
```

`memory.recall` 回傳紀錄的**清單**，每筆是 `{ key, value, meta }` —— 不是 `{ records }` 包裝。查詢形狀取決於處理常式：

| 處理常式 | 查詢 |
|---|---|
| `plugin:recency`（預設） | `{ store?, kind: "all" \| "prefix" \| "text" \| "time", prefix?, text?, k?, from?, to? }` |
| `plugin:semantic` | `{ text, store?, k? }`（設定檔上需要有 `embed_model`） |

不帶 `store` 時，依時間喚回讀 `dialogue`，所以要在 `facts` 上做全文搜尋就得指明儲存區（`{ kind: "text", store: "facts", text: "...", k: 5 }`）。時間視窗是 `kind: "time"` 加 RFC3339 的 `from` 與 `to`。喚回回傳處理常式給什麼就是什麼；依時間與語意都回傳紀錄清單。設定檔、儲存區與隔離詳見[記憶](memory.md)指南。

### 原始儲存

`store.*` 是更底層的表 API，受 `memory` 能力閘門控管，作用域由引擎決定（私有儲存依使用者給列分鍵，外加 service 列與升級前的列；綁定集區的儲存由租戶授權治理）。

```ts
store.put("table", "key", value, { ttl: 3600, vector: [...] });
store.get("table", "key");                 // { value, found }
store.query("table", { kind: "all" });     // { records: [{ key, value, meta }, ...] }
store.delete("table", "key");
```

查詢種類：`all`、`prefix`（`prefix: "..."`）、`text`（`text: "...", k: n`）、`vector`（`vector: [...], k: n`）、`time_range`（`from`、`to` 為 RFC3339）。後端必須支援該種類：宣告了 `requires: ["vector"]`（或 `["text_search"]`）的儲存區，後端不支援時啟動就失敗。給不支援 `vector` 特性的後端傳向量是明確報錯，絕不靜默回傳空結果。

### 檔案

使用者範圍的檔案儲存區（`files` 能力）存放專案樹，帶引擎原生的快照版本管理。位元組留在引擎側；`put` 接受文字或 base64，`read` 回傳中繼資料與控制代碼。

```ts
files.put("notes", "inbox/1.md", msg.text);
files.list("notes", "inbox/");
files.commit("notes", { ref: "main", message: "note " + msg.id });
files.checkout("notes", "main");
```

暫存檔依工作階段隔離並帶 TTL：

```ts
files.scratch.put("page.html", body);
files.scratch.read("page.html");
files.scratch.list();
```

---

## 5. 能力

能力在執行時閘門控管指令碼面。被拒絕的操作會拋出一個明確的 `Error`，點名操作、代理與缺少的能力 —— 不會變成莫名其妙的 undefined 屬性錯誤。

| 能力 | 閘門 |
|---|---|
| `llm.chat` | `llm.chat`、`llm.stream`、`llm.forward`、`llm.embed`、`llm.rerank`、`openai.request` |
| `memory` | `memory.*`、`store.*` |
| `tools` | `tools.list`、`tools.run`（宣告的工具遵循可見性規則） |
| `apis` | `api.list`、`api.call`、`api.batch`、`api.<pkg>.<action>` |
| `agent.send` | `agent.send`（`agent.reply` 由請求歸屬授權，`agent.list` 依聯絡範圍且不受閘門控管） |
| `agent.request` | `agent.request` |
| `agent.spawn` | `agent.spawn` |
| `net.http` | `http.request`、`http.get`、`http.post`、`os.env` |
| `net.mail` | `mail.*` |
| `shell.exec` | `shell.*` |
| `net.ssh` | `ssh.*` |
| `files` | `files.*` |
| `session.state` | `session.state.*` |
| `users` | `user.current`、`user.usage`、`user.has_credential`、`user.get`、`user.list` |
| `channel.push` | `session.push`、`session.push_user` |

代理不寫 `capabilities` 時的預設值：`llm.chat`、`memory`、`tools`、`apis`、`agent.send`、`net.http`、`users`。`net.mail`、`shell.exec`、`net.ssh`、`files`、`session.state` 與 `channel.push` 必須明確宣告。

不受此表閘門控管的：

- `scheduler.*` —— 計時器是引擎傳遞的訊息，不是權限；
- `runtime.triggers()` —— 唯讀的部署資料；
- `credential.get(name)` —— 不受閘門控管，但依代理允許清單（`credentials: [...]`）；每次存取都記日誌並計數。

`plugins.allow_capabilities` 是部署上限：代理不能宣告超出它的能力，衍生出的子工作階段拿到上層與範本的交集，`plugins.enforce_capabilities: false` 則為舊組態恢復「只宣告」的行為。別依賴它。

---

## 6. API 參考

下面的簽章都來自 prelude。失敗會拋 `Error`；失敗不該搞崩迴圈的地方，用 `try { ... } catch (err) { ... }`。

### 6.1 `session`

```ts
session.inbox() -> Message                  // 停駐
session.send(text, opts?)                   // 回覆目前作用中的訊息
session.send(text, { attachments: parts })
session.push(channel, reply_to, text, opts?) // 能力：channel.push
session.push_user(uuid, text)                // 能力：channel.push；身分層
session.exit()                               // 結束工作階段（永不恢復）
session.state.set(key, value)                // 能力：session.state
session.state.get(key) -> value | null | undefined
session.state.delete(key)
session.state.list() -> { [key]: value }
```

### 6.2 日誌與時間

```ts
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
time.sleep(seconds)       // 讓出執行；停駐期間工作階段處於閒置
os.time() -> Unix seconds // UTC；不接受參數
os.date([format,] t?) -> date object | string
os.env(name) -> string    // 引擎的環境變數；未設定時為 ""
```

`os.date` 預設回傳物件：`{ year, month, day, hour, min, sec, wday, yday, isdst }`，`wday` 1 = 週日，全部 UTC。傳 strftime 字串則回傳文字：`os.date("%Y-%m-%d %H:%M:%S", t)`。支援的指令：`%Y %y %m %d %e %H %I %M %S %p %A %a %B %b %j %w %u %Z %%`。沒有本地時區；前導 `!` 當作無操作接受。相比之下，cron 觸發器依 `runtime.timezone_offset_hours` 匹配。

### 6.3 `agent`

```ts
agent.info() -> {
  name, session_id, address, model, instructions,
  history_budget,
  memory: { stores: { [name]: { backend, table, features, locality } },
            recall, embed_model, rerank_model, oversample },
  shell, skills, capabilities,
  agent_model, instructions_append, ...          // 依使用者的覆寫
}
agent.config() -> {
  name, model, instructions_path | instructions_prompt,
  prompts: { [key]: "text" },
  goal?: { type, success_signal, max_turns, on_goal_met },
  extras: { ... }
}
agent.send(address, payload)                    // 能力：agent.send
agent.request(address, payload, timeout_s?) -> Message  // 能力：agent.request
agent.reply(request_id, payload)                // 依請求擁有者授權
agent.list() -> [ { session_id, agent, parent_id } ]    // 以聯絡人為範圍
agent.spawn(profile, spec?) -> { address, session_id, agent } // 能力：agent.spawn
```

- `agent.info().instructions` 是套用了依使用者層之後的系統提示詞；`agent_model` 與 `instructions_append` 把覆寫片段暴露出來，迴圈想據此分支的話可以用。
- `agent.info().memory.stores[name].locality` 是 `deployment`、`region` 或 `instance`：儲存區的資料能覆蓋多遠。持久對話資料（`dialogue`/`facts` 儲存區）該放在 `deployment` 或 `region` 儲存上；`instance` 儲存隨行程消亡，那它是快取，不是歷史。單一執行個體部署可以有意用一個，區域單元裡想這麼做則必須在持久儲存區上設 `locality_ok: true`。
- `agent.config()` 是作為資料的部署設定檔。密鑰參照渲染成不透明標記 `{env: "VAR"}` / `{cred: "service"}`；解析後的值永遠不過橋。
- 位址：設定好的靜態代理是 `agent:<name>`，作用中工作階段（以及衍生出的子工作階段 —— 用 `spawn_result.address`）是 `session:<id>`。`send`/`request` 不接受 `:new`。
- `agent.request` 逾時即取消；逾時單位是**秒**。
- `agent.reply` 必須用核心蓋章來源資訊裡的請求 ID：`msg.provenance.request_id`。絕不要從 `msg.payload` 裡拼。
- `agent.spawn` 需要一個 `profiles.agent` 範本。子工作階段的能力是上層 ∩ 範本，聯絡人也是上層 ∩ 範本。`spec` 為相容而接受，但在此版本中**不生效**；請改在範本裡設定。

### 6.4 `scheduler`

```ts
scheduler.every(interval_s) -> { id }
scheduler.after(delay_s) -> { id }
scheduler.cron(expr) -> { id }
scheduler.cancel(id)
```

計時器把 `Message{ type: "timer", payload: { timer_id: id } }` 傳遞到同一個工作階段。它們在記憶體裡，且是工作階段層級的：重新啟動就忘，而熱重載會重新啟動迴圈但**不清除**計時器，所以盲目重註冊會重複。先取消記住的 ID（見 [`scheduled.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/scheduled.ts)）。

`scheduler.cron` 是個子集：`*/N * * * *` 與 `30 9 * * *` 這類固定形式；日、月、星期必須是 `*`，`*/N` 分鐘不能和固定小時搭配。更複雜的排程請用 `every`/`after`（或引擎觸發器）。

### 6.5 `llm`

```ts
llm.chat(messages, opts?) -> {
  text, thinking, thinking_blocks, usage, tool_calls
}
llm.stream(messages, opts?) -> Iterable<string>
llm.embed(inputs, opts?) -> { vectors, usage }
llm.rerank(query, documents, opts?) -> { results: [ { index, score } ] }
```

`messages` 是 `{ role, content }` 輪次的陣列，多模態輪次用 `{ role, parts: [...] }`。工具歷史用 `tool_calls`、`tool_call_id`，以及 `role: "tool"` 輪次上的 `tool_result`/`content`。開啟 thinking 續接 Anthropic 工具迴圈時，在 assistant 輪重放 `thinking_blocks`。

`llm.chat` 的 `opts`：

```ts
{
  model: "default",          // models: 中的一個項目；"" = 代理預設值
  temperature: 0.7,
  max_tokens: 4096,
  thinking: "off"|"low"|"medium"|"high"|"xhigh"|"max",
  tools: tools.list(),       // 模型可見的工具規格
  tool_choice: "auto"|"none",
}
```

`usage` 是 `{ input, output, cached, cache_write, reasoning }`，取決於供應商上報了什麼。

**串流。** `llm.stream` 回傳標準的 ES6 可疊代物件。用 `for (const delta of llm.stream(messages, opts))` 疊代；提前跳離會觸發 `return()`，自動關閉底層串流（也可以手動呼叫 `iter.next()` / `iter.close()`）。它只產出文字增量 —— 用量與工具呼叫請用 `llm.chat`。見 [`stream.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/stream.ts)。

**嵌入。** `llm.embed` 接受單一字串、字串陣列，或字串與片段物件（`{ type: "image", handle: ..., url: ... }`）混合的陣列；`merged: true` 把所有東西折成一個向量（一個 Jina 內容群組）。`llm.rerank` 回傳指向 `documents` 的、從 0 開始的索引。

### 6.6 `memory` 與 `store`

```ts
memory.write(record) -> true
memory.recall(query, opts?) -> [ { key, value, meta } ]

store.put(table, key, value, opts?)   // opts: { ttl: seconds, vector: [...] }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records: [ { key, value, meta } ] }
store.delete(table, key)
store.scopes(table) -> [ "user:<uuid>", ... ]   // 僅限維護來源
```

記憶紀錄與查詢的形狀見[記憶](#memory)。`store.scopes` 只能從引擎觸發的維護來源（`system` / `scheduler` 輪次）到達；頻道或代理跳轉輪次會被拒絕。夜間蒸餾器靠它列舉使用者範圍，而不必讀某個使用者的輪次。

### 6.7 `tools` 與 `tool.def`

```ts
tools.list() -> [ { type: "function", function: { name, description, parameters } } ]
tools.run(name, args, opts?) -> result
// opts: { idempotency_key?: "..." }
```

在 TypeScript/JS 頂層、`loop()` 之前宣告一個模型可見的工具：

```ts
tool.def({
  name: "script:shout",
  description: "Uppercase the text and return it.",
  params: {
    type: "object",
    properties: { text: { type: "string" } },
    required: ["text"],
  },
  handler: function(args) {
    return { ok: true, shouted: String(args.text || "").toUpperCase() };
  },
});
```

宣告的工具會併入 `tools.list()`，並由 `tools.run` 分派；拋錯的處理常式回傳 `{ ok: false, error: ... }`，而不是把迴圈搞死。可見性和 Go 註冊的工具完全一樣，跟隨代理的 `skills` 與 `tools.policy.default` —— 被隱藏的宣告工具既不列出也不可分派。Go 原則（`forbidden`、`needs_confirm`）管不到迴圈自己宣告的處理常式。

如果某個工具（Go 的，或透過覆寫宣告的）帶 `needs_confirm: true`，執行環境會攔下呼叫，建立一筆綁定工作階段、工作區、合資格核准者、動作修訂版、參數與目標位址的單次核准紀錄，並暫停等待已驗證確認（頻道 `[ref:<id>]` / `confirm.decision` 或 Studio 內嵌脫敏核准卡片）。指令碼輸入不能偽造或傳入 `confirmed: true`；`opts.idempotency_key` 會在執行環境儲存中持久化寫入意圖，使重試能重播已完成結果，並在因中斷而結果不確定時故障關閉（等待提示上限為十分鐘）。

### 6.8 `shell`

```ts
shell.run(command, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out,
                                 truncated, changed, deleted, conflicts, skipped }
shell.start(command, opts?) -> { id, state: "running" }   // 結果會送到信箱
shell.status(job_id) / shell.cancel(job_id)

// opts: { project, environment, workdir, env, timeout }   // timeout 單位為秒
```

每次呼叫都是檔案儲存區裡某個專案上、某個沙箱裡的一條命令。命令在專案裡寫的東西結束時會被存下；別的一概活不到下一次呼叫，而下一次呼叫可以在任何執行個體上跑。非零退出碼是結果；跑不起來的作業會拋錯。`shell.spawn`、`shell.exec`、`shell.write` 與 `shell.destroy` 作為沙箱相容墊片保留（需要 `shell.exec`）。遠端 SSH 連線改用 `ssh.spawn`、`ssh.exec`、`ssh.read`、`ssh.write` 與 `ssh.close`（需要 `net.ssh`）。完整規則見指令碼外掛開發參考。

```ts
const conn = ssh.spawn({ host: "box:22", user: "dev" })
const out  = ssh.exec(conn.id, "uname -a")
ssh.write(conn.id, "/tmp/flag", "1")
const txt  = ssh.read(conn.id, "/tmp/flag")
ssh.close(conn.id)
```

### 6.9 `http` 與 `os`

```ts
http.request(opts) -> {
  ok, status, headers, body,
  truncated?, saved?, save_error?
}
http.get(url, opts?)
http.post(url, body, opts?)

// opts: {
//   method, url, headers, body, json, query, timeout,
//   auth: { service: "..." },      // 儲存的憑證，由 Go 解析
//   save_to: "name",               // 本文也會存入檔案暫存區
// }
```

任何 HTTP 狀態都是成功的請求結果（`ok` 說請求完成了，`status` 說伺服器說了什麼）。回應主體上限 10 MiB（被截斷時置 `truncated`）；`save_to` 讓大回應主體不進入 JS，並回傳 `saved: { name, handle, size }`。私有、回送、連結本地與保留位址在撥號器裡就被拒絕。需要 `net.http`（它同時閘門控管 `os.env`）。

`builtin:fetch` **工具**是另一套更豐富的介面：它額外支援 `follow`/`max_redirects`、`user_agent`、`max_bytes`，以及 `insecure`（關閉 TLS 驗證，總會記日誌並計數）。該由模型呼叫時用這個工具；迴圈自己要送請求時用 `http.request`。

### 6.10 `mail`

```ts
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
// opts: { host, port: 993, user, mailbox: "INBOX", unseen: false,
//         limit: 10, auth: { service: "..." } }
// messages: [ { seq, uid, flags, subject, from, to, date, body } ]

mail.smtp_send(opts) -> { ok, sent }
// opts: { host, port: 465, user, from, to: [...], subject,
//         text_body, auth: { service: "..." } }
```

密碼在呼叫時從憑證儲存區解析，永遠不過橋。`mail.*` 不受 HTTP 私有位址防護約束。需要 `net.mail`。

### 6.11 `files`

```ts
files.put(project, path, text | { data: base64, handle: "media:...", mime: "..." }, opts?) -> { ok, entry }
files.read(project, path) -> { ok, entry }
files.list(project, prefix?) -> { ok, entries }
files.delete(project, path) -> { ok: true }
files.projects() -> { ok, projects }
files.commit(project, { ref: "main", message: "..." }?) -> { ok, commit }
files.checkout(project, ref | commit_id) -> { ok, manifest }

files.scratch.put(name, text | { data: base64, mime: "..." }) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

作用域由引擎依輪次身分解析；迴圈無法指定另一個使用者的專案。一次提交是帶父指標與具名參照的全樹快照 —— 版本管理是引擎原生的，不是 git 伺服器。需要 `files`。

### 6.12 `user`

```ts
user.current() -> { ok, registered, id, display_name, identities }
user.usage() -> { ok, input, output, cached, reasoning, calls, failed,
                  billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { ok, service, present }
user.get(id) -> profile          // 僅限維護來源
user.list() -> [ profile ]       // 僅限維護來源
```

關於本輪所服務使用者的唯讀資訊。`has_credential` 回答密鑰存不存在，永遠不是它的值。`get`/`list` 僅限引擎觸發的維護輪次。

### 6.13 `runtime` 與 `credential`

```ts
runtime.triggers() -> {
  ok,
  triggers: [ {
    name, kind, run_on_boot,
    target: { profile }, payload,
    event: { channel, match } | cron | every,
  } ]
}

credential.get(name) -> { ok, name, value }
```

`runtime.triggers()` 是啟動時快照的合併觸發器清單，迴圈與路由處理常式都能用。`credential.get` 只對代理 `credentials:` 允許清單裡的名字成功；每次存取都記日誌並計數，值絕不該發給使用者。

### 6.14 `json`

```ts
json.encode(value) -> string
json.decode(text) -> value
```

過橋的一切都做 JSON 編碼。後果是：

- JS 原生區分空陣列 `[]` 與空物件 `{}`；陣列序列化成 JSON 陣列，普通物件序列化成 JSON 物件；
- 函式、symbol 與循環參照物件過不去；
- `undefined` 屬性被省略，而 `undefined` 的陣列槽位與 `NaN`/`±Infinity` 編碼成 `null`；
- `json.decode` 要求字串，格式有誤時拋錯。

### 6.15 `api`（外部 API）

```ts
api.list() -> [ { name, version, description, runtime: "script" | "wasm", actions } ]
api.call(pkg, action, args?, opts?) -> result   // opts: { idempotency_key?: "..." }
api.batch(calls) -> [ { ok, data?, error? } ]   // calls: [{ api, action, args, idempotency_key? }]
api.<pkg>.<action>(args?) -> result              // api.call 的 Proxy 簡寫
```

外部 API 是獨立的 TypeScript（`goja`）或 WebAssembly（`wazero`）套件，在 `apis:` 下設定（`apis` 能力，在預設集合裡）。每個套件在自己的隔離執行環境、自己的權限與逾時下執行。直接呼叫 `api.call` 與 `api.batch` 會通過與 `api:<pkg>.<action>` 工具封裝相同的能力、`tools.policy`、託管模式 `kind`、核准與 `idempotency_key` 驗證。`api.call` 與 `api.<pkg>.<action>` 出錯會拋，而 `api.batch` 回傳逐次呼叫的 `{ ok, data, error }`（任何需要確認的批次項目會單獨失敗，必須改用 `api.call` 呼叫）。標記為 `expose_as_tool: true` 的操作，在代理同時擁有 `apis` 與 `tools` 時，也會作為 `api:<pkg>.<action>` 工具出現。

---

## 7. 模式

下面每個模式在 `examples/loops/` 裡都有可執行檔案。想看成套的部署組合 —— 帶記憶的 QQ 群組機器人、Telegram RAG 機器人、GitHub 提取請求程式碼代理、chat/completions 端點 —— 見 [`examples/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples) 裡的情境。

### 7.1 請求／回應

[`echo.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/echo.ts) —— 最小的迴圈。

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    session.send("echo: " + (msg.text || ""));
  }
}
```

### 7.2 帶記憶的對話代理

[`chat.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/chat.ts) 喚回最近的輪次、組裝提示詞、裁到歷史預算、回覆，並記錄本輪。

```ts
const info = agent.info();
let messages = [
  { role: "system", content: info.instructions || "You are a helpful assistant." },
];
const hits = memory.recall({ store: "dialogue", kind: "text", text: msg.text, k: 5 });
for (const hit of hits) {
  const v = hit.value;
  if (v && typeof v === "object") {
    if (v.text) messages.push({ role: "user", content: v.text });
    if (v.reply) messages.push({ role: "assistant", content: v.reply });
  }
}
messages.push({ role: "user", content: msg.text || "" });
messages = token_budget(messages, info.history_budget || 6000);
const reply = llm.chat(messages);
session.send(reply.text || "");
memory.write({ type: "turn", id: msg.id, text: msg.text, reply: reply.text });
```

`token_budget` 是載入每個工作階段的支援片段；它保留系統訊息與最新的訊息，從最舊的歷史開始丟，直到估算值放得下。一次工具往來 —— 請求了工具的 assistant 輪加上回答它的結果 —— 會作為一個整體保留或丟棄，所以裁剪過的歷史一定是供應商能接受的歷史。

### 7.3 串流

[`stream.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/stream.ts) —— 用 `for (const delta of llm.stream(messages, opts))` 疊代；提前跳離會觸發 `return()` 自動關閉串流（也可以手動呼叫 `.close()`）。記住：只產出文字增量字串。

### 7.4 多輪工具迴圈

[`tool_loop.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/tool_loop.ts) —— 完整形態：

1. 帶上 `tools: tools.list()` 送出使用者輪次；
2. 回覆裡有 `tool_calls`，就附加 **assistant** 輪（包括它的工具呼叫與思考區塊）；
3. 逐個執行，附加帶 `tool_call_id` 的 `role: "tool"` 輪；
4. 再問一次 —— 並且繼續迴圈，因為模型可能再要一個工具；
5. 在最終的文字回覆或輪數上限處停下。

只跑工具卻從不把結果送回去、或從不追問的迴圈，會得到一個看起來無視工具的模型。

### 7.5 在 TypeScript/JS 裡宣告工具

在頂層用 `tool.def`（見 API 參考）。處理常式在工作階段 VM 上跑，可以呼叫任何操作；拋出的錯誤變成 `{ ok: false, error: ... }`。

### 7.6 計時器與週期工作

[`scheduled.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/scheduled.ts) 冪等地註冊計時器，並用 `payload.timer_id` 區分它們。計時器輪次沒有頻道；不要在裡面 `session.send`。

### 7.7 常駐代理

[`daemon.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/daemon.ts) 處理 `persistent: true` 代理啟動時收到的合成 `boot` 訊息，然後服務使用者訊息與自己的計時器觸發。常駐代理就是一個工作階段；在叢集裡恰好由一個執行個體擁有它。

### 7.8 多代理委派

[`delegate.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/delegate.ts) 從 `profiles.agent` 範本衍生一個子工作階段，並等待請求／回覆：

```ts
const child = agent.spawn("researcher");
const answer = agent.request(child.address, { question: msg.text }, 120);
```

在子工作階段一側，用核心蓋章的 ID 應答請求：

```ts
if (msg.type === "agent") {
  const rid = msg.provenance && msg.provenance.request_id;
  if (rid) agent.reply(rid, { answer: "..." });
} else if (msg.type === "user") {
  session.send("...");
}
```

靜態對等方可以依 `agent:<name>` 定址（受 `can_contact` 約束）。租戶跟著跳轉走；子工作階段寫的是同一個使用者的範圍，除非它用 `inherit_user: false` 退出了。

### 7.9 檔案與快照

[`files.ts`](https://github.com/hulipa487/agentflow-studio/blob/main/examples/loops/files.ts) 寫一則筆記、列出專案、提交一個快照。要還原某棵樹用 `files.checkout`，要在工作樹上跑命令用 `shell.run("make", { project: "..." })`。

### 7.10 帶儲存憑證的 HTTP

```ts
const r = http.request({
  url: "https://api.example.com/v1/items",
  headers: { Accept: "application/json" },
  auth: { service: "example_api" },   // 由 Go 解析；永遠不會進入 JS
  query: { limit: 10 },
  timeout: 20,
});
if (r.status === 200) {
  const data = json.decode(r.body);
  session.send("found " + String((data.items || []).length) + " items");
} else {
  session.send("upstream said " + String(r.status));
}
```

大回應主體可以加 `save_to: "response.json"`，再用 `files.scratch.read` 讀。目標受位址防護約束，除非部署用 `net.http.allow_private` 退出。

### 7.11 郵件

```ts
const inbox = mail.imap_fetch({
  host: "imap.example.com", user: "bot@example.com",
  mailbox: "INBOX", unseen: true, limit: 5,
  auth: { service: "mail_password" },
});
```

### 7.12 維護迴圈

`system`/`scheduler` 來源的輪次可以列舉作用域與設定檔：

```ts
for (const scope of store.scopes("dialogue")) {
  // scope 的形式為 "user:<uuid>"；以明確指定範圍的鍵讀寫
}
```

引擎觸發的工作（夜間壓縮、蒸餾器）才是預期用法。頻道輪次會被拒絕，所以一個使用者永遠列舉不了另一個使用者的作用域。

---

## 8. 韌性

### 依輪捕捉失敗

```ts
try {
  handler(msg);
} catch (err) {
  log.error("turn failed: " + String(err));
  session.send("Sorry — that turn failed.");
}
```

未捕捉的錯誤會讓迴圈崩潰，然後每秒重新啟動一次；給使用者看的錯誤幾乎總比崩潰迴圈好。

### 帶退避的重試

```ts
function withRetry(fn: () => unknown, attempts: number): [boolean, unknown] {
  for (let i = 1; i <= attempts; i++) {
    try {
      return [true, fn()];
    } catch (_) {
      if (i < attempts) time.sleep(0.5 * i);
    }
  }
  return [false, "retries exhausted"];
}
```

`time.sleep` 會讓給 Go，不阻塞作業系統執行緒。

### 冪等

工作階段可能在輪次中途重新啟動（崩潰、熱重載、執行個體故障轉移）。如果某一輪有外部副作用，就在它之前或之後往 `session.state` 裡記個標記，進來時檢查。跨重新啟動沒有 exactly-once 保證。

### 給每個外部呼叫設邊界

給 `http.request` 設 `timeout`；用 `agent.request` 的逾時；限制工具輪數；用 `token_budget` 限制歷史。能永遠等下去的迴圈，最終一定會永遠佔住一個工作階段。

---

## 9. 提示詞組裝與 Token 預算

可靠的提示詞形態：

```ts
const info = agent.info();
let messages = [
  { role: "system", content: info.instructions || "" },
];
// 召回的歷史（最舊 -> 最新）
// 目前的使用者輪次
messages = token_budget(messages, info.history_budget || 6000);
const reply = llm.chat(messages, { model: info.model });
```

- `info.instructions` 已經包含任何依使用者的指令層。
- `token_budget` 依 `ceil(chars / 4)` 估算 Token，保留系統訊息與最新訊息，從最舊的歷史開始丟直到放得下。估算涵蓋一則訊息的文字、重放的思考區塊，以及它發起的工具呼叫的參數。
- **一次工具往來永不拆開。** 帶 `tool_calls` 的 assistant 輪和它後面的 `role = "tool"` 訊息一起保留或丟棄。工具迴圈的輪次之間，最新的訊息就是工具結果，所以即使這一整個交換本身超預算，也會被完整保留。
- **大的工具結果依它的重量計費。** 它和其他訊息一樣計數，所以一個巨大的回應主體可能擠掉真實歷史，而引擎裡沒有任何地方會截斷工具結果：限制工具輸出（`builtin:fetch` 有 `max_bytes`，預設 1 MiB），在迴圈裡對大的結果做摘要。
- `token_budget` 只裁剪，不修復。已經含有一個沒有對應呼叫的工具結果的歷史，會原樣傳下去。
- 預算在呼叫前強制執行：宣告的 `budget.tokens_per_day` 是每個代理的硬池；依使用者配額從帳冊裡扣。
- 思考 Token 在 `usage.reasoning` 裡回報；它們計入預算。

---

## 10. 測試與偵錯

### 在 Studio 裡

1. 啟動執行環境並連線 Studio（`runtime.studio.enabled`，預設開啟）。
2. 在建構器裡編輯迴圈，按 **驗證** —— 它會檢查 prelude 指令、剝掉 TypeScript 註記、編譯程式碼區塊，並驗證代理定義。
3. **套用** 後開啟遊樂場。每一輪都串流輸出追蹤：`turn.start`、`op.start`/`op.end`（模型、工具、耗時、Token）、安全決策、回覆與迴圈錯誤。
4. 設定 `runtime.studio.trace_payloads: true` 可以把提示詞與回覆文字捕捉進追蹤（預設關閉）。

### 從終端機

```bash
./agentflow run -config agentflow.yaml -log-level debug
```

`debug` 會記錄每次操作分派、LLM 請求、記憶操作與工具呼叫。迴圈裡的 `log.info` 出現在同一條流裡。AgentFlow Studio 的日誌檢視（`/admin/api/logs` SSE）就是同一個來源。

### 熱重載

儲存迴圈檔案；執行中的工作階段在下一個暫停點用全新狀態重新啟動。新檔案編譯不過或宣告了錯誤的 prelude 版本時，正在跑的版本原樣留著。`session.state`、記憶、儲存區與檔案撐得過重新啟動；JS 區域變數不行。

### 範例迴圈

`examples/loops/` 下的每個檔案都由測試套件檢查（`examples.test.ts` 與 `vm.CompileCheck`）—— 它必須宣告目前的 prelude 版本並乾淨編譯。把它們當起點，然後把代理的 `loop:` 指向你的副本。

---

## 11. 坑點

- **沒有指令就不跑。** 開頭註解區塊裡的 `// af-prelude-version: N` 是必要的。版本不匹配是硬拒絕，不是警告。
- **從 `loop()` 回傳就是重新啟動它。** 要結束工作階段請呼叫 `session.exit()`。
- **未捕捉的錯誤就是崩潰迴圈。** 把輪次包進 `try { ... } catch (err) { ... }`。
- **`session.send` 需要作用中的傳入訊息。** 在 `timer`、`cron` 與 `boot` 輪次上會失敗；用 `push`/`push_user`，或者留著狀態。
- **計時器會跨熱重載重複。** 重新註冊前先取消記住的 ID；計時器在記憶體裡，重新啟動就丟。
- **`llm.stream` 只產出文字增量。** 用 `for (const delta of llm.stream(...))` 疊代；提前跳離會觸發 `return()` 自動關閉串流（也可以手動呼叫 `iter.close()`）。
- **工具迴圈需要第二輪。** 附加 assistant 的工具呼叫輪與每個工具結果，然後再問一次。`token_budget` 不統計工具結果。
- **`agent.spawn(profile, spec)` 會忽略 `spec`。** 請設定衍生範本。
- **`agent.request` 的逾時是秒**，而 `agent.reply` 需要 `msg.provenance.request_id`，不是 payload 裡的欄位。
- **身分來自來源資訊。** `payload.user_uuid` 被忽略；迴圈無法擴大租戶。
- **`memory.recall` 回傳清單**，而 `store.query` 回傳 `{ records }`。
- **作用域由引擎側強制執行。** `store.scopes` 與 `user.get/list` 僅限維護；頻道輪次會被拒絕。
- **`os` 不是完整的 Node/Lua `os` 函式庫。** 只有 `os.env`、`os.time` 與 `os.date`，全部 UTC。沒有 `require`、`fs`、`process`、`fetch`、`setTimeout`，也沒有通訊端函式庫。
- **過橋的值都做 JSON 編碼。** 空陣列 `[]` 與空物件 `{}` 保留各自的 JSON 型別，而函式、symbol 與循環參照物件過不了橋。
- **能力是被強制執行的，不是建議。** 被收回的操作會拋出點名能力的拒絕；`files`、`net.mail`、`shell.exec`、`net.ssh`、`session.state` 與 `channel.push` 要明確宣告（`apis` 在預設值裡）。
- **掛鉤。** 引擎只直接分派 `safety.script_filters`、`safety.ingress`、`safety.egress`、`gateway.route`，以及設定好的外部 API（`apis`）操作；工作階段 VM 裡其他一切都是全域約定，由 prelude 或你的迴圈依名字呼叫（`memory_routing_table`、`memory_recall_handler`、`token_budget` 等）。
- **`scheduler.cron` 是個子集。** 日／月／星期的排程請用 `every`/`after` 或引擎觸發器。
- **`http.request` 在撥號器裡拒絕私有目的地**（包括重新導向跳轉），除非部署設了 `net.http.allow_private`。

---

## 12. 速查表

| 操作 | 能力 | 阻塞 | 說明 |
|---|---|---|---|
| `session.inbox` | — | 暫停 | 郵件迴圈 |
| `session.send` | — | 是 | 回覆作用中訊息；安全 + 日誌 |
| `session.push` / `push_user` | `channel.push` | 是 | 主動傳出 |
| `session.exit` | — | 否 | 結束工作階段 |
| `session.state.*` | `session.state` | 是 | 持久化的工作階段層級鍵值 |
| `log.*` | — | 內聯 | 日誌等級 |
| `time.sleep` | — | 讓出 | 閒置，不阻塞 |
| `agent.info` / `config` | — | 內聯 | 身分與設定檔 |
| `agent.send` | `agent.send` | 是 | `can_contact` |
| `agent.reply` / `agent.list` | — | 是／內聯 | 請求歸屬／依聯絡範圍 |
| `agent.request` | `agent.request` | 是 | 秒 |
| `agent.spawn` | `agent.spawn` | 是 | 衍生範本 |
| `scheduler.*` | — | 是 | 工作階段計時器 |
| `llm.chat` / `stream` / `embed` / `rerank` | `llm.chat` | 是 | 計量／預算 |
| `memory.*` / `store.*` | `memory` | 是 | 帶範圍 |
| `tools.list` / `run` | `tools` | 是 | 確認攔截 |
| `api.*` | `apis` | 是 | 外部 TS/Wasm 套件 |
| `shell.*` | `shell.exec` | 是 | 一次性沙箱作業；相容控制代碼 |
| `ssh.*` | `net.ssh` | 是 | 持續性遠端 SSH 連線 |
| `http.*` / `os.env` | `net.http` | 是 | 位址防護 |
| `mail.*` | `net.mail` | 是 | 無位址防護 |
| `files.*` | `files` | 是 | 使用者範圍 |
| `user.*` | `users` | 是 | get/list 僅限維護 |
| `runtime.triggers` | — | 內聯 | 部署資料 |
| `credential.get` | 允許清單 | 是 | 記日誌；絕不回顯 |
| `json.*` | — | 內聯 | 橋編碼 |

---

## 13. 支援片段、覆寫與路由處理常式

### 支援片段

四個內建實作在迴圈**之前**求值進每個工作階段狀態：`token_budget`、`routing_table`、`recency`、`semantic`。它們定義迴圈可以呼叫的全域：

| 片段 | 全域 | 使用方 |
|---|---|---|
| `plugin:token_budget` | `token_budget(messages, budget)` | 你的迴圈 |
| `plugin:routing_table` | `memory_routing_table(record)` | `memory.write` |
| `plugin:recency` | `memory_recall_handler(query, opts)` | `memory.recall` |
| `plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | `memory.recall` |

要替換其中一個，就在 `plugins.dir` 裡放一個同名檔案（例如 `plugins/recency.ts`）。它會在整個部署範圍內覆寫內建實作；覆寫檔案受監看，且每次工作階段啟動時載入，所以編輯後無需重新啟動行程即可生效。覆寫同樣受版本閘門。

### 路由處理常式

`gateway.route`（預設 `plugin:per_chat`）是另一個面：每行程一個 Goja VM 狀態，位於任何工作階段之外。它的 `loop()` 接收 `{ message: Message, agent: "..." }` 項，決定訊息去哪：

```ts
function loop() {
  while (true) {
    const item = session.inbox();
    const msg = item.message;
    af.op({
      type: "deliver",
      agent: item.agent,
      key: (msg.channel || "?") + ":" + (msg.from || "?"),
      message: msg,
    });
  }
}
```

`deliver` 接受 `{ agent, key, message, identity_group?, kind? }`。不給 `kind` 時，訊息進入 `key` 對應的對話工作階段，首次訊息時建立並一直保留。`kind: "ephemeral"` 給訊息一個屬於自己的工作階段，只服務一輪就消失：不佔租約、不寫收件匣列、ID 永不回收，所以它是無狀態流量的正確選擇。給它一個每次請求都唯一的鍵；內建處理常式為 [OpenAI 相容頻道](openai-api.md)就是這麼做的：

```ts
if (msg.payload && msg.payload.api === "openai.chat") {
  const conv = msg.payload.conversation_id;
  if (conv !== null && conv !== undefined && conv !== "") {
    af.op({
      type: "deliver", agent: item.agent, message: msg,
      key: "conv:" + msg.channel + ":" + String(conv),
    });
  } else {
    af.op({
      type: "deliver", agent: item.agent, message: msg, kind: "ephemeral",
      key: "req:" + String(msg.payload.request_id),
    });
  }
}
```

未知的 `kind` 會被拒絕，訊息不會傳遞；ephemeral 傳遞也不能攜帶 `identity_group`。像上面範例給聊天頻道那樣用 `msg.from` 給這類請求分鍵，在共用金鑰的頻道上是個錯誤：所有呼叫方都是同一個傳送者，於是會共用一個工作階段。拿了訊息卻在取下一則之前沒傳遞的處理常式，等於把它丟了；正在等這個請求的頻道（OpenAI 相容頻道）會立刻回 503。

路由處理常式只能呼叫 `inbox`、`deliver`、`router.state.*`、`runtime.triggers` 與 `log`；其他一律依名稱拒絕。它不能送頻道回覆，也不能呼叫模型 —— 它負責路由，幹活的是工作階段迴圈。`router.state` 是持久的、部署範圍的狀態（每個鍵預設 30 天 TTL），用於放聊天的路由模式這類東西。預設的 `per_chat` 處理常式還實作了 `/group <uuid>` / `/personal` 切換，由引擎驗證提議的成員資格；產品想要不同原則就整體替換處理常式，但引擎側的驗證不是可選項。

---

## 延伸閱讀

- [完整組態參考](configuration.md) —— 代理、記憶、能力、觸發器、MCP、頻道。
- [指令碼外掛開發](loops-reference.md) —— 內建文件裡的 API 表與模態支援矩陣。
- [`examples/loops/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/loops) —— 上面每個可執行的模式。
- [`agentflow.yaml`](https://github.com/hulipa487/agentflow/blob/main/agentflow.yaml) —— 範例部署（儲存庫把這個檔案 gitignore 掉了，因為它是本機執行個體組態；納入版本管理的範例部件在 `examples/` 下）。
