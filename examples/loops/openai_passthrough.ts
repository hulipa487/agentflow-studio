// af-prelude-version: 2
// Example loop: openai_passthrough — make an agent look exactly like a model.
//
// Use with an `openai` channel when you want to forward the client's own
// request (messages, tools, tool_choice, sampling parameters) to the agent's
// model and return the model's text and tool calls unchanged. The client is
// the agent here: its system prompt, its tools, its conversation. AgentFlow
// contributes identity, metering, safety and routing — not meaning.
//
// Tool calls are never executed by the engine. A client that wants its tools
// run must come back with a tool result, which the next request carries and
// this loop forwards like any other turn.

function loop() {
  while (true) {
    session.inbox();
    const reply = llm.forward();
    session.reply(reply);
  }
}
