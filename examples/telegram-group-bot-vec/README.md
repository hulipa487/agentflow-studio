# Telegram 群聊机器人（embedding + SQLite vector 记忆版）

AgentFlow 运行时上的 TypeScript Telegram 群机器人。与 FTS5 版共用同一份 loop/prompt，
记忆改为 bge-m3 embedding + SQLite 向量检索，不使用 FTS5。

## 包含文件

```
agentflow.yaml                # 单 bot 可部署配置（密钥走环境变量）
.env.example                  # 需要填写的环境变量
loops/telegram-group-bot.ts   # 群记忆 / 主动发言 / 工具循环
prompts/telegram-group-bot.md # 人格与工具使用说明
```

## 部署

```bash
cd examples/telegram-group-bot-vec
cp .env.example .env
# 编辑 .env，填入 Telegram Bot Token、模型密钥、BGE_KEY、Gemini/xAI 搜索密钥及 Cloudflare 凭据
set -a; . ./.env; set +a
/path/to/agentflow validate -config agentflow.yaml
/path/to/agentflow run -config agentflow.yaml
```

`bge-m3` 使用 OpenAI 兼容接口：

```yaml
bge-m3:
  provider: openai
  model: BAAI/bge-m3
  api_key: ${BGE_KEY}
  base_url: https://api.siliconflow.cn/v1
```

实测输出 1024 维向量。若使用其他 embedding 服务，改 `base_url` / `model` 即可。

为了接收未 @ 机器人的普通群消息（用于上下文记忆与主动接话），需要在
`@BotFather` 中关闭群组隐私模式（`/setprivacy` → `Disable`），或在群内将机器人设为管理员；
否则 Telegram 只会推送 `@机器人` 和回复机器人的消息。

## 记忆

- `session.state` 按群保存最近 20 轮，重启不丢。
- `dialogue` store：`requires: [kv, vector]`、`scope: agent`；
  同群不同成员共享一份记忆。
- `recall: plugin:semantic`：写入时用 bge-m3 生成 1024 维向量，保存在 SQLite
  `embedding` 列；召回时 embed 当前消息，做余弦相似度扫描。
- `recall_k: 64`：先取 64 条向量近邻，再按 `chat` 过滤。比 FTS 版大，是因为
  `scope: agent` 的向量表按 agent 共享，可能混有多个群/私聊，先多取再过滤。
- 每条入站消息多一次 embedding 调用，每次召回多一次；SQLite vector 是暴力
  余弦扫描（默认最多扫 1000 行），适合单机中小群，不适合超大规模历史库。

## 主动参与

与 FTS 版完全一致：未 @、未引用、未叫名字的群消息先过规则，再让模型判断是否
值得插话；不适合时只输出 `[NO_REPLY]`。详细开关见 `agentflow.yaml` 的 `extras`。

## 工具

- `builtin:web_search`：支持 `google_search`（默认，需要 `GEMINI_API_KEY`）与 `x_search`（需要 `XAI_API_KEY`）。
- `builtin:fetch`：读取群友贴出的静态网页；默认拒绝私网/回环/云元数据地址。
- `builtin:browser`：使用 Cloudflare Browser Rendering 抓取需要 JavaScript 渲染的动态页面，需要 `CLOUDFLARE_ACCOUNT_ID` 与 `CLOUDFLARE_API_TOKEN`。

## 与 FTS5 版并行

- 两个 agent 的记忆表互相独立，不会自动迁移历史。
- 一个 Telegram Bot Token 只能对应一个长轮询实例；要同时跑两个 bot，需要两个
  Telegram 机器人账号，各自使用独立 `.env`、`gateway.listen` 和 `runtime.persistence`。
- 不要用相同的 Bot Token 同时起两个 `polling` 渠道，否则会导致更新冲突。
