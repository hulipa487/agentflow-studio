// af-prelude-version: 2
// Flagship QQ Group Bot starter.
// Ordinary group rules live in Studio Starter Settings (agent.config().extras):
//   respond_on_mention, respond_on_reply, respond_in_c2c, respond_in_guild,
//   bot_names, trigger_phrase, reply_on_relevant, cooldown_sec,
//   store_all_messages, retention_turns.
// Read-only web tools (builtin:web_search, builtin:fetch) are exposed when the
// `tools` capability and `skills` allow-list grant them.

const MAX_TOOL_ROUNDS = 4;

interface QQGroupTurn {
  who: string;
  text: string;
  reply: string;
  reply_to?: string;
}

interface QQGroupSettings {
  respond_on_mention: boolean;
  respond_on_reply: boolean;
  respond_in_c2c: boolean;
  respond_in_guild: boolean;
  bot_names: string;
  trigger_phrase: string;
  reply_on_relevant: boolean;
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
    respond_on_reply: readQQBoolSetting(extras, "respond_on_reply", true),
    respond_in_c2c: readQQBoolSetting(extras, "respond_in_c2c", true),
    respond_in_guild: readQQBoolSetting(extras, "respond_in_guild", false),
    bot_names: typeof extras.bot_names === "string" ? extras.bot_names.trim() : "",
    trigger_phrase: typeof extras.trigger_phrase === "string" ? extras.trigger_phrase.trim() : "",
    reply_on_relevant: readQQBoolSetting(extras, "reply_on_relevant", true),
    cooldown_sec: readQQNumSetting(extras, "cooldown_sec", 15, 0),
    store_all_messages: readQQBoolSetting(extras, "store_all_messages", true),
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

function parseQQNames(raw: string, botUsername: string): string[] {
  const parts = (raw || "").split(/[,，、;；\n]/);
  const out: string[] = [];
  const seen: Record<string, boolean> = {};
  for (let i = 0; i < parts.length; i++) {
    const s = parts[i].trim();
    if (s && !seen[s.toLowerCase()]) {
      seen[s.toLowerCase()] = true;
      out.push(s);
    }
  }
  const uname = (botUsername || "").trim();
  if (uname && !seen[uname.toLowerCase()]) {
    out.push(uname);
  }
  return out;
}

function matchesAnyQQName(text: string, names: string[]): boolean {
  if (!text || names.length === 0) return false;
  const lower = text.toLowerCase();
  for (let i = 0; i < names.length; i++) {
    if (lower.indexOf(names[i].toLowerCase()) >= 0) return true;
  }
  return false;
}

function matchesQQTrigger(text: string, phrase: string): boolean {
  if (!phrase) return false;
  return text.toLowerCase().indexOf(phrase.toLowerCase()) >= 0;
}

function shouldAnswerQQ(
  msg: { text?: string; channel?: string; payload?: Record<string, unknown> },
  cfg: QQGroupSettings
): { mode: "direct" | "passive" | "skip"; reason: string } {
  if (msg.channel === "studio") {
    return { mode: "direct", reason: "studio_test" };
  }
  const p = (msg.payload || {}) as Record<string, unknown>;
  const scene = String(p.scene || "group");

  if (scene === "c2c") {
    if (!cfg.respond_in_c2c) {
      return { mode: "skip", reason: "c2c_disabled" };
    }
    return { mode: "direct", reason: "c2c_chat" };
  }
  if (scene === "guild" || scene === "dm") {
    if (!cfg.respond_in_guild) {
      return { mode: "skip", reason: "guild_disabled" };
    }
    return { mode: "direct", reason: scene };
  }

  const text = msg.text || "";
  const mentioned = p.mentioned === true;
  const replyToBot = p.is_reply_to_bot === true;
  const names = parseQQNames(cfg.bot_names, typeof p.bot_username === "string" ? p.bot_username : "");
  const named = matchesAnyQQName(text, names);
  const triggered = matchesQQTrigger(text, cfg.trigger_phrase);
  const direct =
    (cfg.respond_on_mention && mentioned) ||
    (cfg.respond_on_reply && replyToBot) ||
    named ||
    triggered;

  if (!direct) {
    if (!cfg.reply_on_relevant) {
      return { mode: "skip", reason: "not_mentioned" };
    }
    if (p.mentions_others === true) {
      return { mode: "skip", reason: "mentions_others" };
    }
  }

  if (cfg.cooldown_sec > 0) {
    const elapsed = Date.now() - loadQQLastReplyMs();
    if (elapsed >= 0 && elapsed < cfg.cooldown_sec * 1000) {
      return { mode: "skip", reason: "cooldown" };
    }
  }

  if (!direct) {
    return { mode: "passive", reason: "relevance_check" };
  }
  const reason = mentioned
    ? "mentioned"
    : replyToBot
    ? "reply_to_bot"
    : named
    ? "bot_name"
    : "trigger_phrase";
  return { mode: "direct", reason: reason };
}

function buildReplyToLabel(p: Record<string, unknown>): string {
  const rText = typeof p.reply_to_text === "string" ? p.reply_to_text.trim() : "";
  if (!rText) return "";
  let rAuthor = typeof p.reply_to_author === "string" ? p.reply_to_author.trim() : "";
  if (!rAuthor && p.is_reply_to_bot === true) {
    rAuthor = typeof p.bot_username === "string" && p.bot_username ? p.bot_username : "you";
  }
  return rAuthor ? rAuthor + ": " + rText : rText;
}

function formatQQUserLine(who: string, text: string, replyTo?: string): string {
  const line = who + ": " + text;
  return replyTo ? "[Replying to " + replyTo + "]\n" + line : line;
}

function pushQQUserMessage(messages: ChatMessage[], content: string): void {
  if (!content) return;
  const n = messages.length;
  if (n > 0 && messages[n - 1].role === "user") {
    messages[n - 1].content = String(messages[n - 1].content || "") + "\n" + content;
    return;
  }
  messages.push({ role: "user", content: content });
}

function buildQQMessages(
  msg: { text?: string; from?: string; payload?: Record<string, unknown> },
  cfg: QQGroupSettings,
  turns: QQGroupTurn[],
  instructions: string,
  historyBudget: number,
  passiveCheck: boolean
): ChatMessage[] {
  const p = (msg.payload || {}) as Record<string, unknown>;
  const who = String(p.name || p.username || p.member_openid || p.user_openid || msg.from || "member");
  const scene = String(p.scene || "group");
  const text = msg.text || "";

  let sys = instructions || "You are a concise, helpful member of a QQ group chat.";
  if (scene === "group" || scene === "guild") {
    sys += "\nConversation scene: " + scene + ".";
  }
  if (passiveCheck) {
    sys +=
      "\nPassive group observation mode: the latest message did not explicitly @-mention or quote you. " +
      "Read the recent group messages above to judge whether the speaker is addressing you, continuing your topic, or asking something in the group where a short reply from you is helpful and welcome. " +
      "If it is casual chatter between other members or you do not need to interject, reply with ONLY the exact token [NO_REPLY].";
  }

  // Task 2 customization hook: route support questions to a group-specific knowledge base here.
  // For example, match support keywords or p.chat_id and prepend memory.recall({ store: "knowledge", ... }) passages.

  const messages: ChatMessage[] = [{ role: "system", content: sys }];
  const start = Math.max(0, turns.length - cfg.retention_turns);
  for (let i = start; i < turns.length; i++) {
    const t = turns[i];
    pushQQUserMessage(messages, formatQQUserLine(t.who, t.text, t.reply_to));
    if (t.reply) {
      messages.push({ role: "assistant", content: t.reply });
    }
  }
  const replyTo = buildReplyToLabel(p);
  pushQQUserMessage(messages, formatQQUserLine(who, text, replyTo));
  return token_budget(messages, historyBudget || 6000);
}

function loadQQTools() {
  try {
    return tools.list() || [];
  } catch (_) {}
  return [];
}

function runQQTurn(messages: ChatMessage[], model: string): string {
  const available = loadQQTools();
  for (let round = 0; round < MAX_TOOL_ROUNDS; round++) {
    const opts: Record<string, unknown> = {};
    if (model) {
      opts.model = model;
    }
    if (available.length > 0) {
      opts.tools = available;
      opts.tool_choice = "auto";
    }
    const reply = available.length > 0 || model ? llm.chat(messages, opts) : llm.chat(messages);
    const calls = reply.tool_calls || [];
    if (calls.length === 0) {
      return reply.text || "";
    }
    messages.push({
      role: "assistant",
      content: reply.text || "",
      tool_calls: calls,
      thinking_blocks: reply.thinking_blocks,
    });
    for (let i = 0; i < calls.length; i++) {
      const call = calls[i];
      let body: string;
      try {
        const result = tools.run(call.name, call.args || {});
        body = json.encode(result);
      } catch (err) {
        body = json.encode({ ok: false, error: String(err) });
      }
      messages.push({
        role: "tool",
        tool_call_id: call.id,
        content: body,
      });
    }
  }
  const finalReply = model ? llm.chat(messages, { model: model }) : llm.chat(messages);
  return finalReply.text || "";
}

function isQQNoReply(text: string): boolean {
  const trimmed = text.trim();
  return (
    trimmed === "" ||
    trimmed.indexOf("[NO_REPLY]") >= 0 ||
    /^NO_REPLY[.。!！]?$/i.test(trimmed)
  );
}

function loop(): void {
  while (true) {
    const msg = session.inbox();
    const cfg = readQQSettings();
    const p = (msg.payload || {}) as Record<string, unknown>;
    const who = String(p.name || p.username || p.member_openid || p.user_openid || msg.from || "member");
    const chat = String(p.chat_id || msg.reply_to || "chat");
    const text = msg.text || "";
    const replyTo = buildReplyToLabel(p);
    const turns = loadQQTurns();

    if (!/\S/.test(text)) {
      session.skip("empty_message");
      continue;
    }

    const decision = shouldAnswerQQ(msg, cfg);
    if (decision.mode === "skip") {
      if (cfg.store_all_messages) {
        const turn: QQGroupTurn = { who: who, text: text, reply: "" };
        if (replyTo) turn.reply_to = replyTo;
        turns.push(turn);
        saveQQTurns(turns, cfg.retention_turns);
        rememberQQTurn(chat, who, text, "");
      }
      session.skip(decision.reason);
      continue;
    }

    const passiveCheck = decision.mode === "passive";
    const info = agent.info();
    const messages = buildQQMessages(msg, cfg, turns, info.instructions, info.history_budget, passiveCheck);
    const answer = passiveCheck ? (llm.chat(messages).text || "") : runQQTurn(messages, info.model || "");

    if (passiveCheck && isQQNoReply(answer)) {
      if (cfg.store_all_messages || cfg.reply_on_relevant) {
        const turn: QQGroupTurn = { who: who, text: text, reply: "" };
        if (replyTo) turn.reply_to = replyTo;
        turns.push(turn);
        saveQQTurns(turns, cfg.retention_turns);
        rememberQQTurn(chat, who, text, "");
      }
      session.skip("not_relevant");
      continue;
    }

    session.send(answer);
    saveQQLastReplyMs(Date.now());
    const turn: QQGroupTurn = { who: who, text: text, reply: answer };
    if (replyTo) turn.reply_to = replyTo;
    turns.push(turn);
    saveQQTurns(turns, cfg.retention_turns);
    rememberQQTurn(chat, who, text, answer);
  }
}
