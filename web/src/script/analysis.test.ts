import { describe, expect, it } from 'vitest'
import { analyzeSource, type AnalysisContext } from './analysis'
import { complete, hoverAtPosition } from './service'
import type { Diagnostic } from './token'

const goodLoop = `// af-prelude-version: 2

function loop() {
  const msg = session.inbox();
  const reply = llm.chat([
    { role: "user", content: msg.text },
  ]);
  session.send(reply.text);
}
`

const codes = (diags: Diagnostic[]) => diags.map((d) => d.code)
const hasCode = (diags: Diagnostic[], code: string) => diags.some((d) => d.code === code)
const analyze = (src: string, ctx: AnalysisContext) => analyzeSource(src, ctx).diagnostics

describe('analyzer', () => {
  it('accepts a clean TypeScript/JS loop', () => {
    expect(analyze(goodLoop, { preludeVersion: 2 })).toEqual([])
  })

  it('enforces the directive rules', () => {
    let diags = analyze('function loop() {}\n', { preludeVersion: 2 })
    expect(codes(diags)).toContain('prelude.missing-directive')

    diags = analyze('// af-prelude-version: 1\nfunction loop() {}\n', { preludeVersion: 2 })
    expect(codes(diags)).toContain('prelude.version-mismatch')

    diags = analyze('// af-prelude-version: 2\n// af-prelude-version: 2\nfunction loop() {}\n', { preludeVersion: 2 })
    expect(codes(diags)).toContain('prelude.duplicate-directive')
  })

  it('requires a top-level loop()', () => {
    let diags = analyze('// af-prelude-version: 2\nconst x = 1;\n', { preludeVersion: 2 })
    expect(codes(diags)).toContain('entry.missing-loop')

    diags = analyze('// af-prelude-version: 2\nfunction outer() { function loop() {} }\n', { preludeVersion: 2 })
    expect(codes(diags)).toContain('entry.local-loop')
  })

  it('reports unknown globals and fields', () => {
    const src = `// af-prelude-version: 2
function loop() {
  const msg = session.inbox();
  log.info(mesage);
  const r = llm.chta({});
}
`
    const diags = analyze(src, { preludeVersion: 2 })
    expect(codes(diags)).toContain('global.unknown')
    expect(codes(diags)).toContain('field.unknown')
    expect(diags.some((d) => d.code === 'field.unknown' && d.message.includes('did you mean "chat"'))).toBe(true)
  })

  it('checks capabilities only when known, including external api calls', () => {
    const src = `// af-prelude-version: 2
function loop() {
  const msg = session.inbox();
  memory.recall(msg.text);
  api.weather.current({ city: "NYC" });
}
`
    let diags = analyze(src, { preludeVersion: 2, capabilities: ['llm.chat'] })
    expect(diags.filter((d) => d.code === 'capability.missing').map((d) => d.capability)).toEqual(['memory', 'apis'])

    diags = analyze(src, { preludeVersion: 2, capabilities: ['memory', 'apis'] })
    expect(hasCode(diags, 'capability.missing')).toBe(false)

    diags = analyze(src, { preludeVersion: 2, capabilities: null })
    expect(hasCode(diags, 'capability.missing')).toBe(false)
  })

  it('flags top-level yields and a return from loop()', () => {
    const src = `// af-prelude-version: 2
const greeting = llm.chat([{ role: "user", content: "hi" }]);
function loop() {
  if (true) {
    return;
  }
}
`
    const diags = analyze(src, { preludeVersion: 2 })
    expect(codes(diags)).toContain('api.top-level-yield')
    expect(codes(diags)).toContain('api.return-loop')
  })

  it('accepts TypeScript annotations, arrow callbacks, and regex literals cleanly', () => {
    const src = `// af-prelude-version: 2
type Reply = { text: string };
interface Pair { a: number; b: number; }

function format(value: string, count: number): { label: string } {
  const cleaned = value.replace(/sk-[A-Za-z0-9]+/g, "[REDACTED]");
  const label: string = cleaned;
  const arr = [label, String(count)] as string[];
  const out: string[] = [];
  for (const part of arr) {
    out.push(part.trim());
  }
  return { label: out.join(":") };
}

function loop() {
  const msg = session.inbox();
  session.send(format(String(msg.text || ""), 1).label);
}
`
    expect(analyze(src, { preludeVersion: 2 })).toEqual([])
  })

  it('reports unclosed brackets and sandbox-unavailable globals', () => {
    let diags = analyze('// af-prelude-version: 2\nfunction loop() {\n  const x = (1;\n}\n', { preludeVersion: 2 })
    expect(codes(diags)).toContain('syntax.unclosed-bracket')

    diags = analyze('// af-prelude-version: 2\nfunction loop() {\n  const fsMod = require("fs");\n  fetch("https://example.com");\n}\n', {
      preludeVersion: 2,
    })
    expect(codes(diags).filter((c) => c === 'sandbox.unavailable')).toHaveLength(2)
  })

  it('completes and hovers', () => {
    const res = analyzeSource(goodLoop, { preludeVersion: 2 })
    const items = complete(res.model, 5, 4)
    expect(items.length).toBeGreaterThan(0)
    expect(items.some((it) => it.label.includes('llm'))).toBe(true)

    const hover = hoverAtPosition(res.model, 4, 20)
    expect(hover).not.toBeNull()
    expect(hover?.markdown).toContain('llm.chat')

    const fieldItems = complete(res.model, 4, 18)
    expect(fieldItems.length).toBeGreaterThan(0)
  })

  it('is lenient about globals when importing a package', () => {
    let src = `// af-prelude-version: 2
// af-import: greeting
function loop() {
  session.send(Greeting.hi("Ada"));
}
`
    expect(hasCode(analyze(src, { preludeVersion: 2 }), 'global.unknown')).toBe(false)

    src = `// af-prelude-version: 2
function loop() {
  session.send(Greeting.hi("Ada"));
}
`
    expect(hasCode(analyze(src, { preludeVersion: 2 }), 'global.unknown')).toBe(true)
  })

  it('skips entry-point loop() checks when requireLoop is false', () => {
    const chunk = `// af-prelude-version: 2
var M = {
  greet() {
    return "ok";
  },
};
`
    expect(codes(analyze(chunk, { preludeVersion: 2 }))).toContain('entry.missing-loop')
    expect(analyze(chunk, { preludeVersion: 2, requireLoop: false })).toEqual([])

    const missingDir = 'const x = 1;\n'
    expect(codes(analyze(missingDir, { preludeVersion: 2, requireLoop: false }))).toEqual(['prelude.missing-directive'])

    const badSyntax = '// af-prelude-version: 2\nconst x = (\n'
    expect(codes(analyze(badSyntax, { preludeVersion: 2, requireLoop: false }))).toContain('syntax.unclosed-bracket')
  })
})
