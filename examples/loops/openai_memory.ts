// af-prelude-version: 2
// openai_memory — pass-through with memory, the "exceptional" side of the
// OpenAI channel.
//
// The default loop forwards the client's request untouched, because a client
// that is itself an agent should get exactly a model. This loop is the other
// choice: it remembers things about the person and prepends what it recalled
// as a system message, while still forwarding the client's own conversation,
// tools and sampling parameters.
//
// The facts store is user-scoped by the engine (the turn's identity, from the
// bearer token), so one person's memories never reach another's request. With
// auth: key or none there is no tenant, and the recall is empty — the loop
// still works, it just has nothing to remember.

function factsSystemMessage(msg: Message): ChatMessage | null {
  const hits = memory.recall({
    store: "facts",
    kind: "text",
    text: msg.text || "",
    k: 5,
  });
  if (!hits || hits.length === 0) {
    return null;
  }
  const lines: string[] = [];
  for (const hit of hits) {
    const v = hit.value;
    let text: unknown = null;
    if (v && typeof v === "object") {
      text = v.text || v.fact || v.value;
    } else if (typeof v === "string") {
      text = v;
    }
    if (typeof text === "string" && text !== "") {
      lines.push("- " + text);
    }
  }
  if (lines.length === 0) {
    return null;
  }
  return {
    role: "system",
    content: "Known facts about the person you are talking to:\n" + lines.join("\n"),
  };
}

function loop() {
  while (true) {
    const msg = session.inbox();
    const facts = factsSystemMessage(msg);
    const opts = facts ? { prepend: [facts] } : undefined;
    const reply = llm.forward(opts);
    session.reply(reply);
  }
}
