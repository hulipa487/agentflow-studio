// The offset-based surface and the editor sources, beyond what the Go tests cover.
import { CompletionContext } from '@codemirror/autocomplete'
import { EditorState } from '@codemirror/state'
import { describe, expect, it } from 'vitest'
import {
  analyze,
  children,
  completionsAt,
  hoverAt,
  lookup,
  luauSources,
  luauSupport,
  plainDoc,
  preludeEntries,
  stripSnippet,
  summarize,
} from './index'

const loop = `-- af-prelude-version: 1
function loop()
  local msg = session.inbox()
  memory.recall(msg.text)
  session.send(mesage)
end
`

describe('analyze', () => {
  it('returns offset ranges, 1-based lines and the Go rule ids', () => {
    const diags = analyze(loop, { preludeVersion: 1, capabilities: null })
    expect(diags).toHaveLength(1)
    const d = diags[0]
    expect(d.code).toBe('global.unknown')
    expect(d.severity).toBe('warning')
    expect(d.message).toBe('unknown global "mesage"')
    expect(loop.slice(d.from, d.to)).toBe('mesage')
    expect(d.line).toBe(5)
  })

  it('skips capability checks for null and flags every gated call for an empty list', () => {
    const code = (caps: string[] | null) =>
      analyze(loop, { preludeVersion: 1, capabilities: caps }).filter((d) => d.code === 'capability.missing')
    expect(code(null)).toEqual([])
    expect(code(['memory'])).toEqual([])
    const none = code([])
    expect(none).toHaveLength(1)
    expect(none[0].message).toBe('memory.recall requires capability "memory", which this agent does not enable')
    expect(loop.slice(none[0].from, none[0].to)).toBe('memory')
    expect(code(['llm.chat'])).toHaveLength(1)
  })

  it('compares the directive against the runtime version, defaulting to 1', () => {
    const src = '-- af-prelude-version: 2\nfunction loop()\nend\n'
    expect(analyze(src, { preludeVersion: 2, capabilities: null })).toEqual([])
    for (const version of [1, 0]) {
      const diags = analyze(src, { preludeVersion: version, capabilities: null })
      expect(diags.map((d) => d.message)).toEqual(['the chunk declares prelude version 2, but this runtime provides 1'])
      expect(diags[0]).toMatchObject({ from: 0, to: 1, line: 1, severity: 'error' })
    }
  })

  it('keeps ranges inside the document for chunk-level findings', () => {
    const src = '-- af-prelude-version: 1\nlocal x = 1\n'
    const diags = analyze(src, { preludeVersion: 1, capabilities: null })
    expect(diags.map((d) => d.code)).toEqual(['entry.missing-loop'])
    expect(diags[0]).toMatchObject({ from: src.length, to: src.length, line: 3 })
    expect(analyze('', { preludeVersion: 1, capabilities: null }).every((d) => d.from === 0 && d.to === 0)).toBe(true)
  })

  it('maps code-point columns to UTF-16 offsets', () => {
    const src = '-- af-prelude-version: 1\nfunction loop()\n  log.info("🙂🙂", mesage)\nend\n'
    const diags = analyze(src, { preludeVersion: 1, capabilities: null })
    expect(diags).toHaveLength(1)
    expect(src.slice(diags[0].from, diags[0].to)).toBe('mesage')
  })

  it('sorts by position and reports syntax errors with their ranges', () => {
    const src = '-- af-prelude-version: 1\nfunction loop()\n  local s = "open\n  local t = (1\nend\n'
    const diags = analyze(src, { preludeVersion: 1, capabilities: null })
    expect(diags.map((d) => [d.code, src.slice(d.from, d.to)])).toEqual([
      ['syntax.unclosed-string', '"open'],
      ['syntax.unclosed-bracket', '('],
    ])
  })
})

describe('completion and hover by offset', () => {
  const opts = { preludeVersion: 1, capabilities: null }

  it('lists a namespace after a dot and filters by the typed prefix', () => {
    const src = '-- af-prelude-version: 1\nfunction loop()\n  session.\nend\n'
    const at = src.indexOf('session.') + 'session.'.length
    const all = completionsAt(src, at, opts)
    expect(all.path).toBe('session')
    expect(all.items.map((it) => it.label)).toEqual(['exit', 'inbox', 'push', 'push_user', 'reply', 'send', 'send_delta', 'state'])
    expect(all.from).toBe(at)

    const typed = src.replace('session.', 'session.pu')
    const some = completionsAt(typed, at + 2, opts)
    expect(some.items.map((it) => it.label)).toEqual(['push', 'push_user'])
    expect(typed.slice(some.from, some.to)).toBe('pu')
    expect(some.items[0].detail).toBe('session.push(channel, reply_to, text, opts?)')
    expect(some.items[0].documentation).toBe('Send proactively to a channel. reply_to may be nil.\n\ncapability: channel.push')

    const nested = src.replace('session.', 'session.state.')
    expect(completionsAt(nested, at + 6, opts).items.map((it) => it.label)).toEqual(['delete', 'get', 'list', 'set'])
    const llm = src.replace('session.', 'llm.')
    expect(completionsAt(llm, at - 4, opts).items.map((it) => it.label)).toEqual(['chat', 'embed', 'forward', 'rerank', 'stream'])
  })

  it('offers locals in scope, prelude globals, the stdlib and chunk functions at the root', () => {
    const src = '-- af-prelude-version: 1\nlocal function helper() end\nfunction loop()\n  local msg = 1\n  \nend\n'
    const at = src.indexOf('  \n') + 2
    const labels = completionsAt(src, at, opts).items.map((it) => it.label)
    for (const name of ['msg', 'helper', 'loop', 'session', 'llm', 'pairs', 'string', 'token_budget']) {
      expect(labels).toContain(name)
    }
    expect(labels).toEqual([...labels].sort())
    // A local shadows the namespace of the same name.
    const shadow = '-- af-prelude-version: 1\nfunction loop()\n  local session = {}\n  session.\nend\n'
    expect(completionsAt(shadow, shadow.lastIndexOf('session.') + 8, opts).items).toEqual([])
  })

  it('describes prelude symbols, locals and stdlib names', () => {
    const at = loop.indexOf('recall')
    const h = hoverAt(loop, at, opts)
    expect(h?.markdown).toBe(
      '**memory.recall(query, opts?) -> [ { key, value, meta } ]**\n\nRecall memories. Returns a list, unlike store.query\'s { records }.\n\n*capability:* `memory`',
    )
    expect(h?.entry?.path).toBe('memory.recall')
    expect(loop.slice(h!.from, h!.to)).toBe('memory.recall')
    expect(plainDoc(h!.markdown)).toBe(
      "memory.recall(query, opts?) -> [ { key, value, meta } ] · Recall memories. Returns a list, unlike store.query's { records }. · capability: memory",
    )

    const local = hoverAt(loop, loop.indexOf('msg.text'), opts)
    expect(local?.markdown).toBe('**msg**\n\nlocal variable, declared at line 3.')
    expect(hoverAt(loop, loop.indexOf('function'), opts)).toBeNull()

    const stream = hoverAt('llm.stream', 5, opts)
    expect(stream?.markdown).toContain('Call `next_delta()` directly; a generic `for` fails across the bridge.')
    expect(hoverAt('user.list', 6, opts)?.markdown).toContain('*maintenance provenance only (system or scheduler turns)*')
    expect(hoverAt('pairs(x)', 2, opts)?.markdown).toBe('**pairs**\n\nLua 5.1 standard library.')
  })
})

describe('catalog', () => {
  it('carries the whole prelude', () => {
    expect(preludeEntries).toHaveLength(107)
    expect(new Set(preludeEntries.map((e) => e.path)).size).toBe(107)
    expect(children('').map((e) => e.path)).toEqual([
      'af', 'agent', 'credential', 'files', 'http', 'json', 'llm', 'log', 'mail', 'memory',
      'memory_recall_handler', 'memory_routing_table', 'memory_semantic_recall_handler', 'openai', 'os', 'runtime', 'scheduler',
      'session', 'shell', 'store', 'time', 'token_budget', 'tool', 'tools', 'user',
    ])
    expect(preludeEntries.filter((e) => e.maintenance).map((e) => e.path)).toEqual(['store.scopes', 'user.get', 'user.list'])
    expect(preludeEntries.filter((e) => e.yields)).toHaveLength(65)
    expect(preludeEntries.filter((e) => e.capability)).toHaveLength(54)
    expect(lookup('llm.stream')).toMatchObject({ capability: 'llm.chat', yields: true, kind: 'function' })
    // Every field hangs off a documented parent.
    for (const e of preludeEntries) {
      const dot = e.path.lastIndexOf('.')
      if (dot > 0) expect(lookup(e.path.slice(0, dot))?.kind).toBe('table')
    }
  })

  it('reduces snippets to their default text', () => {
    expect(stripSnippet('send(${1:text})')).toBe('send(text)')
    expect(stripSnippet('push(${1:channel}, ${2:reply_to}, ${3:text})')).toBe('push(channel, reply_to, text)')
    expect(stripSnippet('spawn({ ${1:provider = "ssh"} })')).toBe('spawn({ provider = "ssh" })')
    expect(stripSnippet(lookup('tool.def')!.insert!)).toBe(
      'def({\n\tname = "lua:tool",\n\tdescription = "...",\n\tparams = { type = "object", properties = {} },\n\thandler = function(args)\n\t\t\n\tend,\n})',
    )
    expect(stripSnippet('a$0b${2}c$')).toBe('abc$')
  })
})

describe('status line', () => {
  it('summarizes like the desktop editor', () => {
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
  const sources = luauSources({ preludeVersion: 1, capabilities: [] })
  const stateOf = (doc: string) => EditorState.create({ doc })
  const complete = (doc: string, explicit = false) => {
    const pos = doc.indexOf('|')
    const state = stateOf(doc.replace('|', ''))
    return sources.completions(new CompletionContext(state, pos, explicit))
  }

  it('builds an extension that a state accepts, and reuses it for equal options', () => {
    const ext = luauSupport({ preludeVersion: 1, capabilities: ['llm.chat'] })
    expect(luauSupport({ preludeVersion: 1, capabilities: ['llm.chat'] })).toBe(ext)
    expect(luauSupport({ preludeVersion: 1, capabilities: [] })).not.toBe(luauSupport({ preludeVersion: 1, capabilities: null }))
    expect(EditorState.create({ doc: loop, extensions: [ext] }).doc.toString()).toBe(loop)
  })

  it('lints the document', () => {
    const diags = sources.diagnostics(stateOf(loop))
    expect(diags.map((d) => [d.severity, d.message])).toEqual([
      ['warning', 'memory.recall requires capability "memory", which this agent does not enable'],
      ['warning', 'unknown global "mesage"'],
    ])
  })

  it('completes automatically after an identifier or a field access', () => {
    const fields = complete('function loop()\n  llm.|\nend')
    expect(fields?.options.map((o) => o.label)).toEqual(['chat', 'embed', 'forward', 'rerank', 'stream'])
    expect(fields?.options[0]).toMatchObject({
      type: 'function',
      detail: 'llm.chat(messages, opts?) -> { text, thinking, usage, tool_calls }',
      apply: 'chat(messages, opts)',
    })
    expect(fields?.filter).toBe(false)

    const typed = complete('function loop()\n  session.se|\nend')
    expect(typed?.options.map((o) => o.label)).toEqual(['send', 'send_delta'])
    expect((typed?.to ?? 0) - (typed?.from ?? 0)).toBe(2)

    expect(complete('function loop()\n  sess|\nend')?.options.map((o) => o.label)).toEqual(['session'])
    expect(complete('function loop()\n  session:|\nend')?.options).toHaveLength(8)
  })

  it('stays quiet where nothing was typed, and in comments and strings', () => {
    expect(complete('function loop()\n  |\nend')).toBeNull()
    expect(complete('local a = "x" ..|')).toBeNull()
    expect(complete('local a = f().|')).toBeNull()
    expect(complete('-- wait for the next|\n')).toBeNull()
    expect(complete('--[[ block\n next| ]]')).toBeNull()
    expect(complete('local s = "the next|"')).toBeNull()
    expect(complete('local s = "the next|')).toBeNull()
    expect(complete('local n = 12|')).toBeNull()
    expect(complete('local n = zzz|')).toBeNull()
    // Ctrl+Space always asks.
    // 25 prelude globals, 26 stdlib names and loop itself.
    expect(complete('function loop()\n  |\nend', true)?.options.length).toBe(52)
    expect(complete('-- wait for the next|\n', true)?.options.map((o) => o.label)).toEqual(['next'])
  })

  it('hovers the symbol under the pointer only', () => {
    const state = stateOf(loop)
    const at = loop.indexOf('inbox')
    expect(sources.hover(state, at, 1)?.entry?.path).toBe('session.inbox')
    expect(sources.hover(state, at + 5, -1)?.entry?.path).toBe('session.inbox')
    // Past the name's last character: the "(" is not part of the symbol.
    expect(sources.hover(state, at + 5, 1)).toBeNull()
    expect(sources.hover(state, 0, -1)).toBeNull()
  })
})
