// af-prelude-version: 2
// Single-turn chat/completions.
//
// The OpenAI channel turns every POST /v1/chat/completions into one turn: the
// request's last user message is msg.text, and the session exists for this
// request only (unless the client sends a conversation id). This loop answers
// that one message with the agent's own instructions and model, and nothing
// else from the client's request — a small, predictable endpoint for scripts,
// form fills and classification calls.
//
// To forward the client's whole request (its messages, tools and sampling
// parameters) instead, use examples/loops/openai_passthrough.ts.

function loop() {
  while (true) {
    const msg = session.inbox();
    const info = agent.info();
    const reply = llm.chat([
      { role: "system", content: info.instructions || "You are a helpful assistant." },
      { role: "user", content: msg.text || "" },
    ]);
    session.reply({
      text: reply.text || "",
      finish_reason: reply.finish_reason,
      usage: reply.usage,
    });
  }
}
