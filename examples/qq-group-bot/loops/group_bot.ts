// af-prelude-version: 2
// Flagship QQ Group Bot starter.
// Ordinary group rules live in Studio Starter Settings (agent.config().extras):
//   respond_on_mention, respond_in_c2c, respond_in_guild, trigger_phrase,
//   cooldown_sec, store_all_messages, retention_turns.

interface QQGroupTurn {
  who: string;
  text: string;
  reply: string;
}

interface QQGroupSettings {
  respond_on_mention: boolean;
  respond_in_c2c: boolean;
  respond_in_guild: boolean;
  trigger_phrase: string;
  cooldown_sec: number;
  store_all_messages: boolean;
  retention_turns: number;
}

function readQQBoolSetting(extras: Record<string, unknown>, key: string, fallback: boolean): boolean {
  return typeof extras[key] === "boolean" ? (extras[key] as boolean) : fallback;
}

function readQQNumSetting(extras: Record<string, unknown>, key: string, fallback: number, min: number): number {
  const val = Number(extras[key]);
  return Number.isFinite(val) && val >= min ? val : fallback;
}

function readQQSettings(): QQGroupSettings {
  const extras = ((agent.config() || {}).extras || {}) as Record<string, unknown>;
  return {
    respond_on_mention: readQQBoolSetting(extras, "respond_on_mention", true),
    respond_in_c2c: readQQBoolSetting(extras, "respond_in_c2c", true),
    respond_in_guild: readQQBoolSetting(extras, "respond_in_guild", false),
    trigger_phrase: typeof extras.trigger_phrase === "string" ? extras.trigger_phrase.trim() : "",
    cooldown_sec: readQQNumSetting(extras, "cooldown_sec", 15, 0),
    store_all_messages: readQQBoolSetting(extras, "store_all_messages", false),
    retention_turns: Math.max(1, Math.floor(readQQNumSetting(extras, "retention_turns", 20, 1))),
  };
}

function loadQQTurns(): QQGroupTurn[] {
  try {
    const v = session.state.get("turns");
    if (Array.isArray(v)) return v as QQGroupTurn[];
  } catch (_) {}
  return [];
}

function saveQQTurns(turns: QQGroupTurn[], limit: number): void {
  while (turns.length > limit) turns.shift();
  try {
    session.state.set("turns", turns);
  } catch (_) {}
}

function loadQQLastReplyMs(): number {
  try {
    const v = Number(session.state.get("last_reply_ms"));
    return Number.isFinite(v) ? v : 0;
  } catch (_) {}
  return 0;
}

function saveQQLastReplyMs(ts: number): void {
  try {
    session.state.set("last_reply_ms", ts);
  } catch (_) {}
}

function rememberQQTurn(chat: string, who: string, text: string, reply: string): void {
  try {
    memory.write({ type: "turn", chat: chat, who: who, text: text, reply: reply });
  } catch (_) {}
}

function matchesQQTrigger(text: string, phrase: string): boolean {
  if (!phrase) return false;
  return text.toLowerCase().indexOf(phrase.toLowerCase()) >= 0;
}

function shouldAnswerQQ(
  msg: { text?: string; channel?: string; payload?: Record<string, unknown> },
  cfg: QQGroupSettings
): { ok: true; reason: string } | { ok: false; reason: string } {
  if (msg.channel === "studio") {
    return { ok: true, reason: "studio_test" };
  }
  const p = (msg.payload || {}) as Record<string, unknown>;
  const scene = String(p.scene || "group");

  if (scene === "c2c") {
    if (!cfg.respond_in_c2c) {
      return { ok: false, reason: "c2c_disabled" };
    }
    return { ok: true, reason: "c2c_chat" };
  }
  if (scene === "guild" || scene === "dm") {
    if (!cfg.respond_in_guild) {
      return { ok: false, reason: "guild_disabled" };
    }
    return { ok: true, reason: scene };
  }

  const text = msg.text || "";
  const mentioned = p.mentioned === true;
  const triggered = matchesQQTrigger(text, cfg.trigger_phrase);
  const matched = (cfg.respond_on_mention && mentioned) || triggered;

  if (!matched) {
    return { ok: false, reason: "not_mentioned" };
  }
  if (cfg.cooldown_sec > 0) {
    const elapsed = Date.now() - loadQQLastReplyMs();
    if (elapsed >= 0 && elapsed < cfg.cooldown_sec * 1000) {
      return { ok: false, reason: "cooldown" };
    }
  }
  return { ok: true, reason: triggered ? "trigger_phrase" : "mentioned" };
}

function buildQQMessages(
  msg: { text?: string; from?: string; payload?: Record<string, unknown> },
  cfg: QQGroupSettings,
  turns: QQGroupTurn[],
  instructions: string,
  historyBudget: number
): Array<{ role: string; content: string }> {
  const p = (msg.payload || {}) as Record<string, unknown>;
  const who = String(p.name || p.username || p.member_openid || p.user_openid || msg.from || "member");
  const scene = String(p.scene || "group");
  const text = msg.text || "";

  let sys = instructions || "You are a concise, helpful member of a QQ group chat.";
  if (scene === "group" || scene === "guild") {
    sys += "\nConversation scene: " + scene + ".";
  }

  // Task 2 customization hook: route support questions to a group-specific knowledge base here.
  // For example, match support keywords or p.chat_id and prepend memory.recall({ store: "knowledge", ... }) passages.

  const messages: Array<{ role: string; content: string }> = [{ role: "system", content: sys }];
  const start = Math.max(0, turns.length - cfg.retention_turns);
  for (let i = start; i < turns.length; i++) {
    const t = turns[i];
    messages.push({ role: "user", content: t.who + ": " + t.text });
    if (t.reply) {
      messages.push({ role: "assistant", content: t.reply });
    }
  }
  messages.push({ role: "user", content: who + ": " + text });
  return token_budget(messages, historyBudget || 6000);
}

function loop(): void {
  while (true) {
    const msg = session.inbox();
    const cfg = readQQSettings();
    const p = (msg.payload || {}) as Record<string, unknown>;
    const who = String(p.name || p.username || p.member_openid || p.user_openid || msg.from || "member");
    const chat = String(p.chat_id || msg.reply_to || "chat");
    const text = msg.text || "";
    const turns = loadQQTurns();

    if (!/\S/.test(text)) {
      session.skip("empty_message");
      continue;
    }

    const decision = shouldAnswerQQ(msg, cfg);
    if (!decision.ok) {
      if (cfg.store_all_messages) {
        turns.push({ who: who, text: text, reply: "" });
        saveQQTurns(turns, cfg.retention_turns);
        rememberQQTurn(chat, who, text, "");
      }
      session.skip(decision.reason);
      continue;
    }

    const info = agent.info();
    const messages = buildQQMessages(msg, cfg, turns, info.instructions, info.history_budget);
    const reply = llm.chat(messages);
    const answer = reply.text || "";

    session.send(answer);
    saveQQLastReplyMs(Date.now());
    turns.push({ who: who, text: text, reply: answer });
    saveQQTurns(turns, cfg.retention_turns);
    rememberQQTurn(chat, who, text, answer);
  }
}
