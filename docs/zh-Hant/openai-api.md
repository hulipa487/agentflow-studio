# OpenAI 相容 API

AgentFlow 可以把一個代理公開成 OpenAI **chat completions** 端點。任何會說 Chat Completions API 的用戶端 —— 代理框架、程式設計助理、聊天前端，或直接呼叫 SDK 的程式碼 —— 只需要一個 base URL 和一把 API 金鑰。不必安裝轉換層，也不必另開連接埠：這個頻道和其他所有頻道一樣掛在同一個 `gateway.listen` 上。

這個端點是一個**頻道**，不是代理。它負責認證、解析、關聯回覆、說 SSE。它不判斷語意：路由處理常式挑工作階段，代理的迴圈決定這個請求是什麼。用內建路由加上透傳迴圈（`examples/loops/openai_passthrough.ts`）時，每個請求都得到自己的一輪工作階段，並原樣轉發給代理的模型，所以一個本身就是代理的用戶端 —— 有自己的系統提示詞、自己的工具、自己的對話 —— 拿到的就恰好是一個模型。

## 快速開始

當代理的迴圈用 `llm.forward()` 轉發請求時，它就以自己的模型身分作答（隨附版本是 `examples/loops/openai_passthrough.ts`）。頻道不會替你挑迴圈：列在 `agents` 下的代理跑的就是它自己 `loop:` 指定的迴圈。

```yaml
agents:
  coder:
    loop: ./examples/loops/openai_passthrough.ts
    model: default
    capabilities: [llm.chat]      # llm.forward 與 openai.request 受它管控

gateway:
  listen: ":8080"
  channels:
    - name: api
      type: openai
      agents: [coder]
      auth: key
      keys: ["${COMPAT_API_KEY}"]
```

把用戶端指向 `http://<host>:8080/v1` 並帶上金鑰，模型名用 `coder`。

## 組態

```yaml
gateway:
  channels:
    - name: api
      type: openai
      path: /v1/                  # 提供 <path>chat/completions 與 <path>models
      agents: [coder, helper]     # 以模型 ID 公開（只有一個時可用 `agent:`）
      auth: key                   # user_token | key | none
      keys: ["${COMPAT_API_KEY}"] # 僅限 auth: key；可使用密鑰參照
      timeout: 600s               # 整個請求的時間上限
      max_request_bytes: 33554432 # POST 本文上限（預設 32 MiB）
      journal: metadata           # full | metadata | off；此類型預設為 metadata
      expose_reasoning: false     # 以 reasoning_content 傳送模型的推理內容
      media:                      # 內嵌圖片與檔案時需要
        allow: ["image/*"]
```

| 鍵 | 型別 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `path` | string | `/v1/` | 各端點掛載的前綴，必須以 `/` 結尾。發往其下其他路徑的請求會得到 OpenAI 格式的 404 信封 |
| `agents` | list | — | 以模型 ID 形式公開的代理。每個名字都必須是已設定的代理。只有一個時可用簡寫 `agent:`；兩者同時設定會報錯 |
| `auth` | string | — | 必填。`user_token`（依使用者的 Bearer 權杖，需要 `runtime.identity.enabled`）、`key`（來自 `keys` 的共用金鑰，以 Bearer 權杖或 `x-api-key` 送出），或 `none`（僅在 `gateway.listen` 為回送位址時可用） |
| `keys` | list | — | `auth: key` 接受的共用金鑰。`${VAR}` 與 `cred:<service>` 參照在啟動時解析；解析不出來會跳過該頻道並給出警告 |
| `timeout` | duration | `600s` | 整個請求，包括工具輪次 |
| `max_request_bytes` | int | `33554432` | POST 請求主體上限；更大的請求回傳 413 `request_too_large` |
| `journal` | string | `metadata` | `full` 保留文字與附件，`metadata` 只留 ID 與大小、**絕不留文字**，`off` 什麼都不寫。其他頻道類型預設 `full` |
| `expose_reasoning` | bool | `false` | 以 `reasoning_content` 傳送模型的推理內容。這會把模型的思考過程暴露給呼叫端 |
| `media` | map | 關閉 | 頻道的媒體原則。內嵌圖片或檔案（data URL）不在 `media.allow` 裡就回傳 400 |

`auth: user_token` 的身分來自 Bearer 權杖，並打進訊息由核心掌管的來源資訊裡，而不是取自請求主體。用 `auth: key` 或 `none` 時，請求沒有使用者：記憶範圍、配額與下文依使用者的限制對它都不適用。請求裡的 `user` 欄位與 `metadata` 會作為**不受信任**的資料傳給迴圈。

## 請求

| 欄位 | 處理方式 |
| --- | --- |
| `model` | `agents` 裡的代理名。未知則 404 `model_not_found` |
| `messages` | 角色有 `system`、`developer`、`user`、`assistant`、`tool`。內容是字串或片段：`text`、`image_url`、`input_audio`、`file`、`video_url` |
| `stream`、`stream_options.include_usage` | 支援 |
| `tools`、`tool_choice`、`parallel_tool_calls` | 轉發給模型。AgentFlow **絕不執行**：模型的工具呼叫會原樣回傳，用戶端在下一個請求裡把結果送回來 |
| `temperature`、`top_p`、`max_tokens`、`max_completion_tokens`、`stop`、`seed`、`response_format` | 代理的供應商支援就轉發（見[供應商](#providers)） |
| `n` | 只支援 1；更大的值回傳 400 |
| `user`、`metadata` | 作為不受信任的資料傳給迴圈。`metadata.conversation_id` 或 `X-AgentFlow-Conversation` 要求標頭可以要求一個對話工作階段 |
| `Idempotency-Key` 要求標頭 | 見[工作階段](#sessions) |
| 其他 | 接受並忽略 |

內嵌媒體（`data:` URL）依頻道的 `media:` 原則儲存，以控制代碼的形式到達迴圈與模型。遠端 URL 會交給自己去抓 URL 的供應商；執行環境絕不抓取用戶端給的 URL。

`GET <path>models` 列出公開的代理，`GET <path>models/{id}` 描述其中一個。

## 回應與錯誤

回應是標準的 `chat.completion` 與 `chat.completion.chunk` 物件。`finish_reason` 用模型自己的值（`stop`、`length`、`tool_calls`、`content_filter`）。工具呼叫的 `arguments` 是模型給的參數文字，原樣不動。`usage` 把 `input`→`prompt_tokens`、`output`→`completion_tokens`、`cached`→`prompt_tokens_details.cached_tokens`、`reasoning`→`completion_tokens_details.reasoning_tokens`。

串流下，文字邊產生邊到達（見[串流](#streaming)），工具呼叫在最後以一個完整區塊到達 —— 協定允許只有一個 `arguments` 片段 —— 然後是 `finish_reason` 區塊、設定了 `stream_options.include_usage` 時的用量區塊，以及 `data: [DONE]`。閒置的串流每 15 秒送一則 SSE 註解，讓代理保持連線。

錯誤使用 OpenAI 的信封 `{"error": {"message", "type", "code"}}`：

| 情況 | 狀態碼 | `code` |
| --- | --- | --- |
| 憑證缺失或錯誤 | 401 | `invalid_api_key` |
| 未知模型 | 404 | `model_not_found` |
| JSON 有誤、`n > 1`、不支援的內容片段、頻道不接受的媒體 | 400 | `invalid_request_error` |
| 代理的模型無法兌現的選項（見[供應商](#providers)） | 400 | `invalid_request_error` |
| HTTP 方法不對 | 405 | `method_not_allowed` |
| 請求過大 | 413 | `request_too_large` |
| 使用者配額或代理預算用盡 | 429 | `insufficient_quota` |
| 達到工作階段上限 | 429 | `rate_limit_exceeded`（帶 `Retry-After`） |
| 輸入被安全原則攔截 | 400 | `content_filter`；設定檔有拒絕文案時就是它 |
| 輸出被安全原則攔截 | 200 | `finish_reason: "content_filter"`；內容是設定檔的備援文案，或為空 |
| 模型呼叫失敗 | 502 | `upstream_error`（上游自己的訊息只記日誌，不回傳） |
| 代理的迴圈失敗 | 500 | `server_error`（只記日誌，不回傳） |
| 路由處理常式沒有傳遞請求 | 503 | `server_unavailable` |
| 正在排空（就緒為 false） | 503 | `server_unavailable` |
| 請求逾時 | 504 | `timeout` |

**串流請求把晚到的錯誤寫進串流裡。** 串流回應在本輪執行之前就開始（HTTP 200 加角色區塊），所以只有在那之前能確定的情況才有 HTTP 狀態碼：401、404、請求格式錯誤的 400、405、413，以及排空時的 503。之後的一切 —— 限額、輸入被攔、模型失敗、逾時 —— 都以一則 `data: {"error": {...}}` 事件到達，`type` 與 `code` 同上，隨後是 `data: [DONE]`。如果文字已經送出去了，串流會以 `finish_reason: "stop"` 正常結束，錯誤只記日誌。

## 工作階段

內建路由處理常式（`plugin:per_chat`）給每個 OpenAI 請求一個自己的工作階段：一個**短暫**工作階段，服務恰好一輪就消失。它的鍵每次請求都不同，所以不佔租約、不寫收件匣列，也永遠不會被回收。

並行短暫工作階段依使用者限制（`runtime.limits.ephemeral_per_tenant`，預設 16，`0` 關閉），與對話工作階段分開計算。這個限制需要使用者，所以只在 `auth: user_token` 下生效；`auth: key` 或 `none` 沒有依呼叫端的限制，共用金鑰應該放在你自己的限流器後面。

想要對話的用戶端用 `metadata.conversation_id` 或 `X-AgentFlow-Conversation` 要求標頭明確宣告。這會路由到正常的具名工作階段，鍵由頻道與對話 ID 組成。ID 由用戶端選擇，不依呼叫方隔離：用共用金鑰時，兩個呼叫方送同一個 ID 就會共用同一個工作階段。

`Idempotency-Key` 只對仍在執行的請求有效：同一呼叫方、同一請求主體、同一鍵的重試會加入正在跑的那一輪，並收到完整答案（串流會從第一個區塊重播）。一輪結束後的重試是新的一輪，換了請求主體或換了呼叫方也是新的一輪。訊息 ID 由鍵衍生（雜湊，絕不是要求標頭本身）；請求 ID 每輪都是新的。

用戶端斷線就結束請求；短暫工作階段沒人聽了，引擎就把它停掉，不會再把 Token 花在沒人讀的答案上。對話的輪次則會繼續跑完。

**區域單元。** 單輪工作階段在任何執行個體上都能用。對話工作階段只有在請求到達擁有該對話的執行個體時才會被應答：送到別處的請求會被排給擁有者，而擁有者沒有可應答的連線，`llm.forward` 在擁有者那邊也讀不到落在另一個執行個體上的請求主體。`agentflow validate` 與 `agentflow doctor` 會對這種組合提出警告。在限制解除之前，在負載平衡器上把對話固定到一個執行個體，或者乾脆別在區域單元裡用對話 ID。

## 迴圈收到什麼

請求主體不過指令碼橋。一個大的程式設計請求留在 Go 裡，訊息只帶中繼資料與一個控制代碼：

```ts
{
  api: "openai.chat",
  request_id: "…", request_ref: "…",
  model: "coder", stream: true,
  message_count: 12, has_tools: true, bytes: 48211,
  conversation_id: null,          // 來自標頭或 metadata，不受信任
  user: "…", metadata: { … },   // 不受信任
}
```

`msg.text` 是最後一則使用者訊息的文字（最後一則是工具結果時為空）；安全設定檔傳入檢查的就是它。

| 呼叫 | 用途 |
| --- | --- |
| `openai.request()` | 完整請求，作為物件：`{ model, stream, messages, tools, tool_choice, params, user, metadata }`。超過 4 MiB 的 JSON 會被拒絕 |
| `llm.forward(opts?)` | 把本輪的請求轉發給模型，不必先實體化。回傳 `{ text, tool_calls, finish_reason, usage, thinking, thinking_blocks }` |
| `session.reply(t)` | 以結構化答案結束本輪：`{ text, tool_calls, finish_reason, usage, reasoning }` |
| `session.send_delta(text)` | 送出答案的一個文字片段 |
| `session.send(text)` | 把文字附加到答案；迴圈去取下一則訊息時本輪結束 |

`llm.forward`、`openai.request` 與其他 `llm.*` 呼叫需要 `llm.chat` 能力。

## 透傳迴圈

`examples/loops/openai_passthrough.ts` 只有八行：

```ts
// af-prelude-version: 2
function loop() {
  while (true) {
    session.inbox();
    const reply = llm.forward();
    session.reply(reply);
  }
}
```

不帶參數呼叫 `llm.forward()`，會把用戶端自己的訊息、工具、工具選擇與取樣參數送給本輪所跑的模型：代理的 `model`，或使用者有個人設定檔時用他的模型。它和每次模型呼叫走同一條計量路徑：使用者配額、代理預算與用量帳冊都適用，帳冊列的 kind 是 `forward`。

### 串流

用戶端要求串流、且代理的安全設定檔沒有過濾器時，文字增量邊到邊轉發。設定檔裡有過濾器時，答案會先緩衝、檢查，再整塊送出 —— 安全優先於串流，用戶端仍然收到合法的串流。

### 推理

`expose_reasoning: true` 時，模型報告的推理內容會以 `reasoning_content` 送出，放在訊息裡或工具呼叫前的一個區塊裡。文字被安全設定檔拒絕的回覆不帶推理內容。

## 自己寫迴圈

頻道不附加任何原則；迴圈可以豐富請求，也可以不以模型身分、而以助理身分作答。下面這個透傳迴圈會先補上喚回到的事實：

```ts
function loop() {
  while (true) {
    const msg = session.inbox();
    const facts = memory.recall({ store: "facts", kind: "text", text: msg.text || "", k: 5 });
    let opts = undefined;
    if (facts && facts.length > 0) {
      const lines = facts.map((hit) => "- " + String(hit.value.text || ""));
      opts = { prepend: [{ role: "system", content: lines.join("\n") }] };
    }
    session.reply(llm.forward(opts));
  }
}
```

`agentflow-studio` 裡的 `examples/loops/openai_memory.ts` 是這個模式的完整版本。

`llm.forward` 的選項：`model`（另一個 `models:` 項目）、`messages`（取代用戶端的）、`prepend` / `append`（在前後加輪次）、`tools`（在用戶端的基礎上加定義）、`params`（覆寫取樣：`temperature`、`max_tokens`、`top_p`、`stop`、`seed`、`response_format`、`parallel_tool_calls`、`tool_choice`、`thinking`）以及 `max_tokens_cap`（給用戶端的 `max_tokens` 設上限）。

迴圈自己處理回覆時，有三點要知道：

- 回覆裡的工具呼叫有 `args`（物件，供讀取）與 `arguments`（模型自己的 JSON 文字）。`session.reply` 在 `arguments` 存在時會送它，以保留模型序列化出的精確參數。如果你改了某個呼叫的 `args`，請把它的 `arguments` 設成 `undefined` 或 `null`。
- 用 `session.send_delta` 送過片段、再用完整文字回覆的迴圈，文字只會送一次：回覆只補上片段沒有攜帶的部分。
- 不用 `session.reply` 結束本輪的迴圈照樣會作答：它用 `session.send` 或 `session.send_delta` 送出去的內容就是答案，`finish_reason` 為 `"stop"`。什麼也沒產生的輪次就是一條空的助理訊息。

別把持久狀態放在指令碼區域變數裡。單輪工作階段沒有狀態要保，對話工作階段閒置時會被卸載（見 [`runtime.sessions.idle_unload`](configuration.md)）；請用 `session.state.*`、記憶或儲存區。

## 供應商

用戶端送的東西會經由代理模型的供應商轉發過去，而各家是不一樣的。

| 供應商 | 說明 |
| --- | --- |
| `openai` | 上面說的一切原樣送出，包括 `developer` 訊息。`max_completion_tokens` 以 `max_tokens` 送出 |
| `openai-responses` | `stop` 與 `seed` 沒有對應項，會被丟棄 |
| `anthropic` | `system` 與 `developer` 訊息會合併進系統提示詞。`seed` 被丟棄。`response_format` 除 `{"type": "text"}` 之外一律回傳 400 |
| `gemini` | 用戶端的 `tools` 會被 400 拒絕，所以會送工具的程式設計用戶端用不了 Gemini 代理。`temperature`、`top_p`、`parallel_tool_calls` 被丟棄。`response_format` 除 `text` 之外，以及以遠端 URL 給出的媒體，都會被 400 拒絕 |

**輸出上限。** 用戶端既沒送 `max_tokens` 也沒送 `max_completion_tokens` 時，用模型項目自己的 `max_tokens`，預設 4096。程式設計用戶端經常不設上限，指望拿到模型自己的最大值，所以請把公開代理所用模型的 `max_tokens` 設成你希望這類請求獲得的值。被上限截斷的答案會以 `finish_reason: "length"` 回報。

只有當省略某個欄位不會改變用戶端拿到的結果種類時，才會把它丟掉；`response_format` 與具名 `tool_choice` 則直接拒絕 —— 因為要求 JSON 卻悄悄收到散文的呼叫方根本分辨不出來。

## 維運

- **稽核。** 用 `journal: metadata`（此類型的預設值）時，日誌為每個請求與每個答案各留一列，含 ID、大小、使用者與耗時，不含文字。
- **指標。** `agentflow_ephemeral_sessions`（作用中的單輪工作階段）、`agentflow_ephemeral_cap_drops`（被限額拒絕的請求）、`agentflow_sessions_unloaded`（被卸載的閒置對話），與常見的 `agentflow_llm_*` 和 `agentflow_egress_*` 計數器並列。
- **Doctor。** `agentflow doctor` 報告每個 `openai` 頻道的認證方式與公開的代理，並對 `auth: none` 提出警告。

## 相容性

**目前還沒有用戶端被列為已驗證。** 回放套件跑的是依 OpenAI API 參考手寫的樣本。只有當某個用戶端的一對真實請求／回應被提交到 `cmd/agentflow/testdata/chatapi` 並由測試回放後，這個用戶端才會被加進清單。產生這類記錄的錄製器在執行環境儲存庫的 `internal/drivers/chatapi/cmd/record`；把用戶端的 base URL 指向它，然後讓用戶端走一遍一般聊天、串流、一次工具呼叫及其後續。
