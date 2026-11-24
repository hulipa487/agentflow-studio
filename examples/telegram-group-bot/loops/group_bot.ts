// af-prelude-version: 2
// Flagship Telegram Group Bot starter.
// Ordinary group rules live in Studio Starter Settings (agent.config().extras):
//   respond_on_mention, respond_on_reply, respond_in_dm, trigger_phrase,
//   cooldown_sec, store_all_messages, retention_turns.
// Read-only web tools (builtin:web_search, builtin:browser, builtin:fetch) are
// exposed when the `tools` capability and `skills` allow-list grant them.

const MAX_TOOL_ROUNDS = 4;

interface GroupTurn {
  who: string;
  text: string;
  reply: string;
}

interface GroupSettings {
  respond_on_mention: boolean;
  respond_on_reply: boolean;
  respond_in_dm: boolean;
  trigger_phrase: string;
  cooldown_sec: number;
  store_all_messages: boolean;
  retention_turns: number;
}

function readBoolSetting(extras: Record<string, unknown>, key: string, fallback: boolean): boolean {
  return typeof extras[key] === "boolean" ? (extras[key] as boolean) : fallback;
}

function readNumSetting(extras: Record<string, unknown>, key: string, fallback: number, min: number): number {
  const val = Number(extras[key]);
  return Number.isFinite(val) && val >= min ? val : fallback;
}

function readSettings(): GroupSettings {
  const extras = ((agent.config() || {}).extras || {}) as Record<string, unknown>;
  return {
    respond_on_mention: readBoolSetting(extras, "respond_on_mention", true),
    respond_on_reply: readBoolSetting(extras, "respond_on_reply", true),
    respond_in_dm: readBoolSetting(extras, "respond_in_dm", true),
    trigger_phrase: typeof extras.trigger_phrase === "string" ? extras.trigger_phrase.trim() : "",
    cooldown_sec: readNumSetting(extras, "cooldown_sec", 15, 0),
    store_all_messages: readBoolSetting(extras, "store_all_messages", false),
    retention_turns: Math.max(1, Math.floor(readNumSetting(extras, "retention_turns", 20, 1))),
  };
}

function loadTurns(): GroupTurn[] {
  try {
    const v = session.state.get("turns");
    if (Array.isArray(v)) return v as GroupTurn[];
  } catch (_) {}
  return [];
}

function saveTurns(turns: GroupTurn[], limit: number): void {
  while (turns.length > limit) turns.shift();
  try {
    session.state.set("turns", turns);
  } catch (_) {}
}

function loadLastReplyMs(): number {
  try {
    const v = Number(session.state.get("last_reply_ms"));
    return Number.isFinite(v) ? v : 0;
  } catch (_) {}
  return 0;
}

function saveLastReplyMs(ts: number): void {
  try {
    session.state.set("last_reply_ms", ts);
  } catch (_) {}
}

function rememberTurn(chat: string, who: string, text: string, reply: string): void {
  try {
    memory.write({ type: "turn", chat: chat, who: who, text: text, reply: reply });
  } catch (_) {}
}

function matchesTrigger(text: string, phrase: string): boolean {
  if (!phrase) return false;
  return text.toLowerCase().indexOf(phrase.toLowerCase()) >= 0;
}

function shouldAnswer(
  msg: { text?: string; channel?: string; payload?: Record<string, unknown> },
  cfg: GroupSettings
): { ok: true; reason: string } | { ok: false; reason: string } {
  const p = (msg.payload || {}) as Record<string, unknown>;
  const chatType = String(p.chat_type || "");
  const isStudio = msg.channel === "studio";
  const isPrivate = isStudio || chatType === "private" || chatType === "";

  if (isPrivate) {
    if (!isStudio && !cfg.respond_in_dm) {
      return { ok: false, reason: "dm_disabled" };
    }
    return { ok: true, reason: isStudio ? "studio_test" : "private_chat" };
  }

  const text = msg.text || "";
  const mentioned = p.mentioned === true;
  const replyToBot = p.is_reply_to_bot === true;
  const triggered = matchesTrigger(text, cfg.trigger_phrase);
  const matched =
    (cfg.respond_on_mention && mentioned) ||
    (cfg.respond_on_reply && replyToBot) ||
    triggered;

  if (!matched) {
    return { ok: false, reason: "not_mentioned" };
  }
  if (cfg.cooldown_sec > 0) {
    const elapsed = Date.now() - loadLastReplyMs();
    if (elapsed >= 0 && elapsed < cfg.cooldown_sec * 1000) {
      return { ok: false, reason: "cooldown" };
    }
  }
  return { ok: true, reason: triggered ? "trigger_phrase" : replyToBot ? "reply_to_bot" : "mentioned" };
}

function pushUserMessage(messages: ChatMessage[], content: string): void {
  if (!content) return;
  const n = messages.length;
  if (n > 0 && messages[n - 1].role === "user") {
    messages[n - 1].content = String(messages[n - 1].content || "") + "\n" + content;
    return;
  }
  messages.push({ role: "user", content: content });
}

function buildMessages(
  msg: { text?: string; from?: string; payload?: Record<string, unknown> },
  cfg: GroupSettings,
  turns: GroupTurn[],
  instructions: string,
  historyBudget: number
): ChatMessage[] {
  const p = (msg.payload || {}) as Record<string, unknown>;
  const who = String(p.name || p.username || msg.from || "member");
  const chatTitle = String(p.chat_title || "");
  const replyToText = String(p.reply_to_text || "");
  const text = msg.text || "";

  let sys = instructions || "You are a concise, helpful member of a Telegram group chat.";
  if (chatTitle) {
    sys += "\nGroup: " + chatTitle;
  }

  // Task 2 customization hook: route support questions to a group-specific knowledge base here.
  // For example, match support keywords or p.chat_id and prepend memory.recall({ store: "knowledge", ... }) passages.

  const messages: ChatMessage[] = [{ role: "system", content: sys }];
  const start = Math.max(0, turns.length - cfg.retention_turns);
  for (let i = start; i < turns.length; i++) {
    const t = turns[i];
    pushUserMessage(messages, t.who + ": " + t.text);
    if (t.reply) {
      messages.push({ role: "assistant", content: t.reply });
    }
  }
  let userLine = who + ": " + text;
  if (replyToText) {
    userLine = "[Replying to: " + replyToText + "]\n" + userLine;
  }
  pushUserMessage(messages, userLine);
  return token_budget(messages, historyBudget || 6000);
}

function loadTools() {
  try {
    return tools.list() || [];
  } catch (_) {}
  return [];
}

function runTurn(messages: ChatMessage[], model: string): string {
  const available = loadTools();
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

function loop(): void {
  while (true) {
    const msg = session.inbox();
    const cfg = readSettings();
    const p = (msg.payload || {}) as Record<string, unknown>;
    const who = String(p.name || p.username || msg.from || "member");
    const chat = String(p.chat_id || msg.reply_to || "chat");
    const text = msg.text || "";
    const turns = loadTurns();

    if (!/\S/.test(text)) {
      session.skip("empty_message");
      continue;
    }

    const decision = shouldAnswer(msg, cfg);
    if (!decision.ok) {
      if (cfg.store_all_messages) {
        turns.push({ who: who, text: text, reply: "" });
        saveTurns(turns, cfg.retention_turns);
        rememberTurn(chat, who, text, "");
      }
      session.skip(decision.reason);
      continue;
    }

    const info = agent.info();
    const messages = buildMessages(msg, cfg, turns, info.instructions, info.history_budget);
    const answer = runTurn(messages, info.model || "");

    session.send(answer);
    saveLastReplyMs(Date.now());
    turns.push({ who: who, text: text, reply: answer });
    saveTurns(turns, cfg.retention_turns);
    rememberTurn(chat, who, text, answer);
  }
}
