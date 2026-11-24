// Ports of the desktop builder's tests (builder_checks_test.go,
// builder_banner_test.go) plus the document the form renders.
import { describe, expect, it } from 'vitest'
import type { AgentDetail, Catalog, Template } from '@/api/types'
import {
  MEMORY_INLINE,
  memoryOptions,
  NAME_INVALID,
  NAME_REQUIRED,
  atoi,
  buildYAML,
  checkLabels,
  effectiveCapabilities,
  goJSON,
  headerStatus,
  initialForm,
  lf,
  loopStatus,
  matchingTemplateSettings,
  nameError,
  parseExtrasObject,
  toggleCheck,
  updateExtrasSetting,
} from './model'

const catalog: Catalog = {
  models: ['default', 'fast'],
  tools: ['builtin:fetch', 'builtin:web_search'],
  capabilities: ['llm.chat', 'memory', 'tools'],
  default_capabilities: ['llm.chat', 'tools'],
  safety_profiles: ['default', 'none', 'strict'],
  memory_profiles: [],
  prompt_keys: [],
  prelude_version: 2,
  templates: [],
}

const echo: Template = { name: 'echo', title: 'Echo', description: '', loop: '// af-prelude-version: 2\nfunction loop() {}\n' }

function detailOf(agent: Partial<AgentDetail['agent']>, over: Partial<AgentDetail> = {}): AgentDetail {
  return {
    ok: true,
    name: 'a',
    source: 'config',
    agent_yaml: '',
    agent: {
      loop: '',
      model: '',
      history_budget: 0,
      safety: '',
      shell: '',
      skills: null,
      capabilities: null,
      can_contact: null,
      persistent: false,
      credentials: null,
      inherit_user: null,
      extras: null,
      ...agent,
    },
    loop: '',
    instructions: '',
    files: { mode: 'file', profile_path: '', loop_path: '', instructions_path: '', managed: true, mtime: 0 },
    persisted: false,
    spawn: false,
    persistent: false,
    model: '',
    capabilities: null,
    skills: null,
    sessions: 0,
    ...over,
  }
}

describe('check grids', () => {
  it('selects, toggles and clears in grid order', () => {
    const labels = checkLabels(['llm.chat', 'memory', 'tools'], ['memory'])
    let selected = labels.filter((l) => ['memory'].includes(l))
    expect(selected).toEqual(['memory'])
    selected = toggleCheck(labels, selected, 'tools', true)
    selected = toggleCheck(labels, selected, 'llm.chat', true)
    expect(selected).toEqual(['llm.chat', 'memory', 'tools'])
    selected = toggleCheck(labels, selected, 'memory', false)
    expect(selected).toEqual(['llm.chat', 'tools'])
  })

  it('keeps entries the catalog does not list', () => {
    expect(checkLabels(['a', 'b'], ['b', 'x'])).toEqual(['a', 'b', 'x'])
    expect(checkLabels(null, null)).toEqual([])
  })
})

describe('name validation', () => {
  it('reports the messages validate and apply show', () => {
    expect(nameError('')).toBe(NAME_REQUIRED)
    expect(nameError('   ')).toBe(NAME_REQUIRED)
    expect(nameError('has space')).toBe(NAME_INVALID)
    expect(nameError('x'.repeat(65))).toBe(NAME_INVALID)
    expect(nameError(' researcher_2-b ')).toBe('')
  })
})

describe('initialForm', () => {
  it('starts a blank agent on the runtime defaults', () => {
    const f = initialForm('', null, null, catalog)
    expect(f).toMatchObject({ name: '', model: '', safety: 'default', history: '6000', persistent: false, capabilities: [], skills: [], extras: '' })
  })

  it('seeds a template with the default capabilities', () => {
    const f = initialForm('', null, echo, catalog)
    expect(f.capabilities).toEqual(['llm.chat', 'tools'])
    expect(f.safety).toBe('default')
  })

  it('loads a definition, defaulting an unset safety and budget', () => {
    const f = initialForm('a', detailOf({ model: 'fast', capabilities: ['tools', 'llm.chat'], skills: ['builtin:fetch'], can_contact: ['pm', 'qa'], extras: { b: 1, a: { d: 1, c: 2 } } }), null, catalog)
    expect(f.model).toBe('fast')
    expect(f.safety).toBe('default')
    expect(f.history).toBe('6000')
    expect(f.capabilities).toEqual(['llm.chat', 'tools'])
    expect(f.canContact).toBe('pm, qa')
    expect(f.extras).toBe('{\n  "a": {\n    "c": 2,\n    "d": 1\n  },\n  "b": 1\n}')
  })

  it('leaves safety unselected when the deployment has no default profile', () => {
    expect(initialForm('', null, null, { ...catalog, safety_profiles: ['strict'] }).safety).toBe('')
    expect(initialForm('', null, null, { ...catalog, safety_profiles: [] }).safety).toBe('default')
  })
})

describe('buildYAML', () => {
  it('renders a new agent like the desktop builder', () => {
    expect(buildYAML(initialForm('', null, null, catalog), null)).toBe('loop: "./agent.ts"\nsafety: "default"\nhistory_budget: 6000\n')
    const f = { ...initialForm('', null, echo, catalog), name: ' echo2 ' }
    expect(buildYAML(f, null)).toBe('loop: "./echo2.ts"\nsafety: "default"\nhistory_budget: 6000\ncapabilities: ["llm.chat","tools"]\n')
  })

  it('writes every key in order and omits the empty ones', () => {
    const f = {
      ...initialForm('a', null, null, catalog),
      model: 'fast',
      safety: 'strict',
      history: ' 4000 ',
      persistent: true,
      capabilities: ['llm.chat'],
      skills: ['builtin:fetch', 'builtin:web_search'],
      canContact: 'pm, researcher\nqa,,',
      credentials: 'openai',
      shell: ' sandbox ',
      extras: '{"z": 1, "a": "<b>&"}',
    }
    expect(buildYAML(f, null)).toBe(
      'loop: "./a.ts"\n' +
        'model: "fast"\n' +
        'safety: "strict"\n' +
        'history_budget: 4000\n' +
        'persistent: true\n' +
        'capabilities: ["llm.chat"]\n' +
        'skills: ["builtin:fetch","builtin:web_search"]\n' +
        'can_contact: ["pm","researcher","qa"]\n' +
        'credentials: ["openai"]\n' +
        'shell: "sandbox"\n' +
        'extras: {"a":"\\u003cb\\u003e\\u0026","z":1}\n',
    )
  })

  it('keeps the loop path of a loaded definition', () => {
    const d = detailOf({ loop: '/srv/loops/a.ts' })
    expect(buildYAML(initialForm('a', d, null, catalog), d)).toBe('loop: "/srv/loops/a.ts"\nsafety: "default"\nhistory_budget: 6000\n')
  })

  it('leaves out a history budget that is not a positive integer', () => {
    for (const history of ['', '0', '-5', '6k', '1.5']) {
      expect(buildYAML({ ...initialForm('a', null, null, catalog), history }, null)).toBe('loop: "./a.ts"\nsafety: "default"\n')
    }
  })

  it('writes the memory profile after the shell, and keeps an inline one as it is', () => {
    const f = { ...initialForm('a', null, null, catalog), shell: 'sandbox', ssh: 'prod-box', memory: 'group_chat' }
    expect(buildYAML(f, null)).toBe('loop: "./a.ts"\nsafety: "default"\nhistory_budget: 6000\nshell: "sandbox"\nssh: "prod-box"\nmemory: "group_chat"\n')
    const sshDetail = detailOf({ ssh: 'prod-box' })
    expect(initialForm('a', sshDetail, null, catalog).ssh).toBe('prod-box')
    const inline = { stores: { facts: { backend: 'main_db', table: 'notes' } }, recall: 'plugin:recency' }
    const d = detailOf({ memory_inline: inline })
    const g = initialForm('a', d, null, catalog)
    expect(g.memory).toBe(MEMORY_INLINE)
    expect(buildYAML(g, d)).toBe(
      'loop: "./a.ts"\nsafety: "default"\nhistory_budget: 6000\nmemory: {"recall":"plugin:recency","stores":{"facts":{"backend":"main_db","table":"notes"}}}\n',
    )
    expect(buildYAML({ ...g, memory: '' }, d)).toBe('loop: "./a.ts"\nsafety: "default"\nhistory_budget: 6000\n')
  })

  it('rejects extras that are not JSON', () => {
    expect(() => buildYAML({ ...initialForm('a', null, null, catalog), extras: '{nope' }, null)).toThrow(/^extras is not valid JSON: /)
  })
})

describe('helpers', () => {
  it('encodes like encoding/json', () => {
    expect(goJSON('a\u2028b')).toBe('"a\\u2028b"')
    expect(goJSON({ b: [{ d: 1, c: null }], a: true })).toBe('{"a":true,"b":[{"c":null,"d":1}]}')
    expect(goJSON('C:\\loops\\a.ts')).toBe('"C:\\\\loops\\\\a.ts"')
  })

  it('parses integers like strconv.Atoi', () => {
    expect(atoi('42')).toBe(42)
    expect(atoi('+7')).toBe(7)
    expect(atoi('-3')).toBe(-3)
    expect(atoi('4 2')).toBeNull()
    expect(atoi('1e3')).toBeNull()
    expect(atoi('')).toBeNull()
  })

  it('falls back to the deployment defaults for capability checks', () => {
    expect(effectiveCapabilities(['memory'], catalog)).toEqual(['memory'])
    expect(effectiveCapabilities([], catalog)).toEqual(['llm.chat', 'tools'])
    expect(effectiveCapabilities([], { ...catalog, default_capabilities: null })).toEqual([])
  })

  it('names the header state', () => {
    expect(headerStatus(true, null).text).toBe('Unsaved changes')
    expect(headerStatus(false, null)).toEqual({ text: 'Not applied', tone: 'warning' })
    expect(headerStatus(false, detailOf({}, { persisted: true }))).toEqual({ text: 'Saved', tone: 'success' })
    expect(headerStatus(false, detailOf({}))).toEqual({ text: 'Live', tone: 'accent' })
  })

  it('summarises the loop diagnostics', () => {
    const d = (severity: 'error' | 'warning' | 'info') => ({ from: 0, to: 1, line: 1, severity, message: 'm' })
    expect(loopStatus([], false, '')).toEqual({ text: 'no issues', tone: 'success' })
    expect(loopStatus([d('info')], false, '')).toEqual({ text: 'no issues', tone: 'success' })
    expect(loopStatus([d('warning')], false, '')).toEqual({ text: '1 warning', tone: 'warning' })
    expect(loopStatus([d('error'), d('error'), d('warning')], false, '')).toEqual({ text: '2 errors · 1 warning', tone: 'danger' })
    expect(loopStatus([d('error')], true, '')).toEqual({ text: 'checking…', tone: 'neutral' })
    expect(loopStatus([], false, 'boom')).toEqual({ text: 'language service error: boom', tone: 'danger' })
  })

  it('normalises line endings for the editors', () => {
    expect(lf('a\r\nb\rc\n')).toBe('a\nb\nc\n')
    expect(lf(undefined)).toBe('')
  })
})

describe('memoryOptions', () => {
  it('offers none, the built-in profile and the named ones', () => {
    const c = { ...catalog, memory_profiles: ['notes', 'conversational'] }
    expect(memoryOptions(c, '').map((o) => o.value)).toEqual(['', 'conversational', 'notes'])
  })

  it('keeps a profile the catalog does not list, and an inline one', () => {
    expect(memoryOptions(catalog, 'gone').at(-1)).toEqual({ value: 'gone', label: 'gone · not in catalog' })
    expect(memoryOptions(catalog, MEMORY_INLINE).at(-1)?.value).toBe(MEMORY_INLINE)
  })
})

describe('starter settings schema and template seeding', () => {
  const tgTpl: Template = {
    name: 'telegram-group-bot',
    title: 'Telegram group bot',
    description: 'Group bot starter',
    capabilities: ['llm.chat', 'memory', 'session.state'],
    memory: 'conversational',
    instructions: 'Answer mentions politely.',
    extras: { respond_on_mention: true, cooldown_sec: 15, trigger_phrase: '' },
    settings: [
      { key: 'respond_on_mention', label: 'Reply on mention', type: 'bool', default: true },
      { key: 'cooldown_sec', label: 'Cooldown', type: 'number', default: 15, min: 0, max: 3600 },
      { key: 'trigger_phrase', label: 'Trigger phrase', type: 'string', default: '' },
    ],
    loop: '// af-prelude-version: 2\nfunction loop() {}\n',
  }

  it('seeds capabilities, memory, and extras from a template', () => {
    const form = initialForm('', null, tgTpl, { ...catalog, templates: [tgTpl] })
    expect(form.capabilities).toEqual(['llm.chat', 'memory', 'session.state'])
    expect(form.memory).toBe('conversational')
    const parsed = parseExtrasObject(form.extras)
    expect(parsed).toEqual({ cooldown_sec: 15, respond_on_mention: true, trigger_phrase: '' })
  })

  it('matches starter settings and syncs changes back into extras JSON', () => {
    const c = { ...catalog, templates: [tgTpl] }
    const form = initialForm('', null, tgTpl, c)
    const settings = matchingTemplateSettings(c, parseExtrasObject(form.extras))
    expect(settings.map((s) => s.key)).toEqual(['respond_on_mention', 'cooldown_sec', 'trigger_phrase'])

    const nextExtras = updateExtrasSetting(form.extras, 'cooldown_sec', 30)
    expect(parseExtrasObject(nextExtras)).toEqual({
      cooldown_sec: 30,
      respond_on_mention: true,
      trigger_phrase: '',
    })
  })

  it('picks the best-matching starter template when presets share common setting keys', () => {
    const qqTpl: Template = {
      name: 'qq-group-bot',
      title: 'QQ group bot',
      description: 'QQ group bot starter',
      capabilities: ['llm.chat', 'memory', 'session.state'],
      memory: 'conversational',
      extras: { respond_on_mention: true, respond_in_c2c: true, respond_in_guild: false, cooldown_sec: 15 },
      settings: [
        { key: 'respond_on_mention', label: 'Reply on mention', type: 'bool', default: true },
        { key: 'respond_in_c2c', label: 'Reply in C2C', type: 'bool', default: true },
        { key: 'respond_in_guild', label: 'Reply in Guild', type: 'bool', default: false },
        { key: 'cooldown_sec', label: 'Cooldown', type: 'number', default: 15 },
      ],
      loop: '// af-prelude-version: 2\nfunction loop() {}\n',
    }
    const both = { ...catalog, templates: [tgTpl, qqTpl] }
    const qqForm = initialForm('', null, qqTpl, both)
    expect(matchingTemplateSettings(both, parseExtrasObject(qqForm.extras)).map((s) => s.key)).toEqual([
      'respond_on_mention',
      'respond_in_c2c',
      'respond_in_guild',
      'cooldown_sec',
    ])
    const tgForm = initialForm('', null, tgTpl, both)
    expect(matchingTemplateSettings(both, parseExtrasObject(tgForm.extras)).map((s) => s.key)).toEqual([
      'respond_on_mention',
      'cooldown_sec',
      'trigger_phrase',
    ])
  })
})
