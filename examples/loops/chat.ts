// af-prelude-version: 2
// A conversational loop with memory:
//   1. recall earlier turns from the dialogue store (full-text query),
//   2. build the prompt (system instructions + recalled turns + this turn),
//   3. trim it to the agent's history budget,
//   4. ask the model, reply, and record the turn.
//
// The memory block is skipped when the agent has no memory profile, so the
// same loop also works as a plain chat agent.

function loop() {
  while (true) {
    const msg = session.inbox();
    const info = agent.info();
    const text = msg.text || "";

    let messages: ChatMessage[] = [
      { role: "system", content: info.instructions || "You are a helpful assistant." },
    ];

    const mem = info.memory || {};
    const stores = mem.stores || {};
    const hasDialogue = stores.dialogue !== undefined && stores.dialogue !== null;

    if (hasDialogue) {
      const hits = memory.recall({
        store: "dialogue",
        kind: "text",
        text,
        k: 5,
      });
      for (const hit of hits || []) {
        const v = hit.value;
        if (v && typeof v === "object") {
          if (typeof v.text === "string" && v.text !== "") {
            messages.push({ role: "user", content: v.text });
          }
          if (typeof v.reply === "string" && v.reply !== "") {
            messages.push({ role: "assistant", content: v.reply });
          }
        }
      }
    }

    messages.push({ role: "user", content: text });
    // token_budget is a support chunk loaded into every session.
    messages = token_budget(messages, info.history_budget || 6000);

    const reply = llm.chat(messages);
    session.send(reply.text || "");

    if (hasDialogue) {
      memory.write({
        type: "turn",
        id: msg.id,
        text,
        reply: reply.text || "",
        from: msg.from,
      });
    }
  }
}
