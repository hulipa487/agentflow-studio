// af-prelude-version: 2
// QQ group chatbot with memory and context.
//
// A QQ group hears the bot only when someone @-mentions it, and the router
// keeps one session per group (payload.chat_id = "group:<group_openid>"). For
// every message the loop builds the prompt from three layers:
//
//   context  the group's last exchanges, word for word. They live in
//            session.state, so they survive the session being unloaded when
//            the group goes quiet, and a restart.
//   memory   older exchanges from this group that match the new message,
//            found by full-text recall over the dialogue store.
//   facts    what members asked the bot to keep ("/remember ..."), recalled
//            the same way.
//
// Every speaker is named in the prompt, so the model can tell members apart
// and answer the right person.
//
// Commands: /remember <fact>, /forget (clears the recent context).

const CONTEXT_TURNS = 10; // exchanges kept word for word
const RECALL_K = 4;       // older exchanges or facts recalled per message
const MAX_LINE = 800;     // characters kept from one message in the context

interface Exchange {
  who: string;
  text: string;
  reply: string;
}

function chatOf(msg: Message): string {
  const p = msg.payload || {};
  return String(p.chat_id || msg.reply_to || "local");
}

function speakerOf(msg: Message): string {
  const p = msg.payload || {};
  return String(p.name || p.username || p.member_openid || msg.from || "member");
}

function clip(s: unknown): string {
  const str = String(s || "");
  if (str.length > MAX_LINE) {
    return str.slice(0, MAX_LINE) + "…";
  }
  return str;
}

function loadContext(): Exchange[] {
  const ctx = session.state.get("context");
  if (!Array.isArray(ctx)) {
    return [];
  }
  return ctx as Exchange[];
}

function saveExchange(ctx: Exchange[], who: string, text: string, reply: string) {
  ctx.push({ who, text: clip(text), reply: clip(reply) });
  while (ctx.length > CONTEXT_TURNS) {
    ctx.shift();
  }
  session.state.set("context", ctx);
}

// recall returns this group's records from a store that match text. The
// store is shared by every group the bot is in, so other groups' rows are
// filtered out here.
function recall(store: string, chat: string, text: string, skip?: Record<string, boolean>): Record<string, unknown>[] {
  const hits = memory.recall({ store, kind: "text", text, k: RECALL_K * 3 }) || [];
  const out: Record<string, unknown>[] = [];
  for (const hit of hits) {
    const v = hit.value;
    if (v && typeof v === "object" && v.chat === chat && !(skip && skip[String(v.text || "")])) {
      out.push(v);
      if (out.length >= RECALL_K) {
        break;
      }
    }
  }
  return out;
}

function buildPrompt(info: AgentInfo, chat: string, who: string, text: string, ctx: Exchange[]): ChatMessage[] {
  const messages: ChatMessage[] = [
    { role: "system", content: info.instructions || "You are a friendly member of this QQ group." },
  ];

  const facts = recall("facts", chat, text);
  if (facts.length > 0) {
    const lines = facts.map((f) => "- " + String(f.text) + " (from " + String(f.by || "a member") + ")");
    messages.push({ role: "system", content: "Facts this group asked you to remember:\n" + lines.join("\n") });
  }

  // Older exchanges, minus the ones the context already holds word for word.
  const recent: Record<string, boolean> = {};
  for (const e of ctx) {
    recent[e.text] = true;
  }
  const older = recall("dialogue", chat, text, recent);
  if (older.length > 0) {
    const lines = older.map((e) => "- " + String(e.who) + ": " + clip(e.text) + "\n  you: " + clip(e.reply));
    messages.push({ role: "system", content: "Earlier in this group (may be relevant):\n" + lines.join("\n") });
  }

  for (const e of ctx) {
    messages.push({ role: "user", content: e.who + ": " + e.text });
    messages.push({ role: "assistant", content: e.reply });
  }
  messages.push({ role: "user", content: who + ": " + text });

  // token_budget is a support chunk: it trims the oldest turns first and
  // keeps the system messages and the newest turn whole.
  return token_budget(messages, info.history_budget || 6000);
}

function loop() {
  while (true) {
    const msg = session.inbox();
    const text = msg.text || "";
    const chat = chatOf(msg);
    const who = speakerOf(msg);

    const factMatch = text.match(/^\s*\/remember\s+(.+)$/);
    if (factMatch) {
      const fact = factMatch[1];
      memory.write({
        type: "fact",
        key: chat + ":" + String(os.time()) + ":" + String(fact.length),
        chat,
        text: fact,
        by: who,
      });
      session.send("Noted, I'll remember: " + fact);
    } else if (/^\s*\/forget\s*$/.test(text)) {
      session.state.delete("context");
      session.send("Cleared what I remember of the recent conversation here.");
    } else if (/\S/.test(text)) {
      const ctx = loadContext();
      const reply = llm.chat(buildPrompt(agent.info(), chat, who, text, ctx));
      const answer = reply.text || "";
      session.send(answer);
      memory.write({ type: "turn", id: msg.id, chat, who, text, reply: answer });
      saveExchange(ctx, who, text, answer);
    }
  }
}
