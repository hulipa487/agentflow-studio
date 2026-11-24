# Telegram 群聊机器人（FTS5 记忆版）

AgentFlow 运行时上的 TypeScript Telegram 群机器人。与向量版共用同一份 loop/prompt，
记忆使用 SQLite FTS5 文本召回，并与 QQ 群聊机器人保持一致的人格、记忆与主动参与策略。

## 包含文件

```
agentflow.yaml                # 单 bot 可部署配置（密钥走环境变量）
.env.example                  # 需要填写的环境变量
loops/telegram-group-bot.ts   # 群记忆 / 主动发言 / 工具循环
prompts/telegram-group-bot.md # 人格与工具使用说明
```

## 部署

```bash
cd examples/telegram-group-bot-fts
cp .env.example .env
# 编辑 .env，填入 Telegram Bot Token、模型密钥、Gemini/xAI 搜索密钥及 Cloudflare 浏览器凭据
set -a; . ./.env; set +a
/path/to/agentflow validate -config agentflow.yaml
/path/to/agentflow run -config agentflow.yaml
```

为了接收未 @ 机器人的普通群消息（用于上下文记忆与主动接话），需要在
`@BotFather` 中关闭群组隐私模式（`/setprivacy` → `Disable`），或在群内将机器人设为管理员；
否则 Telegram 只会推送 `@机器人` 和回复机器人的消息。

## 记忆

- `session.state` 按群保存最近 20 轮，重启不丢。
- `dialogue` store 使用 `requires: [kv, text_search]`、`scope: agent`；
  同群不同成员共享一份记忆。
- `recall: plugin:recency`，按当前消息做 FTS5 文本召回，取回更早的相关旧事。
- 中文长期召回受 FTS5 分词限制；近期上下文由 `session.state` 保证。

## 主动参与

未 @、未引用、未叫名字的群消息会先过规则（太短、@ 了别人、刚说完、评估冷却、
主动发言冷却、每小时上限），再让模型判断是否值得插话；不适合时只输出
`[NO_REPLY]`。详细开关见 `agentflow.yaml` 的 `extras`。

## 工具

- `builtin:web_search`：支持 `google_search`（默认，需要 `GEMINI_API_KEY`）与 `x_search`（需要 `XAI_API_KEY`）。
- `builtin:fetch`：读取群友贴出的静态网页；默认拒绝私网/回环/云元数据地址。
- `builtin:browser`：使用 Cloudflare Browser Rendering 抓取需要 JavaScript 渲染的动态页面，需要 `CLOUDFLARE_ACCOUNT_ID` 与 `CLOUDFLARE_API_TOKEN`。

## 与现有部署并行

这是一个独立单 bot 配置。如果机器上已有 AgentFlow 实例在跑：

- 改 `gateway.listen` 避免端口冲突；
- 改 `runtime.persistence` 指向独立数据库；
- 一个 Telegram Bot Token 只能对应一个长轮询实例，不要用同一个 Token 再起一个
  `polling` 渠道，否则会导致更新冲突。
