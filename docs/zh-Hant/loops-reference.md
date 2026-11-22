## Lua 外掛程式開發

AgentFlow 外掛程式是標準的 Lua 5.1 指令碼，由內建純 Go 虛擬機器（`gopher-lua`，附帶可暫停的 `pcall`/`xpcall`、`utf8`、`bit32` 與 `table.unpack`/`pack` 輔助函式）執行。YAML 決定哪個代理執行哪個外掛程式；外掛程式決定行為。本文件中的長篇指南（迴圈）涵蓋執行模型、完整 API、可執行的範例模式與常見陷阱清單；本節是參考摘要。

外掛程式有三種：

- **迴圈外掛程式**：定義一個全域 `loop()` 函式，在工作階段的整個生命週期內持有其協程。標準寫法是 `while true do local msg = session.inbox() ... end`。
- **支援程式碼區塊**：在迴圈外掛程式*之前*載入到每個工作階段狀態中。支援程式碼區塊定義迴圈會讀取的全域變數與輔助函式。`plugin:token_budget`、`plugin:routing_table`、`plugin:recency` 及 `plugin:semantic` 等內建項目都是支援程式碼區塊。
- **遮蔽（shadow）**：放在 `plugins/` 中、與某個內建項目同名的使用者外掛程式。它會在載入時於整個執行個體範圍內取代該內建項目。

### 非同步橋接

公開給 Lua 的唯一原生基本操作是 `__af_op(json_request)`。它會讓出協程；Go 執行該請求（直接執行或交給工作者池）；然後協程以 `(json_response, ok)` 恢復執行。prelude 把這個原始基本操作包裝成下文記載的易用資料表。由於每次呼叫都會讓出執行，忙碌的執行個體依然成本低廉：不會有任何作業系統執行緒因等待模型、資料庫或 HTTP 回應而被阻塞。

```
-- prelude 替你做的事（概念上）：
local function op(req)
  local resp, ok = __af_op(json.encode(req))
  if not ok then error(resp, 0) end
  return json.decode(resp)
end
```

會阻塞的操作包括 `send`、`llm.chat`、`tools.run`、所有 `store.*`、所有 `shell.*`、`http.request`、`mail.*` 操作、`agent.*` 以及 `scheduler.*`。`inbox`、`log`、`agent.info` 等快速操作則直接在 actor goroutine 上執行。

### 訊息生命週期

`session.inbox()` 會暫停協程，直到工作階段信箱收到訊息。訊息是一個資料表，至少包含：

```
{
  id = "msg-...",
  type = "user",           -- "user" | "system" | "timer" | "cron" | "agent" | "shell.result" | ...
  from = "webhook:alice",  -- 由核心標記的傳送者
  text = "hello",
  channel = "webhook",
  reply_to = "...",
  ts = 1234567890,
  -- 由核心指定，迴圈永遠無法控制。user_uuid 是此輪次所屬的租戶；
  -- 沒有任何東西標記該訊息時此欄位不存在，訊息不屬於任何租戶
  -- （未連結的帳號）時為 ""。身分請從這裡讀取，
  -- 絕不要從 payload 讀取：跳轉的 payload 是依呼叫端的 Lua 重建的。
  provenance = { kind = "agent", principal = "agent:planner", user_uuid = "u_xxxx" }
}
```

`session.send(text)` 透過目前的頻道回覆（即目前訊息的頻道與回覆對象）。`session.push(channel, reply_to, text)` 會主動傳送，需要 `channel.push` 能力。`session.push_user(uuid, text)` 依身分 UUID 推送給某位使用者。`session.exit()` 會乾淨地結束工作階段。

### 撰寫迴圈外掛程式

最精簡的回聲迴圈如下（每個程式碼區塊都以 prelude 版本開頭）：

```
-- af-prelude-version: 1
log.info("echo agent online")

function loop()
  while true do
    local msg = session.inbox()
    log.info("message from " .. (msg.from or "?"))
    time.sleep(0.3)
    session.send("echo: " .. (msg.text or ""))
  end
end
```

ReAct 風格的迴圈會呼叫 LLM，並視需要呼叫工具：用 `memory.recall` 取得歷史，將 `tools.list` 的工具定義傳給 `llm.chat`，用 `tools.run` 執行傳回的工具呼叫，再用 `session.send` 回覆。把它寫成一般的迴圈檔案，並將 `loop:` 指向它即可。

### 內建 Lua API 參考

下列所有資料表只在代理的能力允許時才會注入。呼叫已停用的 API 會引發錯誤。

#### `session` — 核心

```
session.inbox() -> Message            -- 暫停直到下一則信箱訊息
session.send(text)                    -- 回覆目前的傳入訊息
session.send(text, { attachments = parts }) -- 附帶媒體傳送（見下文）
session.reply(t)                      -- 以結構化答覆結束此輪次：
                                      -- { text, tool_calls, finish_reason, usage, reasoning }
session.send_delta(text)              -- 串流答覆的一個片段
session.push(channel, reply_to, text) -- 主動傳出；能力：channel.push
session.push(channel, reply_to, text, { attachments = parts })
session.push_user(uuid, text)         -- 主動傳出給某個使用者 UUID；能力：channel.push
session.exit()                        -- 結束此工作階段

session.state.set(key, value)         -- 能力：session.state（需宣告）
session.state.get(key)                -- 從未設定過該鍵時為 nil
session.state.delete(key)
session.state.list()                  -- 此工作階段的 { key = value }
```

頻道接收了媒體時，訊息會帶有一個由媒體部分描述元（`{type, mime, handle, name}`）組成的 `attachments` 陣列。請原封不動地將它們轉交給 `llm.chat` 或 `session.send`；執行環境會在供應商／頻道邊界把 Blob 控制代碼（"media:<sha256>"）解析為位元組，原始媒體永遠不會進入 Lua。

`session.reply(t)` 是回覆的結構化形式：`tool_calls`（`{ id, name, args, arguments }`）、`finish_reason`（`stop`、`tool_calls`、`length`、`content_filter`）、`usage` 和 `reasoning` 會傳到能表達它們的頻道，而其文字與 `session.send` 走相同的安全檢查與訊息日誌路徑。`llm.forward` 或 `llm.chat` 的結果可以原樣傳入：其 `thinking` 會被當作回覆的推理內容，而只有設定為公開推理內容的頻道才會傳送它。工具呼叫的 `arguments`（模型自己產生的 JSON 文字）會優先於其 `args` 資料表傳送，因為 Lua 資料表無法區分 `[]` 與 `{}`；若你修改了某個呼叫的 `args`，請將其 `arguments` 設為 `nil`。每個輪次最多一個回覆；在無法表達工具呼叫的頻道上，帶有工具呼叫的回覆會直接失敗，而不是默默丟棄它們。文字被傳出安全檢查拒絕的回覆，會以 `finish_reason = "content_filter"`、不帶工具呼叫也不帶推理內容的形式送出；其內容是設定檔的替代回覆文字（如有），否則為空。

`session.send_delta(text)` 用於串流答覆。只有在安全設定檔不可能丟棄傳出內容時，引擎才會即時轉送片段；否則會先保留，並在輪次結束時釋出檢查過的完整內容。同時使用串流並呼叫 `session.reply` 的迴圈，應讓回覆帶上完整文字：無論用戶端是否要求串流，回覆只會補上片段尚未帶出的部分。

`session.state` 是僅屬於此工作階段的小型持久鍵值儲存區：一個游標、一個步驟計數、上一次說過的話。它在重新啟動後仍然保留，在叢集中則跟著工作階段移到下一個持有它的執行個體，因為資料列存放在共用儲存區中。鍵可以包含字母、數字與 `. _ - :`，最長 128 個字元；值可以是任何能以 JSON 編碼的 Lua 值，最大 64 KiB；一個工作階段最多可持有 256 個鍵。此功能需主動啟用：請在代理的 `capabilities` 中宣告 `session.state`，因為它是一個持久化介面，不應預設開放。

#### `llm` — 能力：`llm.chat`

```
llm.chat(messages, opts?) -> { text, thinking, thinking_blocks, usage, tool_calls }
llm.stream(messages, opts?) -> iterator
llm.forward(opts?) -> { text, tool_calls, finish_reason, usage, thinking, thinking_blocks }
llm.embed(inputs, opts?) -> { vectors, usage }   -- 字串和／或部分資料表
llm.rerank(query, documents, opts?) -> { results } -- results: [{ index, score }]，從 0 起算

-- opts: { model = "default", temperature = 0.7, max_tokens = 4096,
--         thinking = "low"|"medium"|"high"|"xhigh"|"max"|"off",
--         tools = [...], tool_choice = "auto" | "none" | "required" | "function:<name>" | { name = "<name>" },
--         parallel_tool_calls = true, top_p = 0.9, stop = {...} (or one string), seed = 1,
--         response_format = { type = "json_schema", json_schema = {...} } }

-- forward opts: { model, messages, prepend, append, tools, params, max_tokens_cap }
```

`llm.forward` 是 OpenAI 頻道的直通呼叫。不帶參數時，它會把用戶端自己的請求（訊息、工具、tool_choice 與取樣參數）轉送給此輪次所用的模型（代理的模型，或使用者資料中的模型），走的是與 `llm.stream` 相同的計量串流路徑：預算與配額都適用，用量帳本會記下一筆 kind 為 `forward` 的資料列。`opts.messages` 會取代用戶端的訊息清單，`prepend`／`append` 在其前後加入輪次，`tools` 加入定義，`params` 覆寫取樣參數，`max_tokens_cap` 則限制用戶端的值。供應商無法表達的選項會被捨去而不是拒絕，但 `response_format` 和具名的 `tool_choice` 例外，它們會讓呼叫失敗。結果中的 `finish_reason` 是模型自己的值（輸出上限截斷答覆時為 `length`），每個工具呼叫除了解碼後的 `args`，還帶有 `arguments`，即模型的引數文字。只有在用戶端要求串流且代理的安全設定檔沒有任何過濾器時，它才會即時轉送文字片段；否則會傳回完整文字供 `session.reply` 使用。它不會結束 HTTP 回應。模型呼叫失敗時會引發以 `upstream_error:` 開頭的錯誤。若不在 OpenAI 頻道的輪次中且沒有 `opts.messages`，它會傳回錯誤。`openai.request()` 以資料表傳回完整請求，JSON 超過 4 MiB 時會拒絕：請改為轉送該請求，而不是把它具體化。兩者都需要 `llm.chat` 能力。`llm.chat` 的回覆與 `llm.stream` 的最後一個訊框也都帶有 `finish_reason`。

訊息是由 `{ role, content, tool_calls?, tool_call_id?, tool_result?, thinking_blocks? }` 組成的陣列。**多模態輪次**使用 `{ role, parts = {...} }`；每個部分都是一個資料表：

```
{ type = "text",  text = "..." }
{ type = "image"|"audio"|"video"|"file", mime = "image/png", handle = "media:...",
  name = "photo.jpg" }   -- handle 來自 msg.attachments（也可用 data base64 / url）
```

供應商支援矩陣（不支援的部分類型會如實報錯；來源可以是 base64 `data`、`url` 或供應商的檔案參照）：

| 供應商 | 圖片 | 音訊 | 影片 | PDF／檔案 |
| --- | --- | --- | --- | --- |
| anthropic | ✅ base64/url | ❌ | ✅ base64/url（MiniMax 影片區塊） | ✅ document（base64） |
| openai (chat) | ✅ image\_url | ✅ input\_audio (wav/mp3) | ✅ video\_url，MiniMax/Kimi/GLM 慣例 | ✅ file\_url (url)，GLM |
| openai-responses | ✅ input\_image | ✅ input\_audio (wav/mp3) | ✅ input\_video（url／檔案參照） | ✅ input\_file（data 或 file\_id） |
| gemini (interactions) | ✅ files api | ✅ files api | ✅ files api | ✅ files api |

影片來源遵循生態系的慣例，例如 MiniMax `mm_file://<file_id>`、Kimi `ms://<file_id>`，並以 URL／檔案參照的形式原樣傳遞。SDK 的型別化請求聯集未涵蓋的形式（`video_url`、GLM 的 `file_url`、Anthropic 影片區塊、Responses 的 `input_video` 與 `input_audio` 項目）是用 `param.Override` 建構的；這是 SDK 本身的原始 JSON 逃生口，在未設定任何型別化變體時會原樣序列化。Blob 儲存區的 `handle` 會自動解析為 base64。

對 gemini 而言，每個媒體部分都會上傳到 **Gemini Files API**（可續傳上傳）並以 URI 參照，絕不會以內嵌 base64 傳送。`data`（base64）、`url` 與 Blob 儲存區 `handle` 來源都可使用（url 會先下載）；相同位元組的重複上傳會在檔案的 48 小時存續期間內依內容雜湊去重複，期滿後 Google 會自動刪除檔案。Anthropic／openai 也直接接受 `url` 來源。`llm.stream` 傳回兩個值：`next_delta, close`。請直接呼叫 `next_delta()`（結束時傳回 nil），絕不要放在泛型 `for ... in` 中：迭代器會跨越橋接讓出執行，而 Lua 5.1 不允許從泛型 for 的 C/Go 邊界這麼做。提前離開時請呼叫 `close()`；迭代器在結束時會自行關閉。它只傳回文字片段；需要用量與工具呼叫時請使用 `llm.chat`。

**多模態嵌入**遵循 Jina 慣例：`llm.embed` 接受字串與部分資料表混合的陣列，帶有媒體的批次會把每個項目序列化為型別化文件（`{"text"}`、`{"image"|"video"|"audio"|"pdf": url|base64}`），也就是 `jina-embeddings-v5-omni-small`（文字、圖片、影片、音訊與 PDF 共用同一個向量空間）和 `jina-clip-v2` 使用的形式。純文字批次仍是一般字串，因此任何相容 OpenAI 的 `/embeddings` 端點（OpenAI、Ollama、vLLM、TEI、OpenRouter）看到的都是與以往完全相同的請求。選項：`task`（`retrieval.query` | `retrieval.passage` | `text-matching` | `clustering` | `classification`）、`dimensions`（Matryoshka 截斷）、`merged = true`（把所有輸入合併成一個嵌入，即 Jina 的內容群組；PDF 只能單一輸入）。Anthropic 與 Gemini 供應商在這裡沒有嵌入 API，會明確失敗。`llm.rerank` 需要一個 `provider: rerank` 的模型項目，且該項目支援 `POST {base_url}/rerank`（TEI、vLLM、Jina、Cohere）。代理宣告了預算時，`llm.embed` 會依 Token 計量。`gemini` 供應商（Interactions API）不支援串流：在它上面使用 `llm.stream`，會以單一片段產生完整回覆。

**思考內容（回應端）。** 供應商送來的任何推理內容都會被動地呈現在回覆上：`reply.thinking` 是人類可讀的文字（沒有時為空字串），`reply.thinking_blocks` 是供應商的原始區塊（Anthropic 的 `{type="thinking", thinking, signature}`／`redacted_thinking`、Responses 的推理項目、Gemini 的 `thought` 部分；在 Chat Completions 上為 nil，因為那裡的推理內容只是一個純字串），而 `usage.reasoning` 會在供應商另外回報時計算思考 Token。是否有推理內容是依供應商、依模型別名而定的執行環境特性，並非契約：服務端的組態可能對推理 Token 計費，卻完全不在串流中提供推理內容，所以 `usage.reasoning` 才是可靠的稽核訊號，而 `reply.thinking` 為空本身並不代表故障。引擎索取的推理內容絕不超過思考等級本身所要求的，也從不儲存它；要不要永久保存由迴圈決定。在啟用思考的 Anthropic 多回合工具使用中，請把區塊原樣傳回到 assistant 輪次上：`thinking_blocks = reply.thinking_blocks`（否則 Messages API 會拒絕這個接續請求；其他供應商會忽略此欄位）。

```
-- 多模態嵌入（透過 api.jina.ai/v1 使用 jina-embeddings-v5-omni-small）
local r = llm.embed({
    "find me the cat photo",                    -- 文字
    { type = "image", handle = msg.attachments[1].handle },  -- Blob 儲存區媒體
  }, { model = "omni", task = "retrieval.query", dimensions = 512 })

-- 為整筆多模態記錄產生單一向量（當設定檔指定了 omni embed_model 時，
-- memory.write 會對帶有附件的記錄這麼做）：
local r = llm.embed({ "trip to the beach", img_part }, { model = "omni", merged = true })
```

#### `agent` — 能力：`agent.send`、`agent.request`、`agent.spawn`

```
agent.info() -> { name, session_id, address, model, instructions,
                  history_budget, memory, shell, skills, capabilities,
                  memory.stores[name] = { backend, table, features, locality }, ... }
agent.config() -> { name, model, instructions_path, instructions_prompt?,
                    prompts = {  = "", ... },
                    goal = {type, success_signal, max_turns, on_goal_met}?,
                    extras = { ... } }
agent.send(address, payload)
agent.request(address, payload, timeout?) -> Message
agent.reply(request_id, payload)
agent.list() -> [{ session_id, agent, parent_id }]
agent.spawn(profile) -> { address, session_id, agent }
```

儲存區的 `locality` 是 `deployment`、`region` 或 `instance`。持久的對話資料應放在涵蓋整個部署或整個區域的儲存區；執行個體本機的儲存區是一種快取，其內容會隨處理程序結束而消失（在 regional 單元中，放在這種儲存區上的持久儲存區除非設定 `locality_ok: true`，否則驗證時會被拒絕）。

`agent.spawn` 需要一個 `profiles.agent` 範本；子代理的能力是父代理與範本能力的交集。早期版本接受的第二個引數目前保留、不會套用；請改為設定範本。

`agent.config()` 以資料形式傳回此代理自己的設定檔：組態名稱、模型、指令的*路徑*（若系統提示詞來自 `prompts` 登錄，則為 `instructions_prompt`，此時 `instructions_path` 為空）、部署中已解析且依名稱索引的 `prompts` 登錄、提升為 `goal` 的設定檔 `extras.goal`，以及其餘的 `extras`（pm 選項區塊、工作流程名稱…）。其中的密鑰參照會呈現為不透明的標記（`{env="VAR"}`／`{cred="service"}`）：已解析的密鑰永遠不會跨越 Lua 橋接。

#### `user` — 能力：`users`

```
user.current() -> { registered, id, display_name, identities }
user.usage() -> { registered, input, output, cached, reasoning, calls,
                  failed, billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { service, present }
user.get(id) -> { ... }    -- 僅限維護來源
user.list() -> [ { ... } ] -- 僅限維護來源
```

關於此輪次所屬使用者的唯讀資訊。`users` 是預設能力，所以迴圈不必宣告任何東西就能呼叫這些函式。迴圈只會得知名稱與頻道，絕不會得知電子郵件，也絕不會得知憑證的值：`has_credential` 回答的是某個金鑰是否存在，而不是它的內容。`get`／`list` 僅限由引擎觸發的維護來源（`system` 或 `scheduler` 輪次）使用，這讓逐使用者的維護迴圈成為可能，又不會把某位使用者的資料暴露給另一位使用者的輪次。

#### `runtime` 與 `credential` — 以資料形式呈現的部署組態

```
runtime.triggers() -> { ok, triggers = [ { name, kind, run_on_boot,
                          target = { profile }, payload,
                          event = { channel, match }   -- kind = "event"
                          | cron                        -- kind = "cron"
                          | every } ] }                 -- kind = "every"

credential.get(name) -> { ok, name, value }              -- 僅限允許清單中的名稱
```

`runtime.triggers()` 傳回合併後的 `triggers/*.yaml`（或頂層 `triggers:`）清單，於啟動時建立快照：請從引擎讀取你的 CRON\_TASKS/ROUTES，而不是把資料表寫死在 Lua 原始碼中。它在**兩個** Lua 介面中的回應方式相同：代理迴圈與閘道路由處理常式（路由器的操作分派會提供完全相同的酬載），因此事件路由可以讀取驅動它的觸發器清單。運算式會原樣傳遞，其形式也不受引擎排程影響：`every:`／`cron:` 項目本身也會由引擎觸發（請參閱組態參考中的 `triggers`），所以迴圈只在需要*觀察*部署的排程時才需要這個呼叫。`credential.get(name)` 會擷取一個引擎全域儲存的密鑰，但**僅限**代理的設定檔在 `credentials: [...]` 下列出了該名稱時（清單為空或不存在則拒絕，並以錯誤說明原因）。每次存取都會記錄（代理、名稱、允許／拒絕）並計數（`agentflow_credential_gets`／`agentflow_credential_gets_denied`）；其值永遠不會寫入訊息日誌，也不應在回覆中原樣輸出。

`router.state` 是路由處理常式自己的持久鍵值儲存區，用來存放處理常式不能忘記的東西，例如某個聊天上次被路由到哪個代理。它位於執行環境儲存區中，所以每個執行個體讀到的值都相同，重新啟動或容錯移轉也不會遺失任何資料：`router.state.get(key)`（從未設定過該鍵時為 nil）、`router.state.set(key, value, {ttl_seconds = n})`、`router.state.delete(key)` 以及 `router.state.list(prefix)`。值可以是任何能以 JSON 編碼的 Lua 值，最大 64 KiB；鍵可以包含字母、數字與 `. _ - :`（128 個字元）。除非處理常式要求不同的存留期（`ttl_seconds = 0` 會保留到被刪除為止），否則鍵會在最後一次寫入後保留 30 天，這也是處理常式以高基數值作為鍵時儲存區得以受限的原因。此命名空間只定義在路由器的狀態中，因此代理迴圈無法存取它；而且它提供的是一致性，而非序列化：兩個執行個體可能交錯對同一個鍵進行讀取－修改－寫入，但傳遞本身由工作階段租約序列化。

#### `scheduler` — 不受能力管控

```
scheduler.every(interval) -> timer_id    -- 單位為秒；隨工作階段消失
scheduler.after(delay) -> timer_id
scheduler.cron(expr) -> timer_id          -- cron 子集（見下文）
scheduler.cancel(timer_id)
```

計時器會把一則 `Message{ type = "timer" }` 排入所屬工作階段的信箱，並帶有 `msg.payload.timer_id`，讓同一個工作階段能區分自己的各個計時器。它們從不在計時器 goroutine 中呼叫 Lua，所以 VM 始終保持單執行緒。`scheduler.cron` 只支援 5 欄位 cron 的**子集**：`*/N * * * *`（每 N 分鐘）以及 `30 9 * * *` 這類固定分鐘／小時的形式；日、月與星期欄位必須是 `*`，而且 `*/N` 分鐘不能搭配固定的小時（間隔無法錨定在某個小時上，否則會被默默捨棄）。其他寫法一律拒絕，而不是重新解讀；需要更複雜的排程時，請使用 `every`／`after`（或自行重新排程）。

這些是*工作階段範圍*的計時器：它們隨工作階段一起消失。**不存在 `scheduler` 能力**：這些操作在 actor 自己的分派中處理，從不經過能力閘門，所以任何迴圈無論宣告了什麼，都可以註冊計時器。閘門涵蓋的是可透過處理常式對應表觸及的操作（`llm.*`、`memory`／`store.*`、`tools.*`、`shell.*`、`http.request`、`mail.*`、`agent.*` 位址）；計時器是引擎傳遞的一則訊息，而不是一項權限。在 YAML 中宣告的部署全域排程（`every:`／`cron:` 觸發器）則屬於引擎基礎設施：由引擎觸發，不涉及任何能力，並以 `Message{ type = "cron" }` 傳遞，所以迴圈不必自己執行計時器迴圈就能被排程。迴圈需要自行判斷時間時，請使用 `os.time()`／`os.date()`（UTC）。另請注意，計時器狀態存放在記憶體中：重新啟動後，所有計時器都會被遺忘，直到其迴圈重新註冊為止。

#### `memory` 與 `store` — 能力：`memory`

```
memory.write(record) -> boolean          -- 由 plugin:routing_table 路由
memory.recall(query, opts?) -> [ { key, value, meta } ] -- 目前作用中的召回處理常式

store.put(table, key, value, opts?)      -- opts: { ttl = seconds, vector = {...} }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records }
store.delete(table, key)
```

儲存區查詢接受 `{ kind = "all" | "prefix" | "text" | "vector" | "time_range", ... }`；向量查詢使用 `{ kind = "vector", vector = {...}, k = n }`。對後端不具備 `vector` 功能的儲存區傳入 `vector` 會明確報錯，絕不會默默捨棄。代理記憶請優先使用 `memory.write`／`memory.recall`；只有在需要直接存取資料表時才使用 `store.*`。

**逐使用者範圍（私有儲存區）。** 源自頻道的輪次只讀寫自己的使用者範圍，外加 `service` 資料列（由沒有自己租戶的情境寫入：引擎觸發的工作、未連結的帳號，以及以 `inherit_user: false` 選擇退出的代理）和 `legacy` 資料列（在此功能推出前寫入：升級前的歷史仍可由該代理的所有使用者讀取，且不會再被寫入）。引擎觸發的情境（`system`／`scheduler` 來源，例如 distiller 這類夜間維護）會以可看見範圍的鍵讀取所有範圍，並以明確指定範圍的方式寫入；維護工作階段可以透過 \`store.scopes\` 列舉某個繫結的使用者範圍（僅限維護：頻道與代理跳轉工作階段會被拒絕）。這由引擎強制執行，不需要在迴圈程式碼中過濾。綁定記憶池的儲存區（`pool: <name>`）則改由租戶授權管控，而私有儲存區可以用 `scope: agent` 為每個代理重新加入一個記憶池。

#### `tools` — 能力：`tools`

```
tools.list() -> [ToolSpec]
tools.run(name, args, opts?) -> ToolResult
-- opts: { confirmed = true, confirm_id = "..." }

tool.def({ name = "lua:shout",
           description = "Shout the text back",
           params = { type = "object",
                      properties = { text = { type = "string" } },
                      required = { "text" } },
           handler = function(args)
             return { ok = true, shouted = string.upper(args.text) }
           end })
```

若某個工具宣告了 `needs_confirm: true`，第一次呼叫會傳回 `needs_confirm` 回應。執行環境會詢問使用者，然後以 `opts.confirmed = true` 重新呼叫該工具。你的迴圈只會看到最終結果。（等待確認的時間有上限：十分鐘，之後呼叫會被拒絕。排程器輪次上需要確認的工具沒有頻道可以回答，若無限期等待，工作階段就會永遠停在那裡。）

**工具迴圈由迴圈自己負責。** 引擎在被要求時執行工具並傳回結果；它不會主導對話。帶著 `tools` 呼叫 `llm.chat`、用 `tools.run` 執行傳回的呼叫、把結果以 `role = "tool"` 輪次送回後再次詢問，這些都是程式碼區塊的工作，包括繞*第二*圈，而這正是最容易忘記的：模型看到第一個結果後若再要求另一個工具，除非程式碼區塊再次檢查 `reply.tool_calls`，否則它拿不到。迴圈要保存的內容也是如此：工具呼叫及其結果是下一個輪次應該看到的歷史；只儲存 assistant 文字的程式碼區塊，召回的對話裡模型會提到一個已不存在的工具結果。

`tool.def` 在 Lua 中宣告一個模型可見的工具，是取代手動拼湊額外工具定義的引擎標準做法。宣告的工具會合併到 `tools.list` 中（與 Go 工具相同的 `{ type = "function", function = {...} }` 形式，因此可原樣傳入 `llm.chat`），而 `tools.run` 會分派給 Lua 處理常式。處理常式在迴圈協程上同步執行，本身也可以呼叫任何操作（`llm.chat`、`memory.*`…）。宣告的名稱會在清單與執行兩方面遮蔽同名的 Go 註冊工具（遮蔽在 `skills` 過濾之後才套用，所以此代理看不到的工具絕不會壓過它同名的 Go 工具）。處理常式的錯誤會以 `{ ok = false, error = ... }` 傳回，而不會終止迴圈。宣告的工具是迴圈自己的程式碼：Go 工具原則（`forbidden`、`needs_confirm`、`permission: forbidden`）仍然不會管控它們。請在程式碼區塊的頂層、`loop()` 之前宣告。

**可見性**：宣告的工具依照與 Go 工具完全相同的規則，由 `skills` 和 `tools.policy.default` 過濾：`skills` 列出它時就會顯示；`skills` 非空但未列出時隱藏；`skills` 為空時依 `default` 全部顯示或全部隱藏。這讓一個共用的迴圈目錄可以攜帶只有部分代理能取得的工具。對於設定了 `default: none` 且沒有 `skills` 的部署，這是一項行為變更：宣告的工具以往一律列出，現在則不會。`tools.run` 套用相同的規則：代理看不到的宣告名稱會落到 Go 路徑，並以「不可用」被拒絕，與看不到的註冊工具完全一樣。註冊工具一向在分派時強制執行，而 `tools.run` 是幻覺或注入的工具呼叫所經過的、面向模型的路徑，所以兩者一致。想使用自己處理常式的迴圈，請直接呼叫該函式。

**覆寫宣告工具的文字**：`tools.policy.overrides` 可以指名一個宣告的工具，把它面向模型的文字從程式碼區塊移到組態中：說明（字面值，或來自 `prompts` 登錄的 `{prompt: <key>}`）以及 `params.<param>.description`。沒有任何覆寫指名該工具時，傳給 `tool.def` 的規格仍是後備值，而覆寫絕不會碰觸宣告結構描述的型別或 `required`。解析發生在建立 `tools.list()` 時，因此重新載入的 `file:` 提示詞會在下一次呼叫時生效，不需重新啟動。覆寫絕不會新增或移除清單項目：可見性來自 `skills` 與原則預設值，絕不來自覆寫。已宣告但被隱藏的工具仍算是已宣告，所以指名它的覆寫不會被回報為沒有指名任何東西。

#### `shell` — 能力：`shell.exec`

```
shell.run(cmd, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out, truncated,
                             project, environment, changed, deleted, conflicts, skipped }
shell.start(cmd, opts?) -> { id, state = "running", ... }     -- 結果會送到收件匣
shell.status(job_id)    -> job
shell.cancel(job_id)    -> job

-- opts: { project = "site", environment = "python", workdir = "app",
--         env = { KEY = "val" }, timeout = 120 }             -- timeout 單位為秒

-- 控制代碼：SSH 是真正的連線，沙箱則是相容性墊片。
shell.spawn(opts) -> { id, provider, state }
shell.exec(id, cmd, opts?) -> the same result as shell.run
shell.write(id, path, content)
shell.destroy(id)
```

**沙箱 shell 這種東西並不存在。** `shell.run` 是在一個一次性沙箱（[gVisor](https://gvisor.dev) `runsc` 或 [microsandbox](https://github.com/superradcompany/microsandbox) `msb`）中執行的一個命令，沙箱為該命令啟動、執行完即銷毀。沙箱會得到兩樣東西：一個宣告的**環境**作為工具鏈根目錄，以及檔案儲存區中的一個**專案**，以讀寫方式掛載在 `/workspace`，命令就從那裡開始執行。`/workspace` 以外的暫時狀態在結束後即捨棄。沒有網路。命令結束時，引擎會比較專案目錄與它當初放進去的內容，並把差異寫回專案的工作樹，也就是 `files.*` 讀寫的同一棵樹。何時 `files.commit` 由迴圈決定。

所以迴圈要遵守的規則很簡短：

- **命令在專案中寫入的內容會保留，其他一概不保留。** 背景處理程序、shell 變數、安裝的套件或 `/workspace` 以外的檔案，都不會留到下一次呼叫。
- **非零的 `exit_code` 與 `timed_out` 都是結果**，就像 `http.request` 傳回的 HTTP 500。完全無法執行的工作，例如此主機上沒有沙箱、未知的環境、佇列逾時、沙箱因超出記憶體上限而被終止，則會引發錯誤，所以請用 `pcall` 包住呼叫。命令被終止前寫入的內容仍會被儲存。
- **在同一個執行個體上，每個專案一次只執行一個工作**；對同一專案的第二次呼叫會等待第一次完成（最長 `runtime.sandbox.queue_timeout`）。
- **寫回絕不會覆蓋較新的檔案。** 工作變更過、但在它執行期間也被別人變更的路徑，會保留儲存區中的版本，並列在 `conflicts` 中。無法儲存的路徑（超過單一檔案上限、超過工作的寫回限制、符號連結）會連同原因列在 `skipped` 中，並從目錄中移除，讓目錄永遠不會含有儲存區沒有的東西。
- **任何執行個體都能執行下一個命令。** 執行個體上的專案目錄只是檔案儲存區的快取。

設定的層次：呼叫的 `opts` 會覆寫專案的 `.agentflow/shell.yaml`，後者又覆寫代理的 shell 設定檔。環境必須是部署所宣告的其中之一；時間上限受 `runtime.sandbox.max_timeout` 限制。專案的資訊清單可以設定 `environment`、`env`、`workdir`、`timeout` 和 `ignore`，除此之外別無其他：

```
# .agentflow/shell.yaml，位於專案的工作樹中
environment: node
env: { NODE_ENV: test }
workdir: app
timeout: 120s
ignore: ["node_modules/", "dist/"]
```

符合 `ignore`（來自設定檔或資訊清單）的路徑既不會被儲存，也不會被清除：它們留在產生它們的執行個體上，作為盡力而為的快取（相依套件目錄、建置快取），在其他執行個體上則就是不存在。

**背景工作。** `shell.start` 會將同樣的工作排入佇列並立即返回，時間上限較長（`runtime.sandbox.background_max_timeout`）。其結果會以一則 `type = "shell.result"` 的訊息送到工作階段的收件匣，工作內容放在 `msg.payload` 中（`state` 為 `finished`、`failed` 或 `cancelled`；`stdout` 與 `stderr` 為最後 16 KiB）。結果輪次以啟動該工作的使用者身分執行。`shell.status` 可在工作執行期間從任何執行個體讀取它，`shell.cancel` 則在執行它的執行個體上將其停止。所在執行個體已終止的工作會回報為 `failed`，且不會重新執行。工作階段結束時，其背景工作會被取消。

**控制代碼。** 使用 `provider = "ssh"` 的 `shell.spawn` 會開啟一條連到真實遠端機器的連線（`host`、`user`、`password` 或 `key_file`）；`exec`、`write` 和 `destroy` 都作用於它，它存在於撥出連線的執行個體中，並在工作階段結束時關閉。SSH spawn 可以帶上 `project` + `ref` 以及一個 `volumes = { "x:/dir" }` 項目，在控制代碼回報就緒前把檔案快照複製到遠端機器的 `/dir`，或者用 `scratch_mount` 複製工作階段的暫存檔案。

不指定供應商的 `shell.spawn` 會傳回一個*沙箱控制代碼*，它的存在是為了讓針對舊版容器 shell 撰寫的迴圈繼續運作：它只是把收到的選項編碼進 id 而已。對它呼叫 `shell.exec` 會執行一個工作，`shell.write` 會寫入專案，`shell.destroy` 則什麼也不做。它不需要任何儲存空間，在任何執行個體上都能運作。有三個選項的意義改變了：`image` 指名一個宣告的環境，`volumes[1]` 的容器路徑是專案的掛載位置（取代 `/workspace`），而 `"none"` 以外的 `network`、`"main"` 以外的 `ref` 以及 `scratch_mount` 都會被拒絕，並附上說明原因的訊息。

繫結了 shell 設定檔（`agents.<name>.shell`）的代理也會取得會隱含解析該設定檔的登錄工具，讓模型永遠不必處理控制代碼：`builtin:shell.exec`（`{ command }` → `shell.run` 的結果）、`builtin:shell.write`（`{ path, content }`）、`builtin:fs.read`（`{ path }` → `{ ok, content, path, truncated }`）、`builtin:fs.write`（`{ path, content }`）和 `builtin:shell.destroy`。使用沙箱設定檔時，檔案工具會直接讀寫專案（不會為它們啟動沙箱），路徑可以相對於專案，也可以位於 `/workspace` 之下。使用 SSH 設定檔時，這些工具每個工作階段保留一條連線，在第一次使用時開啟。沒有繫結設定檔的代理會如實得到「不可用」，無法執行沙箱的主機也一樣；沒有 `shell.exec` 能力的代理則完全看不到這些工具，即使在 `tools.policy.default: all` 之下也是如此。除了 `fs.read` 之外，其餘預設都是 `needs_confirm: true`，可依部署透過 `tools.policy.overrides` 放寬。

`builtin:fetch` 是用於 API 與一般網頁的通用 HTTP 用戶端，不需要任何帳戶或憑證，因此與 `web_search` 和 `browser` 不同，它永遠不會如實回報「不可用」。選項：`url`（必填）、`method`、`headers`、`body`（原始內容）或 `json`（會序列化；除非已有標頭設定，否則會設定 `Content-Type`）、`query`、`timeout_ms`、`follow`／`max_redirects`、`user_agent`、`max_bytes`、`insecure`，以及 `save_to`（回應本文也會存入此工作階段的檔案暫存空間，可透過 `files.scratch.read` 讀取）。它傳回 `{ok, status, status_text, headers, body, final_url, redirects?, content_type?, content_length?, size, truncated?, tls_verified, time_cost_ms}`，*任何* HTTP 狀態都是如此，因為 404 是模型應該讀到的答案，而不是工具失敗；`ok` 表示請求是否完成，`status` 表示伺服器怎麼說。

本文有大小上限（`max_bytes`，預設 1 MiB），結果一律回報真實的 `size`，截斷時會設定 `truncated`。這個上限不是可有可無的貼心設計：工具結果會整個交給模型，下游沒有任何東西會截斷它，而迴圈的 Token 預算器完全不計算工具結果，所以過大的本文會擠掉真正的對話歷史，而不是被修剪。目的地受上文所述的 `net` 防護限制，拒絕時會傳回指名該位址的錯誤，計入 `agentflow_http_private_blocked`，並記錄在發出呼叫的代理名下。

`insecure: true` 會略過 TLS 憑證驗證。它一律允許（沒有組態閘門），但絕不會悄悄進行：略過時會遞增 `agentflow_http_insecure_tls`、記錄一則指名代理與 URL 的 WARN，並在結果中回報 `tls_verified: false`。它只略過憑證檢查；位址防護仍然適用，因為對方聲稱自己是誰，與連線實際通往何處，是兩個不同的問題。此工具與 `web_search` 一樣是 `Autonomous: true`；想對它要求確認的部署，可以透過 `tools.policy.overrides` 設定 `needs_confirm`。

#### `http` 與 `os` — 能力：`net.http`

```
http.request(opts) -> { ok, status, headers, body, truncated? }
http.get(url, opts?)
http.post(url, body, opts?)

-- opts: { method, url, headers, body, json, query, timeout, auth,
--         save_to = "name" }  -- 本文也會存入此工作階段的檔案暫存空間
-- auth = { service = "..." } 會在請求時解析一個已儲存的憑證。

os.env(name) -> string | nil
os.time() -> number                        -- Unix 秒數（UTC）
os.date([format,] time?) -> table | string -- UTC；除非指定格式，否則傳回資料表
```

**掛鐘時間。** `os.time()` 傳回整數的 Unix 秒數，不接受任何引數（這不是 Lua 的 `os.time(table)`）。`os.date()` 傳回日期*資料表*：`{ year, month, day, hour, min, sec, wday, yday, isdst }`，其中 `wday` 1 = 星期日 … 7 = 星期六，`yday` 為 1–366，`isdst` 一律為 `false`；它接受一個選用的 Unix 秒數時間（預設：現在）。這個資料表可以用 JSON 編碼，所以能放在酬載中傳遞。傳入 strftime 字串即可改為取得文字：`os.date("%Y-%m-%d %H:%M:%S", t)`。支援的指示詞為 `%Y %y %m %d %e %H %I %M %S %p %A %a %B %b %j %w %u %Z %%`；未知的指示詞會原樣保留。**一切都是 UTC**：沙箱沒有本地時區，所以各欄位在每部主機上都相同，開頭的 `!`（`os.date("!*t")`）會被接受但不起作用。相對地，cron 觸發器是依 `runtime.timezone_offset_hours` 比對的；兩者之間的換算由你的迴圈負責。

#### `files` — 能力：`files`

```
files.put(project, path, content|{data=base64, handle=..., mime=...}, {mime}?) -> { ok, entry }
files.read(project, path) -> { ok, entry }        -- entry: {path, handle, size, mime, ts, revision}
files.list(project, prefix?) -> { ok, entries }
files.projects() -> { ok, projects }             -- 此輪次範圍內的專案
files.delete(project, path) -> { ok = true }
files.commit(project, {ref="main", message}?) -> { ok, commit }   -- 快照；父鏈即為歷史
files.checkout(project, ref|commit_id) -> { ok, manifest }        -- manifest: {commit, files=[{path,handle,size}]}
files.scratch.put(name, content|{data=base64, mime=...}) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

一個**以使用者為範圍的檔案儲存區**：具名的專案樹，其範圍由引擎依輪次的身分解析（源自頻道的輪次，以及繼承了租戶的委派輪次為 `user:<uuid>`，其他情況為 `agent:<name>`），迴圈永遠無法指名其他範圍。位元組存放在內容定址的 Blob 儲存區中（`files:` 組態；本機檔案系統或相容 S3 的儲存空間，未變更的檔案會在各快照之間免費去重複）；快照中繼資料（工作樹、提交、ref、暫存）存放在執行環境儲存區的 `files_meta` 資料表中，由引擎擁有，可用一般 SQL 檢視。版本控制是**引擎原生的快照，而不是 git 伺服器**：一次提交記錄完整的樹（路徑 → handle），附帶父指標與訊息，並移動一個具名的 ref；checkout 會解析 ref 或提交 id。原始位元組永遠不會跨越 Lua 橋接：`put` 接受 utf-8 內容或 base64 `data`，`read` 傳回 handle 與中繼資料，而具體化在引擎端進行：shell spawn checkout、`scratch_mount`，或 `http.request`／`builtin:fetch`／`builtin:browser` 的 `save_to`。暫存空間以工作階段為單位（依呼叫的工作階段而非使用者區分），存留期由 `files.scratch_ttl` 決定（預設 24h，讀取時延遲到期，並在啟動時清理）。一個 `files` 能力管控整個系列；它不在預設集合中，必須明確宣告。若啟動時儲存區被停用（例如無法解析 S3 憑證），每個操作都會如實失敗。

#### `mail` — 能力：`net.mail`

```
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
-- opts: { host, port = 993, user, mailbox = "INBOX", unseen = false,
--          limit = 10, auth = { service = "..." } }
-- messages: [{ seq, uid, flags, subject, from, to, date, body }]
-- 透過隱含式 TLS 連線；本文上限 256 KiB（超過時截斷並加上標記）

mail.smtp_send(opts) -> { ok, sent }
-- opts: { host, port = 465, user, from, to = {...}, subject, text_body, auth }
-- 連接埠 465 = 隱含式 TLS；587/25 = STARTTLS
```

與 `http.request` 一樣，`auth = { service = "..." }` 指名一個已儲存的憑證，由 Go 在呼叫時解析，密碼永遠不會跨越 Lua 橋接。主機、連接埠與使用者名稱由迴圈提供（通常透過 `os.env`），所以 Lua 原始碼中不會有任何密鑰。`net.mail` 不在預設能力集合中；代理必須宣告它。

#### 核心工具函式

```
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
json.encode(value) / json.decode(text)
time.sleep(seconds)        -- 讓出執行；絕不阻塞執行緒
```

### 撰寫支援程式碼區塊

支援程式碼區塊會在迴圈外掛程式之前於工作階段狀態中求值。它們可以定義供迴圈或其他支援程式碼區塊讀取的全域變數。例如，若要取代預設的召回策略，可以用 `plugins/recency.lua` 檔案遮蔽 `plugin:recency`，並在其中定義 `memory_recall_handler(query, opts)`。

```
-- plugins/recency.lua（遮蔽範例）
function memory_recall_handler(query, opts)
  local info = agent.info()
  local store_name = (info.memory and info.memory.stores.dialogue) or "dialogue"
  local res = store.query(store_name, { kind = "all" })
  return res.records or {}
end
```

### 錯誤處理

外部呼叫一律用 `pcall` 包住，這樣供應商出狀況或命令有誤時，工作階段才不會陷入當機迴圈：

```
local ok, reply = pcall(llm.chat, messages, { model = info.model })
if not ok then
  log.warn("llm.chat failed: " .. tostring(reply))
  session.send("⚠️ I couldn't reach my model right now.")
  return
end
```

橋接會以 JSON 字串傳回錯誤；prelude 會將其解開，讓堆疊追蹤指向你的程式碼，而不是 prelude。

### 確認與權限

工具與 shell 內建項目可以宣告 `needs_confirm`。第一次 `tools.run` 會傳回一個帶有 `needs_confirm = true` 和 `confirm_id` 的結果；actor 會詢問使用者，然後以 `opts.confirmed = true` 和相同的 `confirm_id` 重新呼叫該工具。你的迴圈只會看到最終結果，所以大多數情況下不需要自己處理確認。

### 限制與注意事項

- `llm.stream` 傳回的是閉包：請直接呼叫它，絕不要放在泛型 `for ... in` 中。
- 標準函式庫包含 base、coroutine、table、string、math、bit32、utf8 和 buffer。`io` 與 `debug` 無法使用，真正的 `os` 函式庫也不行：prelude 只提供 `os.env`、`os.time` 和 `os.date`（全部為 UTC，沙箱中沒有本地時區）。
- Lua 的原始網路存取只能透過 `http.request` 與 `mail.*` 操作。沒有 socket 函式庫。HTTP 路徑（此操作與 `builtin:fetch` 工具）會在撥號器中拒絕私有、回送、連結本機與保留的目的地，因此光有一個 URL 並不足以觸及主機自己的網路；請參閱 `net` 區塊。`mail.*` *不*受此防護涵蓋，仍會撥接迴圈指定的任何主機。
- 所有跨越橋接的值都以 JSON 編碼。資料表必須可序列化；函式與 userdata 無法跨越。
- 外掛程式支援熱重新載入：重新載入監看程式一律執行（`runtime.reload.watch` 沒有作用）。編輯迴圈檔案或指令檔案，會在下一個輪次從頭重新啟動工作階段協程；編輯 `gateway.route` 背後的檔案，會在訊息之間重建路由器的狀態（路由在那裡執行，而不是在工作階段中）；編輯被遮蔽的支援程式碼區塊，則會重新啟動每個執行中的工作階段，新的程式碼區塊就是在那時載入。這些都不需要重新啟動處理程序，而且只要新版本無法編譯，就都不會套用：已在執行的版本會保持不變。

### 實用範例

把以下任一範例放進迴圈檔案，並將代理的 `loop:` 指向它：

#### 工具迴圈

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local info = agent.info()
      local tools = tools.list()
      local ok, reply = pcall(llm.chat,
        { { role = "user", content = msg.text } },
        { model = info.model, tools = tools, tool_choice = "auto" })
      if ok and reply.tool_calls then
        for _, tc in ipairs(reply.tool_calls) do
          local r = tools.run(tc.name, tc.args)
          log.info("tool result: " .. json.encode(r))
        end
        session.send(reply.text or "done")
      elseif ok then
        session.send(reply.text or "")
      else
        session.send("error: " .. tostring(reply))
      end
    end
  end
end
```

#### Shell 迴圈

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local ok, r = pcall(shell.run, "echo hello from " .. msg.text)
      if ok then
        session.send("exit=" .. r.exit_code .. " stdout=" .. r.stdout)
      else
        session.send("the shell is not available: " .. tostring(r))
      end
    end
  end
end
```

#### 多代理請求

```
function loop()
  while true do
    local msg = session.inbox()
    if msg.type == "user" and msg.text then
      local ok, answer = pcall(agent.request, "agent:expert", { question = msg.text }, 30)
      if ok then
        session.send("expert says: " .. (answer.payload and answer.payload.answer or "?"))
      else
        session.send("expert unavailable: " .. tostring(answer))
      end
    end
  end
end
```
