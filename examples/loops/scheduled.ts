// af-prelude-version: 2
// Session timers.
//
// Timers are ordinary mailbox messages (type="timer", payload.timer_id), so
// the loop stays single-threaded: there is no callback running beside your
// code. They are session-scoped and in memory, which has two consequences:
//
//   * a process restart forgets them; the loop re-registers on its next start
//   * a hot reload restarts the VM runtime but not the session, so timers
//     registered by the previous run are still live. Re-registering blindly
//     would duplicate them.
//
// ensureTimer below cancels any remembered id first, so it is idempotent
// across reloads and harmless after a restart (an unknown id cancels with an
// error that try/catch swallows). It needs the session.state capability.

function ensureTimer(key: string, seconds: number): string {
  const old = session.state.get(key);
  if (old) {
    try {
      scheduler.cancel(String(old));
    } catch (_) {
      // ignore unknown id after a restart
    }
  }
  const t = scheduler.every(seconds);
  session.state.set(key, t.id);
  return t.id;
}

function loop() {
  const tick = ensureTimer("timer:tick", 60);
  const sweep = ensureTimer("timer:sweep", 3600);
  log.info("timers registered: " + tick + ", " + sweep);

  while (true) {
    const msg = session.inbox();

    if (msg.type === "timer") {
      const id = msg.payload && msg.payload.timer_id;
      if (id === tick) {
        const n = Number(session.state.get("ticks") || 0) + 1;
        session.state.set("ticks", n);
        log.debug("tick " + n + " at " + os.date("%H:%M:%S"));
      } else if (id === sweep) {
        log.info("hourly sweep at " + os.date("%Y-%m-%d %H:%M:%S"));
      }
      // A timer turn has no channel: session.send would fail. Push to a
      // known recipient (needs channel.push + its channel/reply_to), or
      // record state for the next user turn to report.
    } else if (msg.type === "user") {
      session.send("ticks so far: " + String(session.state.get("ticks") || 0));
    }
  }
}
