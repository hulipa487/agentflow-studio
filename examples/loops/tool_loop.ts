// af-prelude-version: 2
// A multi-round tool loop.
//
// The engine executes one tool when asked; driving the conversation is the
// loop's job. That includes replaying the assistant turn that requested the
// tools, appending each result as a role="tool" message, and asking again —
// a model that requests a second tool will not get it otherwise.
//
// Needs: llm.chat and tools. The agent must be able to see the tools it uses
// (skills, or tools.policy.default).

const MAX_ROUNDS = 4;

function handle(msg: Message) {
  const info = agent.info();
  const messages: ChatMessage[] = [
    { role: "system", content: info.instructions || "Use the available tools when they help." },
    { role: "user", content: msg.text || "" },
  ];
  const available = tools.list();

  for (let round = 0; round < MAX_ROUNDS; round++) {
    const opts: Record<string, unknown> = { model: info.model };
    if (available.length > 0) {
      opts.tools = available;
      opts.tool_choice = "auto";
    }

    const reply = llm.chat(messages, opts);
    const calls = reply.tool_calls || [];
    if (calls.length === 0) {
      session.send(reply.text || "");
      return;
    }

    // Replay the assistant turn verbatim. thinking_blocks matters on
    // Anthropic when thinking is enabled; other providers ignore it.
    messages.push({
      role: "assistant",
      content: reply.text || "",
      tool_calls: calls,
      thinking_blocks: reply.thinking_blocks,
    });

    for (const call of calls) {
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

  session.send("I stopped after " + MAX_ROUNDS + " tool rounds without a final answer.");
}

function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.text && msg.text !== "") {
      try {
        handle(msg);
      } catch (err) {
        log.error("tool turn failed: " + String(err));
        session.send("Something went wrong while handling that.");
      }
    }
  }
}
