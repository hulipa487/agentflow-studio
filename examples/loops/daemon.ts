// af-prelude-version: 2
// A daemon (agents.<name>.persistent: true).
//
// The supervisor delivers a synthetic boot message at startup:
//   { type: "boot", from: "system:supervisor", text: "", channel: "" }
// so the loop can do setup work before any traffic. A daemon also receives
// ordinary user messages and its own timer messages. There is no channel on a
// boot or timer turn, so session.send is not available there: keep state and
// report it on the next user turn, or use session.push / session.push_user
// (cap: channel.push).
//
// Timers are re-registered idempotently for the same reason as scheduled.ts:
// a hot reload restarts the VM runtime but the session (and its timers) lives on.

function ensureTimer(key: string, seconds: number): string {
  const old = session.state.get(key);
  if (old) {
    try {
      scheduler.cancel(String(old));
    } catch (_) {
      // ignore stale timer id after a restart
    }
  }
  const t = scheduler.every(seconds);
  session.state.set(key, t.id);
  return t.id;
}

function loop() {
  const tick = ensureTimer("timer:daemon", 300);

  while (true) {
    const msg = session.inbox();

    if (msg.type === "boot") {
      session.state.set("booted_at", os.time());
      log.info("daemon booted at " + os.date("%Y-%m-%d %H:%M:%S"));
    } else if (msg.type === "timer") {
      if (msg.payload && msg.payload.timer_id === tick) {
        const last = session.state.get("last_tick") || 0;
        session.state.set("last_tick", os.time());
        log.debug("daemon tick; previous " + String(last));
      }
    } else if (msg.type === "user") {
      const bootedAt = Number(session.state.get("booted_at") || os.time());
      session.send("daemon up since " + os.date("%Y-%m-%d %H:%M:%S", bootedAt));
    }
  }
}
