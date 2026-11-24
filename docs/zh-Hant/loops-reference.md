## 指令碼外掛開發

AgentFlow 外掛是用純 Go 編譯（編譯前先剝掉 TypeScript 型別）、再由內建的純 Go JavaScript VM（`goja`）執行的 TypeScript / JavaScript 指令碼；另外還有用 TypeScript（`goja`）或 WebAssembly（`wazero`）寫的外部 API 擴充。YAML 決定哪個代理跑哪個外掛，行為由外掛定義。本套文件裡的長文指南（指令碼迴圈）講執行模型、完整 API、可執行的模式與一份坑點清單；這一節是參考摘要。

外掛分三種：

- **迴圈外掛** —— 定義全域或匯出的 `loop()` 函式，在它的整個生命週期內獨佔該工作階段的 VM。標準寫法是 `while (true) { const msg = session.inbox(); ... }`。
- **支援片段** —— 在迴圈外掛*之前*載入每個工作階段狀態。支援片段定義迴圈要讀的全域與輔助函式。`plugin:token_budget`、`plugin:routing_table`、`plugin:recency` 與 `plugin:semantic` 這些內建實作都是支援片段。
- **覆寫** —— 放在 `plugins/` 裡、與某個內建同名的使用者外掛。它在載入時於執行個體範圍內取代內建實作。

### 非同步橋

暴露給 JS 的核心原生原語是 `__af_op(json_request)`。指令碼呼叫某個操作時，VM 的工作 goroutine 掛在一個 Go channel 上；Go 執行請求（在 actor goroutine 上內聯執行，或丟到工作池裡）；接著 VM 帶著 `[json_response, ok]` 恢復。prelude 把這層原始原語包裝成下文記錄的凍結全域命名空間。因為暫停的工作階段 goroutine 是在 Go channel 上等待，忙碌的執行個體依然很輕 —— 沒有作業系統執行緒卡在 JS 裡等模型、資料庫或 HTTP 回應。

```ts
// prelude 替你做的事（概念上）：
function op(req: Record<string, unknown>) {
  const [resp, ok] = __af_op(json.encode(req));
  if (!ok) throw new Error(String(resp));
  return json.decode(resp);
}
```

### 訊息生命週期

工作階段的訊息長這樣：

```ts
{
  id: "msg-...",
  type: "user",           // "user" | "system" | "timer" | "cron" | "agent" | "shell.result" | ...
  from: "webhook:alice",  // 由核心標記的傳送者
  text: "hello",
  channel: "webhook",
  reply_to: "...",
  ts: 1234567890,
  // 由核心指定，迴圈永遠無法控制。user_uuid 是此輪次所屬的租戶；
  // 沒有任何東西標記該訊息時此欄位不存在，訊息不屬於任何租戶
  // （未連結的帳號）時為 ""。身分請從這裡讀取，
  // 絕不要從 payload 讀取：跳轉的 payload 是依呼叫端的指令碼重建的。
  provenance: { kind: "agent", principal: "agent:planner", user_uuid: "u_xxxx" },
}
```

`session.send(text)` 透過作用中頻道回覆（目前訊息所屬的頻道與回覆目標）。`session.push(channel, reply_to, text)` 是主動送出，需要 `channel.push` 能力。`session.push_user(uuid, text)` 依身分 UUID 推給使用者。`session.exit()` 乾淨地結束工作階段。

### 撰寫迴圈外掛

一個最小的 echo 迴圈長這樣（每個程式碼區塊都以 prelude 版本宣告開頭）：

```ts
// af-prelude-version: 2
log.info("echo agent online");

function loop() {
  while (true) {
    const msg = session.inbox();
    log.info("message from " + (msg.from || "?"));
    time.sleep(0.3);
    session.send("echo: " + (msg.text || ""));
  }
}
```

ReAct 風格的迴圈會呼叫 LLM，可選地呼叫工具：用 `memory.recall` 取歷史，把 `tools.list` 的工具定義傳給 `llm.chat`，用 `tools.run` 執行回傳的工具呼叫，最後用 `session.send` 回覆。把它寫成普通的 `.ts` 或 `.js` 迴圈檔案，把 `loop:` 指向它即可。

### 內建指令碼 API 參考

下面所有命名空間在載入時注入並凍結為唯讀；代理缺少所需能力時，受能力閘門控管的操作會在呼叫時直接報錯。完整的 TypeScript 宣告見 `docs/ts/af.d.ts`（文件與編輯器裡都有）。

#### `session` —— 核心

```ts
session.inbox() -> Message            // 暫停直到下一則信箱訊息
session.send(text)                    // 回覆目前的傳入訊息
session.send(text, { attachments: parts }) // 附帶媒體傳送（見下文）
session.reply(t)                      // 以結構化答覆結束此輪次：
                                      // { text, tool_calls, finish_reason, usage, reasoning }
session.send_delta(text)              // 串流答覆的一個片段
session.push(channel, reply_to, text) // 主動傳出；能力：channel.push
session.push(channel, reply_to, text, { attachments: parts })
session.push_user(uuid, text)         // 主動傳出給某個使用者 UUID；能力：channel.push
session.exit()                        // 結束此工作階段

session.state.set(key, value)         // 能力：session.state（需宣告）
session.state.get(key)                // 從未設定過該鍵時為 null/undefined
session.state.delete(key)
session.state.list()                  // 此工作階段的 { [key]: value }
```

頻道吃進媒體時，訊息會帶一個 `attachments` 陣列，元素是媒體片段描述元（`{ type, mime, handle, name }`）。原樣轉進 `llm.chat` 或 `session.send` 即可；執行環境會在一側（供應商或頻道）把 blob 控制代碼（`"media:<sha256>"`）解析成位元組 —— 原始媒體永遠不進 JS VM。

`session.reply(t)` 是結構化回覆：`tool_calls`（`{ id, name, args, arguments }`）、`finish_reason`（`stop`、`tool_calls`、`length`、`content_filter`）、`usage` 與 `reasoning` 會送到能表達它們的頻道，文字則走和 `session.send` 相同的安全與日誌路徑。`llm.forward` 或 `llm.chat` 的結果可以直接傳進去：其中的 `thinking` 會被當作回覆的推理內容，只有設定為對外暴露的頻道才會送出它。工具呼叫的 `arguments`（模型自己的原始 JSON 文字）優先於 `args` 物件送出，以保留供應商的精確格式；如果你改過某個呼叫的 `args`，就把它的 `arguments` 設成 `null` 或 `undefined`。每輪最多一則回覆；在無法表達工具呼叫的頻道上，帶工具呼叫的回覆會報錯，而不是悄悄丟掉。文字被傳出安全攔截的回覆以 `finish_reason = "content_filter"` 傳遞，不帶工具呼叫也不帶推理內容；其內容是設定檔的備援文案（如果設了），否則為空。

`session.send_delta(text)` 用於串流回答。只有安全設定檔無法丟棄傳出時，引擎才即時轉發片段；否則先扣住，本輪結束時把檢查過的完整內容一次放出。既串流又呼叫 `session.reply` 的迴圈，應讓回覆帶上完整文字：不論用戶端有沒有要求串流，回覆只會補上片段尚未攜帶的部分。

`session.state` 是本工作階段專屬的小型持久鍵值儲存：一個游標、一個步數、最後說過的話。它撐得過重新啟動，在叢集裡也會跟著工作階段走到下一個擁有它的執行個體，因為列存在共用儲存區裡。鍵可含字母、數字與 `. _ - :`，最長 128 個字元；值是任何可 JSON 編碼的 JS 值，上限 64 KiB；一個工作階段最多 256 個鍵。它需要明確開啟 —— 在代理的 `capabilities` 裡宣告 `session.state`，因為這是一塊持久化面，不該預設發出去。

#### `llm` —— 能力：`llm.chat`

```ts
llm.chat(messages, opts?) -> { text, thinking, thinking_blocks, usage, tool_calls }
llm.stream(messages, opts?) -> Iterable<string>
llm.forward(opts?) -> { text, tool_calls, finish_reason, usage, thinking, thinking_blocks }
llm.embed(inputs, opts?) -> { vectors, usage }     // 字串和／或部分物件
llm.rerank(query, documents, opts?) -> { results } // results: [{ index, score }]，從 0 起算

// opts: { model: "default", temperature: 0.7, max_tokens: 4096,
//         thinking: "low"|"medium"|"high"|"xhigh"|"max"|"off",
//         tools: [...], tool_choice: "auto" | "none" | "required" | "function:<name>" | { name: "<name>" },
//         parallel_tool_calls: true, top_p: 0.9, stop: [...] (or one string), seed: 1,
//         response_format: { type: "json_schema", json_schema: {...} } }

// forward opts: { model, messages, prepend, append, tools, params, max_tokens_cap }
```

`llm.forward` 是 OpenAI 頻道的透傳。不帶參數時，它把用戶端自己的請求 —— 訊息、工具、`tool_choice` 與取樣參數 —— 轉發給本輪所跑的模型（代理的，或使用者的個人模型），走與 `llm.stream` 同一條計量路徑：預算與配額適用，用量帳冊會多一列 kind 為 `forward`。`opts.messages` 取代用戶端的訊息清單，`prepend`/`append` 在前後加輪次，`tools` 加定義，`params` 覆寫取樣，`max_tokens_cap` 給用戶端的值設上限。供應商表達不了的選項會被丟棄而不是拒絕，但 `response_format` 與具名 `tool_choice` 例外，它們會讓呼叫失敗。結果的 `finish_reason` 是模型自己的值（輸出上限截斷時為 `length`），每個工具呼叫都帶 `arguments` —— 模型的參數文字 —— 與解碼後的 `args` 並列。只有當用戶端要求串流、且代理的安全設定檔沒有過濾器時，它才即時轉發文字增量；否則整段文字回傳給 `session.reply`。它不負責結束 HTTP 回應。模型呼叫失敗會拋出以 `upstream_error:` 開頭的錯誤。在 OpenAI 頻道輪次之外又不給 `opts.messages` 時，它回傳錯誤。`openai.request()` 把完整請求作為物件回傳，超過 4 MiB 的 JSON 會被拒絕 —— 該轉發這個請求，而不是把它實體化。兩者都需要 `llm.chat` 能力。`llm.chat` 的回覆與 `llm.stream` 的最後一幀也帶 `finish_reason`。

訊息是 `{ role, content, tool_calls?, tool_call_id?, tool_result?, thinking_blocks? }` 的陣列。**多模態輪次**用 `{ role, parts: [...] }`，每個片段是一個物件：

```ts
{ type: "text", text: "..." }
{ type: "image"|"audio"|"video"|"file", mime: "image/png", handle: "media:...",
  name: "photo.jpg" }   // handle 來自 msg.attachments（也可用 data base64 / url）
```

供應商支援矩陣（不支援的片段型別會如實報錯；來源可以是 base64 `data`、`url`，或供應商的檔案參照）：

| 供應商 | image | audio | video | PDF / 檔案 |
| --- | --- | --- | --- | --- |
| anthropic | ✅ base64/url | ❌ | ✅ base64/url（MiniMax video block） | ✅ document（base64） |
| openai (chat) | ✅ image\_url | ✅ input\_audio（wav/mp3） | ✅ video\_url —— MiniMax/Kimi/GLM 慣例 | ✅ file\_url（url）—— GLM |
| openai-responses | ✅ input\_image | ✅ input\_audio（wav/mp3） | ✅ input\_video（url/file ref） | ✅ input\_file（data 或 file\_id） |
| gemini (interactions) | ✅ files api | ✅ files api | ✅ files api | ✅ files api |

影片來源遵循生態裡的慣例 —— MiniMax 的 `mm_file://<file_id>`、Kimi 的 `ms://<file_id>` —— 作為 URL／檔案參照原樣透傳。SDK 的型別化請求聯合體沒有建模的形狀（`video_url`、GLM 的 `file_url`、Anthropic 的 video block、Responses 的 `input_video` 與 `input_audio` 項目）用 `param.Override` 建構 —— 那是 SDK 自帶的原始 JSON 逃生口，未設定型別化變體時依原樣序列化。blob 儲存的 `handle` 會自動解析成 base64。

對 gemini，每個媒體片段都會上傳到 **Gemini Files API**（可續傳上傳），再以 URI 參照 —— 絕不用內聯 base64。`data`（base64）、`url` 與 blob 儲存的 `handle` 三種來源都行（url 會先下載）；相同位元組的重複上傳在檔案 48 小時生命週期內會依內容雜湊去重，之後 Google 自動刪除。Anthropic/openai 也直接接受 `url` 來源。`llm.stream` 回傳標準的 ES6 可疊代物件（`[Symbol.iterator]`、`next()`、`close()`、`return()`）。用 `for (const delta of llm.stream(messages, opts))` 疊代；提前 break 會觸發 `return()`，乾淨地關閉底層串流（也可以手動呼叫 `iter.next()` 與 `iter.close()`）。它只產出文字增量字串 —— 要拿用量與工具呼叫請用 `llm.chat`。

**多模態嵌入**遵循 Jina 慣例：`llm.embed` 接受字串與片段物件混合的陣列，帶媒體的批次會把每項序列化成帶型別的 doc（`{"text"}`、`{"image"|"video"|"audio"|"pdf": url|base64}`）—— 這正是 `jina-embeddings-v5-omni-small`（文字、圖像、影片、音訊與 PDF 在同一向量空間）和 `jina-clip-v2` 使用的形狀。純文字批次仍是普通字串，所以任何 OpenAI 相容的 `/embeddings` 端點（OpenAI、Ollama、vLLM、TEI、OpenRouter）看到的都是歷史上那個一模一樣的請求。選項：`task`（`retrieval.query` | `retrieval.passage` | `text-matching` | `clustering` | `classification`）、`dimensions`（Matryoshka 截斷）、`merged: true`（把所有輸入折成一個嵌入 —— 一個 Jina 內容群組；PDF 只支援單輸入）。Anthropic 與 Gemini 供應商在這裡沒有嵌入 API，會明確失敗。`llm.rerank` 需要一個 `provider: rerank` 的模型項目，說 `POST {base_url}/rerank`（TEI、vLLM、Jina、Cohere）。代理宣告了預算時，`llm.embed` 依 Token 計費。`gemini` 供應商（Interactions API）是非串流的：對它呼叫 `llm.stream` 會把完整回覆作為單一增量產出。

**推理內容（回應側）。** 供應商送來的推理內容會被動地暴露在回覆上 —— `reply.thinking` 是給人讀的文字（沒有時為空字串），`reply.thinking_blocks` 是供應商的原始區塊（Anthropic 的 `{type: "thinking", thinking, signature}` / `redacted_thinking`、Responses 的 reasoning 項目、Gemini 的 `thought` 片段；Chat Completions 上為 null/undefined，那裡推理只是一個純字串），而 `usage.reasoning` 在供應商單獨上報時統計思考 Token。推理內容的有無是各供應商、各模型別名的執行時屬性，不是契約：某個服務設定可以既按推理 Token 計費又從串流裡完全隱去推理內容，所以 `usage.reasoning` 才是可靠的稽核訊號，`reply.thinking` 為空本身並不說明出故障。引擎從不索取超出思考等級已經要求的推理內容，也從不儲存它 —— 要不要持久化是迴圈的決定。Anthropic 開啟 thinking 的多輪工具呼叫，要在 assistant 輪把區塊原樣傳回去：`thinking_blocks: reply.thinking_blocks`（否則 Messages API 會拒絕續接；其他供應商忽略該欄位）。

```ts
// 多模態嵌入（透過 api.jina.ai/v1 使用 jina-embeddings-v5-omni-small）
const r = llm.embed([
  "find me the cat photo",                                // 文字
  { type: "image", handle: msg.attachments[0].handle },   // Blob 儲存區媒體
], { model: "omni", task: "retrieval.query", dimensions: 512 });

// 為整筆多模態記錄產生單一向量（當設定檔指定了 omni embed_model 時，
// memory.write 會對帶有附件的記錄這麼做）：
const r2 = llm.embed(["trip to the beach", imgPart], { model: "omni", merged: true });
```

#### `agent` —— 能力：`agent.send`、`agent.request`、`agent.spawn`

```ts
agent.info() -> { name, session_id, address, model, instructions,
                  history_budget, memory, shell, skills, capabilities,
                  memory.stores: { [name]: { backend, table, features, locality }, ... } }
agent.config() -> { name, model, instructions_path, instructions_prompt?,
                    prompts: { [name]: "", ... },
                    goal?: { type, success_signal, max_turns, on_goal_met },
                    extras: { ... } }
agent.send(address, payload)
agent.request(address, payload, timeout?) -> Message
agent.reply(request_id, payload)
agent.list() -> [{ session_id, agent, parent_id }]
agent.spawn(profile) -> { address, session_id, agent }
```

儲存區的 `locality` 是 `deployment`、`region` 或 `instance`。持久化的對話資料該放在部署級或區域級的儲存上；執行個體本地的儲存只是快取，內容隨行程一起消失（區域單元裡，持久儲存區放在這種儲存上會在驗證時被拒絕，除非設了 `locality_ok: true`）。

`agent.spawn` 需要一個 `profiles.agent` 範本；子工作階段的能力是上層與範本能力的交集。早期版本接受的第二個參數已保留但不再生效 —— 請改在範本裡設定。

`agent.config()` 把這個代理自己的設定檔作為資料回傳：組態名、模型、指令的*路徑*（如果系統提示詞來自 `prompts` 登錄表則為 `instructions_prompt`，此時 `instructions_path` 為空）、部署解析後的 `prompts` 登錄表（依名稱索引）、設定檔裡提升到 `goal` 的 `extras.goal`，以及其餘的 `extras`（一個 pm 選項區塊、一個工作流名，等等）。裡面的密鑰參照會渲染成不透明標記（`{env: "VAR"}` / `{cred: "service"}`）—— 解析後的密鑰永遠不會過指令碼橋。

#### `user` —— 能力：`users`

```ts
user.current() -> { registered, id, display_name, identities }
user.usage() -> { registered, input, output, cached, reasoning, calls,
                  failed, billable, used, limit, remaining, unlimited, in_flight }
user.has_credential(service) -> { service, present }
user.get(id) -> { ... }    // 僅限維護來源
user.list() -> [ { ... } ] // 僅限維護來源
```

關於本輪所屬使用者的唯讀資訊。`users` 是預設能力，所以迴圈不用宣告就能呼叫。迴圈拿到的是一個名字和一個頻道，永遠拿不到電郵，也拿不到憑證值 —— `has_credential` 回答的是密鑰存不存在，而不是它是什麼。`get`/`list` 僅限引擎觸發的維護來源（`system` 或 `scheduler` 輪次），正因如此才能寫依使用者的維護迴圈，又不會把一個使用者的資料暴露給另一個使用者的輪次。

#### `runtime` 與 `credential` —— 部署組態即資料

```ts
runtime.triggers() -> { ok, triggers: [ { name, kind, run_on_boot,
                          target: { profile }, payload,
                          event: { channel, match }   // kind = "event"
                          | cron                      // kind = "cron"
                          | every } ] }               // kind = "every"

credential.get(name) -> { ok, name, value }           // 僅限允許清單中的名稱
```

`runtime.triggers()` 回傳啟動時快照的合併後 `triggers/*.yaml`（或頂層 `triggers:`）清單 —— 讓迴圈從引擎讀 CRON\_TASKS/ROUTES，而不是把表硬編碼進指令碼。它在**兩個**指令碼面上回答方式一致：代理迴圈與閘道路由處理常式（路由器的操作分派回傳完全相同的酬載），所以事件路由能讀到驅動它的觸發器清單。表示式原樣透傳，引擎的排程也不會改變形狀：`every:`/`cron:` 項目同時由引擎自己觸發（見組態參考裡的 `triggers`），所以迴圈要呼叫這個只是為了*觀察*部署的排程。`credential.get(name)` 僅在代理設定檔把該名字列在 `credentials: [...]` 下時，才去取全域儲存的密鑰（空清單或缺失 = 拒絕，並給出說明）。每次存取都會記日誌（代理、名稱、允許／拒絕）並計數（`agentflow_credential_gets` / `agentflow_credential_gets_denied`）；值絕不寫進日誌，也絕不該回顯到回覆裡。

`router.state` 是路由處理常式自己的持久鍵值儲存 —— 處理常式把不能忘的東西放在這裡，比如某個聊天上次被路由到哪個代理。它住在執行環境儲存區裡，所以每個執行個體讀到同一個值，重新啟動或故障轉移也不會遺失：`router.state.get(key)`（鍵從未設定時為 null/undefined）、`router.state.set(key, value, { ttl_seconds: n })`、`router.state.delete(key)` 與 `router.state.list(prefix)`。值是任何可 JS`router.state.list(prefix)`。值是任何可 JSON 編碼的 JS 值，上限 64 KiB；鍵可含字母、數字與 `. _ - :`（128 個字元）。鍵在最後一次寫入後保留 30 天，除非處理常式要求別的存活期（`ttl_seconds: 0` 表示保留到被刪除），這正是處理常式以高基數值為鍵時儲存不至於膨脹的原因。該命名空間只在路由器的狀態裡定義，所以代理迴圈搆不到它 —— 而且它是「一致性」而非「序列化」：兩個執行個體可能對同一個鍵交錯讀寫，而傳遞本身由工作階段租約序列化。

#### `scheduler` —— 不受能力閘門控管

```ts
scheduler.every(interval) -> timer_id    // 單位為秒；隨工作階段消失
scheduler.after(delay) -> timer_id
scheduler.cron(expr) -> timer_id          // cron 子集（見下文）
scheduler.cancel(timer_id)
```

計時器會把 `Message{ type: "timer" }` 放進所屬工作階段的收件匣 —— 帶上 `msg.payload.timer_id`，這樣一個工作階段就能分辨自己的多個計時器。它們從不在計時器 goroutine 裡呼叫指令碼，所以每個工作階段 VM 始終保持單執行緒。`scheduler.cron` 只支援 5 欄位 cron 的**子集**：`*/N * * * *`（每 N 分鐘）與 `30 9 * * *` 這類固定分／時形式；日、月、星期欄位必須是 `*`，`*/N` 分鐘也不能和固定小時搭配（步長沒法錨定到某個小時，所以會被悄悄丟掉）。其他寫法一律拒絕而不是重新解釋 —— 更複雜的排程請用 `every`/`after`（或自我重排）。

這些是*工作階段層級*計時器：它們隨工作階段一起消亡。**沒有 `scheduler` 能力** —— 這些操作在 actor 自己的分派裡處理，從不經過能力閘門，所以任何迴圈都能註冊計時器，無論它宣告了什麼。閘門只管透過處理常式對應能到達的操作（`llm.*`、`memory`/`store.*`、`tools.*`、`api.*`、`shell.*`、`http.request`、`mail.*`、`agent.*` 的位址）；計時器是引擎傳遞的訊息，不是一項權限。在 YAML 裡宣告的部署層級排程（`every:`/`cron:` 觸發器）則屬於引擎基礎設施 —— 由引擎觸發，不涉及任何能力，以 `Message{ type: "cron" }` 傳遞，所以迴圈不必自己跑一個計時迴圈才能被排程。迴圈需要自己判斷時鐘時，用 `os.time()`/`os.date()`（UTC）。另外注意計時器狀態在記憶體裡：重新啟動會忘掉所有計時器，直到迴圈重新註冊。

#### `memory` 與 `store` —— 能力：`memory`

```ts
memory.write(record) -> boolean                         // 由 plugin:routing_table 路由
memory.recall(query, opts?) -> [ { key, value, meta } ] // 目前作用中的召回處理常式

store.put(table, key, value, opts?)                     // opts: { ttl: seconds, vector: [...] }
store.get(table, key) -> { value, found }
store.query(table, query) -> { records }
store.delete(table, key)
```

儲存查詢接受 `{ kind: "all" | "prefix" | "text" | "vector" | "time_range", ... }`；向量查詢用 `{ kind: "vector", vector: [...], k: n }`。給不支援 `vector` 特性的後端傳 `vector` 是明確報錯，絕不悄悄丟棄。代理記憶優先使用 `memory.write`/`memory.recall`；只有需要原始表存取時才用 `store.*`。

**依使用者隔離（私有儲存）。** 來自頻道的輪次只讀寫自己的使用者範圍和 `service` 列（由沒有自身租戶的上下文寫入：引擎觸發的工作、未連結的帳號，以及用 `inherit_user: false` 退出的代理）和 `legacy` 列（此功能上線前寫入的 —— 升級前的歷史對該代理的每個使用者仍可讀，且不會再寫入）。引擎觸發的上下文（`system`/`scheduler` 來源 —— 像蒸餾器這樣的夜間維護）用作用域可見的鍵讀取所有範圍，並在寫入時明確指定範圍；維護工作階段可以透過 `store.scopes` 列舉某個綁定的使用者範圍（僅限維護：頻道與代理跳轉工作階段會被拒絕）。這是引擎強制的，不靠迴圈程式碼過濾。綁定集區的儲存（`pool: <name>`）改由租戶授權治理，私有儲存也可以每個代理用一個 `scope: agent` 重新加入某個集區。

#### `tools` —— 能力：`tools`

```ts
tools.list() -> [ToolSpec]
tools.run(name, args, opts?) -> ToolResult
// opts: { confirmed: true, confirm_id: "..." }

tool.def({
  name: "script:shout",
  description: "Shout the text back",
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

如果工具宣告了 `needs_confirm: true`，第一次呼叫會回傳 `needs_confirm` 回應。執行環境去問使用者，然後帶 `opts.confirmed = true` 重新呼叫。你的迴圈只看最終結果。（等待是有界的：十分鐘，逾時即拒絕 —— 排程器輪次上的確認式工具沒有頻道可應答，無界等待會把工作階段永久掛住。）

**工具迴圈是迴圈自己的事。** 引擎只在被要求時執行工具並回傳結果，不驅動對話。用 `tools` 呼叫 `llm.chat`、把回傳的東西透過 `tools.run` 跑一遍、把它們作為 `role: "tool"` 輪次送回去再問一次 —— 這都是程式碼區塊的工作，包括*第二*輪，而這正是最容易忘的：看到第一個結果後還要再要工具呼叫的模型，只有程式碼區塊再檢查一次 `reply.tool_calls` 才能拿到它。迴圈持久化什麼也是同理：工具呼叫及其結果是下一輪該看到的歷史，只存助理文字的程式碼區塊，會讓模型在後續對話裡參照一個已經不存在的工具結果。

`tool.def` 在 TypeScript/JS 裡宣告模型可見的工具 —— 這是取代手工拼裝額外工具定義的標準做法。宣告的工具會併入 `tools.list`（與 Go 工具同樣的 `{ type: "function", function: {...} }` 形狀，所以會原樣流進 `llm.chat`），`tools.run` 分派給 JS 處理常式。處理常式在迴圈 VM 上同步執行，自身可以呼叫任何操作（`llm.chat`、`memory.*` 等）。宣告的名字會在清單與執行兩處覆蓋同名 Go 工具（覆蓋發生在 `skills` 過濾之後，所以此代理看不到的工具不會壓制它指向的 Go 工具）。處理常式拋錯會以 `{ ok: false, error: ... }` 回傳，而不是殺掉迴圈。宣告的工具是迴圈自己的程式碼：Go 的工具原則（`forbidden`、`needs_confirm`、`permission: forbidden`）仍然管不到它們。請在程式碼區塊頂層、`loop()` 之前宣告。

**可見性**：宣告的工具按治理 Go 工具的同一個規則被 `skills` 與 `tools.policy.default` 過濾 —— `skills` 點名了就列出，`skills` 非空但沒點名就隱藏，`skills` 為空則完全依 `default` 處理。正因如此，一個共用迴圈目錄才能承載只給部分代理用的工具。對 `default: none` 且沒有 `skills` 的部署來說這是行為變化：宣告的工具過去無條件列出，現在不是了。`tools.run` 遵循同一規則 —— 代理看不到的宣告名字會落到 Go 路徑並以不可用拒絕，與看不見的已註冊工具完全一樣。已註冊工具本來就總在執行時分派處校驗，而 `tools.run` 正是模型可見、幻覺或注入的工具呼叫會走的路徑，所以兩者一致。想呼叫自己處理常式的迴圈，直接呼叫那個函式即可。

**覆寫已宣告工具的文字**：`tools.policy.overrides` 可以點名一個已宣告的工具，把面向模型的文字從程式碼區塊移到組態裡 —— 描述（字面值，或來自 `prompts` 登錄表的 `{prompt: <key>}`）與 `params.<param>.description`。沒有覆寫點名時，傳給 `tool.def` 的規格仍是後備，而覆寫永遠不改動宣告 schema 的型別或 `required`。解析發生在建立 `tools.list()` 的時候，所以重新載入的 `file:` 提示詞在下次呼叫就生效，無須重新啟動。覆寫從不增刪清單項目：可見性只來自 `skills` 與原則預設值。已宣告但被隱藏的工具仍算已宣告，所以點名它的覆寫不會被報成「什麼也沒點到」。

#### `api` —— 能力：`apis`（外部 API）

```ts
api.list() -> [APIDescriptor]                 // { name, version, description, runtime: "script"|"wasm", actions }
api.call(pkg, action, args?) -> unknown
api.batch(calls) -> [{ ok, data?, error? }]   // calls: [{ api, action, args }]
api.<pkg>.<action>(args?) -> unknown          // api.call(pkg, action, args) 的唯讀 Proxy 簡寫
```

外部 API 是可插拔的套件，用沙箱化的 TypeScript（`goja`）指令碼（`apis/<pkg>/...`）或 WebAssembly（`wazero`）模組實作 —— 與工作階段狀態隔離，經宿主邊界呼叫。宣告為 `expose_as_tool: true` 的操作在代理同時持有 `apis` 與 `tools` 時，也會作為 `api:<pkg>.<action>` 工具出現在 LLM 面前（持有 `apis` 的迴圈也可以直接呼叫）。每個操作都在自己的套件權限與逾時下執行；`api.call` / `api.<pkg>.<action>` 的錯誤會拋出，而 `api.batch` 回傳逐次呼叫的 `{ ok, data, error }`。

#### `shell` —— 能力：`shell.exec`

```ts
shell.run(cmd, opts?)   -> { ok, exit_code, stdout, stderr, duration_ms, timed_out, truncated,
                             project, environment, changed, deleted, conflicts, skipped }
shell.start(cmd, opts?) -> { id, state: "running", ... }     // 結果會送到收件匣
shell.status(job_id)    -> job
shell.cancel(job_id)    -> job

// opts: { project: "site", environment: "python", workdir: "app",
//         env: { KEY: "val" }, timeout: 120 }                 // timeout 單位為秒

// 沙箱控制代碼：把作業選項編碼進 id 的相容性墊片。
shell.spawn(opts) -> { id, provider, state }
shell.exec(id, cmd, opts?) -> the same result as shell.run
shell.write(id, path, content)
shell.destroy(id)
```

**沙箱 Shell 並不是一個持續存在的東西。** `shell.run` 是在一次性沙箱（[gVisor](https://gvisor.dev) `runsc` 或 [microsandbox](https://github.com/superradcompany/microsandbox) `msb`）裡跑一條命令，為這條命令啟動、跑完銷毀。沙箱拿到兩樣東西：一個宣告的**環境**作為工具鏈根，以及從檔案儲存區掛到 `/workspace`（命令的起始目錄）的一個**專案**，可讀寫。`/workspace` 之外的暫存狀態會被丟棄。沒有網路。命令結束後，引擎比對專案目錄和它放進去的內容，把差異寫回專案的工作樹 —— 也就是 `files.*` 讀寫的那棵樹。什麼時候 `files.commit` 由迴圈決定。

所以迴圈的規則很短：

- **命令在專案裡寫的東西會留下，別的一概不留。** 背景行程、Shell 變數、裝的套件、`/workspace` 之外的檔案，都活不到下一次呼叫。
- **非零 `exit_code` 與 `timed_out` 也是結果**，就像 `http.request` 收到 500 一樣。根本跑不起來的作業 —— 這台主機沒沙箱、環境未知、佇列逾時、沙箱記憶體超限被殺 —— 會拋錯，所以請把呼叫包在 `try { ... } catch (err) { ... }` 裡。命令在被殺之前寫下的東西仍會存下來。
- **一個執行個體上每個專案同時只跑一個作業**；同一專案上的第二次呼叫會等第一次（直到 `runtime.sandbox.queue_timeout`）。
- **回寫永不覆蓋更新的檔案。** 作業改過、但期間被別人改過的路徑保留儲存區裡的版本，並列在 `conflicts` 中。存不進去的路徑（超過單檔上限、超過作業回寫上限、符號連結）帶原因列在 `skipped` 中，並從目錄裡移除，免得目錄裡留著儲存區中沒有的東西。
- **任何執行個體都能跑下一條命令。** 執行個體上的專案目錄只是檔案儲存區的快取。

設定分層：呼叫的 `opts` 覆蓋專案的 `.agentflow/shell.yaml`，後者覆蓋代理的 Shell 設定檔。環境必須是部署宣告過的；時限受 `runtime.sandbox.max_timeout` 限制。專案清單可以設 `environment`、`env`、`workdir`、`timeout` 與 `ignore`，別的都不行：

```yaml
# .agentflow/shell.yaml，位於專案的工作樹中
environment: node
env: { NODE_ENV: test }
workdir: app
timeout: 120s
ignore: ["node_modules/", "dist/"]
```

被 `ignore`（來自設定檔或清單）匹配的路徑既不儲存也不清理：它們作為盡力而為的快取留在建立它們的執行個體上 —— 相依目錄、建置快取 —— 在另一個執行個體上就是不存在。

**背景作業。** `shell.start` 把同樣的作業排隊後立即回傳，時限更長（`runtime.sandbox.background_max_timeout`）。結果以 `type: "shell.result"` 的訊息到達工作階段收件匣，作業在 `msg.payload` 裡（`state` 為 `finished`、`failed` 或 `cancelled`；`stdout` 與 `stderr` 是最後 16 KiB）。結果輪次以啟動作業的使用者身分執行。`shell.status` 可以在作業執行時（從任何執行個體）讀取，`shell.cancel` 在執行它的執行個體上停止它。執行個體死掉的作業會報為 `failed`，不會重跑。工作階段結束時，它的背景作業會被取消。

**沙箱控制代碼。** `shell.spawn` 回傳一個*沙箱控制代碼*，它存在的意義是讓為容器 Shell 寫的迴圈在重新啟動和跨執行個體時也能乾淨工作：它只是把拿到的選項編碼進 id。對它 `shell.exec` 會跑作業，`shell.write` 寫進本機專案，`shell.destroy` 什麼也不做。它不需要儲存，在任何執行個體上都能用。有三個選項在沙箱裡有意義：`image` 指定一個宣告的環境，`volumes[0]` 的容器路徑是專案的掛載點（而不是 `/workspace`），而 `network` 非 `"none"`、`ref` 非 `"main"` 以及 `scratch_mount` 會被拒絕，並說明原因。傳入 `provider: "ssh"` 或 SSH 控制代碼會被拒絕 —— 請改用 `ssh.*`。

綁定了 Shell 設定檔的代理（`agents.<name>.shell`）還會得到隱式操作本機沙箱專案的登錄表工具，模型因此從不直接接觸控制代碼：`builtin:shell.exec`（`{ command }` → `shell.run` 的結果）、`builtin:fs.read`（`{ path, handle_id? }` → `{ ok, content, path, truncated }`）與 `builtin:fs.write`（`{ path, content, handle_id? }`）。這些檔案工具直接讀寫專案 —— 不會為它們啟沙箱 —— 路徑可以相對於專案，也可以寫在 `/workspace` 下；選填的 `handle_id` 只接受 `shell.spawn` 回傳的沙箱控制代碼。沒綁設定檔的代理拿到的是如實的不可用，沙箱跑不起來的主機也一樣；沒有 `shell.exec` 能力的代理根本看不到這些工具，即使 `tools.policy.default: all` 也一樣。`builtin:shell.exec` 與 `builtin:fs.write` 預設 `needs_confirm: true` —— 可透過 `tools.policy.overrides` 依部署放寬。

#### `ssh` —— 能力：`net.ssh`

```ts
ssh.spawn(opts?)             -> { id, provider, state }
ssh.exec(id, cmd, opts?)     -> { stdout, stderr, exit_code, duration_ms }
ssh.read(id, path)           -> { ok, path, content }
ssh.write(id, path, content) -> true
ssh.close(id)                -> true
```

**遠端 SSH 控制代碼。** `ssh.spawn` 會連到真實遠端機器（`host`、`user`、`password` 或 `key_file`、`workdir`、`env`、`shell_opts`），除非在 `opts` 中覆寫，否則預設繼承綁定 `agents.<name>.ssh` 設定檔的設定；`ssh.exec`、`ssh.read`、`ssh.write` 與 `ssh.close` 作用其上。連線活在建連的那個執行個體上，並在呼叫 `ssh.close` 或工作階段結束時關閉。`ssh.spawn` 也可以帶 `project` + `ref` 與一項 `volumes: ["x:/dir"]`，在控制代碼回報就緒前把檔案快照複製到遠端的 `/dir`，或用 `scratch_mount` 複製工作階段的暫存檔。

綁定了 SSH 設定檔的代理（`agents.<name>.ssh`）還會得到登錄表工具，每個工作階段依需求開啟並重用一條 SSH 連線（或接收來自 `ssh.spawn` 的顯式 `handle_id`）：`builtin:ssh.exec`（`{ command, handle_id? }` → `{ ok, tool, handle_id, stdout, stderr, exit_code, duration_ms }`）、`builtin:ssh.read`（`{ path, handle_id? }` → `{ ok, tool, handle_id, path, content }`）、`builtin:ssh.write`（`{ path, content, handle_id? }`）與 `builtin:ssh.close`（`{ handle_id? }`）。沒綁 SSH 設定檔（且未提供 `handle_id`）的代理拿到的是如實的不可用；沒有 `net.ssh` 能力的代理根本看不到這些工具，即使 `tools.policy.default: all` 也一樣。除 `builtin:ssh.read` 外都預設 `needs_confirm: true` —— 可透過 `tools.policy.overrides` 依部署放寬。

`builtin:fetch` 是面向 API 與普通頁面的通用 HTTP 用戶端，不需要帳號或憑證 —— 所以和 `web_search`、`browser` 不同，它永遠不會如實的不可用。選項：`url`（必填）、`method`、`headers`、`body`（原始）或 `json`（序列化；除非已有要求標頭，否則會設 `Content-Type`）、`query`、`timeout_ms`、`follow` / `max_redirects`、`user_agent`、`max_bytes`、`insecure`，以及 `save_to`（回應主體會同時落到本工作階段的暫存檔區，可用 `files.scratch.read` 讀取）。它回傳 `{ok, status, status_text, headers, body, final_url, redirects?, content_type?, content_length?, size, truncated?, tls_verified, time_cost_ms}` —— 對*任何* HTTP 狀態都回傳，因為 404 是模型該讀到的答案，而不是工具失敗；`ok` 表示請求是否完成，`status` 表示伺服器說了什麼。

回應主體有上限（`max_bytes`，預設 1 MiB），結果總會報告真實的 `size`，被截斷時置 `truncated`。這個上限不是錦上添花：工具結果整塊交給模型，下游不做任何截斷，而迴圈的 Token 預算器根本不統計工具結果 —— 所以超大的回應主體會擠掉真實的對話歷史，而不是被裁掉。目標受前文所述的 `net` 防護約束，拒絕會以點名位址的錯誤回傳，計入 `agentflow_http_private_blocked`，並記在呼叫方代理名下。

`insecure: true` 跳過 TLS 憑證驗證。它永遠被允許 —— 沒有組態開關 —— 但絕不靜默：跳過會遞增 `agentflow_http_insecure_tls`，記一條點名代理與 URL 的 WARN，並在結果裡報 `tls_verified: false`。它只跳過憑證檢查；位址防護仍然生效，因為「對端自稱是誰」和「連線實際去哪」是兩個不同的問題。這個工具和 `web_search` 一樣是 `Autonomous: true`；想要確認的部署可以透過 `tools.policy.overrides` 設 `needs_confirm`。

#### `http` 與 `os` —— 能力：`net.http`

```ts
http.request(opts) -> { ok, status, headers, body, truncated? }
http.get(url, opts?)
http.post(url, body, opts?)

// opts: { method, url, headers, body, json, query, timeout, auth,
//         save_to: "name" }  // 本文也會存入此工作階段的檔案暫存空間
// auth: { service: "..." } 會在請求時解析一個已儲存的憑證。

os.env(name) -> string | null | undefined
os.time() -> number                        // Unix 秒數（UTC）
os.date([format,] time?) -> DateFields | string // UTC；除非指定格式，否則傳回物件
```

**牆上時鐘。** `os.time()` 回傳整秒 Unix 時間，不接受參數。`os.date()` 回傳日期物件 —— `{ year, month, day, hour, min, sec, wday, yday, isdst }`，其中 `wday` 1 = 週日 … 7 = 週六，`yday` 1–366，`isdst` 恆為 `false` —— 接受一個選用的 Unix 秒時間（預設現在）。該物件可 JSON 編碼，所以能放進酬載。傳 strftime 字串則回傳文字：`os.date("%Y-%m-%d %H:%M:%S", t)`。支援的指令有 `%Y %y %m %d %e %H %I %M %S %p %A %a %B %b %j %w %u %Z %%`；未知指令原樣保留。**一切都是 UTC** —— 沙箱沒有本地時區，所以欄位在每台主機上都一樣，前導 `!`（`os.date("!*t")`）會被當作無操作接受。相比之下，cron 觸發器依 `runtime.timezone_offset_hours` 匹配；兩者之間的換算由你的迴圈負責。

#### `files` —— 能力：`files`

```ts
files.put(project, path, content | { data, handle?, mime? }, { mime }?) -> { ok, entry }
files.read(project, path) -> { ok, entry }        // entry: { path, handle, size, mime, ts, revision }
files.list(project, prefix?) -> { ok, entries }
files.projects() -> { ok, projects }              // 此輪次範圍內的專案
files.delete(project, path) -> { ok: true }
files.commit(project, { ref: "main", message }?) -> { ok, commit } // 快照；父鏈即為歷史
files.checkout(project, ref | commit_id) -> { ok, manifest }       // manifest: { commit, files: [{ path, handle, size }] }
files.scratch.put(name, content | { data, mime? }) -> { ok, entry }
files.scratch.read(name) -> { ok, entry }
files.scratch.list() -> { ok, entries }
```

一個**使用者範圍的檔案儲存區**：具名專案樹，其範圍由引擎依輪次身分解析（來自頻道的輪次和繼承了租戶的委派輪次是 `user:<uuid>`，否則 `agent:<name>`）—— 迴圈永遠無法指定別的範圍。位元組存在內容定址的 blob 儲存區裡（`files:` 組態；本機檔案系統或 S3 相容 —— 未變的檔案在快照之間自動去重）；快照中繼資料（工作樹、提交、參照、暫存檔）存在執行環境儲存區的 `files_meta` 表裡，由引擎掌管，可以用普通 SQL 檢視。版本管理是**引擎原生快照，不是 git 伺服器**：一次提交記錄完整樹（路徑 → 控制代碼）並帶父指標與說明，同時移動一個具名參照；checkout 解析參照或提交 ID。原始位元組不過指令碼橋 —— `put` 接受 utf-8 內容或 base64 `data`，`read` 回傳控制代碼加中繼資料，實體化在引擎側完成：Shell 衍生時的 checkout、`scratch_mount`，或 `http.request` / `builtin:fetch` / `builtin:browser` 的 `save_to`。暫存檔依工作階段隔離（鍵是呼叫工作階段，不是使用者），TTL 由 `files.scratch_ttl` 控制（預設 24h，讀取時惰性過期 + 啟動時清掃）。一個 `files` 能力管住這一整族；它不在預設集合裡，需要明確宣告。啟動時儲存區被停用（例如 S3 憑證解析不出來）的話，每個操作都會如實失敗。

#### `mail` —— 能力：`net.mail`

```ts
mail.imap_fetch(opts) -> { ok, mailbox, count, messages }
// opts: { host, port: 993, user, mailbox: "INBOX", unseen: false,
//         limit: 10, auth: { service: "..." } }
// messages: [{ seq, uid, flags, subject, from, to, date, body }]
// 透過隱含式 TLS 連線；本文上限 256 KiB（超過時截斷並加上標記）

mail.smtp_send(opts) -> { ok, sent }
// opts: { host, port: 465, user, from, to: [...], subject, text_body, auth }
// 連接埠 465 = 隱含式 TLS；587/25 = STARTTLS
```

和 `http.request` 一樣，`auth: { service: "..." }` 指向一個由 Go 在呼叫時解析的已儲存憑證 —— 密碼永遠不過指令碼橋。主機、連接埠與使用者名來自迴圈（通常經 `os.env`），指令碼原始碼裡因此不存任何密鑰。`net.mail` 不在預設能力集裡；代理必須宣告它。

#### 核心工具

```ts
log.debug(msg) / log.info(msg) / log.warn(msg) / log.error(msg)
json.encode(value) / json.decode(text)
time.sleep(seconds)        // 讓出給 Go 計時器；絕不阻塞作業系統執行緒
```

### 撰寫支援片段

支援片段在迴圈外掛之前求值進工作階段狀態。它們可以定義迴圈或其他支援片段要讀的全域。例如，要換掉預設的喚回策略，就在 `plugins/recency.ts` 裡覆寫 `plugin:recency`，定義一個 `memory_recall_handler(query, opts)`。

```ts
// plugins/recency.ts（遮蔽範例）
function memory_recall_handler(query: Record<string, unknown>, opts?: Record<string, unknown>) {
  const info = agent.info();
  const storeName = (info.memory && info.memory.stores.dialogue) || "dialogue";
  const res = store.query(storeName, { kind: "all" });
  return res.records || [];
}
```

### 錯誤處理

外部呼叫一律包在 `try { ... } catch (err) { ... }` 裡，免得某個抽風的供應商或壞命令把工作階段搞成崩潰迴圈：

```ts
try {
  const reply = llm.chat(messages, { model: info.model });
} catch (err) {
  log.warn("llm.chat failed: " + String(err));
  session.send("⚠️ I couldn't reach my model right now.");
  return;
}
```

橋拋出的是從 Go 回應解包出來的 `Error`，所以堆疊指向你的程式碼，而不是 prelude。

### 確認與權限

工具與 Shell 內建項可以宣告 `needs_confirm`。第一次 `tools.run` 回傳帶 `needs_confirm: true` 與 `confirm_id` 的結果；actor 去問使用者，然後帶 `opts.confirmed = true` 和同一個 `confirm_id` 重新呼叫。你的迴圈只看最終結果，所以多數情況下不必手動處理確認。

### 限制與注意事項

- `llm.stream` 回傳 ES6 可疊代物件 —— 用 `for (const delta of llm.stream(...))`，或手動呼叫 `iter.next()` / `iter.close()`。
- 標準函式庫就是 Goja 的 ECMAScript 執行環境（`Object`、`Array`、`Map`、`Set`、`JSON`、`RegExp`、`Math`、`Date`、`Proxy`、沒有事件迴圈的 `Promise`，以及型別化陣列）。Node／瀏覽器 API（`require`、`fs`、`process`、`fetch`、`setTimeout`、`setInterval`）都不可用，也沒有宿主 `os` 函式庫：prelude 只提供 `os.env`、`os.time` 與 `os.date`（全部 UTC —— 沙箱裡沒有本地時區）。
- 指令碼的原始網路存取只能經由 `http.request`、`api.*`（外部 API 被授予宿主 HTTP 時）與 `mail.*` 操作。沒有通訊端函式庫。HTTP 那幾條路徑 —— `http.request`、`ExtHost.http` 與 `builtin:fetch` —— 在撥號器裡拒絕私有、回送、連結本地與保留位址，所以光有 URL 並不足以搆到宿主自己的網路；見 `net` 區塊。`mail.*` *不*受該防護約束，仍會撥向迴圈指定的任何主機。
- 過橋的所有值都做 JSON 編碼。物件與陣列必須可序列化；函式與 symbol 過不去。
- 外掛可熱重載：重載監看器總會執行（`runtime.reload.watch` 無效）。改迴圈檔案或指令檔案會在下一輪從零重新啟動工作階段 VM；改 `gateway.route` 背後的檔案會在訊息之間重建路由器狀態（路由在那兒跑，不在工作階段裡）；改被覆寫的支援片段會重新啟動所有作用中工作階段，新片段就在那時載入。都不需要重新啟動行程，而且新版本編譯不過就不會被套用 —— 正在跑的版本原樣留著。

### 實用範例

把下面任意一個放進迴圈檔案，把代理的 `loop:` 指向它即可：

#### 工具迴圈

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.type === "user" && msg.text) {
      const info = agent.info();
      const toolsList = tools.list();
      try {
        const reply = llm.chat(
          [{ role: "user", content: msg.text }],
          { model: info.model, tools: toolsList, tool_choice: "auto" },
        );
        if (reply.tool_calls && reply.tool_calls.length > 0) {
          for (const tc of reply.tool_calls) {
            const r = tools.run(tc.name, tc.args || {});
            log.info("tool result: " + json.encode(r));
          }
          session.send(reply.text || "done");
        } else {
          session.send(reply.text || "");
        }
      } catch (err) {
        session.send("error: " + String(err));
      }
    }
  }
}
```

#### Shell 迴圈

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.type === "user" && msg.text) {
      try {
        const r = shell.run("echo hello from " + msg.text);
        session.send("exit=" + r.exit_code + " stdout=" + r.stdout);
      } catch (err) {
        session.send("the shell is not available: " + String(err));
      }
    }
  }
}
```

#### 多代理請求

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.type === "user" && msg.text) {
      try {
        const answer = agent.request("agent:expert", { question: msg.text }, 30);
        session.send("expert says: " + ((answer.payload && answer.payload.answer) || "?"));
      } catch (err) {
        session.send("expert unavailable: " + String(err));
      }
    }
  }
}
```
