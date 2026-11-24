# 不重新編譯 Go 也能擴充 AgentFlow

AgentFlow 有四個擴充層面。前三個只需組態、TypeScript/JS 指令碼或 Wasm 外部 API；只有最後一個會改動二進位檔。

## 1. 宣告式 HTTP 工具

外部 API 不必寫指令碼或 Go，就能成為模型可見的工具：

```yaml
tools:
  http:
    - name: crm_lookup
      description: Look up a customer by email address.
      method: GET
      url: https://api.example.com/v1/customers
      query:
        email: "{email}"
      auth:
        service: crm
      params:
        type: object
        properties:
          email: {type: string}
        required: [email]
      response: customer
      timeout: 15s
      hosts: [api.example.com]
```

這個工具會註冊為 `http:crm_lookup`。執行環境負責：

- 模型看到的 JSON Schema；
- URL、查詢參數與 JSON 本文中的 `{arg}` 範本替換；
- 依租戶解析憑證（`auth.service`；值永遠不會傳到模型或訊息日誌）；
- 共用的位址防護（`net.http.allow_private`）與選用的主機允許清單；
- 回應大小上限，以及選用的點號路徑選取（`customer.id`、`items[0].name`）。

非 GET 工具預設為 `needs_confirm: true`；要改變這點，請明確設定它。當某個值恰好是 `{arg}` 時，本文範本會保留其 JSON 型別。

也可以匯入 OpenAPI 3 文件，而不必逐一手寫每個操作：

```bash
agentflow tools import-openapi openapi.yaml -out tools.yaml \
  -base-url https://api.example.com -auth crm -prefix crm
```

匯入工具會輸出一段 `tools:` 區塊供你審閱；它不會在執行時擷取規格。加上 `-read-only` 即只匯入 GET/HEAD。

## 2. MCP 伺服器

MCP 仍是接入任意外部工具的邊界。stdio 子行程或 Streamable HTTP 端點宣告在 `mcp.servers` 下；它的工具會以 `<server>/<tool>` 的名稱出現。HTTP 伺服器接受 `token:`，值可以是字面值、`${VAR}` 或 `cred:<service>`，並與 `http.request` 和 `builtin:fetch` 受相同的位址防護約束。

## 3. 頻道橋接

AgentFlow 未內建編譯的聊天平台，可以透過外部配接器加入。配接器把平台事件轉換成以下協定，並接收回覆；AgentFlow 不需要任何 Go 變更：

```yaml
gateway:
  channels:
    - name: slack
      type: bridge
      agent: support
      path: /bridge/slack/
      deliver_url: http://127.0.0.1:9100/deliver
      secret: "${SLACK_BRIDGE_SECRET}"
      token: "${SLACK_BRIDGE_TOKEN}"   # 選填，deliver_url 使用的 bearer 權杖
```

傳入（配接器 → 執行環境）：

```http
POST /bridge/slack/
X-Agentflow-Signature: sha256=<hex HMAC-SHA256 of the raw body>

{"from":"U123","chat_id":"C456","text":"hello","attachments":[]}
```

傳出（執行環境 → 配接器）：

```json
{"to":"C456","text":"...","attachments":[{"type":"image","mime":"image/png","handle":"media:..."}]}
```

HMAC 是必要的。執行環境信任配接器轉送平台的寄件者身分，但橋接的連結帳號會記錄為「自稱」；連結帳號仍須通過一般的頻道內 `/link` 驗證挑戰，才能取得使用者資料範圍。

## 4. 指令碼套件與外部 API

套件是一個存放依版本控管的 TypeScript/JS 原始碼的資料夾。先宣告，再從迴圈匯入：

```yaml
packages:
  greeting: ./packages/greeting
```

```ts
// af-prelude-version: 2
// af-import: greeting

function loop() {
  while (true) {
    const msg = session.inbox();
    session.send(Greeting.hello(msg.text));
  }
}
```

套件可以包含 `manifest.yaml`：

```yaml
name: greeting
version: "1"
prelude_api: 2
files: [greeting.ts]
```

沒有 manifest 時，`*.ts` 與 `*.js` 檔案依字典順序載入。套件是原始碼層級的組合，不是隔離的模組：它們共用工作階段的 Goja 虛擬機器狀態與能力集合（能力仍由代理控制）。代理的 `loop:` 參照也可直接使用 `package:<name>`。編輯套件會重新啟動可能匯入它的工作階段。若需要隔離的可重用整合，請在 `apis` 下設定 TypeScript/JS 或 `wazero` Wasm 模組，並透過 `api.*` 從迴圈呼叫。

## 5. 仍然需要 Go 的部分

編譯式驅動程式仍需編譯：新的 LLM 傳輸協定、新的記憶／向量後端、新的搜尋引擎或 shell 供應商，以及新的原生 Go *種類*安全過濾器。以模型為基礎的內容審核使用 `safety.classifiers`（`classifier:<name>`），自訂指令碼掛鉤則使用 `safety.script_filters`（`script:<name>`）。替代做法如下：

- 新的模型廠商使用 OpenAI 相容端點（`provider: openai` + `base_url`）；
- 新的搜尋／API 供應商使用 MCP、外部 API（以 Goja 或 `wazero` Wasm 實作的 `apis.*`）或宣告式 HTTP 工具；
- 新的聊天平台使用橋接；
- 共用原則使用 TypeScript/JS 套件；
- 不重新編譯就做內容審核：使用分類器提示詞加上標籤原則，或 `safety.script_filters` 掛鉤。

## 6. 租戶控制

四個部署層級的開關可補足產品層面的缺口，且不改變推論路徑：

```yaml
runtime:
  users:
    unregistered: reject        # 捨棄未連結的帳號，而不是歸入服務層
    push_same_tenant: true      # session.push_user 只能以目前租戶為目標
  credentials:
    engine_wide: maintenance    # credential.get() 只在引擎觸發的輪次中可用
  admin:
    operators:                  # 具名的管理權杖；X-Operator 標示操作者
      alice: "${ALICE_ADMIN_TOKEN}"
```

租戶生命週期端點為 `GET /admin/api/usage`、`GET /admin/api/users/{id}/export` 與 `DELETE /admin/api/users/{id}?confirm=<id>`。刪除路徑會移除使用者資料、連結帳號、權杖、憑證、用量、訊息日誌與檔案中繼資料；記憶資料列依其儲存區的保留期限處理，以內容定址的 blob 則由檔案 GC 回收。管理 API 仍只有一個權限等級：操作人員名稱用來標示是誰執行了操作，但不會區分讀取／寫入權限。

## 7. 部署前先測試

```bash
agentflow validate -config agentflow.yaml
agentflow loop check loops/support.ts
agentflow loop test scenarios/refund.yaml
```

情境就是資料：

```yaml
agent: support
loop: ../loops/support.ts
llm:
  - match: refund
    text: "I can help with that."
messages:
  - from: alice
    text: "I need a refund"
expect_contains: ["help"]
```

`loop test` 會以假的 LLM／工具／頻道相依項目執行真正的 actor，因此不需要憑證，也不需要網路。完整的情境結構請見 `internal/looptest`。

## 掛鉤與擴充點

沒有掛鉤登錄表。引擎本身只分派三個點，其餘都是**由指令碼呼叫的命名慣例**：外掛提供一個全域變數，名稱就是呼叫端要找的名稱，這就是全部的約定。兩者的差別決定了外掛程式碼必須放在哪裡，所以這裡兩種都列出來。

### 由核心分派

無論迴圈做什麼，這些點在每一輪都會執行。替換迴圈處理常式也無法移除它們——前兩個存在的目的正是如此。

| 點 | 種類 | 實作者 | 說明 |
| --- | --- | --- | --- |
| `safety.ingress` | 過濾器 | Go，由核心擁有 | 在迴圈看到訊息之前執行；可以捨棄訊息、略過這一輪，或改寫文字。基準過濾器中只有 `affect-guard` 會捨棄訊息，`classifier:<name>` 與 `script:<name>` 過濾器可依原則封鎖或重新導向。 |
| `safety.egress` | 過濾器 | Go，由核心擁有 | 在回覆送達頻道之前執行；可以捨棄回覆（並可附上由核心撰寫的替代回覆）。 |
| `gateway.route` | 處理常式 | 由 `gateway.route` 指定的指令碼程式碼區塊，預設為 `plugin:per_chat` | 每個行程一個 Goja 虛擬機器。讀取訊息並傳回工作階段位址。路由處理常式只能呼叫 `inbox`、`deliver`、`runtime.triggers`、`router.state.*` 與 `log`；其他呼叫都會依名稱被拒絕。 |

### 由指令碼呼叫的命名慣例

這些是一般的全域變數，不是引擎會呼叫的進入點。當部署設定了對應行為時，prelude 或迴圈會依名稱呼叫它們；要遮蔽內建項目，就在 `plugins/<name>.ts` 中定義同名的全域變數。

| 設定方式 | 指令碼必須定義的全域變數 | 呼叫者 |
| --- | --- | --- |
| 記憶設定檔 `write: plugin:routing_table` | `memory_routing_table(record)` | prelude 的 `memory.write` |
| 記憶設定檔 `recall: plugin:recency` | `memory_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 記憶設定檔 `recall: plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 支援程式碼區塊 `plugin:token_budget` | `token_budget(messages, budget)` | 迴圈，在需要修剪提示詞時 |
| 迴圈外掛 `plugin:per_chat` | `loop()` | 沒有——它*本身*就是迴圈，不是輔助函式 |

> **本節過去的內容**：先前列出了 26 個掛鉤——`gateway.ingress`、`loop.before_turn`、`session.migrate`、`shell.before_exec`、`agent.start`、`core.on_reload` 等——附有種類、簽章與預設值，並聲稱由核心分派。實際上並不存在分派器，所以定義這些名稱的外掛會默默地永遠不被呼叫。其中兩個指向內建項目（`shell.before_exec` 的 `plugin:exec_policy`、`shell.teardown` 的 `plugin:ttl`），它們會載入每個工作階段狀態，卻沒有任何東西呼叫；兩者現已移除。**Shell 指令只受 `shell.exec` 能力控管——沒有指令原則過濾器。** 迴圈當機時會重新啟動，檔案變更時會熱重新載入，這正是 `session.resume` 與 `core.on_reload` 想要達成的；需要在這兩種情況之間保留狀態的迴圈，請使用 `session.state.*`。
