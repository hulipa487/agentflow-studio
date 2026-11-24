// The builder's form model: what the Config tab edits, how it is seeded from
// a loaded definition or a template, and how it becomes the agent document
// sent to the runtime. A port of the desktop Studio's builder.go; buildYAML
// emits the same bytes the Go builder does for the same form.
import type { AgentDetail, AgentJSON, Catalog, Template, TemplateSetting } from '@/api/types'
import { hasMessage, t } from '@/i18n'
import { splitList } from '@/lib/format'
import type { ScriptDiagnostic } from '@/script'
import type { Tone } from '@/state/ui'

export const AGENT_NAME_RE = /^[A-Za-z0-9_-]{1,64}$/

/** The model picker's first entry: the definition names no model. */
export const DEFAULT_MODEL_OPTION = 'default (per call)'
/** The label the picker shows for DEFAULT_MODEL_OPTION's entry. */
export const DEFAULT_MODEL_LABEL_KEY = 'builder.defaultModel'

// Message keys: nameError returns one of these, the caller translates it.
export const NAME_REQUIRED = 'builder.nameRequired'
export const NAME_INVALID = 'builder.nameInvalid'

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
  ssh: string
  /** '' is no memory; MEMORY_INLINE keeps the definition's inline profile. */
  memory: string
  /** Inline memory profile definition when memory is MEMORY_INLINE. */
  memoryInline: Record<string, unknown> | null
  extras: string
}

/** The memory picker's value for an inline profile the form keeps as it is. */
export const MEMORY_INLINE = '(inline)'
export const MEMORY_BUILTIN = 'conversational'

/**
 * The memory picker's choices: none, the built-in profile, the deployment's
 * named profiles, and whatever the definition uses that the catalog does not
 * list, so opening an agent never silently rewrites its memory.
 */
export function memoryOptions(catalog: Catalog, current: string): { value: string; label: string }[] {
  const opts = [
    { value: '', label: t('builder.memoryNone') },
    { value: MEMORY_BUILTIN, label: t('builder.memoryBuiltin') },
  ]
  const named = (catalog.memory_profiles ?? []).filter((p) => p !== MEMORY_BUILTIN)
  for (const p of named) opts.push({ value: p, label: p })
  if (current === MEMORY_INLINE) opts.push({ value: MEMORY_INLINE, label: t('builder.memoryInline') })
  else if (current !== '' && current !== MEMORY_BUILTIN && !named.includes(current)) {
    opts.push({ value: current, label: t('builder.notInCatalog', { name: current }) })
  }
  return opts
}

export interface Notice {
  text: string
  tone: Tone
}

export function displayName(name: string): string {
  return name === '' ? t('builder.newAgent') : name
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
  const trimmed = text.trim()
  if (trimmed === '') return ''
  try {
    JSON.parse(trimmed)
    return ''
  } catch (err) {
    return t('builder.extrasInvalid', { error: err instanceof Error ? err.message : String(err) })
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
    ag = {
      safety: 'default',
      history_budget: 6000,
      capabilities: tpl.capabilities ? [...tpl.capabilities] : [...(catalog.default_capabilities ?? [])],
      skills: tpl.skills ? [...tpl.skills] : [],
      memory: tpl.memory ?? '',
      memory_inline: tpl.memory_inline ?? null,
      extras: tpl.extras ?? null,
    }
  }
  // An unset profile shows as "default", but only when the deployment has one
  // by that name; a profile the definition names itself is always kept.
  let safety = ag.safety ?? ''
  if (safety === '' && safetyOptions(catalog).includes('default')) safety = 'default'

  const extras = ag.extras ?? null
  const memoryInline = ag.memory_inline && Object.keys(ag.memory_inline).length > 0 ? ag.memory_inline : null
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
    ssh: ag.ssh ?? '',
    memory: memoryInline ? MEMORY_INLINE : (ag.memory ?? ''),
    memoryInline,
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
  let loopRef = name === '' ? './agent.ts' : `./${name}.ts`
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
  const ssh = form.ssh.trim()
  if (ssh !== '') write('ssh', ssh)
  if (form.memory === MEMORY_INLINE) {
    const inlineProfile = form.memoryInline ?? detail?.agent?.memory_inline
    if (inlineProfile) write('memory', inlineProfile)
  } else if (form.memory !== '') {
    write('memory', form.memory)
  }
  // Preserve structured fields not yet exposed as dedicated form controls so
  // editing the loop or metadata never silently strips them.
  if (detail?.agent?.instructions_prompt) write('instructions', { prompt: detail.agent.instructions_prompt })
  if (detail?.agent?.inherit_user !== undefined && detail.agent.inherit_user !== null) {
    write('inherit_user', detail.agent.inherit_user)
  }
  if (detail?.agent?.budget && Object.keys(detail.agent.budget).length > 0) {
    write('budget', detail.agent.budget)
  }
  const extras = form.extras.trim()
  if (extras !== '') {
    let v: unknown
    try {
      v = JSON.parse(extras)
    } catch (err) {
      throw new Error(t('builder.extrasInvalid', { error: err instanceof Error ? err.message : String(err) }))
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
  if (dirty) return { text: t('builder.statusUnsaved'), tone: 'accent' }
  if (!detail) return { text: t('builder.statusNotApplied'), tone: 'warning' }
  if (detail.persisted) return { text: t('builder.statusSaved'), tone: 'success' }
  return { text: t('builder.statusLive'), tone: 'accent' }
}

/** Checks the name the way validate and apply do; returns the message key or ''. */
export function nameError(name: string): string {
  const n = name.trim()
  if (n === '') return NAME_REQUIRED
  if (!AGENT_NAME_RE.test(n)) return NAME_INVALID
  return ''
}

/** The loop editor's status line: errors lead, warnings follow, anything else is clean. */
export function loopStatus(diagnostics: ScriptDiagnostic[], checking: boolean, failure: string): Notice {
  if (failure) return { text: t('script.status.failed', { error: failure }), tone: 'danger' }
  if (checking) return { text: t('script.status.checking'), tone: 'neutral' }
  const errors = diagnostics.filter((d) => d.severity === 'error').length
  const warnings = diagnostics.filter((d) => d.severity === 'warning').length
  if (errors > 0) {
    const text = warnings > 0 ? t('script.status.errorsAndWarnings', { errors, warnings }) : t('script.status.errors', { errors })
    return { text, tone: 'danger' }
  }
  if (warnings > 0) return { text: t('script.status.warnings', { warnings }), tone: 'warning' }
  return { text: t('script.status.noIssues'), tone: 'success' }
}

/** Parses form.extras when it is a JSON object, returning null otherwise. */
export function parseExtrasObject(raw: string): Record<string, unknown> | null {
  const trimmed = raw.trim()
  if (trimmed === '') return {}
  try {
    const parsed = JSON.parse(trimmed)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : null
  } catch {
    return null
  }
}

/** Finds the catalog template whose schema-driven settings best match the agent's extras keys. */
export function matchingTemplateSettings(catalog: Catalog, extrasObj: Record<string, unknown> | null): TemplateSetting[] {
  if (!extrasObj) return []
  let best: TemplateSetting[] = []
  let bestHits = 0
  let bestRatio = 0
  for (const tpl of catalog.templates ?? []) {
    const settings = tpl.settings ?? []
    if (settings.length === 0) continue
    const hits = settings.filter((s) => s.key in extrasObj).length
    if (hits === 0) continue
    const ratio = hits / settings.length
    if (hits > bestHits || (hits === bestHits && ratio > bestRatio)) {
      best = settings
      bestHits = hits
      bestRatio = ratio
    }
  }
  return best
}

export function settingLabel(setting: Pick<TemplateSetting, 'key' | 'label'>): string {
  const key = 'builder.setting.' + setting.key + '.label'
  return hasMessage(key) ? t(key) : setting.label
}

export function settingDescription(setting: Pick<TemplateSetting, 'key' | 'description'>): string | undefined {
  const key = 'builder.setting.' + setting.key + '.description'
  return hasMessage(key) ? t(key) : setting.description
}

/** Updates one key inside form.extras JSON while keeping existing keys and deterministic sorting. */
export function updateExtrasSetting(raw: string, key: string, value: unknown): string {
  const base = parseExtrasObject(raw) ?? {}
  const next = { ...base, [key]: value }
  return JSON.stringify(sortKeys(next), null, 2)
}

/** CodeMirror stores lines LF-joined; normalising up front keeps a load from reading as an edit. */
export function lf(text: string | null | undefined): string {
  return (text ?? '').replace(/\r\n?/g, '\n')
}
