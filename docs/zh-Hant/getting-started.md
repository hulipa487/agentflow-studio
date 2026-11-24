# AgentFlow

以純 Go 打造、單一執行檔、可自行架設的 LLM 代理執行環境，內建 TypeScript/JS 迴圈與 Wasm 外部 API。一般需求靠設定，特殊需求靠寫程式。

## 這是什麼

AgentFlow 執行由 LLM 驅動的代理：它們會記憶、會呼叫工具、能執行 Shell 命令、能排程背景工作、能與其他代理對話，也能透過 Webhook、Telegram、QQ 與 OpenAI 相容用戶端服務使用者。每個代理都在 YAML 中宣告式定義；預設行為以外的一切，都來自可熱重載的 TypeScript/JS 外掛，這些外掛掛接在核心公開的版本化介面上。

## 設計原則

- **核心提供能力，外掛提供行為，YAML 負責串接。**
- **每個工作階段一個 VM 狀態** —— Actor 模型，只透過訊息溝通。
- **失敗就據實回報** —— 工具不可用就傳回結構化錯誤，絕不捏造結果。
- **內建功能也是外掛** —— 隨執行檔發佈，可依名稱覆寫，與使用者外掛遵循同一套約定。
- **能力邊界是一套訊息協定** —— 而不是某種語言的 API。

## 技術堆疊

| 決策 | 選擇 | 理由 |
| --- | --- | --- |
| 執行環境 | Go（`CGO_ENABLED=0`） | 純 Go 單一執行檔；每個 actor 一個 goroutine；交叉編譯零負擔 |
| 指令碼與外部 API | TypeScript/JS（`goja`）+ Wasm（`wazero`） | 純 Go 實作（無 cgo）；TypeScript 迴圈與擴充；隔離的 Wasm 外部 API 模組 |
| 並行 | Actor + 工作池 | 指令碼 VM 在非同步操作上暫停；不佔用作業系統執行緒 |
| 介面與文件 | AgentFlow Studio | 無頭執行環境提供 `/admin/api/`；Studio 提供桌面／網頁工作台與離線文件 |

> **目前狀態**：核心執行環境（actor、記憶、工具、Shell、多代理、排程器、預算、安全、指標）已實作並通過測試。儲存後端：SQLite、Redis 與 PostgreSQL 用於鍵值與文件資料列，另有一個記憶體內易失性儲存區；向量搜尋支援 pgvector、Qdrant、Redis Stack/RediSearch、啟用 vector 擴充的 PostgreSQL，或 SQLite 有上限的暴力掃描 —— 絕不會默默回傳空結果。LLM 供應商涵蓋四種聊天 API 格式（Anthropic Messages、OpenAI Chat Completions、OpenAI Responses、Gemini Interactions），外加一個 `rerank` 供應商；每一種都可指向官方主機，或透過 `base_url` 指向任何相容端點；各模型的 `server_tools` 可啟用供應商原生工具，例如 Google Search 檢索依據。頻道包括 Webhook（同步或非同步）、GitHub Webhook（`ghhook`）、Telegram（輪詢、Webhook 或自動設定的 `auto`）、QQ 機器人、`openai`（Chat Completions），以及 `bridge` —— 一套 HMAC 簽章的配接器協定，讓獨立行程的服務能接上任何其他聊天平台。它們都掛載在同一個共用 HTTP 監聽器上，內建 `/health` 探測端點。除此之外，執行環境還提供統一的媒體與檔案儲存區、由核心掌管的稽核日誌、用於語意記憶的嵌入與重新排序、身分與使用者層（使用者資料、頻道連結、OIDC、每位使用者的配額）、租戶（群組、成員資格、共用記憶集區與授權）、具快照版本管理的使用者範圍檔案儲存區、加密憑證儲存區、由引擎觸發的排程觸發器、組態目錄、僅能附加的日誌平面、單一與區域兩種部署層級、透過 stdio 或 Streamable HTTP 連線的 MCP 伺服器、宣告式 HTTP 工具，以及供 AgentFlow Studio 連線的無頭管理 API（`/admin/api/`）。

## 安裝指南

### 先決條件

- **Go 1.26+** —— AgentFlow 是 100% 純 Go（`CGO_ENABLED=0`）。任何作業系統上都不需要 C/C++ 編譯器、git 子模組或 `zig`。
- **GNU make**（選用）—— 對 `go build`、`go test` 與 `go vet` 的便利封裝。

### Windows / Linux / macOS

1. 安裝 [Go](https://go.dev/dl/) 1.26 或更新版本。
2. 複製並建置：

```
git clone https://github.com/hulipa487/agentflow.git
cd agentflow
go build -o agentflow ./cmd/agentflow   # Windows 上為 agentflow.exe，或直接執行 make
go test ./...
go vet ./...
```

交叉編譯用 Go 標準環境變數即可（例如 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/agentflow`，或 `make TARGET_OS=linux`）。

### 或者從範本開始

`agentflow init` 會寫出一份可直接執行的入門部署（`-layout dir` 產生組態目錄）；`agentflow validate` 與 `agentflow doctor` 能在不啟動引擎的情況下檢查組態：

```
./agentflow init                 # 寫入 agentflow.yaml
./agentflow validate             # 載入組態並編譯每個迴圈
./agentflow doctor               # 離線檢查：迴圈、憑證、模型、頻道
./agentflow doctor -online      # 同時連線到儲存區與後端（regional 的預設行為）
./agentflow                      # 執行；先尋找 agentflow.yaml，再找 config.yaml
```

### 驗證安裝

把一個最小 echo 迴圈存成 `echo.ts`（第一行是 prelude API 約定 —— 沒有這一行的程式碼區塊會被拒絕）：

```
// af-prelude-version: 2
function loop() {
  while (true) {
    const msg = session.inbox();
    session.send("echo: " + (msg.text || ""));
  }
}
```

再寫一份不需要模型的組態，把它接到 Webhook 上，存成 `agentflow.yaml`：

```
version: "1"
agents:
  echo:
    loop: ./echo.ts
gateway:
  listen: ":8080"
  channels:
    - type: webhook
      path: /webhook/
      agent: echo
```

執行它，然後用 curl 請求這個 Webhook：

```
./agentflow run -config agentflow.yaml
# 在另一個終端機中：
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'
```

你應該會在回應裡看到 `echo: hello`。注意結尾的斜線 —— Webhook 掛載的是 `/webhook/` 子樹，對不帶斜線的 `/webhook` 發 POST 會回傳 `307` 重新導向，而 `curl -d` 不會跟隨它。所有東西都在這一個連接埠上，包括管理平面的探測端點：

```
curl http://127.0.0.1:8080/healthz   # 存活檢查
curl http://127.0.0.1:8080/readyz    # 就緒檢查（啟動中或排空中時回傳 503）
curl http://127.0.0.1:8080/health    # 頻道監聽器自己的探測端點 -> {"ok":true}
```

### 疑難排解

| 症狀 | 解決方法 |
| --- | --- |
| 切換分支後出現 `Nothing to be done for 'all'` | Makefile 會追蹤 Go 原始碼與內嵌指令碼檔案，所以這種情況很少見。如果執行檔看起來還是舊的，執行 `make clean && make` 或 `go build ./cmd/agentflow`。 |
| 測試因 SQLite / database locked 失敗 | 序列執行測試（`go test -p 1 ./...`），或在兩次執行之間刪掉殘留的 `data/` 目錄。 |

### 更新

新版本發佈後，拉取程式碼並重新建置：

```
git pull
go build -o agentflow ./cmd/agentflow   # 或 make clean && make
```

## 快速開始

### 建置

AgentFlow 是純 Go（`CGO_ENABLED=0`），直接用 `go` 或 `make` 建置：

```
go build -o agentflow ./cmd/agentflow   # 或 make
go test ./...                           # 或 make test
```

### 執行

最快的路子不需要模型 —— 以檔案為基礎的 echo 迴圈開箱即用。讓執行檔指向你的組態：

```
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'   # 注意結尾的 /
```

想要 LLM 驅動的機器人，就給它接一個真實模型（見下一節），並把代理的 `model` 指向那個項目。

### 或者從情境開始

AgentFlow Studio 儲存庫在 [`examples/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples) 下提供完整的範例部署：每個都有組態、TypeScript 迴圈、人格提示詞、`.env.example`，以及一份 README。挑最接近你需求的那份複製走，或讓執行檔直接指向它：

| 情境 | 展示內容 |
| --- | --- |
| [`qq-group-bot-fts`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/qq-group-bot-fts) | QQ 群組機器人（FTS5）：SQLite FTS5 關鍵字召回（`plugin:recency`）、分群共用上下文、主動 `[NO_REPLY]` 參與、豆包搜尋與 `fetch` |
| [`qq-group-bot-vec`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/qq-group-bot-vec) | QQ 群組機器人（Embedding）：`bge-m3` SQLite 向量語意召回（`plugin:semantic`）、分群共用上下文、主動 `[NO_REPLY]` 參與、豆包搜尋與 `fetch` |
| [`telegram-group-bot-fts`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/telegram-group-bot-fts) | Telegram 群組機器人（FTS5）：SQLite FTS5 關鍵字召回（`plugin:recency`）、分群共用上下文、主動 `[NO_REPLY]` 參與、Google/X 搜尋、`fetch` 與 `browser` |
| [`telegram-group-bot-vec`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/telegram-group-bot-vec) | Telegram 群組機器人（Embedding）：`bge-m3` SQLite 向量語意召回（`plugin:semantic`）、分群共用上下文、主動 `[NO_REPLY]` 參與、Google/X 搜尋、`fetch` 與 `browser` |

```
./agentflow validate -config examples/qq-group-bot-fts/agentflow.yaml
./agentflow run -config examples/qq-group-bot-fts/agentflow.yaml
```

在建構器中建立代理時，Studio 的 **新增代理** 對話方塊會提供同樣四個起始範本（**QQ 群組機器人 (FTS5)**、**QQ 群組機器人 (Embedding)**、**Telegram 群組機器人 (FTS5)** 與 **Telegram 群組機器人 (Embedding)**），內建無需撰寫 TS 的 **範本設定**。

### 接入真實模型

`provider` 決定線路格式。`base_url` 留空則用該供應商的官方主機，也可以指向任何相容端點（Ollama、vLLM、LiteLLM、OpenRouter、企業閘道）：

```
models:
  default:
    provider: openai          # API 格式：anthropic | openai | openai-responses | gemini | rerank
    model: gpt-4o
    base_url: http://127.0.0.1:11434/v1   # Ollama / vLLM / LiteLLM / OpenRouter
    api_key: ${OPENAI_API_KEY}
    thinking: medium        # off | low | medium | high | xhigh | max；留空 = 供應商預設
    server_tools: []          # 供應商原生工具（gemini：google_search；openai-responses：web_search）
```

`provider` 選擇請求／回應格式，`base_url` 選擇主機。免金鑰的本機端點（Ollama）可以留空 `api_key` —— `gemini` 例外，它的用戶端沒金鑰就建構不出來。`thinking` 設定模型的預設思考等級 —— 一套與供應商無關的詞彙（`off` | `low` | `medium` | `high` | `xhigh` | `max`），執行環境會把它對應到各家原生的參數：Anthropic 的 `budget_tokens`、OpenAI 的 `reasoning_effort`，或 Gemini 的 `thinking_level`（外加 `thinking_summaries`，負責要求傳回思考摘要）。留空則什麼都不送，由供應商用自己的預設值；每次呼叫可透過 `llm.chat` 的 `opts.thinking` 覆寫。Gemini 的 SDK 帶來兩個限制：請求裡沒有低於 `minimal` 的等級，所以 `off` 對應成 `minimal` 而不是關掉思考；它的生成組態也沒有 temperature 欄位，因此那裡會按名稱拒絕 `llm.chat{temperature=...}`。`server_tools` 列出在供應商補全內部執行的伺服器端原生工具 —— 執行環境只注入原生工具項目，不附帶用戶端 schema。
