# Telegram Group Bot Starter

A complete, supported Telegram group assistant you can configure in AgentFlow Studio with **zero TypeScript**, diagnose in **Event Center**, and customize in code when your group needs domain-specific logic.

| | |
| --- | --- |
| Channel | `telegram` (long polling by default; webhook & auto supported) |
| Loop | [`loops/group_bot.ts`](loops/group_bot.ts) |
| Memory | Built-in `conversational` profile + bounded per-group context (`retention_turns`) |
| Safety posture | External write tools (`tools`, `apis`, `net.http`) off by default |

## Task 1 — Configure and run with zero TypeScript

1. Create a bot via [@BotFather](https://t.me/BotFather) and copy its bot token.
   - If you want the bot to observe ordinary group messages (`store_all_messages` or keyword `trigger_phrase`), run `/setprivacy` → **Disable** in `@BotFather`.
2. Start AgentFlow + Studio (`agentflow -config examples/telegram-group-bot/agentflow.yaml`) or create a new agent in **Builder → New agent → Telegram group bot**.
3. Open **Channels → New channel → Telegram**, paste your token, click **Test token** to verify the bot `@username` and check its **Privacy Mode** badge, then click **Save** (polling channels go live immediately without restarting).
4. Adjust behavior anytime in **Builder → Config → Starter settings**:
   - **When to respond:** @-mentions, replies to the bot, direct messages, an optional trigger phrase, and per-group cooldown seconds.
   - **Group context & privacy:** every Telegram chat gets its own isolated session (`group_bot|telegram:<chat_id>`) and explicit `retention_turns`; toggle **Remember ignored group messages** on or off.
5. Mention your bot in the Telegram group. Open **Event Center** in Studio to inspect every `answered`, `ignored` (`not_mentioned`, `cooldown`), `dropped`, or `failed` message and jump straight into its session trace.

## Task 2 — Customize in TypeScript (example: group-specific KB routing)

Once your group needs custom behavior, edit [`loops/group_bot.ts`](loops/group_bot.ts) at the `Task 2 customization hook` comment inside `buildMessages(...)` (~15 lines of TypeScript) to route support questions to a group-specific knowledge store:

```ts
if (/support|help|docs|\?/i.test(text)) {
  const storeName = String(p.chat_id) === "-1001234567890" ? "vip_support_kb" : "knowledge";
  const hits = memory.recall({ store: storeName, text, k: 3 }) || [];
  const notes = hits
    .map((h: any) => h?.value?.text)
    .filter((s: unknown): s is string => typeof s === "string" && s.length > 0);
  if (notes.length > 0) {
    sys += "\nRelevant knowledge base passages:\n" + notes.map((n, i) => `[${i + 1}] ${n}`).join("\n");
  }
}
```

## Verify locally without Telegram credentials

```bash
agentflow loop test examples/telegram-group-bot/tests/group_bot.yaml
```
