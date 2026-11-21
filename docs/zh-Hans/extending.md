# 无需重新编译 Go 即可扩展 AgentFlow

AgentFlow 有四个扩展层面。前三个只涉及配置或 Lua；只有最后一个会改动二进制文件。

## 1. 声明式 HTTP 工具

无需 Lua 和 Go，就能把一个外部 API 变成模型可见的工具：

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

该工具注册为 `http:crm_lookup`。运行时负责：

- 模型看到的 JSON Schema；
- URL、查询参数和 JSON 请求体中的 `{arg}` 模板替换；
- 按租户解析凭据（`auth.service`；凭据值永远不会到达模型或消息日志）；
- 共享的地址防护（`net.http.allow_private`）和可选的主机白名单；
- 响应大小上限和可选的点路径选取（`customer.id`、`items[0].name`）。

非 GET 工具默认 `needs_confirm: true`；如需改变，请显式设置。当某个值恰好是 `{arg}` 时，请求体模板会保留其 JSON 类型。

也可以导入 OpenAPI 3 文档，而不必手写每个操作：

```bash
agentflow tools import-openapi openapi.yaml -out tools.yaml \
  -base-url https://api.example.com -auth crm -prefix crm
```

导入器会生成一个 `tools:` 块供你审阅；它不会在运行时获取规范文档。加上 `-read-only` 则只导入 GET/HEAD。

## 2. MCP 服务器

MCP 仍然是接入任意外部工具的边界。stdio 子进程或 Streamable HTTP 端点在 `mcp.servers` 下声明；其工具显示为 `<server>/<tool>`。HTTP 服务器接受一个 `token:`，可以是字面值、`${VAR}` 或 `cred:<service>`，并且与 `http.request` 和 `builtin:fetch` 受同一地址防护约束。

## 3. 桥接渠道

AgentFlow 未内置的聊天平台可以通过外部适配器接入。适配器把平台事件转换为下面的协议并接收回复；AgentFlow 无需改动任何 Go 代码：

```yaml
gateway:
  channels:
    - name: slack
      type: bridge
      agent: support
      path: /bridge/slack/
      deliver_url: http://127.0.0.1:9100/deliver
      secret: "${SLACK_BRIDGE_SECRET}"
      token: "${SLACK_BRIDGE_TOKEN}"   # 可选，用于 deliver_url 的 Bearer 令牌
```

入站（适配器 → 运行时）：

```http
POST /bridge/slack/
X-Agentflow-Signature: sha256=<hex HMAC-SHA256 of the raw body>

{"from":"U123","chat_id":"C456","text":"hello","attachments":[]}
```

出站（运行时 → 适配器）：

```json
{"to":"C456","text":"...","attachments":[{"type":"image","mime":"image/png","handle":"media:..."}]}
```

HMAC 是必需的。适配器被信任会如实转发平台的发送者身份，但桥接来的关联账号只记为“声明的”；关联账号仍须在渠道内完成常规的 `/link` 验证，才能获得用户资料范围。

## 4. Lua 包

包是一个目录，里面是受版本约束的 Lua 源码。先声明，再在循环中导入：

```yaml
packages:
  greeting: ./packages/greeting
```

```lua
-- af-prelude-version: 1
-- af-import: greeting

function loop()
  while true do
    local msg = session.inbox()
    session.send(Greeting.hello(msg.text))
  end
end
```

包中可以包含 `manifest.yaml`：

```yaml
name: greeting
version: "1"
prelude_api: 1
files: [greeting.lua]
```

没有清单文件时，`*.lua` 文件按字典序加载。包是源码层面的组合，而不是相互隔离的模块：它们共享会话的 Lua 状态和能力集（能力集仍由智能体控制）。`package:<name>` 也可以直接用作智能体的 `loop:` 引用。编辑包会重启可能导入它的会话。

## 5. 仍然需要 Go 的部分

编译型驱动仍需编译：新的 LLM 通信协议、新的记忆/向量后端、新的搜索引擎或 shell 提供商，以及新*种类*的安全过滤器。基于模型的内容审核和分类已不再需要 Go——声明一个 `safety.classifiers` 条目，并在方案中以 `classifier:<name>` 引用即可。用 Lua 编写的过滤器仍需要原生钩子。可行的替代办法有：

- 用 OpenAI 兼容端点接入新的模型厂商（`provider: openai` + `base_url`）；
- 用 MCP 或声明式 HTTP 工具接入新的搜索/API 提供商；
- 用桥接接入新的聊天平台；
- 用 Lua 包承载共享策略；
- 用分类器提示词加标签策略实现内容审核，无需重新编译。

## 6. 租户控制

四个部署级开关在不改变推理路径的前提下补齐产品层面的缺口：

```yaml
runtime:
  users:
    unregistered: reject        # 丢弃未关联的账号，而不是归入服务层级
    push_same_tenant: true      # session.push_user 只能发给当前租户
  credentials:
    engine_wide: maintenance    # credential.get() 仅在引擎触发的轮次中可用
  admin:
    operators:                  # 具名管理令牌；X-Operator 标明操作归属
      alice: "${ALICE_ADMIN_TOKEN}"
```

租户生命周期端点为 `GET /admin/api/usage`、`GET /admin/api/users/{id}/export` 和 `DELETE /admin/api/users/{id}?confirm=<id>`。删除操作会移除用户资料、关联账号、令牌、凭据、用量、消息日志和文件元数据；记忆行按其存储区的保留期处理，按内容寻址的数据块由文件 GC 回收。管理 API 仍然只有一个权限级别：操作员名称用于标明操作归属，但并不区分读/写权限。

## 7. 部署前先测试

```bash
agentflow validate -config agentflow.yaml
agentflow loop check loops/support.lua
agentflow loop test scenarios/refund.yaml
```

场景就是数据：

```yaml
agent: support
loop: ../loops/support.lua
llm:
  - match: refund
    text: "I can help with that."
messages:
  - from: alice
    text: "I need a refund"
expect_contains: ["help"]
```

`loop test` 用伪造的 LLM/工具/渠道依赖运行真实的 Actor，因此不需要凭据，也不需要网络。完整的场景结构见 `internal/looptest`。

## 钩子与扩展点

不存在钩子注册表。引擎自己调度三个点，其余一切都是 **Lua 调用的命名约定**：插件提供一个全局变量，名字与调用方要找的一致，这就是全部契约。两者的区别决定了插件代码必须放在哪里，因此下面分别列出。

### 由核心调度

无论循环做什么，这些点在每个轮次都会运行。替换循环处理器无法卸载它们——前两个的意义正在于此。

| 扩展点 | 类型 | 实现方 | 说明 |
| --- | --- | --- | --- |
| `safety.ingress` | 过滤器 | Go，由核心掌管 | 在循环看到消息之前运行；可以丢弃消息、跳过本轮次或改写文本。五个基线过滤器中只有 `affect-guard` 会丢弃消息；`classifier:<name>` 过滤器可以根据模型输出拦截。 |
| `safety.egress` | 过滤器 | Go，由核心掌管 | 在回复到达渠道之前运行；可以丢弃回复（可附带一条由核心撰写的替代回复）。 |
| `gateway.route` | 处理器 | 由 `gateway.route` 指定的 Lua 代码块，默认 `plugin:per_chat` | 每个进程一个 Luau 状态。读取消息并返回一个会话地址。路由处理器只能调用 `inbox`、`deliver`、`runtime.triggers`、`router.state.*` 和 `log`；其他调用都会按名称被拒绝。 |

### Lua 调用的命名约定

这些是普通的全局变量，而不是引擎调用的入口点。当部署配置了相应行为时，prelude 或循环会按名称调用它们；要覆盖某个内置实现，就在 `plugins/<name>.lua` 中定义同名全局变量。

| 配置方式 | Lua 必须定义的全局变量 | 调用方 |
| --- | --- | --- |
| 记忆方案 `write: plugin:routing_table` | `memory_routing_table(record)` | prelude 的 `memory.write` |
| 记忆方案 `recall: plugin:recency` | `memory_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 记忆方案 `recall: plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 辅助代码块 `plugin:token_budget` | `token_budget(messages, budget)` | 循环，在需要裁剪提示词时 |
| 循环插件 `plugin:per_chat` | `loop()` | 无——它本身*就是*一个循环，而不是辅助函数 |

> **本节过去的说法**：它曾列出 26 个钩子——`gateway.ingress`、`loop.before_turn`、`session.migrate`、`shell.before_exec`、`agent.start`、`core.on_reload` 等——附带类型、签名和默认值，并声称由核心调度它们。实际上并不存在调度器，所以定义了这些名称之一的插件会悄无声息地永远不被调用。其中两个指向内置实现（`shell.before_exec` 对应的 `plugin:exec_policy`，`shell.teardown` 对应的 `plugin:ttl`），它们被加载进每个会话状态，却没有任何代码调用；两者现已移除。**shell 命令仅由 `shell.exec` 能力把关——不存在命令策略过滤器。** 循环在崩溃时会重启，在文件变更时会热重载，这正是 `session.resume` 和 `core.on_reload` 想要实现的；需要在这两种情况之间保留状态的循环，请使用 `session.state.*`。
