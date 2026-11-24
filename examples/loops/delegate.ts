// af-prelude-version: 2
// Parent agent delegating to an ephemeral child.
//
// Needs: agent.spawn, agent.request, and a spawn template named here under
// profiles.agent (the "researcher" profile is an example — add one, or change
// the name). The child's capabilities are the intersection of this agent's
// and the template's; nothing can be widened by spawning.
//
// The worker side (the template's loop) answers a request by replying with
// the request id from the core-stamped provenance:
//
//   if (msg.type === "agent") {
//     const rid = msg.provenance && msg.provenance.request_id;
//     if (rid) agent.reply(rid, { answer: "..." });
//   }
//
// Do not build the id from msg.payload: a payload is loop-controlled, the
// provenance is engine-stamped.

function handle(msg: Message) {
  let child;
  try {
    child = agent.spawn("researcher");
  } catch (err) {
    session.send("could not start a researcher: " + String(err));
    return;
  }

  // child.address is a session:<id> address and is the one to use for a
  // spawned child; agent:<name> addresses a configured static agent.
  let answer;
  try {
    answer = agent.request(child.address, { question: msg.text }, 120);
  } catch (err) {
    session.send("the researcher did not answer: " + String(err));
    return;
  }

  const payload = answer.payload || {};
  session.send(payload.answer || json.encode(payload));
}

function loop() {
  while (true) {
    const msg = session.inbox();
    if (msg.type === "user" && msg.text) {
      handle(msg);
    }
  }
}
