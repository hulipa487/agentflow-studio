# QQ 群聊机器人（embedding + SQLite vector 记忆版）

AgentFlow 运行时上的 TypeScript QQ 群机器人。与 FTS5 版共用同一份 loop/prompt，
记忆改为 bge-m3 embedding + SQLite 向量检索，不使用 FTS5。

## 包含文件

```
agentflow.yaml          # 单 bot 可部署配置（密钥走环境变量）
.env.example            # 需要填写的环境变量
loops/qq-group-bot.ts   # 群记忆 / 主动发言 / 工具循环
prompts/qq-group-bot.md # 人格与工具使用说明
```

## 部署

```bash
unzip qq-group-bot-vec.zip
cd qq-group-bot-vec
cp .env.example .env
# 编辑 .env，填入 QQ AppID/Secret、模型密钥、BGE_KEY、doubao 搜索密钥
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

QQ 开放平台的机器人需要加入群，并具备接收群消息的权限；配置里的 `group_all`
场景必须开启，否则平台不会推送未 @ 机器人的普通群消息，主动参与无法工作。

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

- `builtin:web_search`：默认 `doubao` 引擎，需要 `DOUBAO_KEY`。
- `builtin:fetch`：读取群友贴出的网页；默认拒绝私网/回环/云元数据地址。

## 与 FTS5 版并行

- 两个 agent 的记忆表互相独立，不会自动迁移历史。
- 一个 QQ 机器人 App 只能对应一个网关连接。要同时跑两个 bot，需要两个 QQ
  机器人应用；各自使用独立 `.env`、`gateway.listen` 和 `runtime.persistence`。
- 不要用相同的 AppID/Secret 同时起两个渠道，否则同一条群消息会被两个 bot
  各回一次。
