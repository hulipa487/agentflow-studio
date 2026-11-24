// af-prelude-version: 2
// The smallest runnable loop: no model, no tools. It replies with the text it
// received, which makes it the first thing to test after a boot.

function loop() {
  while (true) {
    const msg = session.inbox();
    session.send("echo: " + (msg.text || ""));
  }
}
