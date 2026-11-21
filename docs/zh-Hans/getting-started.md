# AgentFlow

一个纯 Go 构建、内置 Lua 循环、单个二进制文件、可自托管的 LLM 智能体运行时。常规的部分靠配置，特殊的部分靠编程。

## 它是什么

AgentFlow 运行由 LLM 驱动的智能体：它们能记忆、使用工具、执行 shell 命令、调度后台任务、与其他智能体对话，并通过 Webhook、Telegram、QQ 以及 OpenAI 兼容接口为用户提供服务。每个智能体都在 YAML 中声明式定义；默认行为之外的一切都来自可热重载的 Lua 插件，这些插件挂接在核心暴露的版本化接口上。

## 设计原则

- **核心提供能力；插件提供行为；YAML 负责连接。**
- **每个会话一个 Lua 状态** — 参与者（actor）模型，只通过消息传递通信。
- **失败时如实报告** — 不可用的工具返回结构化错误，绝不编造结果。
- **内置功能也是插件** — 随二进制文件发布，可按名称覆盖，与用户插件遵循同一套契约。
- **能力边界是一套消息协议** — 而不是某种语言的 API。

## 技术栈

| 决策 | 选择 | 理由 |
| --- | --- | --- |
| 运行时 | Go（`CGO_ENABLED=0`） | 纯 Go 单二进制；每个参与者一个 goroutine；零配置交叉编译 |
| 脚本 | Lua 5.1（`gopher-lua`） | 纯 Go 实现（无 cgo）；原生协程；带指令预算的沙箱隔离 |
| 并发 | 参与者 + 工作池 | Lua 协程在异步操作上挂起；不占用操作系统线程 |
| 界面与文档 | AgentFlow Studio | 无头运行时提供 `/admin/api/`；Studio 提供桌面/网页工作台与离线文档 |

> **当前状态**：核心运行时（参与者、记忆、工具、shell、多智能体、调度器、预算、安全、指标）已实现并经过测试。存储后端：SQLite、Redis 和 PostgreSQL 用于键值和文档行，一个内存中的易失性存储，以及通过 pgvector、Qdrant、Redis Stack/RediSearch、带 vector 扩展的 PostgreSQL 或 SQLite 有上限的暴力扫描实现的向量搜索 — 绝不会静默返回空结果。LLM 提供商覆盖四种聊天 API 形态（Anthropic Messages、OpenAI Chat Completions、OpenAI Responses、Gemini Interactions），外加一个 `rerank` 提供商，每种都可以指向官方主机，或通过 `base_url` 指向任何兼容端点；按模型设置的 `server_tools` 可启用提供商原生工具，例如 Google Search 检索增强。渠道包括 Webhook（同步或异步）、GitHub Webhook（`ghhook`）、Telegram（轮询、Webhook 或自动配置的 `auto`）、QQ 机器人、`openai`（Chat Completions）以及 `bridge` — 一种 HMAC 签名的适配器协议，让进程外的服务接入任何其他聊天平台。它们都挂载在同一个共享 HTTP 监听器上，并自带 `/health` 探针。此外，运行时还提供统一的媒体与文件存储、由核心掌管的审计记录、用于语义记忆的嵌入和重排、身份与用户层（用户资料、渠道关联、OIDC、按用户配额）、租户（群组、成员关系、共享记忆池和授权）、带快照版本管理的用户范围文件存储、加密凭据存储、由引擎触发的定时触发器、配置目录、仅追加的日志平面、单实例和区域两种部署层级、通过 stdio 或 Streamable HTTP 接入的 MCP 服务器、声明式 HTTP 工具，以及供 AgentFlow Studio 连接的无头管理 API（`/admin/api/`）。

## 安装指南

### 前置条件

- **Go 1.26+** — AgentFlow 是 100% 纯 Go（`CGO_ENABLED=0`）。在任何操作系统上都不需要 C/C++ 编译器、git 子模块或 `zig`。
- **GNU make**（可选）— `go build`、`go test` 与 `go vet` 的便捷封装。

### Windows / Linux / macOS

1. 安装 [Go](https://go.dev/dl/) 1.26 或更新版本。
2. 克隆并构建：

```
git clone https://github.com/hulipa487/agentflow.git
cd agentflow
go build -o agentflow ./cmd/agentflow   # Windows 上为 agentflow.exe，或直接运行 make
go test ./...
go vet ./...
```

交叉编译直接使用标准 Go 环境变量即可（例如 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/agentflow` 或 `make TARGET_OS=linux`）。

### 或者从模板开始

`agentflow init` 会写出一个可直接运行的入门部署（`-layout dir` 生成配置目录），`agentflow validate` 和 `agentflow doctor` 则在不启动引擎的情况下检查配置：

```
./agentflow init                 # 写出 agentflow.yaml
./agentflow validate             # 加载配置并编译每个循环
./agentflow doctor               # 离线检查：循环、凭据、模型、渠道
./agentflow doctor -online      # 同时连接存储和后端（regional 下的默认行为）
./agentflow                      # 运行；先查找 agentflow.yaml，再查找 config.yaml
```

### 验证安装

把一个最简单的 echo 循环保存为 `echo.lua`（第一行是 prelude API 契约 — 缺少这一行的代码块会被拒绝）：

```
-- af-prelude-version: 1
function loop()
  while true do
    local msg = session.inbox()
    session.send("echo: " .. (msg.text or ""))
  end
end
```

再写一个不需要模型的配置，把它连接到 Webhook，保存为 `agentflow.yaml`：

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

运行它，然后用 curl 请求这个 Webhook：

```
./agentflow run -config agentflow.yaml
# 在另一个终端中：
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'
```

你应该会在响应中看到 `echo: hello`。注意末尾的斜杠 — Webhook 挂载的是 `/webhook/` 子树，向不带斜杠的 `/webhook` 发送 POST 会返回 `307` 重定向，而 `curl -d` 不会跟随它。所有内容都在这一个端口上，包括管理平面的探针：

```
curl http://127.0.0.1:8080/healthz   # 存活检查
curl http://127.0.0.1:8080/readyz    # 就绪检查（启动或排空期间返回 503）
curl http://127.0.0.1:8080/health    # 渠道监听器自己的探针 -> {"ok":true}
```

### 故障排查

| 症状 | 解决方法 |
| --- | --- |
| 切换分支后出现 `Nothing to be done for 'all'` | Makefile 会跟踪 Go 源码与嵌入的 Lua 文件，所以这种情况很少见。如果二进制文件看起来仍是旧的，运行 `make clean && make` 或 `go build ./cmd/agentflow`。 |
| 测试因 SQLite / database locked 失败 | 串行运行测试（`go test -p 1 ./...`），或在两次运行之间删除残留的 `data/` 目录。 |

### 更新

新版本发布后，拉取代码并干净地重新构建：

```
git pull
go build -o agentflow ./cmd/agentflow   # 或 make clean && make
```

## 快速开始

### 构建

AgentFlow 为纯 Go 构建（`CGO_ENABLED=0`）— 直接使用 `go` 或 `make` 即可：

```
go build -o agentflow ./cmd/agentflow   # 或 make
go test ./...                           # 或 make test
```

### 运行

最快的路径不需要模型 — 基于文件的 echo 循环开箱即可回复。让二进制文件指向你的配置：

```
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'   # 注意末尾的 /
```

如果要做由 LLM 驱动的机器人，就为它接入一个真实模型（见下一段），并把智能体的 `model` 设为该条目。

### 或者从场景开始

AgentFlow Studio 仓库在 [`examples/`](https://github.com/hulipa487/agentflow-studio/tree/main/examples) 下提供了完整的示例部署：每个都包含配置、循环和提示词、循环测试以及 README。复制最接近你需求的那个，或用运行时直接运行：

| 场景 | 展示内容 |
| --- | --- |
| [`qq-group-chat`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/qq-group-chat) | 带记忆和上下文的 QQ 群聊机器人：每个群一个会话，提示词能区分发言人，近期上下文保存在 `session.state` 中，支持长期召回和 `/remember` 事实 |
| [`telegram-rag`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/telegram-rag) | 基于知识库回答问题的 Telegram 机器人：嵌入存放在 SQLite 中，语义召回，带引用的回答，以及一个用于批量导入的 Webhook |
| [`github-pr-agents`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/github-pr-agents) | 为每个 PR 派生一个代码智能体的 GitHub 拉取请求 Webhook：变更被复制到沙箱项目中，用 shell 命令检查，并在评论中给出评审 |
| [`chat-completions`](https://github.com/hulipa487/agentflow-studio/tree/main/examples/chat-completions) | 由智能体支撑的单轮 OpenAI 兼容 `/v1/chat/completions` 端点 |

```
./agentflow -config examples/telegram-rag/agentflow.yaml
./agentflow loop test examples/telegram-rag/tests/answer.yaml
```

对于从零构建的智能体，Studio 的 **新建智能体** 对话框以入门模板的形式提供同样的几种形态（带记忆的群聊、知识库、单轮 API）。

### 接入真实模型

`provider` 决定线路格式。`base_url` 留空则使用该提供商的官方主机，也可以设为任何兼容端点（Ollama、vLLM、LiteLLM、OpenRouter、企业网关）：

```
models:
  default:
    provider: openai          # API 形态：anthropic | openai | openai-responses | gemini | rerank
    model: gpt-4o
    base_url: http://127.0.0.1:11434/v1   # Ollama / vLLM / LiteLLM / OpenRouter
    api_key: ${OPENAI_API_KEY}
    thinking: medium        # off | low | medium | high | xhigh | max；留空 = 提供商默认值
    server_tools: []          # 提供商原生工具（gemini：google_search；openai-responses：web_search）
```

`provider` 选择请求/响应格式，`base_url` 选择主机。对于无需密钥的本地端点（Ollama），`api_key` 可以留空 — 但 `gemini` 例外，它的客户端没有密钥就无法构建。`thinking` 设置模型默认的思考级别 — 这是一套与提供商无关的词汇（`off` | `low` | `medium` | `high` | `xhigh` | `max`），运行时会把它映射到各提供商的原生参数：Anthropic 的 `budget_tokens`、OpenAI 的 `reasoning_effort`，或 Gemini 的 `thinking_level`（外加 `thinking_summaries`，它负责要求返回思考摘要）。留空则不发送任何值，由提供商使用自己的默认值；每次调用可以通过 `llm.chat` 的 `opts.thinking` 覆盖。Gemini 的 SDK 带来两个限制：请求中没有低于 `minimal` 的档位，所以 `off` 映射为 `minimal`，而不会关闭思考；其生成配置中没有 temperature 字段，所以在那里 `llm.chat{temperature=...}` 会被按名称拒绝。`server_tools` 列出在提供商补全过程中运行的提供商原生服务器端工具 — 运行时只注入原生工具条目，不附带客户端 schema。
