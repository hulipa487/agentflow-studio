// The builder's form model: what the Config tab edits, how it is seeded from
// a loaded definition or a template, and how it becomes the agent document
// sent to the runtime. A port of the desktop Studio's builder.go; buildYAML
// emits the same bytes the Go builder does for the same form.
import type { AgentDetail, AgentJSON, Catalog, Template } from '@/api/types'
import { splitList } from '@/lib/format'
import type { LuauDiagnostic } from '@/lua'
import type { Tone } from '@/state/ui'

export const AGENT_NAME_RE = /^[A-Za-z0-9_-]{1,64}$/

/** The model picker's first entry: the definition names no model. */
export const DEFAULT_MODEL_OPTION = 'default (per call)'

export const NAME_REQUIRED = 'An agent name is required.'
export const NAME_INVALID = 'The name must be 1-64 characters of letters, digits, _ or -.'

export interface BuilderForm {
  name: string
  /** '' is "default (per call)". */
  model: string
  /** '' means nothing is selected; the key is then left out. */
  safety: string
  history: string
  persistent: boolean
  /** Checked entries, kept in the order the grid shows them. */
  capabilities: string[]
  skills: string[]
  canContact: string
  credentials: string
  shell: string
  extras: string
}

export interface Notice {
  text: string
  tone: Tone
}

export function displayName(name: string): string {
  return name === '' ? 'New agent' : name
}

/** The safety picker's choices; a catalog without any falls back to the built-ins. */
export function safetyOptions(catalog: Catalog): string[] {
  const opts = catalog.safety_profiles ?? []
  return opts.length > 0 ? opts : ['default', 'none']
}

/**
 * The labels of a check grid: the catalog's entries, then anything the loaded
 * definition selects that the catalog does not list. The desktop builder
 * drops those silently on the next apply; here they stay visible and checked
 * so a round trip through the form never loses a capability or a skill.
 */
export function checkLabels(catalog: string[] | null, selected: string[] | null): string[] {
  const labels = [...(catalog ?? [])]
  for (const s of selected ?? []) {
    if (!labels.includes(s)) labels.push(s)
  }
  return labels
}

/** Sets one check and returns the selection in grid order. */
export function toggleCheck(labels: string[], selected: string[], label: string, checked: boolean): string[] {
  return labels.filter((l) => (l === label ? checked : selected.includes(l)))
}

/** Recursively orders object keys the way Go's encoding/json writes a map. */
function sortKeys(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(sortKeys)
  if (v !== null && typeof v === 'object') {
    const src = v as Record<string, unknown>
    const out: Record<string, unknown> = {}
    for (const k of Object.keys(src).sort()) out[k] = sortKeys(src[k])
    return out
  }
  return v
}

/**
 * Encodes a value as Go's json.Marshal does: sorted object keys and the
 * HTML-sensitive characters and line separators escaped. Every JSON value is
 * a YAML flow value, which is how the agent document is written.
 */
export function goJSON(v: unknown): string {
  return JSON.stringify(sortKeys(v))
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

/** strconv.Atoi: an optionally signed run of digits, or null. */
export function atoi(s: string): number | null {
  if (!/^[+-]?\d+$/.test(s)) return null
  const n = Number(s)
  return Number.isSafeInteger(n) ? n : null
}

/** True when the history budget field holds what buildYAML will write. */
export function historyValid(text: string): boolean {
  const n = atoi(text.trim())
  return n !== null && n > 0
}

/** The JSON error for the extras field, or '' when it is empty or parses. */
export function extrasError(text: string): string {
  const t = text.trim()
  if (t === '') return ''
  try {
    JSON.parse(t)
    return ''
  } catch (err) {
    return 'extras is not valid JSON: ' + (err instanceof Error ? err.message : String(err))
  }
}

/**
 * Seeds the form. A loaded definition fills it; a template starts from the
 * deployment's default capabilities; a blank agent starts empty. Missing
 * safety and history budget read as the runtime defaults.
 */
export function initialForm(name: string, detail: AgentDetail | null, tpl: Template | null, catalog: Catalog): BuilderForm {
  let ag: Partial<AgentJSON> = detail?.agent ?? {}
  if (tpl) {
    ag = { safety: 'default', history_budget: 6000, capabilities: [...(catalog.default_capabilities ?? [])] }
  }
  // An unset profile shows as "default", but only when the deployment has one
  // by that name; a profile the definition names itself is always kept.
  let safety = ag.safety ?? ''
  if (safety === '' && safetyOptions(catalog).includes('default')) safety = 'default'

  const extras = ag.extras ?? null
  return {
    name,
    model: ag.model ?? '',
    safety,
    history: String(ag.history_budget || 6000),
    persistent: ag.persistent ?? false,
    capabilities: checkLabels(catalog.capabilities, ag.capabilities ?? []).filter((l) => (ag.capabilities ?? []).includes(l)),
    skills: checkLabels(catalog.tools, ag.skills ?? []).filter((l) => (ag.skills ?? []).includes(l)),
    canContact: (ag.can_contact ?? []).join(', '),
    credentials: (ag.credentials ?? []).join(', '),
    shell: ag.shell ?? '',
    extras: extras && Object.keys(extras).length > 0 ? JSON.stringify(sortKeys(extras), null, 2) : '',
  }
}

/**
 * Renders the form as the agent document. JSON values are valid YAML; the
 * runtime overrides the loop and instructions paths on apply. Throws when the
 * extras field is not JSON.
 */
export function buildYAML(form: BuilderForm, detail: AgentDetail | null): string {
  let out = ''
  const write = (key: string, v: unknown) => {
    out += `${key}: ${goJSON(v)}\n`
  }

  const name = form.name.trim()
  let loopRef = name === '' ? './agent.lua' : `./${name}.lua`
  if (detail?.agent?.loop) loopRef = detail.agent.loop
  write('loop', loopRef)
  if (form.model !== '' && form.model !== DEFAULT_MODEL_OPTION) write('model', form.model)
  if (form.safety !== '') write('safety', form.safety)
  const budget = atoi(form.history.trim())
  if (budget !== null && budget > 0) write('history_budget', budget)
  if (form.persistent) write('persistent', true)
  if (form.capabilities.length > 0) write('capabilities', form.capabilities)
  if (form.skills.length > 0) write('skills', form.skills)
  const contacts = splitList(form.canContact)
  if (contacts.length > 0) write('can_contact', contacts)
  const creds = splitList(form.credentials)
  if (creds.length > 0) write('credentials', creds)
  const shell = form.shell.trim()
  if (shell !== '') write('shell', shell)
  const extras = form.extras.trim()
  if (extras !== '') {
    let v: unknown
    try {
      v = JSON.parse(extras)
    } catch (err) {
      throw new Error('extras is not valid JSON: ' + (err instanceof Error ? err.message : String(err)))
    }
    write('extras', v)
  }
  return out
}

/**
 * What the loop will actually run with: an explicit selection, or the
 * deployment defaults when the agent leaves the list empty. An empty result
 * means the catalog has no defaults to reason about, so capability checks
 * should be skipped rather than guessed.
 */
export function effectiveCapabilities(selected: string[], catalog: Catalog): string[] {
  if (selected.length > 0) return selected
  return [...(catalog.default_capabilities ?? [])]
}

/** The title line's one short, unambiguous state. */
export function headerStatus(dirty: boolean, detail: AgentDetail | null): Notice {
  if (dirty) return { text: 'Unsaved changes', tone: 'accent' }
  if (!detail) return { text: 'Not applied', tone: 'warning' }
  if (detail.persisted) return { text: 'Saved', tone: 'success' }
  return { text: 'Live', tone: 'accent' }
}

/** Checks the name the way validate and apply do; returns the message or ''. */
export function nameError(name: string): string {
  const n = name.trim()
  if (n === '') return NAME_REQUIRED
  if (!AGENT_NAME_RE.test(n)) return NAME_INVALID
  return ''
}

export function countLabel(n: number, one: string, many: string): string {
  return n === 1 ? `1 ${one}` : `${n} ${many}`
}

/** The loop editor's status line: errors lead, warnings follow, anything else is clean. */
export function loopStatus(diagnostics: LuauDiagnostic[], checking: boolean, failure: string): Notice {
  if (failure) return { text: 'language service error: ' + failure, tone: 'danger' }
  if (checking) return { text: 'checking…', tone: 'neutral' }
  const errors = diagnostics.filter((d) => d.severity === 'error').length
  const warnings = diagnostics.filter((d) => d.severity === 'warning').length
  if (errors > 0) {
    let text = countLabel(errors, 'error', 'errors')
    if (warnings > 0) text += ' · ' + countLabel(warnings, 'warning', 'warnings')
    return { text, tone: 'danger' }
  }
  if (warnings > 0) return { text: countLabel(warnings, 'warning', 'warnings'), tone: 'warning' }
  return { text: 'no issues', tone: 'success' }
}

/** CodeMirror stores lines LF-joined; normalising up front keeps a load from reading as an edit. */
export function lf(text: string | null | undefined): string {
  return (text ?? '').replace(/\r\n?/g, '\n')
}
