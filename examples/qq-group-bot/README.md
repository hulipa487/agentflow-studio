# QQ Group Bot Starter

A complete, supported QQ group assistant you can configure in AgentFlow Studio with **zero TypeScript**, diagnose in **Event Center**, and customize in code when your group needs domain-specific logic.

| | |
| --- | --- |
| Channel | `qq` (`websocket` gateway by default — no public IP required; `webhook` supported) |
| Loop | [`loops/group_bot.ts`](loops/group_bot.ts) |
| Memory | Built-in `conversational` profile + bounded per-group context (`retention_turns`) |
| Safety posture | External write tools (`tools`, `apis`, `net.http`) off by default |

## Task 1 — Configure and run with zero TypeScript

1. Create a bot in the [QQ Open Platform console](https://bot.q.qq.com), copy its `AppID` and `AppSecret`, and add your server's outbound IP to the platform IP allowlist.
2. Start AgentFlow + Studio (`agentflow -config examples/qq-group-bot/agentflow.yaml`) or create a new agent in **Builder → New agent → QQ group bot**.
3. Open **Channels → New channel → QQ**, select `qq_group_bot`, keep `websocket` mode, pick scenes (`group`, `c2c`, or `group_all` if your bot has full group access), enter your `AppID` and `AppSecret`, click **Test credentials** to verify `/gateway/bot`, then click **Save** (websocket channels go live immediately without restarting).
4. Adjust behavior anytime in **Builder → Config → Starter settings**:
   - **When to respond:** group @-mentions, C2C private chats, Guild/DM scenes, optional `trigger_phrase`, and per-group `cooldown_sec`.
   - **Group context:** every QQ conversation gets its own isolated session (`qq_group_bot|qq:group:<group_openid>`) and explicit `retention_turns`; toggle **Remember ignored group messages** when using `group_all`.
5. Mention your bot in a QQ group (or sandbox group). Open **Event Center** in Studio to inspect `answered`, `ignored` (`not_mentioned`, `cooldown`), `dropped`, or `failed` outcomes and view session traces.

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
