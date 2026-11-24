// af-prelude-version: 2
// A code agent that reviews one pull request, comments, and exits.
//
// Spawned by pr_dispatcher with the job as its first message. It:
//   1. lists the pull request's changed files through the GitHub API and
//      writes them, at the head commit, into a files-store project of its
//      own, with PR.md (title and description) and PR.diff (the patches);
//   2. works through the change with one tool, script:run, which runs a shell
//      command on that project in a gVisor sandbox: cat, grep, a compile or
//      lint check — no network, and nothing outside the project survives;
//   3. posts its review as a comment on the pull request.
//
// The GitHub token is the engine-wide credential "github", allowed for this
// profile by credentials: [github]. It is read once per job and only ever
// placed in a request header.

const MAX_FILES = 40;       // changed files copied into the sandbox project
const MAX_CHANGES = 2000;   // skip files with more changed lines than this
const MAX_ROUNDS = 8;       // tool rounds before the review must be written
const OUTPUT_CHARS = 6000;  // command output handed back to the model

let project: string | null = null; // the current job's sandbox project
let token: string | null = null;

function clip(s: unknown, n: number): string {
  const str = String(s ?? "");
  if (str.length > n) {
    return str.slice(0, n) + "\n…(truncated)";
  }
  return str;
}

tool.def({
  name: "script:run",
  description:
    "Run a shell command in the pull request's checkout: the changed files at the head commit, plus PR.md and PR.diff at the root. There is no network. Use it to read files (cat, sed -n), search (grep -rn) and run quick checks (python -m py_compile, etc.).",
  params: {
    type: "object",
    properties: { command: { type: "string", description: "The shell command to run." } },
    required: ["command"],
  },
  handler(args) {
    if (!project) {
      return { ok: false, error: "no pull request is checked out" };
    }
    try {
      const r = shell.run(String(args.command || ""), { project, timeout: 60 });
      return {
        ok: true,
        exit_code: r.exit_code,
        timed_out: r.timed_out,
        stdout: clip(r.stdout, OUTPUT_CHARS),
        stderr: clip(r.stderr, 2000),
      };
    } catch (err) {
      return { ok: false, error: String(err) };
    }
  },
});

function escapePath(path: string): string {
  return path.replace(/[^A-Za-z0-9\-._~/]/g, (c) => encodeURIComponent(c));
}

function github(method: string, path: string, opts?: { accept?: string; json?: unknown }): HTTPResponse {
  const o = opts || {};
  return http.request({
    method,
    url: "https://api.github.com" + path,
    headers: {
      Accept: o.accept || "application/vnd.github+json",
      Authorization: "Bearer " + token,
      "X-GitHub-Api-Version": "2022-11-28",
      "User-Agent": "agentflow-pr-reviewer",
    },
    json: o.json,
    timeout: 30,
  });
}

// checkout copies the pull request into the job's project.
function checkout(job: Record<string, unknown>): { total: number; copied: number } {
  const r = github("GET", "/repos/" + job.repo + "/pulls/" + job.number + "/files?per_page=100");
  if (r.status !== 200) {
    throw new Error("listing the pull request's files: HTTP " + String(r.status));
  }
  const changed = (json.decode(r.body) || []) as Array<Record<string, unknown>>;
  const diff: string[] = [];
  let copied = 0;

  for (const f of changed) {
    const filename = String(f.filename || "");
    if (f.patch) {
      diff.push("--- " + filename + " (" + String(f.status) + ")\n" + String(f.patch));
    }
    if (f.status !== "removed" && copied < MAX_FILES && Number(f.changes || 0) <= MAX_CHANGES) {
      const c = github(
        "GET",
        "/repos/" + job.repo + "/contents/" + escapePath(filename) + "?ref=" + String(job.head_sha),
        { accept: "application/vnd.github.raw" },
      );
      if (c.status === 200 && !c.truncated) {
        // Binary files are not utf-8 text and are left out.
        try {
          files.put(project!, filename, c.body);
          copied += 1;
        } catch (_) {
          // ignore non-UTF-8 files
        }
      }
    }
  }

  files.put(project!, "PR.md", "# " + String(job.title || "") + "\n\nby @" + String(job.author) + "\n\n" + String(job.body || ""));
  files.put(project!, "PR.diff", diff.join("\n\n"));
  return { total: changed.length, copied };
}

// review runs the tool loop and returns the review text.
function review(job: Record<string, unknown>, total: number, copied: number): string {
  const info = agent.info();
  const runTool: ToolSpec[] = [];
  for (const t of tools.list()) {
    if (t.function && t.function.name === "script:run") {
      runTool.push(t);
    }
  }

  const messages: ChatMessage[] = [
    { role: "system", content: info.instructions || "You review pull requests." },
    {
      role: "user",
      content:
        "Review pull request #" +
        String(job.number) +
        " in " +
        String(job.repo) +
        ": " +
        String(job.title || "") +
        "\n" +
        total +
        " file(s) changed; " +
        copied +
        " are in your checkout. Start with `cat PR.md PR.diff`.",
    },
  ];

  for (let round = 0; round < MAX_ROUNDS; round++) {
    const reply = llm.chat(messages, { tools: runTool, tool_choice: "auto" });
    const calls = reply.tool_calls || [];
    if (calls.length === 0) {
      return reply.text || "";
    }
    messages.push({
      role: "assistant",
      content: reply.text || "",
      tool_calls: calls,
      thinking_blocks: reply.thinking_blocks,
    });
    for (const call of calls) {
      let result: unknown;
      try {
        result = tools.run(call.name, call.args || {});
      } catch (err) {
        result = { ok: false, error: String(err) };
      }
      messages.push({ role: "tool", tool_call_id: call.id, content: json.encode(result) });
    }
  }

  // Out of rounds: ask for the review with what has been seen so far.
  messages.push({ role: "user", content: "Write the review now, from what you have seen." });
  return llm.chat(messages).text || "";
}

function runJob(job: Record<string, unknown>) {
  const cred = credential.get("github");
  if (!cred || !cred.value || cred.value === "") {
    throw new Error("no GitHub token: store one with `agentflow cred set github`");
  }
  token = cred.value;
  project = "pr-" + String(job.repo).toLowerCase().replace(/[^a-z0-9]+/g, "-") + "-" + String(job.number);
  const { total, copied } = checkout(job);
  const text = review(job, total, copied);
  const sha = String(job.head_sha || "").slice(0, 7);
  const r = github("POST", "/repos/" + job.repo + "/issues/" + job.number + "/comments", {
    json: { body: text + "\n\n<sub>Reviewed by an AgentFlow code agent at " + sha + ".</sub>" },
  });
  if (r.status !== 201) {
    throw new Error("posting the review: HTTP " + String(r.status));
  }
  log.info("reviewed " + job.repo + "#" + job.number);
}

function loop() {
  const msg = session.inbox();
  const job = msg.payload || {};
  if (job.repo && job.number) {
    try {
      runJob(job);
    } catch (err) {
      log.error("review of " + String(job.repo) + "#" + String(job.number) + " failed: " + String(err));
    }
  }
  token = null;
  session.exit();
}
