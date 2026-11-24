// af-prelude-version: 2
// GitHub pull request dispatcher.
//
// Every GitHub delivery reaches this one session (the ghhook channel signs
// and forwards events as msg.payload = { source: "github", event, data }).
// For a pull request that was opened, updated, reopened or marked ready, it
// spawns a code_reviewer from the profiles.agent template and hands it the
// job. Each reviewer is its own session with its own sandbox project, works
// the pull request through, comments on it, and exits — so reviews run side
// by side and one slow review never holds up the next event.

const REVIEW_ON: Record<string, boolean> = {
  opened: true,
  synchronize: true,
  reopened: true,
  ready_for_review: true,
};

function jobFrom(data: Record<string, unknown>) {
  const pr = (data.pull_request || {}) as Record<string, unknown>;
  const repoObj = (data.repository || {}) as Record<string, unknown>;
  const repo = repoObj.full_name;
  if (!repo || !pr.number || pr.draft) {
    return null;
  }
  const head = (pr.head || {}) as Record<string, unknown>;
  const user = (pr.user || {}) as Record<string, unknown>;
  return {
    repo,
    number: pr.number,
    head_sha: head.sha,
    title: pr.title || "",
    body: pr.body || "",
    url: pr.html_url,
    author: user.login,
  };
}

function loop() {
  while (true) {
    const msg = session.inbox();
    const p = msg.payload || {};
    const data = p.data || {};
    if (p.source === "github" && p.event === "pull_request" && REVIEW_ON[String(data.action)]) {
      const job = jobFrom(data);
      if (job) {
        try {
          const child = agent.spawn("code_reviewer");
          if (child && child.address) {
            agent.send(child.address, job);
            log.info("review started: " + job.repo + "#" + job.number + " -> " + child.address);
          }
        } catch (err) {
          log.error("could not start a reviewer for " + job.repo + "#" + job.number + ": " + String(err));
        }
      }
    } else if (p.event === "ping") {
      log.info("GitHub webhook connected");
    }
  }
}
