// af-prelude-version: 2
// Streaming text from the model.
//
// llm.stream(...) returns an ES6 iterable that yields text deltas across the
// async bridge. A `for ... of` loop consumes each delta cleanly and closes the
// underlying stream automatically if exited early via break/throw; calling
// stream.close() in finally ensures the provider connection is released.
//
// The iterator yields text deltas only. Use llm.chat when you also need
// usage, tool_calls, or a buffered reply.

function loop() {
  while (true) {
    const msg = session.inbox();
    const info = agent.info();

    const stream = llm.stream(
      [
        { role: "system", content: info.instructions || "Be concise." },
        { role: "user", content: msg.text || "" },
      ],
      { model: info.model },
    );

    const parts: string[] = [];
    try {
      for (const delta of stream) {
        if (typeof delta === "string") {
          parts.push(delta);
        }
      }
    } finally {
      stream.close(); // no-op after done; required after an early exit
    }

    session.send(parts.join(""));
  }
}
