// af-prelude-version: 2
// Telegram bot that answers from a knowledge base (retrieval-augmented
// generation).
//
// Knowledge lives in one vector store. A question is embedded and the closest
// passages come back through memory.recall (the profile's recall handler is
// plugin:semantic, and an optional rerank model sharpens the order). The model
// answers from those passages only and cites them as [1], [2], ...; with no
// passage close enough it says so instead of guessing, without a model call.
//
// Teaching it:
//   /learn <text>  splits the text into passages and stores them with their
//                  embeddings. Only editors may (extras.editors in the agent's
//                  config, as "user:telegram:<id>"), and so may the ingest
//                  webhook, whose secret path is the authorisation — see
//                  README.md for loading a whole document that way.

const PASSAGE_CHARS = 900; // target passage size when splitting a document
const TOP_K = 4;           // passages given to the model
const MIN_SCORE = 0.25;    // rerank score below which a passage is ignored

function isEditor(msg: Message): boolean {
  if (msg.channel === "ingest") {
    return true;
  }
  const extras = ((agent.config() || {}).extras || {}) as Record<string, unknown>;
  const editors = Array.isArray(extras.editors) ? extras.editors : [];
  for (const who of editors) {
    if (who === msg.from) {
      return true;
    }
  }
  return false;
}

// split cuts a document into passages at blank lines, packing paragraphs
// together up to PASSAGE_CHARS so each passage carries some context.
function splitPassages(doc: string): string[] {
  const passages: string[] = [];
  let current = "";
  for (const rawPara of doc.split(/\n\s*\n/)) {
    const para = rawPara.trim();
    if (para !== "") {
      if (current.length > 0 && current.length + para.length + 2 > PASSAGE_CHARS) {
        passages.push(current);
        current = para;
      } else if (current.length > 0) {
        current = current + "\n\n" + para;
      } else {
        current = para;
      }
    }
  }
  if (current.length > 0) {
    passages.push(current);
  }
  return passages;
}

function learn(doc: string, source: string): number {
  const passages = splitPassages(doc);
  const stamp = String(os.time());
  for (let i = 0; i < passages.length; i++) {
    // Explicit targets route straight to the knowledge store; memory.write
    // embeds the text because the profile names an embed_model.
    memory.write({
      targets: [
        {
          store: "knowledge",
          key: "doc:" + stamp + ":" + (i + 1),
          value: { text: passages[i], source },
        },
      ],
    });
  }
  return passages.length;
}

function retrieve(question: string): Array<Record<string, unknown>> {
  const hits = memory.recall({ store: "knowledge", text: question, k: TOP_K }) || [];
  const out: Array<Record<string, unknown>> = [];
  for (const hit of hits) {
    const score = hit.meta && hit.meta.rerank_score;
    const v = hit.value;
    if (v && typeof v === "object" && v.text && (score === undefined || score === null || score >= MIN_SCORE)) {
      out.push(v);
    }
  }
  return out;
}

function answer(question: string, passages: Array<Record<string, unknown>>): string {
  const numbered = passages.map((p, i) => "[" + (i + 1) + "] (" + String(p.source || "knowledge base") + ")\n" + String(p.text));
  const info = agent.info();
  const reply = llm.chat([
    { role: "system", content: info.instructions || "Answer from the passages and cite them." },
    { role: "system", content: "Passages:\n\n" + numbered.join("\n\n") },
    { role: "user", content: question },
  ]);
  return reply.text || "";
}

function loop() {
  while (true) {
    const msg = session.inbox();
    const text = msg.text || "";
    const learnMatch = text.match(/^\s*\/learn\s+([\s\S]+)$/);
    if (learnMatch) {
      const doc = learnMatch[1];
      if (isEditor(msg)) {
        const p = msg.payload || {};
        const source = p.source || p.username || msg.from || "chat";
        const n = learn(doc, String(source));
        session.send("Learned " + n + " passage" + (n === 1 ? "" : "s") + ".");
      } else {
        session.send("Only editors can teach me. Ask the bot's owner to add you.");
      }
    } else if (/^\s*\/start/.test(text)) {
      session.send("Ask me anything about what I've been taught. I answer from my knowledge base and cite it.");
    } else if (/\S/.test(text)) {
      const passages = retrieve(text);
      if (passages.length === 0) {
        session.send("I don't have anything on that in my knowledge base yet.");
      } else {
        session.send(answer(text, passages));
      }
    }
  }
}
