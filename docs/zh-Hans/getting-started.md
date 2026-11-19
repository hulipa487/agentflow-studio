# AgentFlow

一个用 Go 和 Luau 构建、单个二进制文件、可自托管的 LLM 智能体运行时。常规的部分靠配置，特殊的部分靠编程。

## 它是什么

AgentFlow 运行由 LLM 驱动的智能体：它们能记忆、使用工具、执行 shell 命令、调度后台任务、与其他智能体对话，并通过 Webhook 和 Telegram 为用户提供服务。每个智能体都在 YAML 中声明式定义；默认行为之外的一切都来自可热重载的 Lua 插件，这些插件挂接在核心暴露的版本化接口上。

## 设计原则

- **核心提供能力；插件提供行为；YAML 负责连接。**
- **每个会话一个 Luau 状态** — 参与者（actor）模型，只通过消息传递通信。
- **失败时如实报告** — 不可用的工具返回结构化错误，绝不编造结果。
- **内置功能也是插件** — 随二进制文件发布，可按名称覆盖，与用户插件遵循同一套契约。
- **能力边界是一套消息协议** — 而不是某种语言的 API。

## 技术栈

| 决策 | 选择 | 理由 |
| --- | --- | --- |
| 运行时 | Go | 天生适合编排；每个参与者一个 goroutine；驱动生态完善 |
| 脚本 | Luau（cgo） | 面向不受信任的脚本；原生协程；强沙箱隔离 |
| cgo 工具链 | zig cc | 封闭的 C 编译器；可交叉编译；Windows 开发省心 |
| 并发 | 参与者 + 工作池 | Lua 协程在异步操作上挂起；不占用操作系统线程 |

> **当前状态**：核心运行时（参与者、记忆、工具、shell、多智能体、调度器、预算、安全、指标）已实现并经过测试。存储后端：SQLite、Redis 和 PostgreSQL 用于键值和文档行，一个内存中的易失性存储，以及通过 pgvector、Qdrant、Redis Stack/RediSearch、带 vector 扩展的 PostgreSQL 或 SQLite 有上限的暴力扫描实现的向量搜索 — 绝不会静默返回空结果。LLM 提供商覆盖四种聊天 API 形态（Anthropic Messages、OpenAI Chat Completions、OpenAI Responses、Gemini Interactions），外加一个 `rerank` 提供商，每种都可以指向官方主机，或通过 `base_url` 指向任何兼容端点；按模型设置的 `server_tools` 可启用提供商原生工具，例如 Google Search 检索增强。渠道包括 Webhook（同步或异步）、GitHub Webhook（`ghhook`）、Telegram（轮询、Webhook 或自动配置的 `auto`）以及 `bridge` — 一种 HMAC 签名的适配器协议，让进程外的服务接入任何其他聊天平台。它们都挂载在同一个共享 HTTP 监听器上，并自带 `/health` 探针。此外，运行时还提供多模态媒体接收、由核心掌管的审计记录、用于语义记忆的嵌入和重排、身份与用户层（用户资料、渠道关联、OIDC、按用户配额）、租户（群组、成员关系、共享记忆池和授权）、带快照版本管理的用户范围文件存储、加密凭据存储、由引擎触发的定时触发器、配置目录、仅追加的日志平面、单实例和区域两种部署层级、通过 stdio 或 Streamable HTTP 接入的 MCP 服务器、声明式 HTTP 工具、终端仪表盘、浏览器控制台，以及 AgentFlow Studio 所使用的管理 API。

## 安装指南

### 前置条件

- **Go 1.26+** — 运行时使用 Go modules 构建。
- **GNU make** — 驱动 Luau 静态库和 Go 二进制文件的构建。
- **C/C++ 工具链**，用于 Luau 的 cgo 桥接。Makefile 会按宿主操作系统自动选择：
  - **Windows**：[zig](https://ziglang.org/)（封闭式；无需系统 gcc）。
  - **Linux**：系统 gcc（`cc`/`c++`/`ar`），通常来自 `build-essential`。
  - **macOS**：Xcode / Apple Command Line Tools（`clang`/`clang++`/`ar`）。

Luau 本身是 `third_party/luau/` 下的一个 git 子模块，固定在上游 0.731 版本（MIT）— 克隆时加上 `--recurse-submodules`，或先运行 `git submodule update --init`。

### Windows

1. 安装 [Go](https://go.dev/dl/)，并确认 `go version` 显示 1.26 或更新版本。
2. 安装 [Git for Windows](https://git-scm.com/download/win)。
3. 安装 [zig](https://ziglang.org/download/)，并将其加入 PATH。
4. 安装 GNU make，例如 `choco install make` 或 `winget install GnuWin32.Make`，也可以使用 Git Bash/MSYS2 自带的 make。
5. 克隆并构建。Makefile 在 PowerShell、cmd.exe 和 Git Bash 中都能使用：它会检测执行配方的 shell 并使用对应的命令。

```
git clone --recurse-submodules https://github.com/<you>/agentflow.git
cd agentflow
make          # 编译固定版本的 Luau 子模块 -> internal/vm/lib/libluau.a，然后生成 agentflow.exe
make test     # 在启用 cgo 的情况下运行 go test ./...
make vet
```

在 Windows 上，二进制文件是 `agentflow.exe`。`make TARGET_OS=linux` 会用 zig 交叉编译出 Linux 二进制文件。

### Linux

1. 安装 Go 和 C++ 工具链：

```
# Debian/Ubuntu
sudo apt update && sudo apt install -y golang-go build-essential
# Fedora
sudo dnf install -y golang gcc gcc-c++ make
```

2. 克隆并构建：

```
git clone --recurse-submodules https://github.com/<you>/agentflow.git
cd agentflow
make && make test
```

### macOS

1. 从 [go.dev](https://go.dev/dl/) 安装 Go，或使用 Homebrew：`brew install go`。
2. 安装 Xcode Command Line Tools：`xcode-select --install`。
3. 克隆并构建：

```
git clone --recurse-submodules https://github.com/<you>/agentflow.git
cd agentflow
make && make test
```

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
| `internal/vm` 报 `undefined: State` 或 cgo 错误 | 通过 `make` 运行命令；它会设置 `CGO_ENABLED=1` 以及正确的 `CC`/`CXX`。在 Luau 库生成之前，不要直接运行 `go build`。 |
| 切换分支后出现 `Nothing to be done for 'all'` | Makefile 会跟踪 Go 源码、嵌入的 Lua 和控制台文件，所以这种情况很少见。如果二进制文件看起来仍是旧的，运行 `make clean && make`。 |
| Windows：`'CGO_ENABLED' is not recognized` 或 `rm -rf` 报错 | Makefile 版本过旧。拉取最新版本：它会导出自己的变量，并自动选择 cmd.exe 或 sh 配方。 |
| Windows 上出现 `zig: command not found` | 将 zig 加入 PATH 或安装它。用 `zig version` 验证。 |
| 测试因 SQLite / database locked 失败 | 串行运行测试，或在两次运行之间删除残留的 `data/` 目录。 |

### 更新

新版本发布后，拉取代码并干净地重新构建：

```
git pull
make clean && make
```

## 快速开始

### 构建

Makefile 先通过 cgo 编译固定版本 Luau 子模块的 VM 桥接（使用 zig 作为封闭的 C 编译器），然后编译二进制文件：

```
make        # luau 静态库 -> agentflow 二进制文件
make test   # 使用 cgo/zig 工具链运行 go test
```

### 运行

最快的路径不需要模型 — 基于文件的 echo 循环开箱即可回复。让二进制文件指向你的配置：

```
./agentflow run -config agentflow.yaml
curl -X POST localhost:8080/webhook/ -d '{"from":"alice","text":"hello"}'   # 注意末尾的 /
```

如果要做由 LLM 驱动的机器人，就为它接入一个真实模型（见下一段），并把智能体的 `model` 设为该条目。

### 或者从场景开始

运行时仓库在 [`examples/`](https://github.com/hulipa487/agentflow/tree/main/examples) 下提供了完整的部署：每个都包含配置、循环和提示词、循环测试以及 README。复制最接近你需求的那个，或者直接原样运行：

| 场景 | 展示内容 |
| --- | --- |
| [`qq-group-chat`](https://github.com/hulipa487/agentflow/tree/main/examples/qq-group-chat) | 带记忆和上下文的 QQ 群聊机器人：每个群一个会话，提示词能区分发言人，近期上下文保存在 `session.state` 中，支持长期召回和 `/remember` 事实 |
| [`telegram-rag`](https://github.com/hulipa487/agentflow/tree/main/examples/telegram-rag) | 基于知识库回答问题的 Telegram 机器人：嵌入存放在 SQLite 中，语义召回，带引用的回答，以及一个用于批量导入的 Webhook |
| [`github-pr-agents`](https://github.com/hulipa487/agentflow/tree/main/examples/github-pr-agents) | 为每个 PR 派生一个代码智能体的 GitHub 拉取请求 Webhook：变更被复制到 gVisor 沙箱项目中，用 shell 命令检查，并在评论中给出评审 |
| [`chat-completions`](https://github.com/hulipa487/agentflow/tree/main/examples/chat-completions) | 由智能体支撑的单轮 OpenAI 兼容 `/v1/chat/completions` 端点 |

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
