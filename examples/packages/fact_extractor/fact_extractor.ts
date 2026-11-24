// af-prelude-version: 2
// Example package: fact_extractor — cheap after_turn tap that extracts facts.
//
// Import in a loop via `// af-import: fact_extractor` (with `packages:`
// pointing at this directory) and call `fact_extractor_turn(turn)` after the
// assistant reply is sent. Returns a list of memory records to write, or null
// if nothing to remember.
//
// Minimal heuristic: only records simple declarative sentences that look
// like facts ("I am ...", "My name is ...", "The capital of ..."). A
// production chain would run a small model here.

function looksLikeFact(text: string): boolean {
  const s = text || "";
  const patterns = [
    /^[iI] am /,
    /^[mM]y name is /,
    /^[tT]he capital of /,
    /^[oO]ur /,
    /^[tT]his is /,
  ];
  for (const p of patterns) {
    if (p.test(s)) return true;
  }
  return false;
}

function fact_extractor_turn(turn?: { id?: unknown; ts?: unknown; assistant?: string }) {
  const facts = [];
  if (turn && turn.assistant && looksLikeFact(turn.assistant)) {
    facts.push({
      type: "fact",
      key: "fact:" + String(turn.id || turn.ts || "unknown"),
      value: {
        text: turn.assistant,
        source: "after_turn",
      },
    });
  }
  if (facts.length === 0) return null;
  return facts;
}
