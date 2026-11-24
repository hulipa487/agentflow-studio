# 多區域部署

AgentFlow 沒有全域層。多區域部署就是每個區域一套獨立的 `regional` 單元，由前面的應用決定使用者落到哪個單元。各單元執行時互不共用：沒有跨區域資料庫，沒有唯讀複本，沒有跨區域工作階段配置，也沒有 Qdrant 同步。

這是刻意設計的。每個區域就是自己的故障域，熱路徑永遠不付出跨區域往返的代價；資料駐留是單元本身的性質，而不是壓在上面的一層原則；也不存在只寫了一半的「全域」寫入。代價是：真正全域的東西——把使用者路由到所屬區域、共用聊天機器人、計費——都得由入口服務負責。

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

單元裡的每個執行個體都跑這份組態，`runtime.region` 相同，只有監聽位址和可用區標籤不同。驗證器會拒絕：本機 blob 目錄、SQLite 記憶後端、日誌平面目錄、執行環境儲存區以外的叢集儲存區，以及既沒有共用管理權杖也沒有具名操作員的單元。

單元的 Redis 是一個涵蓋整個區域的叢集：工作階段會在單元內的可用區之間遷移，所以 Redis 的 locality 取 `region`。持久化的對話儲存區不能放在 `volatile`（`instance`）上；除非儲存區設了 `locality_ok: true`，否則驗證會拒絕。

## 把使用者固定到區域

使用者到區域的對應由入口服務掌管。它應當：

- 在使用者的所屬單元建立資料（向該單元發 `POST /v1/users/register`），並把對應存在自己的資料庫裡；
- 之後的每個請求——`/v1/users/me`、頻道流量、Studio 管理呼叫——都發往同一個單元；
- 把註冊、邀請、憑證設定以及密碼／身分變更都路由到所屬單元，絕不發給負載平衡器後面的隨機某個單元。

`GET /v1/users/config` 會回傳 `"region"`，`/admin/api/state` 會回傳 `region`、`zone` 和每個作用中執行個體，入口服務（以及操作員）因此能分辨回應來自哪個單元。如果負載平衡器把同一使用者的請求分散到多個單元，它的資料、記憶和檔案就會被拆開；在邊緣固定路由，別靠運氣。

## 共用聊天入口

一個 Telegram 機器人的 webhook 只有一個接收方——或只有一個長輪詢消費者。兩個單元配同一個機器人權杖就會爭搶更新，誰搶到，誰就處理了本屬於另一個單元的使用者。多數聊天平台都是這樣。

做法是架一個橋接入口：

1. 由一個服務持有機器人權杖，接收所有更新。
2. 它把傳送者解析到某個區域（對應與上文相同）。
3. 它把訊息轉發到該單元的 `bridge` 頻道。
4. 單元透過橋接回覆，入口服務再把回覆送出去。

`bridge` 頻道帶 HMAC 簽章，單元只接受來自入口服務的流量。依請求無狀態的頻道（webhook、ghhook）可以直接負載平衡到使用者所在單元；只有一個外部接收方的頻道（Telegram、多數聊天 SDK）則要走入口服務。

## 管理與計費

管理 API 按單元劃分。主控台和 Studio 每次只連一個單元；Studio 的連線設定為每個單元留一筆記錄，所以切區域就是用導覽列底部的帳號切換器，不必重新輸入權杖。

計費靠扇出：對每個單元呼叫 `/admin/api/usage`，再把結果加起來。用量有意不在單元之間複製——跨區域彙總就得對兩本帳冊，而帳冊正是配額決策的依據。

## 在區域之間遷移使用者

`GET /admin/api/users/{id}/bundle?public_key=<base64 X25519 public key>` 回傳一個 zip，內含使用者資料、連結帳號、該使用者範圍內的記憶紀錄、檔案樹與 blob 位元組、群組成員資格，以及用該公開金鑰加密封裝的憑證。目標單元上的 `POST /admin/api/users/import` 接收這個 zip 和對應私鑰，用相同的使用者 uuid 重建資料，用該單元的主金鑰重新加密憑證，並回報哪些內容沒能帶過去（目標單元沒有的群組、未提供的儲存區綁定）。

命令列封裝了兩端：

```
agentflow user keygen eu-move
agentflow user export u_abc -url https://eu-west.internal -pubkey eu-move.pub -out u_abc.zip
agentflow user import u_abc.zip -url https://us-east.internal -key eu-move.key
```

請在使用者閒置時遷移：資料包匯入前，它的工作階段仍屬於舊單元，此前發往新單元的流量會另開一個空白對話。入口服務應當先停止向舊單元路由，等排空（舊單元的 `shutdown_grace` 在這裡幫不上忙——這是路由變更），再匯入，然後把流量發往新單元。

## 災難復原

每個單元都有自己的 PostgreSQL 主庫。區域故障的復原方式是另起一個替代單元，而不是提升某個全域主庫：

1. 為單元的 PostgreSQL 保留一份跨區域備庫（串流複寫或定期快照——引擎不負責管理）。
2. 讓 S3 儲存桶持續複寫到替代區域。
3. 區域失守時，提升備庫，啟動一個單元，把 `runtime.region` 設為替代區域的標籤，指向已提升的資料庫和已複寫的儲存桶。
4. 把入口服務的區域對應重新指向新單元。工作階段會從共用儲存區恢復；`shutdown_grace` 排空對已崩潰的單元不適用，因此舊單元裡的對等執行個體（如果還有存活的）會在工作階段租約 TTL 到期時接管。
5. 在新單元重建身分提供者與機器人憑證。它們屬於環境／組態而非資料，也是最常被漏掉的一步。

單元之間遷移一個使用者，就是同一套資料包機制的小規模版本。整個區域遷移則是一次資料庫提升，再為每個必須更換所屬單元的使用者匯出、匯入資料包。

## 不要這樣做

- 不要讓兩個單元指向同一個 PostgreSQL 來「共用」狀態：租約表會跨區域仲裁工作階段，而這正是本設計刻意不設的全域層，每次續約還要吃跨區域延遲。
- 不要在兩個單元前面放跨區域的 Redis 或 Qdrant。每個區域的 Redis 歸自己；共用一個就把跨區域依賴又引回來了。
- 不要在兩個單元裡用同一個 Telegram 機器人權杖。
- 不要複製用量帳冊再對副本求和；改成扇出讀取。
