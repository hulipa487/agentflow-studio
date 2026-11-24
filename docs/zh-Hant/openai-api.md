# OpenAI 相容 API

AgentFlow 可以把代理公開為 OpenAI **chat completions** 端點。任何使用 Chat Completions API 的用戶端，無論是代理框架、程式設計助理、聊天前端，或單純的 SDK 程式碼，都只需要一個基底 URL 與一把 API 金鑰。不需要安裝轉換層，也沒有第二個連接埠：這個頻道和其他所有頻道一樣，掛載在同一個 `gateway.listen` 上。

這個端點是一個**頻道**，不是代理伺服器。它負責認證、解析、對應回覆，並以 SSE 傳送。它不對內容含義做任何判斷：由路由處理常式選擇工作階段，由代理的迴圈決定請求是什麼。使用內建路由與直通迴圈（`examples/loops/openai_passthrough.ts`）時，每個請求都會得到自己的單輪次工作階段，並原封不動地轉送給代理的模型；所以本身就是代理的用戶端（有自己的系統提示詞、自己的工具、自己的對話）拿到的，就恰好是一個模型。

## 快速入門

當代理的迴圈以 `llm.forward()` 轉送請求時（範例位於 `examples/loops/openai_passthrough.ts`），代理就以其模型的身分回答。頻道不會替你選擇迴圈：列在 `agents` 下的代理，執行的是它自己的 `loop:` 所指定的迴圈。

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

將用戶端指向 `http://<host>:8080/v1` 並帶上金鑰，再以 `coder` 作為模型名稱。

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

| 鍵 | 類型 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `path` | string | `/v1/` | 端點所在的前置路徑；必須以 `/` 結尾。對其下任何其他路徑的請求，會得到 OpenAI 格式的 404 錯誤封包 |
| `agents` | list | — | 以模型 ID 公開的代理。每個名稱都必須是已設定的代理。只有一個時可簡寫為 `agent:`；兩者同時設定會報錯 |
| `auth` | string | — | 必填。`user_token`（每位使用者各自的 bearer 權杖；需要 `runtime.identity.enabled`）、`key`（來自 `keys` 的共用金鑰，以 bearer 權杖或在 `x-api-key` 中傳送），或 `none`（僅限 `gateway.listen` 為回送位址時） |
| `keys` | list | — | `auth: key` 接受的共用金鑰。`${VAR}` 與 `cred:<service>` 參照在啟動時解析；有任何一個無法解析時，會略過此頻道並發出警告 |
| `timeout` | duration | `600s` | 整個請求的時間，包括工具往返 |
| `max_request_bytes` | int | `33554432` | POST 本文上限；超過的請求會得到 413 `request_too_large` |
| `journal` | string | `metadata` | `full` 保留文字與附件，`metadata` 保留 ID 與大小，**絕不保留文字**，`off` 不寫入任何東西。其他頻道類型預設為 `full` |
| `expose_reasoning` | bool | `false` | 以 `reasoning_content` 傳送模型的推理內容。這會讓呼叫方看到模型的思考過程 |
| `media` | map | 關閉 | 頻道的媒體政策。內嵌的圖片或檔案（data URL）除非類型在 `media.allow` 中，否則會以 400 拒絕 |

`auth: user_token` 的身分來自 bearer 權杖，並蓋在訊息中由核心掌管的來源資訊上，而不是從請求本文取得。使用 `auth: key` 或 `none` 時，請求沒有使用者：記憶範圍、配額與下文所述的每位使用者限制都不適用。請求中的 `user` 欄位與 `metadata` 會以**不受信任**的資料傳給迴圈。

## 請求

| 欄位 | 處理方式 |
| --- | --- |
| `model` | `agents` 中的代理名稱。不認得時：404 `model_not_found` |
| `messages` | 角色可為 `system`、`developer`、`user`、`assistant`、`tool`。內容可以是字串或多個部分：`text`、`image_url`、`input_audio`、`file`、`video_url` |
| `stream`, `stream_options.include_usage` | 支援 |
| `tools`, `tool_choice`, `parallel_tool_calls` | 轉送給模型。AgentFlow **絕不執行**：模型的工具呼叫會回傳出去，由用戶端在下一個請求中送回結果 |
| `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stop`, `seed`, `response_format` | 在代理的供應商支援時轉送（參見[供應商](#providers)） |
| `n` | 只能是 1；更大的值回傳 400 |
| `user`, `metadata` | 以不受信任的資料傳給迴圈。`metadata.conversation_id` 或 `X-AgentFlow-Conversation` 標頭會要求使用對話工作階段 |
| `Idempotency-Key` 標頭 | 參見[工作階段](#sessions) |
| 其他任何欄位 | 接受但忽略 |

內嵌媒體（`data:` URL）會依頻道的 `media:` 政策儲存，並以控制代碼的形式送到迴圈與模型。遠端 URL 會傳給本身會擷取 URL 的供應商；執行環境絕不擷取用戶端提供的 URL。

`GET <path>models` 列出公開的代理，`GET <path>models/{id}` 描述其中一個。

## 回應與錯誤

回應是標準的 `chat.completion` 與 `chat.completion.chunk` 物件。`finish_reason` 是模型本身給出的（`stop`、`length`、`tool_calls`、`content_filter`）。工具呼叫的 `arguments` 是模型產生的引數文字，原封不動。`usage` 的對應方式為 `input`→`prompt_tokens`、`output`→`completion_tokens`、`cached`→`prompt_tokens_details.cached_tokens`，以及 `reasoning`→`completion_tokens_details.reasoning_tokens`。

在串流中，文字會隨產生隨送達（參見[串流](#streaming)），工具呼叫則在最後以一個完整的區塊送達（協定允許單一 `arguments` 片段），接著是 `finish_reason` 區塊、設定了 `stream_options.include_usage` 時的用量區塊，以及 `data: [DONE]`。閒置的串流每 15 秒會送出一則 SSE 註解，讓代理伺服器保持連線。

錯誤使用 OpenAI 的封包格式 `{"error": {"message", "type", "code"}}`：

| 情況 | 狀態碼 | `code` |
| --- | --- | --- |
| 缺少憑證或憑證錯誤 | 401 | `invalid_api_key` |
| 不認得的模型 | 404 | `model_not_found` |
| JSON 錯誤、`n > 1`、不支援的內容部分、頻道不接受的媒體 | 400 | `invalid_request_error` |
| 代理的模型無法滿足的選項（參見[供應商](#providers)） | 400 | `invalid_request_error` |
| HTTP 方法錯誤 | 405 | `method_not_allowed` |
| 請求過大 | 413 | `request_too_large` |
| 使用者配額或代理預算用盡 | 429 | `insufficient_quota` |
| 達到工作階段上限 | 429 | `rate_limit_exceeded`（附 `Retry-After`） |
| 輸入被安全機制封鎖 | 400 | `content_filter`；若設定檔有拒絕回覆文字，即以它作為訊息 |
| 輸出被安全機制封鎖 | 200 | `finish_reason: "content_filter"`；內容為設定檔的替代回覆文字，或空白 |
| 模型呼叫失敗 | 502 | `upstream_error`（上游本身的訊息會記錄在日誌中，不會回傳） |
| 代理的迴圈失敗 | 500 | `server_error`（記錄在日誌中，不會回傳） |
| 路由處理常式沒有投遞請求 | 503 | `server_unavailable` |
| 排空中（就緒狀態為 false） | 503 | `server_unavailable` |
| 請求逾時 | 504 | `timeout` |

**串流請求會在串流中回報較晚發生的錯誤。** 串流回應在輪次開始執行之前就已經開始（HTTP 200 與角色區塊），所以只有在那之前已知的狀況才有 HTTP 狀態碼：401、404、格式錯誤請求的 400、405、413，以及排空中的 503。之後發生的一切（達到限制、輸入被封鎖、模型失敗、逾時）都會以一則 `data: {"error": {...}}` 事件送達，帶有相同的 `type` 與 `code`，後面接著 `data: [DONE]`。如果已經送出了文字，串流會以 `finish_reason: "stop"` 正常結束，錯誤則記錄在日誌中。

## 工作階段

內建的路由處理常式（`plugin:per_chat`）會給 OpenAI 請求一個專屬的工作階段：一個**臨時**工作階段，只服務一個輪次就結束。它的鍵每個請求都不同，所以不取得租約、不寫入收件匣資料列，也永遠不需要淘汰。

同時存在的臨時工作階段數量依使用者限制（`runtime.limits.ephemeral_per_tenant`，預設 16，`0` 表示停用），與對話工作階段分開計算。這項限制需要使用者，所以只在 `auth: user_token` 時生效；使用 `auth: key` 或 `none` 時沒有依呼叫方的限制，共用金鑰應放在你自己的速率限制器後方。

想要對話的用戶端，可以用 `metadata.conversation_id` 或 `X-AgentFlow-Conversation` 標頭選擇加入。這會路由到一般的具鍵工作階段，鍵由頻道與對話 ID 組成。ID 由用戶端自訂，且不限定於呼叫方：使用共用金鑰時，送出相同 ID 的兩個呼叫方會共用同一個工作階段。

對仍在執行中的請求，`Idempotency-Key` 會被遵守：同一個呼叫方以相同的鍵、相同的本文重試，會加入正在執行的輪次並收到完整的回答（串流會從第一個區塊重播）。輪次結束後的重試是新的輪次；相同的鍵但本文不同或來自不同呼叫方，也是新的輪次。訊息 ID 由這個鍵衍生（是雜湊值，絕不是標頭本身）；請求 ID 則每個輪次都是新的。

用戶端斷線會結束請求；當臨時工作階段已經沒有人在聆聽時，引擎會停止它，不再為沒有人會讀的回答花費 Token。對話的輪次則會讓它執行完畢。

**區域單元。** 單輪次工作階段在任何執行個體上都能運作。對話工作階段只有在請求抵達擁有該對話的執行個體時才會得到回答：落在其他地方的請求會排入佇列交給擁有者，但擁有者沒有可以回答的連線，而且擁有者上的 `llm.forward` 也讀不到抵達另一個執行個體的請求本文。`agentflow validate` 與 `agentflow doctor` 會對這種組合發出警告。在這項限制解除之前，請在負載平衡器上把對話固定到單一執行個體，或不要在區域單元中使用對話 ID。

## 迴圈收到什麼

請求本文不會跨過指令碼橋接。大型的程式設計請求留在 Go 中，訊息只帶有中繼資料與一個控制代碼：

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

`msg.text` 是最後一則使用者訊息的文字（最後一則訊息是工具結果時為空）；安全設定檔在傳入時檢查的就是它。

| 呼叫 | 用途 |
| --- | --- |
| `openai.request()` | 以物件形式取得完整請求：`{ model, stream, messages, tools, tool_choice, params, user, metadata }`。JSON 超過 4 MiB 時拒絕 |
| `llm.forward(opts?)` | 不具體化請求，直接把該輪次的請求轉送給模型。回傳 `{ text, tool_calls, finish_reason, usage, thinking, thinking_blocks }` |
| `session.reply(t)` | 以結構化的回答結束該輪次：`{ text, tool_calls, finish_reason, usage, reasoning }` |
| `session.send_delta(text)` | 送出回答的一段文字片段 |
| `session.send(text)` | 將文字附加到回答；迴圈要求下一則訊息時，輪次即結束 |

`llm.forward`、`openai.request` 與其他 `llm.*` 呼叫都需要 `llm.chat` 能力。

## 直通迴圈

`examples/loops/openai_passthrough.ts` 只有幾行：

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

不帶引數的 `llm.forward()` 會把用戶端自己的訊息、工具、工具選擇與取樣參數，送給該輪次所使用的模型：代理的 `model`，或使用者有設定時則為使用者資料中的模型。它走的計量路徑與所有模型呼叫相同：使用者的配額、代理的預算與用量帳本都適用，帳本資料列的 kind 為 `forward`。

### 串流

當用戶端要求串流，且代理的安全設定檔沒有任何過濾器時，文字增量會在抵達時即時轉送。如果設定檔有過濾器，回答會先緩衝、檢查，再整段送出：安全優先於串流，而用戶端收到的仍是有效的串流。

### 推理

設定 `expose_reasoning: true` 時，模型回報的推理內容會以 `reasoning_content` 送出，放在訊息上，或在工具呼叫之前的一個區塊中。文字被安全設定檔拒絕的回覆不會帶有推理內容。

## 撰寫自己的迴圈

頻道本身不附加任何政策；迴圈可以豐富請求內容，或改以助理而非模型的身分回答。以下是一個會在前面加上已記住事實的直通迴圈：

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

`agentflow-studio` 儲存庫中的 `examples/loops/openai_memory.ts` 是這個模式的完整版本。

`llm.forward` 的選項：`model`（另一個 `models:` 項目）、`messages`（取代用戶端的訊息）、`prepend` / `append`（在前後加入輪次）、`tools`（在用戶端的工具之外加入定義）、`params`（覆寫取樣參數：`temperature`、`max_tokens`、`top_p`、`stop`、`seed`、`response_format`、`parallel_tool_calls`、`tool_choice`、`thinking`），以及 `max_tokens_cap`（限制用戶端 `max_tokens` 的上限）。

迴圈自行處理回覆時，有三件事要知道：

- 回覆中的工具呼叫同時有 `args`（物件，供讀取）與 `arguments`（模型自己的 JSON 文字）。`session.reply` 在 `arguments` 存在時會原樣送出它，以保留模型輸出的原始 JSON 文字。如果你修改了某個呼叫的 `args`，請將它的 `arguments` 設為 `undefined` 或 `null`。
- 迴圈先以 `session.send_delta` 送出片段，再以完整文字回覆時，文字只會送出一次：回覆只會補上片段尚未包含的部分。
- 迴圈沒有呼叫 `session.reply` 就結束輪次，仍然會回答：它用 `session.send` 或 `session.send_delta` 送出的內容就是回答內容，並帶有 `finish_reason: "stop"`。沒有產生任何內容的輪次，會是一則空的助理訊息。

不要把需要持久保存的狀態放在指令碼區域變數中。單輪次工作階段沒有狀態需要保存，而對話工作階段在閒置時會被卸載（參見 [`runtime.sessions.idle_unload`](configuration.md)）；請使用 `session.state.*`、記憶或儲存區。

## 供應商

用戶端送來的內容，會透過代理模型的供應商轉送給該模型，而各供應商有所不同。

| 供應商 | 說明 |
| --- | --- |
| `openai` | 上述所有內容都原樣送出，包括 `developer` 訊息。`max_completion_tokens` 會以 `max_tokens` 送出 |
| `openai-responses` | `stop` 與 `seed` 沒有對應項目，會被捨棄 |
| `anthropic` | `system` 與 `developer` 訊息會合併到系統提示詞中。`seed` 會被捨棄。`{"type": "text"}` 以外的 `response_format` 會以 400 拒絕 |
| `gemini` | 用戶端的 `tools` 會以 400 拒絕，所以會送出工具的程式設計用戶端無法使用 Gemini 代理。`temperature`、`top_p` 與 `parallel_tool_calls` 會被捨棄。`text` 以外的 `response_format`，以及以遠端 URL 提供的媒體，都會以 400 拒絕 |

**輸出上限。** 當用戶端既沒有送出 `max_tokens` 也沒有送出 `max_completion_tokens` 時，會套用模型項目的 `max_tokens`，其預設值為 4096。程式設計用戶端常省略這項上限，並預期得到模型本身的最大值，所以請把公開代理所用模型的 `max_tokens`，設為你希望這類請求得到的值。因上限而被截斷的回答會以 `finish_reason: "length"` 回報。

只有在省略某個請求欄位不會改變用戶端拿回的內容種類時，才會捨棄它；`response_format` 與指名的 `tool_choice` 則會被拒絕，因為要求 JSON 卻默默收到一般文字的呼叫方，無從察覺。

## 維運

- **稽核。** 使用 `journal: metadata`（此頻道的預設值）時，稽核記錄為每個請求與每個回答各保留一列，包含 ID、大小、使用者與時間資訊，但不含文字。
- **指標。** `agentflow_ephemeral_sessions`（執行中的單輪次工作階段）、`agentflow_ephemeral_cap_drops`（被上限拒絕的請求）與 `agentflow_sessions_unloaded`（已卸載的閒置對話），與一般的 `agentflow_llm_*` 及 `agentflow_egress_*` 計數器並列。
- **Doctor。** `agentflow doctor` 會回報每個 `openai` 頻道的認證模式與公開的代理，並對 `auth: none` 發出警告。

## 相容性

**目前還沒有任何用戶端被列為已驗證。** 重播測試套件執行的是依據 OpenAI API 參考文件手寫的測試資料。只有當某個用戶端擷取到的請求／回應配對提交到 `cmd/agentflow/testdata/chatapi` 下並由測試重播之後，該用戶端才會加入這份清單。用來產生這些擷取資料的錄製工具，是執行環境儲存庫中的 `internal/drivers/chatapi/cmd/record`；將用戶端的基底 URL 指向它，並讓用戶端依序執行一般聊天、串流、一次工具呼叫及其後續請求。
