# 不重新編譯 Go 也能擴充 AgentFlow

AgentFlow 有四個擴充面。前三個靠組態、TypeScript/JS 或 Wasm 外部 API 就能搞定；只有最後一個要改執行檔。

## 1. 宣告式 HTTP 工具

把外部 API 變成模型可見的工具，不必寫指令碼，也不必碰 Go：

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

這個工具註冊為 `http:crm_lookup`。以下這些由執行環境負責：

- 模型看到的 JSON Schema；
- URL、query 與 JSON 請求主體裡的 `{arg}` 樣板替換；
- 依租戶解析憑證（`auth.service`，憑證值永遠不會到模型或日誌裡）；
- 共用的位址防護（`net.http.allow_private`）與選用的主機允許清單；
- 回應大小上限，以及選用的點路徑取值（`customer.id`、`items[0].name`）。

非 GET 工具預設 `needs_confirm: true`；要改就明確設定。當某個值正好是 `{arg}` 時，請求主體樣板會保留它的 JSON 型別。

也可以直接匯入 OpenAPI 3 文件，不必手寫每個操作：

```bash
agentflow tools import-openapi openapi.yaml -out tools.yaml \
  -base-url https://api.example.com -auth crm -prefix crm
```

匯入器會產生一個 `tools:` 區塊供你審閱，執行環境並不會去抓取這份規格。加上 `-read-only` 則只匯入 GET/HEAD。

## 2. MCP 伺服器

任意外部工具的邊界仍然交給 MCP。stdio 子行程或 Streamable HTTP 端點在 `mcp.servers` 下宣告，其工具顯示為 `<server>/<tool>`。HTTP 伺服器接受 `token:`，可以是字面值、`${VAR}` 或 `cred:<service>`，並與 `http.request`、`builtin:fetch` 受同一套位址防護約束。

## 3. 頻道橋接

AgentFlow 沒編譯進去的聊天平台，可以用外部配接器接進來。配接器把平台事件翻譯成下面的協定並接收回覆，AgentFlow 一行 Go 都不用改：

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

HMAC 是必要的。配接器被信任可以轉達平台端的傳送者身分，但橋接拿到的帳號只記為「聲稱」，要取得資料範圍仍需走正常的頻道內 `/link` 驗證。

## 4. 指令碼套件與外部 API

套件是一個目錄，裡面是帶版本閘門的 TypeScript/JS 原始碼。宣告它，然後在迴圈裡匯入：

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

套件裡可以帶 `manifest.yaml`：

```yaml
name: greeting
version: "1"
prelude_api: 2
files: [greeting.ts]
```

沒有 manifest 時，`*.ts` 與 `*.js` 依字典序載入。套件是原始碼組合，不是隔離模組：它們共用工作階段的 Goja VM 狀態與能力集（這些仍由代理控制）。`package:<name>` 也可以直接當作代理的 `loop:` 參照。改動一個套件會重新啟動可能匯入它的工作階段。需要隔離的可重用整合，就在 `apis` 下設定 TypeScript/JS 或 `wazero` Wasm 模組，迴圈裡用 `api.*` 呼叫。

## 5. 哪些還是得靠 Go

編譯進去的驅動還是得編譯：新的 LLM 線路協定、新的記憶／向量後端、新的搜尋引擎或 Shell 提供者，以及新的一類 Go 原生安全過濾器。以模型為後盾的審核用 `safety.classifiers`（`classifier:<name>`），指令碼過濾器用 `safety.script_filters`（`script:<name>`）。其餘的變通做法有：

- 新型號供應商走 OpenAI 相容端點（`provider: openai` + `base_url`）；
- 新的搜尋／API 提供者，用 MCP、外部 API（Goja 裡的 `apis.*` 或 `wazero` Wasm），或宣告式 HTTP 工具；
- 新聊天平台走 bridge；
- 共用原則寫成 TypeScript/JS 套件；
- 不想重新編譯又要做審核，就用分類器提示詞加標籤原則，或掛一個 `safety.script_filters` 掛鉤。

## 6. 租戶控管

四個部署層級開關就能補上產品層的缺口，完全不動推論路徑：

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

租戶生命週期端點有 `GET /admin/api/usage`、`GET /admin/api/users/{id}/export` 與 `DELETE /admin/api/users/{id}?confirm=<id>`。刪除會清掉資料、帳號、權杖、憑證、用量、日誌與檔案中繼資料；記憶紀錄依各自儲存區的保留期限處理，內容定址的 blob 由檔案 GC 回收。管理 API 仍然只有一個權限層級：操作員名稱用於歸屬操作，但讀與寫並不區分層級。

## 7. 部署前先測

```bash
agentflow validate -config agentflow.yaml
agentflow loop check loops/support.ts
agentflow loop test scenarios/refund.yaml
```

情境本身就是資料：

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

`loop test` 用假的 LLM／工具／頻道相依性跑真實的 actor，所以不需要憑證也不需要網路。完整的情境結構見 `internal/looptest`。

## 掛鉤與擴充點

AgentFlow 沒有掛鉤登錄表。引擎自己分派三個點，其餘全靠 **指令碼依約定命名全域函式**：外掛提供一個全域，呼叫方依名字去找，這就是全部的約定。兩種機制決定了外掛程式碼該放哪裡，所以這裡都列出來。

### 由核心分派

不論迴圈怎麼寫，這些每一輪都會跑。替換迴圈處理函式也卸不掉它們 —— 前兩個存在的意義正是如此。

| 位置 | 類型 | 實作方 | 說明 |
| --- | --- | --- | --- |
| `safety.ingress` | 過濾器 | Go，核心掌管 | 在迴圈看到訊息之前執行；可以丟棄訊息、跳過本輪，或改寫文字。基線過濾器裡只有 `affect-guard` 會丟棄，而 `classifier:<name>` 與 `script:<name>` 過濾器可以攔截或重新導向。 |
| `safety.egress` | 過濾器 | Go，核心掌管 | 在回覆送達頻道之前執行；可以丟棄回覆（可附上由核心撰寫的備援文案）。 |
| `gateway.route` | 處理常式 | 由 `gateway.route` 指定的指令碼區塊，預設 `plugin:per_chat` | 每個行程一個 Goja VM 狀態。讀取訊息並傳回工作階段位址。路由處理常式只能呼叫 `inbox`、`deliver`、`runtime.triggers`、`router.state.*` 與 `log`，其他一律依名稱拒絕。 |

### 指令碼依約定呼叫的名稱

這些是普通的全域函式，不是引擎呼叫的入口。部署組態啟用了對應行為時，prelude 或迴圈會依名字呼叫它們；要覆寫內建實作，就在 `plugins/<name>.ts` 裡定義同名全域。

| 組態寫法 | 指令碼需定義的全域 | 呼叫方 |
| --- | --- | --- |
| 記憶設定檔 `write: plugin:routing_table` | `memory_routing_table(record)` | prelude 的 `memory.write` |
| 記憶設定檔 `recall: plugin:recency` | `memory_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 記憶設定檔 `recall: plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 支援片段 `plugin:token_budget` | `token_budget(messages, budget)` | 迴圈，需要裁剪提示詞時 |
| 迴圈外掛 `plugin:per_chat` | `loop()` | 無人呼叫 —— 它本身就是迴圈，不是輔助函式 |

> **本節以前寫的是什麼**：它列了 26 個掛鉤 —— `gateway.ingress`、`loop.before_turn`、`session.migrate`、`shell.before_exec`、`agent.start`、`core.on_reload` 等 —— 還寫了類型、簽章與預設值，並聲稱由核心分派。並不存在這樣的分派器，定義這些名字的外掛從未被呼叫過。其中兩個還點到了內建實作（`shell.before_exec` 對應 `plugin:exec_policy`，`shell.teardown` 對應 `plugin:ttl`），它們被載入每個工作階段狀態卻沒有任何呼叫方；這兩個現在都已移除。**本機 Shell 命令只受 `shell.exec` 能力約束，遠端 SSH 工作階段只受 `net.ssh` 能力約束 —— 不存在命令原則過濾器。** 迴圈崩潰會重新啟動，檔案變更會熱重載，這才是 `session.resume` 與 `core.on_reload` 當初想要的；需要在兩者之間保留狀態的迴圈，請用 `session.state.*`。
