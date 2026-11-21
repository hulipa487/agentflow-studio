# AgentFlow

以純 Go 打造、內建 Lua 迴圈、單一執行檔、可自行架設的 LLM 代理執行環境。一般需求用設定，特殊需求寫程式。

## 這是什麼

AgentFlow 執行由 LLM 驅動的代理：它們能記憶、使用工具、執行 shell 命令、排程背景工作、與其他代理溝通，並透過 Webhook、Telegram、QQ 與 OpenAI 相容介面服務使用者。每個代理都以 YAML 宣告式定義；預設以外的所有行為，都來自可熱重新載入的 Lua 外掛，這些外掛掛接在核心公開的版本化介面上。

## 設計原則

- **核心提供能力，外掛提供行為，YAML 負責串接。**
- **每個工作階段一個 Lua 狀態**：Actor 模型，只透過訊息傳遞溝通。
- **失敗時預設據實以告**：無法使用的工具會回傳結構化錯誤，絕不捏造結果。
- **內建功能也是外掛**：隨執行檔發佈，可依名稱覆蓋，與使用者外掛遵守相同的契約。
- **能力邊界是一套訊息協定**，而不是某種語言的 API。

## 技術堆疊

| 決策 | 選擇 | 理由 |
| --- | --- | --- |
| 執行環境 | Go（`CGO_ENABLED=0`） | 純 Go 單一執行檔；每個 actor 一個 goroutine；零設定交叉編譯 |
| 指令碼 | Lua 5.1（`gopher-lua`） | 純 Go 實作（無 cgo）；原生協程；附指令預算的沙箱隔離 |
| 並行模型 | Actor + worker 池 | Lua 協程在非同步操作上暫停，不佔用作業系統執行緒 |
| 介面與文件 | AgentFlow Studio | 無頭執行環境提供 `/admin/api/`；Studio 提供桌面／網頁工作台與離線文件 |

> **目前狀態**：核心執行環境（actor、記憶、工具、shell、多代理、排程器、預算、安全、指標）已實作並經過測試。儲存後端：SQLite、Redis 與 PostgreSQL 用於鍵值與文件資料列，另有記憶體內的揮發性儲存區，向量搜尋則可透過 pgvector、Qdrant、Redis Stack/RediSearch、啟用 vector 擴充的 PostgreSQL，或 SQLite 有上限的暴力掃描；絕不會默默回傳空結果。LLM 供應商涵蓋四種聊天 API 格式（Anthropic Messages、OpenAI Chat Completions、OpenAI Responses、Gemini Interactions），外加一個 `rerank` 供應商，每一種都可指向官方主機，或透過 `base_url` 指向任何相容端點；各模型的 `server_tools` 可啟用供應商原生工具，例如 Google Search 搜尋依據（grounding）。頻道包括 Webhook（同步或非同步）、GitHub Webhook（`ghhook`）、Telegram（輪詢、Webhook 或自動設定的 `auto`）、QQ 機器人、`openai`（Chat Completions），以及 `bridge`：一套以 HMAC 簽章的配接器協定，讓獨立行程的服務能接上任何其他聊天平台。這些頻道都掛載在同一個共用的 HTTP 監聽器上，並內建 `/health` 探測端點。除此之外，執行環境還提供統一的媒體與檔案儲存區、由核心掌管的稽核記錄、用於語意記憶的嵌入與重新排序、身分與使用者層（使用者資料、頻道連結、OIDC、每位使用者的配額）、租戶（群組、成員資格、共用記憶池與授權）、具快照版本管理的使用者範圍檔案儲存區、加密的憑證儲存區、由引擎觸發的排程觸發器、組態目錄、僅能附加的日誌平面、單一與區域兩種部署層級、透過 stdio 或 Streamable HTTP 連線的 MCP 伺服器、宣告式 HTTP 工具，以及供 AgentFlow Studio 連線的無頭管理 API（`/admin/api/`）。

## 安裝指南

### 先決條件

- **Go 1.26+**：AgentFlow 是 100% 純 Go（`CGO_ENABLED=0`）。在任何作業系統上都不需要 C/C++ 編譯器、git 子模組或 `zig`。
- **GNU make**（選用）：`go build`、`go test` 與 `go vet` 的便利包裝。

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

交叉編譯直接使用標準 Go 環境變數即可（例如 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/agentflow` 或 `make TARGET_OS=linux`）。

### 或從範本開始

`agentflow init` 會寫出一份可直接執行的入門部署（`-layout dir` 則產生組態目錄）；`agentflow validate` 與 `agentflow doctor` 可在不啟動引擎的情況下檢查組態：

```
./agentflow init                 # 寫入 agentflow.yaml
./agentflow validate             # 載入組態並編譯每個迴圈
./agentflow doctor               # 離線檢查：迴圈、憑證、模型、頻道
./agentflow doctor -online      # 同時連線到儲存區與後端（regional 的預設行為）
./agentflow                      # 執行；先尋找 agentflow.yaml，再找 config.yaml
```

### 驗證安裝

把一個最簡單的 echo 迴圈存成 `echo.lua`（第一行是 prelude API 契約，沒有這行的程式碼區塊會被拒絕）：

```
-- af-prelude-version: 1
function loop()
  while true do
    local msg = session.inbox()
    session.send("echo: " .. (msg.text or ""))
  end
end
```

再把一份不需要模型、將它接到 Webhook 的組態存成 `agentflow.yaml`：

```
version: "1"
agents:
  echo:
    loop: ./echo.lua
gateway:
  listen: ":8080"
  channels:
    - type: webhook
      path: /webhook/
      agent: echo
```

執行它，再用 curl 呼叫 Webhook：

```
./agentflow run -config agentflow.yaml
# 在另一個終端機中：
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'
```

回應中應該會看到 `echo: hello`。注意結尾的斜線：Webhook 掛載的是 `/webhook/` 整個子樹，POST 到不含斜線的 `/webhook` 會得到 `307` 重新導向，而 `curl -d` 不會跟隨。所有東西都在這一個連接埠上，包括管理平面的探測端點：

```
curl http://127.0.0.1:8080/healthz   # 存活檢查
curl http://127.0.0.1:8080/readyz    # 就緒檢查（啟動中或排空中時回傳 503）
curl http://127.0.0.1:8080/health    # 頻道監聽器自己的探測端點 -> {"ok":true}
```

### 疑難排解

| 症狀 | 解決方式 |
| --- | --- |
| 切換分支後出現 `Nothing to be done for 'all'` | Makefile 會追蹤 Go 原始碼與內嵌的 Lua 檔案，所以這種情況很少見。如果執行檔看起來仍是舊的，請執行 `make clean && make` 或 `go build ./cmd/agentflow`。 |
| 測試因 SQLite / database locked 而失敗 | 依序執行測試（`go test -p 1 ./...`），或在每次執行之間刪除殘留的 `data/` 目錄。 |

### 更新

有新版本時，拉取後重新完整建置：

```
git pull
go build -o agentflow ./cmd/agentflow   # 或 make clean && make
```

## 快速入門

### 建置

AgentFlow 為純 Go 建置（`CGO_ENABLED=0`），直接使用 `go` 或 `make` 即可：

```
go build -o agentflow ./cmd/agentflow   # 或 make
go test ./...                           # 或 make test
```

### 執行

最快的方式不需要模型：以檔案定義的 echo 迴圈開箱即可回覆。將執行檔指向你的組態：

```
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'   # 注意結尾的 /
```

若要使用以 LLM 驅動的機器人，請接上真正的模型（見下一節），並將代理的 `model` 設為該項目。

### 或從情境範例開始

AgentFlow Studio 儲存庫在 [`examples/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples) 下提供完整的部署範例：每個都包含組態、迴圈與提示詞、迴圈測試，以及一份 README。複製最接近你需求的那一個，或搭配執行環境執行：

| 情境 | 示範內容 |
| --- | --- |
| [`qq-group-chat`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/qq-group-chat) | 具備記憶與上下文的 QQ 群聊機器人：每個群組一個工作階段、能辨識發言者的提示詞、存放在 `session.state` 的近期上下文、長期召回，以及 `/remember` 事實 |
| [`telegram-rag`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/telegram-rag) | 依知識庫作答的 Telegram 機器人：嵌入存放在 SQLite、語意召回、附引用來源的回答，以及用於大量匯入的 Webhook |
| [`github-pr-agents`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/github-pr-agents) | 為每個 PR 衍生一個程式碼代理的 GitHub 拉取請求 Webhook：變更會複製到沙箱專案中，以 shell 命令檢查，並在留言中審查 |
| [`chat-completions`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/chat-completions) | 由代理支撐、單一輪次的 OpenAI 相容 `/v1/chat/completions` 端點 |

```
./agentflow -config examples/telegram-rag/agentflow.yaml
./agentflow loop test examples/telegram-rag/tests/answer.yaml
```

Studio 的 **新增代理** 對話方塊提供相同類型的入門範本（具記憶的群聊、知識庫、單一輪次 API），供你從頭建立代理。

### 接上真正的模型

`provider` 決定傳輸格式。`base_url` 留空會使用該供應商的官方主機，也可以設為任何相容端點（Ollama、vLLM、LiteLLM、OpenRouter、企業閘道）：

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

`provider` 選擇請求／回應格式，`base_url` 選擇主機。對不需金鑰的本機端點（Ollama），`api_key` 可以留空，但 `gemini` 例外：沒有金鑰時它的用戶端無法建立。`thinking` 設定模型的預設思考等級：這是一套與供應商無關的詞彙（`off` | `low` | `medium` | `high` | `xhigh` | `max`），執行環境會對應到各供應商的原生參數：Anthropic 的 `budget_tokens`、OpenAI 的 `reasoning_effort`，或 Gemini 的 `thinking_level`（再加上 `thinking_summaries`，它才是要求回傳摘要的設定）。留空則不送出任何值，由供應商採用自己的預設；每次呼叫可用 `llm.chat` 的 `opts.thinking` 覆寫。Gemini 的 SDK 帶來兩項限制：請求中沒有低於 `minimal` 的等級，所以 `off` 會對應到 `minimal`，而不是停用思考；它的生成組態也沒有 temperature 欄位，所以在那裡 `llm.chat{temperature=...}` 會被指名拒絕。`server_tools` 列出供應商原生、在供應商完成請求的過程中於伺服器端執行的工具；執行環境只會注入原生工具項目，不提供用戶端結構描述。
