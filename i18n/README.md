# Studio message catalogues

Every piece of text either Studio app shows comes from here. There is one
directory per language and one JSON file per namespace:

    en/        English: the source, and the fallback for a missing key
    zh-Hans/   Simplified Chinese (简体中文)
    zh-Hant/   Traditional Chinese (繁體中文)

A key is `<namespace>.<name>` and lives in `<namespace>.json`, so
`channels.saveFailed` is in `en/channels.json`, `zh-Hans/channels.json` and
`zh-Hant/channels.json`. Each file is one flat JSON object of key to message.
The web app bundles these files (`web/src/i18n`), and the desktop app shows
the web app; a test fails when the three languages do not hold the same keys
and the same parameters.

## Reading a message

    import { t } from '@/i18n'
    t('agents.count', { n: agents.length })

Markup in the web app goes through `tr` (`@/i18n/rich`): the message marks a
span with a tag and the caller says what the tag renders as.

    "common.typeToConfirm": "Type <code>{text}</code> to confirm"
    tr('common.typeToConfirm', { text }, { code: (c) => <code>{c}</code> })

## Message syntax

    {name}                                      a parameter
    {n, plural, =0 {…} one {…} other {…}}       a count; # prints it
    {kind, select, a {…} b {…} other {…}}       a choice

English needs `one` and `other`; Chinese has no plural forms, so a Chinese
message uses `other` alone, or drops the plural altogether and writes `{n}`.
A brace that does not open one of these forms is literal, so JSON and YAML
samples can be written as they are.

## What is translated

Everything Studio itself says: titles, labels, hints, buttons, banners, toasts,
dialogs, empty states, tooltips, and the errors Studio composes. Not
translated: config keys and values, identifiers, capability and tool names,
model and profile names, file paths, YAML, Lua, key names (Ctrl, Enter, F1),
product names (AgentFlow, Studio, Telegram, GitHub, Lua, YAML, Webhook), and
whatever the runtime returns, error text included. Those are passed in as
parameters.

Never build a sentence by gluing translated pieces together; word order
differs. Write one message with parameters instead.

## Style

Both Chinese catalogues

- put a space between Chinese and Latin letters or digits: `已保存 3 个模型`;
- use full-width punctuation (，。：；？！（）) in Chinese sentences, and the
  single character `…` for an ellipsis;
- keep `·` separators, arrows, and anything in backticks or code as it is;
- are short and plain, the way a Chinese-speaking operator would say it,
  never a word-for-word rendering of the English.

Simplified uses “ ” for quotation; Traditional uses 「 」. Traditional is
written as Traditional (Taiwan / Hong Kong vocabulary), not converted from
the Simplified text character by character.

## Glossary

Use these terms on every screen.

| English | 简体 | 繁體 |
| --- | --- | --- |
| agent | 智能体 | 代理 |
| runtime | 运行时 | 執行環境 |
| loop (the Lua loop of an agent) | 循环 | 迴圈 |
| script | 脚本 | 指令碼 |
| model | 模型 | 模型 |
| channel | 渠道 | 頻道 |
| memory | 记忆 | 記憶 |
| memory backend | 记忆后端 | 記憶後端 |
| memory pool | 记忆池 | 記憶池 |
| profile (safety, memory) | 方案 | 設定檔 |
| user profile | 用户资料 | 使用者資料 |
| connection (a saved runtime address) | 连接 | 連線 |
| credential | 凭据 | 憑證 |
| secret | 密钥 | 密鑰 |
| API key | API 密钥 | API 金鑰 |
| admin token | 管理令牌 | 管理權杖 |
| token (the sign-in secret) | 令牌 | 權杖 |
| tokens (model usage) | Token | Token |
| safety | 安全 | 安全 |
| classifier | 分类器 | 分類器 |
| user | 用户 | 使用者 |
| tenancy / tenant | 租户 | 租戶 |
| group | 群组 | 群組 |
| member | 成员 | 成員 |
| grant | 授权 | 授權 |
| quota | 配额 | 配額 |
| invite | 邀请 | 邀請 |
| handle (a linked account) | 关联账号 | 連結帳號 |
| usage | 用量 | 用量 |
| log | 日志 | 日誌 |
| config (the runtime's) | 配置 | 組態 |
| settings (Studio's) | 设置 | 設定 |
| documentation | 文档 | 文件 |
| file | 文件 | 檔案 |
| folder | 文件夹 | 資料夾 |
| playground | 演练场 | 遊樂場 |
| builder | 构建器 | 建構器 |
| overview | 概览 | 總覽 |
| session | 会话 | 工作階段 |
| thread (a conversation) | 对话 | 對話 |
| message | 消息 | 訊息 |
| trace | 追踪 | 追蹤 |
| event | 事件 | 事件 |
| tool | 工具 | 工具 |
| capability | 能力 | 能力 |
| template | 模板 | 範本 |
| instance | 实例 | 執行個體 |
| region | 区域 | 區域 |
| endpoint | 端点 | 端點 |
| provider | 提供商 | 供應商 |
| operator (the person using Studio) | 操作员 | 操作人員 |
| apply | 应用 | 套用 |
| save | 保存 | 儲存 |
| persist (write to the config file) | 持久化 | 永久儲存 |
| revert | 还原 | 還原 |
| discard | 放弃 | 捨棄 |
| delete | 删除 | 刪除 |
| remove | 移除 | 移除 |
| create | 创建 | 建立 |
| add | 添加 | 新增 |
| edit | 编辑 | 編輯 |
| copy | 复制 | 複製 |
| refresh / reload | 刷新 / 重新加载 | 重新整理 / 重新載入 |
| restart | 重启 | 重新啟動 |
| validate | 校验 | 驗證 |
| test | 测试 | 測試 |
| send | 发送 | 傳送 |
| search | 搜索 | 搜尋 |
| filter | 筛选 | 篩選 |
| default | 默认 | 預設 |
| enabled / disabled | 已启用 / 已停用 | 已啟用 / 已停用 |
| connected / offline | 已连接 / 离线 | 已連線 / 離線 |
| loading | 正在加载 | 載入中 |
| error | 错误 | 錯誤 |
| warning | 警告 | 警告 |
| required | 必填 | 必填 |
| optional | 可选 | 選填 |
| unsaved | 未保存 | 未儲存 |
| drift (runtime differs from the file) | 偏差 | 偏差 |
| live (running now) | 运行中 | 執行中 |
| keyboard shortcut | 快捷键 | 快速鍵 |
| light / dark | 浅色 / 深色 | 淺色 / 深色 |
| theme | 主题 | 主題 |
| language | 语言 | 語言 |
| window | 窗口 | 視窗 |
| network | 网络 | 網路 |
| server | 服务器 | 伺服器 |
| database | 数据库 | 資料庫 |
| data | 数据 | 資料 |
| information | 信息 | 資訊 |
| support | 支持 | 支援 |
| program / app | 程序 / 应用 | 程式 / 應用程式 |
| video / image / audio | 视频 / 图片 / 音频 | 影片 / 圖片 / 音訊 |
| link (noun) | 链接 | 連結 |
| line / column | 行 / 列 | 行 / 欄 |
