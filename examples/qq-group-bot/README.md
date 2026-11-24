# QQ Group Bot Starter

A complete, supported QQ group assistant you can configure in AgentFlow Studio with **zero TypeScript**, diagnose in **Event Center**, and customize in code when your group needs domain-specific logic.

| | |
| --- | --- |
| Channel | `qq` (`websocket` gateway by default — no public IP required; `webhook` supported) |
| Loop | [`loops/group_bot.ts`](loops/group_bot.ts) |
| Memory | Built-in `conversational` profile + bounded per-group context (`retention_turns`) |
| Secrets | Encrypted credential store (`cred:openai`, `cred:doubao`, `cred:qq_app_secret`) |
| Tools | `builtin:web_search` (`doubao` engine only) + `builtin:fetch` (`tools` capability) |

## Task 1 — Configure and run with zero TypeScript

1. Create a bot in the [QQ Open Platform console](https://bot.q.qq.com), copy its `AppID` and `AppSecret`, and add your server's outbound IP to the platform IP allowlist.
2. Export `CREDENTIALS_MASTER_KEY` and `QQ_APPID`, store `openai` (LLM API key), `doubao` (Doubao search API key), and `qq_app_secret` (QQ AppSecret) in the credential store, then start AgentFlow + Studio (or manage keys from Studio's **Credentials** screen and create a new agent in **Builder → New agent → QQ group bot**):

   ```bash
   export CREDENTIALS_MASTER_KEY=... QQ_APPID=102xxxxxx
   agentflow cred set openai -config examples/qq-group-bot/agentflow.yaml
   agentflow cred set doubao -config examples/qq-group-bot/agentflow.yaml
   agentflow cred set qq_app_secret -config examples/qq-group-bot/agentflow.yaml
   agentflow -config examples/qq-group-bot/agentflow.yaml
   ```

3. Open **Channels → New channel → QQ**, select `qq_group_bot`, keep `websocket` mode, pick scenes (`group`, `group_all`, `c2c`), enter your `AppID` (and `cred:qq_app_secret` as the secret, or verify the saved entry), click **Test credentials** to verify `/gateway/bot`, then click **Save** (websocket channels go live immediately without restarting). Enable full group message push (`接收群全量消息`) in mobile QQ so non-`@` messages reach the bot.
4. Adjust behavior anytime in **Builder → Config → Starter settings**:
   - **When to respond:** group `@`-mentions (`respond_on_mention`), quote-replies to the bot without `@` (`respond_on_reply`), plain-text `bot_names` / aliases, optional `trigger_phrase`, LLM contextual relevance (`reply_on_relevant`, using `[NO_REPLY]` when irrelevant), C2C private chats, Guild/DM scenes, and per-group `cooldown_sec`.
   - **Group context:** every QQ conversation gets its own isolated session (`qq_group_bot|qq:group:<group_openid>`) and explicit `retention_turns`; `store_all_messages` defaults to on so recent group messages stay in working context.
   - **Web tools:** the example grants `tools` with `skills: [builtin:web_search, builtin:fetch]` and configures `search.default: doubao` (`api_key: cred:doubao`).
5. Mention the bot, quote its message, or chat in the QQ group. Open **Event Center** in Studio to inspect `answered`, `ignored` (`not_mentioned`, `not_relevant`, `mentions_others`, `cooldown`), `dropped`, or `failed` outcomes and view session traces.

## Task 2 — Customize in TypeScript (example: group-specific KB routing)

Once your group needs custom behavior, edit [`loops/group_bot.ts`](loops/group_bot.ts) at the `Task 2 customization hook` comment inside `buildQQMessages(...)` (~15 lines of TypeScript) to route support questions to a group-specific knowledge store:

```ts
if (/support|help|docs|\?|帮助/i.test(text)) {
  const storeName = String(p.chat_id) === "group:VIP_OPENID" ? "vip_support_kb" : "knowledge";
  const hits = memory.recall({ store: storeName, text, k: 3 }) || [];
  const notes = hits
    .map((h: any) => h?.value?.text)
    .filter((s: unknown): s is string => typeof s === "string" && s.length > 0);
  if (notes.length > 0) {
    sys += "\nRelevant knowledge base passages:\n" + notes.map((n, i) => `[${i + 1}] ${n}`).join("\n");
  }
}
```

## Verify locally without QQ credentials

```bash
agentflow loop test examples/qq-group-bot/tests/group_bot.yaml
```
