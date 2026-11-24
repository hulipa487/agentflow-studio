// af-prelude-version: 2
// af-import: greeting

function loop() {
  while (true) {
    const msg = session.inbox();
    session.send(Greeting.hello(msg.text));
  }
}
