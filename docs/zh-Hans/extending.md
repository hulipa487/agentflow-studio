# 不重新编译 Go 也能扩展 AgentFlow

AgentFlow 有四个扩展面。前三个靠配置、TypeScript/JS 或 Wasm 外部 API 就能搞定；只有最后一个要改二进制。

## 1. 声明式 HTTP 工具

把外部 API 变成模型可见的工具，不用写脚本，也不用碰 Go：

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

这个工具注册为 `http:crm_lookup`。以下这些由运行时负责：

- 模型看到的 JSON Schema；
- URL、query 和 JSON 请求体里的 `{arg}` 模板替换；
- 按租户解析凭据（`auth.service`，凭据值永远不会到模型或日志里）；
- 共享的地址防护（`net.http.allow_private`）和可选的主机白名单；
- 响应大小上限，以及可选的点路径取值（`customer.id`、`items[0].name`）。

非 GET 工具默认 `needs_confirm: true`；要改就显式设置。当某个值正好是 `{arg}` 时，请求体模板会保留它的 JSON 类型。

也可以直接导入 OpenAPI 3 文档，不必手写每个操作：

```bash
agentflow tools import-openapi openapi.yaml -out tools.yaml \
  -base-url https://api.example.com -auth crm -prefix crm
```

导入器会生成一个 `tools:` 块供你审阅，运行时并不会去拉取这份规范。加 `-read-only` 则只导入 GET/HEAD。

## 2. MCP 服务器

任意外部工具的边界仍然交给 MCP。stdio 子进程或 Streamable HTTP 端点在 `mcp.servers` 下声明，其工具显示为 `<server>/<tool>`。HTTP 服务器接受 `token:`，可以是字面值、`${VAR}` 或 `cred:<service>`，并与 `http.request`、`builtin:fetch` 受同一套地址防护约束。

## 3. 渠道桥接

AgentFlow 没有编译进去的聊天平台，可以用外部适配器接进来。适配器把平台事件翻译成下面的协议并接收回复，AgentFlow 一行 Go 都不用改：

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

HMAC 是必需的。适配器被信任可以转达平台侧的发送者身份，但桥接拿到的账号只记为「声称」，要拿到资料范围仍需走正常的渠道内 `/link` 校验。

## 4. 脚本包与外部 API

包是一个目录，里面是带版本门禁的 TypeScript/JS 源码。声明它，然后在循环里导入：

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

包里可以带 `manifest.yaml`：

```yaml
name: greeting
version: "1"
prelude_api: 2
files: [greeting.ts]
```

没有 manifest 时，`*.ts` 和 `*.js` 按字典序加载。包是源码组合，不是隔离模块：它们共用会话的 Goja VM 状态和能力集（这些仍由智能体控制）。`package:<name>` 也可以直接作为智能体的 `loop:` 引用。改动一个包会重启可能导入它的会话。需要隔离的可复用集成，就在 `apis` 下配置 TypeScript/JS 或 `wazero` Wasm 模块，循环里用 `api.*` 调用。

## 5. 哪些还得靠 Go

编译进去的驱动还是得编译：新的 LLM 线路协议、新的记忆/向量后端、新的搜索引擎或 Shell 提供方，以及新的一类 Go 原生安全过滤器。基于模型的审核用 `safety.classifiers`（`classifier:<name>`），脚本过滤器用 `safety.script_filters`（`script:<name>`）。其余的绕法有：

- 新型号厂商走 OpenAI 兼容端点（`provider: openai` + `base_url`）；
- 新的搜索/API 提供方，用 MCP、外部 API（Goja 里的 `apis.*` 或 `wazero` Wasm），或声明式 HTTP 工具；
- 新聊天平台走 bridge；
- 共享策略写成 TypeScript/JS 包；
- 不想重新编译又要做审核，就用分类器提示词加标签策略，或挂一个 `safety.script_filters` 钩子。

## 6. 租户管控

四个部署级开关就能补上产品层的缺口，完全不动推理路径：

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

租户生命周期端点有 `GET /admin/api/usage`、`GET /admin/api/users/{id}/export` 和 `DELETE /admin/api/users/{id}?confirm=<id>`。删除会清掉资料、账号、令牌、凭据、用量、日志和文件元数据；记忆记录按各自存储的保留期处理，内容寻址的 blob 由文件 GC 回收。管理 API 仍然只有一个权限层级：操作员名字用于归属操作，但读和写并不分级。

## 7. 部署前先测

```bash
agentflow validate -config agentflow.yaml
agentflow loop check loops/support.ts
agentflow loop test scenarios/refund.yaml
```

场景本身就是数据：

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

`loop test` 用假的 LLM/工具/渠道依赖跑真实的 actor，所以不需要凭据也不需要网络。完整的场景结构见 `internal/looptest`。

## 钩子与扩展点

AgentFlow 没有钩子注册表。引擎自己分发三个点，其余全靠 **脚本按约定命名全局函数**：插件提供一个全局，调用方按名字去找，这就是全部约定。两种机制决定了插件代码该放哪儿，所以这里都列出来。

### 由核心分发

无论循环怎么写，这些每轮都会跑。替换循环处理函数也卸不掉它们 —— 前两个存在的意义正是如此。

| 位置 | 类型 | 实现方 | 说明 |
| --- | --- | --- | --- |
| `safety.ingress` | 过滤器 | Go，核心掌管 | 在循环看到消息之前运行；可以丢弃消息、跳过本轮，或改写文本。基线过滤器里只有 `affect-guard` 会丢弃，而 `classifier:<name>` 与 `script:<name>` 过滤器可以拦截或重定向。 |
| `safety.egress` | 过滤器 | Go，核心掌管 | 在回复送达渠道之前运行；可以丢弃回复（可附带由核心撰写的兜底文案）。 |
| `gateway.route` | 处理器 | 由 `gateway.route` 指定的脚本文档块，默认 `plugin:per_chat` | 每个进程一个 Goja VM 状态。读取消息并返回会话地址。路由处理器只能调用 `inbox`、`deliver`、`runtime.triggers`、`router.state.*` 和 `log`，其他一律按名称拒绝。 |

### 脚本按约定调用的名字

这些是普通的全局函数，不是引擎调用的入口。部署配置启用了对应行为时，prelude 或循环会按名字调用它们；要覆写内置实现，就在 `plugins/<name>.ts` 里定义同名全局。

| 配置写法 | 脚本需定义的全局 | 调用方 |
| --- | --- | --- |
| 记忆方案 `write: plugin:routing_table` | `memory_routing_table(record)` | prelude 的 `memory.write` |
| 记忆方案 `recall: plugin:recency` | `memory_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 记忆方案 `recall: plugin:semantic` | `memory_semantic_recall_handler(query, opts)` | prelude 的 `memory.recall` |
| 支持片段 `plugin:token_budget` | `token_budget(messages, budget)` | 循环，需要裁剪提示词时 |
| 循环插件 `plugin:per_chat` | `loop()` | 无人调用 —— 它本身就是循环，不是辅助函数 |

> **本节以前写的是什么**：它列了 26 个钩子 —— `gateway.ingress`、`loop.before_turn`、`session.migrate`、`shell.before_exec`、`agent.start`、`core.on_reload` 等 —— 还写了类型、签名和默认值，并声称由核心分发。并不存在这样的分发器，定义这些名字的插件从未被调用过。其中两个还点到了内置实现（`shell.before_exec` 对应 `plugin:exec_policy`，`shell.teardown` 对应 `plugin:ttl`），它们被加载进每个会话状态却没有任何调用方；这两个现在都已删除。**本地 Shell 命令只受 `shell.exec` 能力约束，远程 SSH 会话只受 `net.ssh` 能力约束 —— 不存在命令策略过滤器。** 循环崩溃会重启，文件变更会热重载，这才是 `session.resume` 和 `core.on_reload` 当初想要的；需要在两者之间保留状态的循环，请用 `session.state.*`。
