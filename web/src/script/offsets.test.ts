import { CompletionContext } from '@codemirror/autocomplete'
import { EditorState } from '@codemirror/state'
import { describe, expect, it } from 'vitest'
import {
  analyze,
  children,
  completionsAt,
  hoverAt,
  lookup,
  plainDoc,
  preludeEntries,
  scriptSources,
  scriptSupport,
  stripSnippet,
  summarize,
} from './index'

const loop = `// af-prelude-version: 2
function loop() {
  const msg = session.inbox();
  memory.recall(msg.text);
  session.send(mesage);
}
`

describe('analyze', () => {
  it('returns offset ranges and 1-based lines', () => {
    const diags = analyze(loop, { preludeVersion: 2, capabilities: null })
    expect(diags).toHaveLength(1)
    const d = diags[0]
    expect(d.code).toBe('global.unknown')
    expect(d.severity).toBe('warning')
    expect(loop.slice(d.from, d.to)).toBe('mesage')
    expect(d.line).toBe(5)
  })

  it('skips capability checks for null and flags every gated call for empty capabilities', () => {
    const capsDiags = (caps: string[] | null) =>
      analyze(loop, { preludeVersion: 2, capabilities: caps }).filter((d) => d.code === 'capability.missing')
    expect(capsDiags(null)).toEqual([])
    expect(capsDiags(['memory'])).toEqual([])
    const none = capsDiags([])
    expect(none).toHaveLength(1)
    expect(none[0].capability).toBe('memory')
    expect(loop.slice(none[0].from, none[0].to)).toBe('memory')
    expect(capsDiags(['llm.chat'])).toHaveLength(1)
  })

  it('compares the directive against the runtime version, defaulting to 2', () => {
    const src = '// af-prelude-version: 3\nfunction loop() {}\n'
    expect(analyze(src, { preludeVersion: 3, capabilities: null })).toEqual([])
    for (const version of [2, 0]) {
      const diags = analyze(src, { preludeVersion: version, capabilities: null })
      expect(diags.map((d) => d.message)).toEqual(['the chunk declares prelude version 3, but this runtime provides 2'])
      expect(diags[0]).toMatchObject({ from: 0, to: 24, line: 1, severity: 'error' })
    }
  })

  it('maps code-point columns to UTF-16 offsets', () => {
    const src = '// af-prelude-version: 2\nfunction loop() {\n  log.info("🙂🙂", mesage);\n}\n'
    const diags = analyze(src, { preludeVersion: 2, capabilities: null })
    expect(diags).toHaveLength(1)
    expect(src.slice(diags[0].from, diags[0].to)).toBe('mesage')
  })

  it('sorts by position and reports syntax errors with ranges', () => {
    const src = '// af-prelude-version: 2\nfunction loop() {\n  const s = "open\n  const t = (1;\n}\n'
    const diags = analyze(src, { preludeVersion: 2, capabilities: null })
    expect(diags.map((d) => [d.code, src.slice(d.from, d.to)])).toEqual([
      ['syntax.unclosed-string', '"open'],
      ['syntax.unclosed-bracket', '('],
    ])
  })
})

describe('completion and hover by offset', () => {
  const opts = { preludeVersion: 2, capabilities: null }

  it('lists a namespace after a dot and filters by prefix', () => {
    const src = '// af-prelude-version: 2\nfunction loop() {\n  session.\n}\n'
    const at = src.indexOf('session.') + 'session.'.length
    const all = completionsAt(src, at, opts)
    expect(all.path).toBe('session')
    expect(all.items.map((it) => it.label)).toEqual(['exit', 'inbox', 'push', 'push_user', 'reply', 'send', 'send_delta', 'skip', 'state'])
    expect(all.from).toBe(at)

    const typed = src.replace('session.', 'session.pu')
    const some = completionsAt(typed, at + 2, opts)
    expect(some.items.map((it) => it.label)).toEqual(['push', 'push_user'])
    expect(typed.slice(some.from, some.to)).toBe('pu')

    const nested = src.replace('session.', 'session.state.')
    expect(completionsAt(nested, at + 6, opts).items.map((it) => it.label)).toEqual(['delete', 'get', 'list', 'set'])
    const llm = src.replace('session.', 'llm.')
    expect(completionsAt(llm, at - 4, opts).items.map((it) => it.label)).toEqual(['chat', 'embed', 'forward', 'rerank', 'stream'])
    const api = src.replace('session.', 'api.')
    expect(completionsAt(api, at - 4, opts).items.map((it) => it.label)).toEqual(['batch', 'call', 'list'])
  })

  it('offers locals in scope, prelude globals, stdlib and chunk functions at root', () => {
    const src = '// af-prelude-version: 2\nfunction helper() {}\nfunction loop() {\n  const msg = 1;\n  \n}\n'
    const at = src.indexOf('  \n') + 2
    const labels = completionsAt(src, at, opts).items.map((it) => it.label)
    for (const name of ['msg', 'helper', 'loop', 'session', 'llm', 'api', 'JSON', 'Math', 'token_budget']) {
      expect(labels).toContain(name)
    }
    expect(labels).toEqual([...labels].sort())

    const shadow = '// af-prelude-version: 2\nfunction loop() {\n  const session = {};\n  session.\n}\n'
    expect(completionsAt(shadow, shadow.lastIndexOf('session.') + 8, opts).items).toEqual([])
  })

  it('describes prelude symbols, locals and stdlib names', () => {
    const at = loop.indexOf('recall')
    const h = hoverAt(loop, at, opts)
    expect(h?.markdown).toContain('memory.recall(query, opts?)')
    expect(h?.entry?.path).toBe('memory.recall')
    expect(loop.slice(h!.from, h!.to)).toBe('memory.recall')
    expect(plainDoc(h!.markdown)).toContain('capability: memory')

    const local = hoverAt(loop, loop.indexOf('msg.text'), opts)
    expect(local?.markdown).toBe('**msg**\n\nlocal variable, declared at line 3.')
    expect(hoverAt(loop, loop.indexOf('function'), opts)).toBeNull()

    expect(hoverAt('user.list', 6, opts)?.markdown).toContain('*maintenance provenance only (system or scheduler turns)*')
    expect(hoverAt('JSON.stringify', 2, opts)?.markdown).toBe('**JSON**\n\nECMAScript / Goja standard library.')
  })
})

describe('catalog', () => {
  it('carries the whole Prelude v2 surface including api', () => {
    expect(preludeEntries.length).toBeGreaterThanOrEqual(110)
    expect(new Set(preludeEntries.map((e) => e.path)).size).toBe(preludeEntries.length)
    expect(children('').map((e) => e.path)).toContain('api')
    expect(lookup('api.call')).toMatchObject({ capability: 'apis', yields: true, kind: 'function' })
    expect(lookup('llm.stream')).toMatchObject({ capability: 'llm.chat', yields: true, kind: 'function' })
    for (const e of preludeEntries) {
      const dot = e.path.lastIndexOf('.')
      if (dot > 0) expect(lookup(e.path.slice(0, dot))?.kind).toBe('table')
    }
  })

  it('reduces snippets to default text', () => {
    expect(stripSnippet('send(${1:text})')).toBe('send(text)')
    expect(stripSnippet('call(${1:pkg}, ${2:action}, ${3:{}})')).toBe('call(pkg, action, {})')
    expect(stripSnippet('a$0b${2}c$')).toBe('abc$')
  })
})

describe('status line', () => {
  it('summarizes errors and warnings', () => {
    expect(summarize([])).toMatchObject({ text: 'no issues', tone: 'success' })
    expect(summarize([{ severity: 'warning' }])).toMatchObject({ text: '1 warning', tone: 'warning' })
    expect(summarize([{ severity: 'error' }, { severity: 'error' }, { severity: 'warning' }])).toMatchObject({
      text: '2 errors · 1 warning',
      tone: 'danger',
      errors: 2,
      warnings: 1,
    })
  })
})

describe('editor sources', () => {
  const sources = scriptSources({ preludeVersion: 2, capabilities: [] })
  const stateOf = (doc: string) => EditorState.create({ doc })
  const complete = (doc: string, explicit = false) => {
    const pos = doc.indexOf('|')
    const state = stateOf(doc.replace('|', ''))
    return sources.completions(new CompletionContext(state, pos, explicit))
  }

  it('builds and caches extensions', () => {
    const ext = scriptSupport({ preludeVersion: 2, capabilities: ['llm.chat'] })
    expect(scriptSupport({ preludeVersion: 2, capabilities: ['llm.chat'] })).toBe(ext)
    expect(scriptSupport({ preludeVersion: 2, capabilities: [] })).not.toBe(scriptSupport({ preludeVersion: 2, capabilities: null }))
    expect(EditorState.create({ doc: loop, extensions: [ext] }).doc.toString()).toBe(loop)
  })

  it('completes after dot and stays quiet in comments and strings', () => {
    const fields = complete('function loop() {\n  llm.|\n}')
    expect(fields?.options.map((o) => o.label)).toEqual(['chat', 'embed', 'forward', 'rerank', 'stream'])
    expect(fields?.filter).toBe(false)

    expect(complete('function loop() {\n  |\n}')).toBeNull()
    expect(complete('// comment|\n')).toBeNull()
    expect(complete('/* block\n next| */')).toBeNull()
    expect(complete('const s = "the next|"')).toBeNull()
    expect(complete('function loop() {\n  |\n}', true)?.options.length).toBeGreaterThan(40)
  })
})
