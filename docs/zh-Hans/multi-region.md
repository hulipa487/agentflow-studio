# 多区域部署

AgentFlow 没有全局层。多区域部署就是每个区域一个独立的 `regional` 单元，由前面的应用决定用户属于哪个单元。各单元在运行时互不共享：没有跨区域数据库，没有只读副本，没有跨区域的会话调度，也没有 Qdrant 同步。

这是有意为之。每个区域是独立的故障域，热路径从不承担跨区域往返；数据驻留是单元本身的属性，而不是叠加在上面的一层策略；也不存在可能只应用了一半的“全局”写入。代价是，凡是真正全局的东西——把用户路由到所属区域、共享的聊天机器人、计费——都归入口服务负责。

## 单元

每个区域一个单元：

```
runtime:
  topology: regional
  region: eu-west
  zone: eu-west-1a
  persistence: postgres://agentflow:${PG_PASSWORD}@db.eu-west.internal:5432/agentflow?sslmode=require
  admin:
    operators: { alice: ${ALICE_ADMIN_TOKEN} }
  shutdown_grace: 5s
media: { backend: s3, s3: { bucket: agentflow-eu-media, region: eu-west-1 } }
files: { backend: s3, s3: { bucket: agentflow-eu-files, region: eu-west-1 } }
memory:
  backends:
    hot: { provider: redis, config: { url: redis://redis.eu-west.internal:6379 } }
    vec: { provider: qdrant, config: { url: http://qdrant.eu-west.internal:6333 } }
```

单元内的每个实例都运行这份配置，`runtime.region` 相同。实例之间只有监听地址和可用区标签不同。校验器会拒绝以下情况：本地 blob 目录、SQLite 记忆后端、日志平面目录、运行时存储以外的集群存储，以及既没有共享管理令牌也没有具名操作员的单元。

单元的 Redis 是一个覆盖整个区域的集群：会话会在单元内的各可用区之间迁移，所以 Redis 的 locality 是 `region`。持久的对话存储不能放在 `volatile`（`instance`）上；除非该存储设置了 `locality_ok: true`，否则校验会拒绝。

## 将用户固定到区域

用户 → 区域的映射由入口服务掌管。它应当：

- 在用户的归属单元中创建用户资料（向该单元发送 `POST /v1/users/register`），并把映射保存在自己的数据库里；
- 把之后的每个请求——`/v1/users/me`、渠道流量、Studio 管理调用——都发往同一个单元；
- 把注册、邀请、凭据设置以及密码 / 身份变更路由到归属单元，绝不发往负载均衡器背后随机的某个单元。

`GET /v1/users/config` 会报告 `"region"`，`/admin/api/state` 会报告 `region`、`zone` 以及每个运行中的实例，因此入口服务（以及操作员）能分辨是哪个单元作出的响应。如果负载均衡器把同一用户的请求分散到多个单元，该用户的资料、记忆和文件就会被拆散；要在边缘固定路由，不要靠运气。

## 共享的聊天入口

一个 Telegram 机器人的 webhook 只有一个接收方——或者只有一个长轮询消费者。两个单元配置同一个机器人令牌会争抢更新，哪个抢到，就由哪个处理本属于另一个单元的用户。大多数聊天平台都是如此。

做法是使用桥接入口：

1. 由一个服务持有机器人令牌，接收所有更新。
2. 它把发送者解析到某个区域（与上文相同的映射）。
3. 它把消息转发到该单元的 `bridge` 渠道。
4. 单元通过桥接回复，再由入口服务发出回复。

`bridge` 渠道使用 HMAC 签名，因此单元只接受来自入口服务的流量。按请求无状态的渠道（webhook、ghhook）可以直接负载均衡到用户所属的单元；只有单一外部接收方的渠道（Telegram、大多数聊天 SDK）则要经过入口服务。

## 管理与计费

管理 API 按单元划分。控制台和 Studio 每次只连接一个单元；Studio 的连接为每个单元保留一个条目，所以切换区域就是使用导航栏底部的账号切换器，而不必重新输入令牌。

计费采用扇出方式：对每个单元调用 `/admin/api/usage`，再把结果相加。用量有意不在单元之间复制——跨区域汇总就得对两本账本做对账，而账本正是配额决策的依据。

## 在区域之间迁移用户

`GET /admin/api/users/{id}/bundle?public_key=<base64 X25519 public key>` 返回一个 zip，其中包含用户资料、关联账号、用户范围内的记忆条目、文件树和 blob 字节、群组成员关系，以及用该公钥加密封存的凭据。目标单元上的 `POST /admin/api/users/import` 接收该 zip 和对应的私钥，以相同的用户 uuid 重建用户资料，用该单元的主密钥重新加密凭据，并报告无法迁移的内容（目标单元没有的群组、目标单元未提供的存储绑定）。

CLI 封装了两端的操作：

```
agentflow user keygen eu-move
agentflow user export u_abc -url https://eu-west.internal -pubkey eu-move.pub -out u_abc.zip
agentflow user import u_abc.zip -url https://us-east.internal -key eu-move.key
```

请在用户空闲时迁移：在数据包导入之前，用户的会话仍属于旧单元，此前发往新单元的流量会创建第二个空白对话。入口服务应当先停止向旧单元路由，等待排空（旧单元的 `shutdown_grace` 在这里帮不上忙——这是一次路由变更），然后导入，再把流量发往新单元。

## 灾难恢复

每个单元都有自己的 PostgreSQL 主库。区域故障的恢复方式是建立一个替代单元，而不是提升某个全局主库：

1. 为单元的 PostgreSQL 保留一个跨区域备库（流复制或定期快照——引擎不负责管理）。
2. 让 S3 存储桶持续复制到替代区域。
3. 区域丢失时，提升备库，并启动一个单元，将 `runtime.region` 设为替代区域的标签，指向已提升的数据库和已复制的存储桶。
4. 把入口服务的区域映射重新指向新单元。会话从共享存储中恢复；`shutdown_grace` 排空不适用于已崩溃的单元，因此旧单元中的对等实例（如果还有存活的）会在会话租约 TTL 到期时交接。
5. 在新单元中重新创建身份提供商和机器人的凭据。它们属于环境 / 配置而非数据，也是最常被遗漏的一步。

在单元之间迁移一个用户，就是规模更小的同一套数据包机制。整个区域的迁移则是一次数据库提升，再为每个必须更换归属单元的用户导出导入数据包。

## 不要这样做

- 不要让两个单元指向同一个 PostgreSQL 来“共享”状态：租约表会跨区域仲裁会话，这正是本设计刻意不设的全局层，而且每次续约都要承担跨区域延迟。
- 不要在两个单元前面放置跨区域的 Redis 或 Qdrant。每个区域的 Redis 归该区域独有；共享的 Redis 会重新引入跨区域依赖。
- 不要在两个单元中使用同一个 Telegram 机器人令牌。
- 不要复制用量账本再对副本求和；应改为扇出读取。
