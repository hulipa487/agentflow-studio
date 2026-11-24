# Telegram Group Bot Starter

A complete, supported Telegram group assistant you can configure in AgentFlow Studio with **zero TypeScript**, diagnose in **Event Center**, and customize in code when your group needs domain-specific logic.

| | |
| --- | --- |
| Channel | `telegram` (long polling by default; webhook & auto supported) |
| Loop | [`loops/group_bot.ts`](loops/group_bot.ts) |
| Memory | Built-in `conversational` profile + bounded per-group context (`retention_turns`) |
| Secrets | Encrypted credential store (`cred:openai`, `cred:telegram`, `cred:gemini`, `cred:xai`, `cred:cloudflare`) |
| Tools | `builtin:web_search` (`google_search` + `x_search`), `builtin:browser` (Cloudflare Browser Run), `builtin:fetch` (`tools` capability) |

## Task 1 — Configure and run with zero TypeScript

1. Create a bot via [@BotFather](https://t.me/BotFather) and copy its bot token.
   - If you want the bot to observe ordinary group messages (`store_all_messages` or keyword `trigger_phrase`), run `/setprivacy` → **Disable** in `@BotFather`.
2. Export `CREDENTIALS_MASTER_KEY` and `CLOUDFLARE_ACCOUNT_ID`, store the secret keys (`openai`, `telegram`, `gemini` for `google_search`, `xai` for `x_search`, and `cloudflare` for `builtin:browser`) in the credential store, then start AgentFlow + Studio (or manage keys from Studio's **Credentials** screen and create a new agent in **Builder → New agent → Telegram group bot**):

   ```bash
   export CREDENTIALS_MASTER_KEY=... CLOUDFLARE_ACCOUNT_ID=...
   for name in openai telegram gemini xai cloudflare; do
     agentflow cred set "$name" -config examples/telegram-group-bot/agentflow.yaml
   done
   agentflow -config examples/telegram-group-bot/agentflow.yaml
   ```

3. Open **Channels → New channel → Telegram**, paste your token (or enter `cred:telegram` / verify the saved channel entry), click **Test token** to verify the bot `@username` and check its **Privacy Mode** badge, then click **Save** (polling channels go live immediately without restarting).
4. Adjust behavior anytime in **Builder → Config → Starter settings**:
   - **When to respond:** @-mentions, replies to the bot, direct messages, an optional trigger phrase, and per-group cooldown seconds.
   - **Group context & privacy:** every Telegram chat gets its own isolated session (`group_bot|telegram:<chat_id>`) and explicit `retention_turns`; toggle **Remember ignored group messages** on or off.
   - **Web tools:** the example grants `tools` with `skills: [builtin:web_search, builtin:browser, builtin:fetch]` and configures `google_search` (`cred:gemini`), `x_search` (`cred:xai`), and Cloudflare Browser Run (`cred:cloudflare`).
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
