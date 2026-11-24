# QQ 群聊机器人（FTS5 记忆版）

AgentFlow 运行时上的 TypeScript QQ 群机器人。与向量版共用同一份 loop/prompt，
记忆使用 SQLite FTS5 文本召回。

## 包含文件

```
agentflow.yaml          # 单 bot 可部署配置（密钥走环境变量）
.env.example            # 需要填写的环境变量
loops/qq-group-bot.ts   # 群记忆 / 主动发言 / 工具循环
prompts/qq-group-bot.md # 人格与工具使用说明
```

## 部署

```bash
unzip qq-group-bot-fts.zip
cd qq-group-bot-fts
cp .env.example .env
# 编辑 .env，填入 QQ AppID/Secret、模型密钥、doubao 搜索密钥
set -a; . ./.env; set +a
/path/to/agentflow validate -config agentflow.yaml
/path/to/agentflow run -config agentflow.yaml
```

QQ 开放平台的机器人需要加入群，并具备接收群消息的权限；配置里的 `group_all`
场景必须开启，否则平台不会推送未 @ 机器人的普通群消息，主动参与无法工作。

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

- `builtin:web_search`：默认 `doubao` 引擎，需要 `DOUBAO_KEY`。
- `builtin:fetch`：读取群友贴出的网页；默认拒绝私网/回环/云元数据地址。

## 与现有部署并行

这是一个独立单 bot 配置。如果机器上已有 AgentFlow 实例在跑：

- 改 `gateway.listen` 避免端口冲突；
- 改 `runtime.persistence` 指向独立数据库；
- 一个 QQ 机器人 App 只能对应一个网关连接，不要用相同的 AppID/Secret 再起一个
  渠道，否则同一条群消息会被两个 bot 各回一次。
