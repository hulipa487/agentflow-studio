# 多區域部署

AgentFlow 沒有全域層。多區域部署就是每個區域各一個獨立的 `regional` 單元（cell），由前方的應用程式決定使用者屬於哪個單元。各單元在執行期間不共用任何東西：沒有跨區域資料庫、沒有唯讀複本、沒有跨區域的工作階段配置，也沒有 Qdrant 同步。

這是刻意的設計。每個區域都是獨立的故障域，熱路徑永遠不必承擔跨區域往返，資料駐留是單元本身的屬性，而不是疊加在上面的原則，也不存在可能只套用一半的「全域」寫入。代價是凡是真正全域的事情，例如把使用者導向其所屬區域、共用的聊天機器人、計費，都歸前端入口負責。

## 單元

每個區域一個單元：

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

單元中的每個執行個體都以相同的 `runtime.region` 執行這份組態。執行個體之間只有監聽位址與可用區標籤不同。驗證器會拒絕本機 Blob 目錄、SQLite 記憶後端、日誌平面目錄、非執行環境儲存區的叢集儲存區，以及沒有共用管理權杖或具名操作人員的單元。

單元的 Redis 是一個涵蓋整個區域的叢集：工作階段會在單元內的各可用區之間移動，所以 Redis 的 locality 是 `region`。持久的對話儲存區不得放在 `volatile`（`instance`）上；除非該儲存區設定了 `locality_ok: true`，否則驗證會拒絕。

## 將使用者固定在某個區域

使用者 → 區域的對應由前端入口負責。它應該：

- 在使用者的所屬單元建立使用者資料（對該單元呼叫 `POST /v1/users/register`），並將對應關係保存在自己的資料庫中；
- 將之後的每個請求，包括 `/v1/users/me`、頻道流量、Studio 管理呼叫，都送往同一個單元；
- 將註冊、邀請、憑證設定以及密碼／身分變更導向所屬單元，絕不送往負載平衡器後方隨機的某個單元。

`GET /v1/users/config` 會回報 `"region"`，`/admin/api/state` 會回報 `region`、`zone` 以及每個執行中的執行個體，讓前端入口（和操作人員）知道是哪個單元回應的。若負載平衡器把同一位使用者的請求分散到多個單元，就會把他的使用者資料、記憶和檔案拆散；請在邊緣固定區域，而不是靠運氣。

## 共用的聊天前端入口

一個 Telegram 機器人的 Webhook 只能有一個接收端，或者只能有一個長輪詢消費者。兩個單元若設定了同一個機器人權杖，就會互相搶奪更新，搶到的那一方還會處理屬於另一個單元的使用者。大多數聊天平台也是如此。

做法是使用橋接前端入口：

1. 由一個服務持有機器人權杖並接收所有更新。
2. 它將傳送者解析到某個區域（與上述相同的對應）。
3. 它把訊息轉送到該單元的 `bridge` 頻道。
4. 單元透過橋接回覆，再由前端入口傳送回覆。

`bridge` 頻道使用 HMAC 簽章，因此單元只接受來自前端入口的流量。每個請求都無狀態的頻道（webhook、ghhook）可以直接負載平衡到使用者的單元；只有單一外部接收端的頻道（Telegram、多數聊天 SDK）則要經過前端入口。

## 管理與計費

管理 API 以單元為單位。主控台與 Studio 一次只連線到一個單元；Studio 的連線設定檔為每個單元各保留一筆，所以切換區域是使用導覽列底部的帳戶切換器，而不必重新輸入權杖。

計費採扇出方式：對每個單元呼叫 `/admin/api/usage` 並加總結果。用量刻意不在單元之間複寫，因為跨區域彙總就得協調兩本帳本，而帳本正是配額決策的依據。

## 在區域之間移轉使用者

`GET /admin/api/users/{id}/bundle?public_key=<base64 X25519 public key>` 會傳回一個 zip，內含使用者資料、連結帳號、使用者範圍內的記憶資料列、檔案樹與 Blob 位元組、群組成員資格，以及以該公鑰密封的憑證。在目的單元上，`POST /admin/api/users/import` 接收這個 zip 與對應的私鑰，以相同的使用者 uuid 重建使用者資料，用該單元的主金鑰重新加密憑證，並回報無法帶過去的項目（目的單元沒有的群組、它未公開的儲存區繫結）。

CLI 包裝了這兩端：

```
agentflow user keygen eu-move
agentflow user export u_abc -url https://eu-west.internal -pubkey eu-move.pub -out u_abc.zip
agentflow user import u_abc.zip -url https://us-east.internal -key eu-move.key
```

請在使用者閒置時移轉：在 bundle 匯入之前，他的工作階段仍屬於舊單元，在此之前送往新單元的流量會建立第二個空白的對話。前端入口應停止導向舊單元、排空（舊單元的 `shutdown_grace` 在這裡幫不上忙，因為這是路由變更）、匯入，然後再把流量送往新單元。

## 災難復原

每個單元都有自己的 PostgreSQL 主要資料庫。區域中斷時，要靠建立一個替代單元來復原，而不是升級某個全域主要資料庫：

1. 為單元的 PostgreSQL 保留一個跨區域的備援（串流複寫或排程快照，引擎不負責管理）。
2. 讓 S3 儲存貯體持續複寫到替代區域。
3. 失去該區域時，將備援升級為主要資料庫，並啟動一個單元，把 `runtime.region` 設為替代區域的標籤，指向升級後的資料庫與複寫的儲存貯體。
4. 將前端入口的區域對應改指向新單元。工作階段會從共用儲存區繼續；`shutdown_grace` 排空不適用於當機的單元，所以舊單元中的對等執行個體（若有倖存）會在工作階段租約 TTL 到期時交接。
5. 在新單元中重新建立身分識別供應商與機器人的憑證。它們屬於環境／組態，而不是資料，也是最常被遺漏的一步。

在單元之間移轉使用者，是同一套 bundle 機制的小規模版本。整個區域的移轉則是資料庫升級，再加上為所屬單元必須變更的使用者各做一個 bundle。

## 不應該做的事

- 不要讓兩個單元指向同一個 PostgreSQL 來「共用」狀態：租約表會跨區域仲裁工作階段，這正是此設計刻意不具備的全域層，而且每次租約續約都要承擔跨區域延遲。
- 不要在兩個單元前方放置跨區域的 Redis 或 Qdrant。每個區域的 Redis 都是自己的；共用一個會重新引入跨區域相依。
- 不要在兩個單元中使用同一個 Telegram 機器人權杖。
- 不要複寫用量帳本再加總各複本；改為扇出讀取。
