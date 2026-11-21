// Port of the desktop Studio's internal/lua/analysis_test.go.
import { describe, expect, it } from 'vitest'
import { analyzeSource, type AnalysisContext } from './analysis'
import { complete, hoverAtPosition } from './service'
import type { Diagnostic } from './token'

const goodLoop = `-- af-prelude-version: 1

function loop()
  local msg = session.inbox()
  local reply = llm.chat({
    { role = "user", content = msg.text },
  })
  session.send(reply.text)
end
`

const codes = (diags: Diagnostic[]) => diags.map((d) => d.code)
const hasCode = (diags: Diagnostic[], code: string) => diags.some((d) => d.code === code)
const analyze = (src: string, ctx: AnalysisContext) => analyzeSource(src, ctx).diagnostics

describe('analyzer', () => {
  it('accepts a clean loop', () => {
    expect(analyze(goodLoop, { preludeVersion: 1 })).toEqual([])
  })

  it('enforces the directive rules', () => {
    let diags = analyze('function loop()\nend\n', { preludeVersion: 1 })
    expect(codes(diags)).toContain('prelude.missing-directive')

    diags = analyze('-- af-prelude-version: 2\nfunction loop()\nend\n', { preludeVersion: 1 })
    expect(codes(diags)).toContain('prelude.version-mismatch')

    diags = analyze('-- af-prelude-version: 1\n-- af-prelude-version: 1\nfunction loop()\nend\n', { preludeVersion: 1 })
    expect(codes(diags)).toContain('prelude.duplicate-directive')
  })

  it('requires a global loop()', () => {
    let diags = analyze('-- af-prelude-version: 1\nlocal x = 1\n', { preludeVersion: 1 })
    expect(codes(diags)).toContain('entry.missing-loop')
    diags = analyze('-- af-prelude-version: 1\nlocal function loop()\nend\n', { preludeVersion: 1 })
    expect(codes(diags)).toContain('entry.local-loop')
  })

  it('reports unknown globals and fields', () => {
    const src = `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  log.info(mesage)
  local r = llm.chta({})
end
`
    const diags = analyze(src, { preludeVersion: 1 })
    expect(codes(diags)).toContain('global.unknown')
    expect(codes(diags)).toContain('field.unknown')
    expect(diags.some((d) => d.code === 'field.unknown' && d.message.includes('did you mean "chat"'))).toBe(true)
  })

  it('checks capabilities only when they are known', () => {
    const src = `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  memory.recall(msg.text)
end
`
    let diags = analyze(src, { preludeVersion: 1, capabilities: ['llm.chat'] })
    expect(codes(diags)).toContain('capability.missing')
    diags = analyze(src, { preludeVersion: 1, capabilities: null })
    expect(hasCode(diags, 'capability.missing')).toBe(false)
  })

  it('refuses llm.stream in a generic for', () => {
    const src = `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  for delta in llm.stream({{ role = "user", content = msg.text }}) do
    log.info(delta)
  end
end
`
    expect(codes(analyze(src, { preludeVersion: 1 }))).toContain('api.stream-for')
  })

  it('flags top-level yields and a return from loop()', () => {
    const src = `-- af-prelude-version: 1
local greeting = llm.chat({ { role = "user", content = "hi" } })
function loop()
  return
end
`
    const diags = analyze(src, { preludeVersion: 1 })
    expect(codes(diags)).toContain('api.top-level-yield')
    expect(codes(diags)).toContain('api.return-loop')
  })

  it('reports unclosed blocks and brackets', () => {
    let diags = analyze('-- af-prelude-version: 1\nfunction loop()\n  if true then\nend\n', { preludeVersion: 1 })
    expect(codes(diags)).toContain('syntax.unclosed-block')
    diags = analyze('-- af-prelude-version: 1\nfunction loop()\n  local x = (1\nend\n', { preludeVersion: 1 })
    expect(codes(diags)).toContain('syntax.unclosed-bracket')
  })

  it('flags Luau-only type syntax without spamming unknown globals and accepts Lua 5.1 type()', () => {
    const src = `-- af-prelude-version: 1
type Reply = { text: string }

local function format(value: string, count: number?): string
  local label: string = value
  return label
end

function loop()
  local msg: Message = session.inbox()
  session.send(format(msg.text, 1))
end
`
    const diags = analyze(src, { preludeVersion: 1 })
    expect(hasCode(diags, 'global.unknown')).toBe(false)
    expect(codes(diags).filter((c) => c === 'syntax.luau-only').length).toBeGreaterThanOrEqual(5)

    const validLua = `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  if type(msg.text) == "string" then
    session.send(msg.text)
  end
end
`
    expect(analyze(validLua, { preludeVersion: 1 })).toEqual([])
  })

  it('flags Luau-only constructs (continue, compound ops, interpolated strings) and removed globals', () => {
    const src = `-- af-prelude-version: 1
function loop()
  local n = 1
  n += 2
  local d = 10 // 3
  local s = \`count: {n}\`
  collectgarbage("count")
  while true do
    continue
  end
end
`
    const diags = analyze(src, { preludeVersion: 1 })
    expect(codes(diags).filter((c) => c === 'syntax.luau-only')).toHaveLength(4)
    expect(codes(diags)).toContain('sandbox.unavailable')
  })

  it('completes and hovers', () => {
    const res = analyzeSource(goodLoop, { preludeVersion: 1 })
    const items = complete(res.model, 5, 4)
    expect(items.length).toBeGreaterThan(0)
    expect(items.some((it) => it.label.includes('llm'))).toBe(true)

    const hover = hoverAtPosition(res.model, 4, 20) // inside llm.chat
    expect(hover).not.toBeNull()
    expect(hover?.markdown).toContain('llm.chat')

    const fieldItems = complete(res.model, 4, 14)
    expect(fieldItems.length).toBeGreaterThan(0)
  })

  it('is lenient about globals when the chunk imports a package', () => {
    let src = `-- af-prelude-version: 1
-- af-import: greeting
function loop()
  session.send(greeting("hi"))
end
`
    expect(hasCode(analyze(src, { preludeVersion: 1 }), 'global.unknown')).toBe(false)
    // The same call without the import is still a typo candidate.
    src = `-- af-prelude-version: 1
function loop()
  session.send(greeting("hi"))
end
`
    expect(hasCode(analyze(src, { preludeVersion: 1 }), 'global.unknown')).toBe(true)
  })
})
