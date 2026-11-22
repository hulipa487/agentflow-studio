// Pure helpers for the Safety screen, ported from the desktop Studio
// (internal/ui/safety.go) so both apps parse and print the same way. Parsers
// throw an Error whose message is the text the form shows.
import type {
  SafetyClassifier,
  SafetyClassifierDraft,
  SafetyLuaFilter,
  SafetyLuaFilterDraft,
  SafetyProfile,
  SafetyProfileDraft,
} from '@/api/types'
import { hasMessage, t } from '@/i18n'

export const CLASSIFIER_PREFIX = 'classifier:'
export const LUA_FILTER_PREFIX = 'lua:'

export const MODES = ['enforce', 'shadow']
export const FAIL_ACTIONS = ['block', 'pass']
export const LUA_FAIL_ACTIONS = ['pass', 'block']
export const SOURCES = ['channel', 'agent', 'scheduler', 'system'] as const
export type Source = (typeof SOURCES)[number]

export function modeLabel(mode: string): string {
  const key = `safety.mode.${mode}`
  return hasMessage(key) ? t(key) : mode
}

export function failActionLabel(action: string): string {
  const key = `safety.failAction.${action}`
  return hasMessage(key) ? t(key) : action
}

export function sourceLabel(source: string): string {
  const key = `safety.sourceKind.${source}`
  return hasMessage(key) ? t(key) : source
}

export function directionLabel(dir: string): string {
  const key = `safety.direction.${dir}`
  return hasMessage(key) ? t(key) : dir
}

export function withCurrentOptions(
  values: readonly string[],
  current: string,
  labelOf: (v: string) => string,
): { value: string; label: string }[] {
  const list = current !== '' && !values.includes(current) ? [...values, current] : [...values]
  return list.map((v) => ({ value: v, label: labelOf(v) }))
}

// ---- formatting -------------------------------------------------------------

/** What an empty chain shows, in the current language. */
export function emptyChain(): string {
  return t('safety.emptyChain')
}

/** The chain as the dispatcher will build it, left to right. */
export function chainText(filters: string[] | null | undefined): string {
  if (!filters || filters.length === 0) return emptyChain()
  return filters.join('  →  ')
}

function sortedLabels(categories: Record<string, string>): string[] {
  // Go's sort.Strings orders by bytes; plain < does the same for these labels,
  // where localeCompare would reorder case and punctuation.
  return Object.keys(categories).sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
}

/** One-line view of a label → action map: "a=allow, b=block". */
export function categorySummary(categories: Record<string, string> | null | undefined): string {
  if (!categories) return '—'
  const keys = sortedLabels(categories)
  if (keys.length === 0) return '—'
  return keys.map((k) => `${k}=${categories[k]}`).join(', ')
}

/** The editable text of a label → action map, one "label: action" per line. */
export function formatCategories(categories: Record<string, string> | null | undefined): string {
  if (!categories) return ''
  return sortedLabels(categories)
    .map((k) => `${k}: ${categories[k]}`)
    .join('\n')
}

/** A float as Go's FormatFloat(f, 'f', -1, 64) prints it: never in exponent form. */
export function formatThreshold(f: number): string {
  const s = String(f)
  return /e/i.test(s) ? f.toFixed(20).replace(/\.?0+$/, '') : s
}

/** What still names a profile: agents first, then spawn profiles. */
export function profileUsage(p: Pick<SafetyProfile, 'agents' | 'spawn_profiles'>): string {
  const parts: string[] = []
  if ((p.agents ?? []).length > 0) parts.push((p.agents ?? []).join(', '))
  if ((p.spawn_profiles ?? []).length > 0) parts.push((p.spawn_profiles ?? []).join(', '))
  return parts.join(', ')
}

/** The "used by" line of a profile row. */
export function profileUsedBy(p: Pick<SafetyProfile, 'agents' | 'spawn_profiles'>): string {
  const agents = p.agents ?? []
  const spawn = p.spawn_profiles ?? []
  if (agents.length === 0 && spawn.length === 0) return t('safety.usedBy.none')
  const parts: string[] = []
  if (agents.length > 0) parts.push(t('safety.usedBy.agents', { list: agents.join(', ') }))
  if (spawn.length > 0) parts.push(t('safety.usedBy.spawn', { list: spawn.join(', ') }))
  return parts.join(' · ')
}

export function profileInUse(p: Pick<SafetyProfile, 'agents' | 'spawn_profiles'>): boolean {
  return (p.agents ?? []).length > 0 || (p.spawn_profiles ?? []).length > 0
}

export function isClassifierFilter(filter: string): boolean {
  return filter.startsWith(CLASSIFIER_PREFIX)
}

export function isLuaFilter(filter: string): boolean {
  return filter.startsWith(LUA_FILTER_PREFIX)
}

// ---- parsing ----------------------------------------------------------------

// Go's %q: a double-quoted, escaped string. JSON's escaping matches it for
// everything an operator can type on one line.
const quote = (s: string) => JSON.stringify(s)

// The decimal forms strconv.ParseFloat accepts; Number() alone would also take
// "", "0x10" and " 1 ".
const FLOAT_RE = /^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/

/**
 * Parses the optional threshold. Blank is undefined, which keeps the runtime
 * default (0.7); an explicit 0 is kept, because it blocks every label.
 */
export function parseOptionalThreshold(raw: string): number | undefined {
  raw = raw.trim()
  if (raw === '') return undefined
  const f = FLOAT_RE.test(raw) ? Number(raw) : NaN
  if (Number.isNaN(f) || f < 0 || f > 1) throw new Error(t('safety.err.thresholdRange'))
  return f === 0 ? 0 : f
}

/**
 * Parses the categories text: one "label: action" per line, blank lines
 * skipped, the action lowercased. The action itself is checked by the runtime.
 */
export function parseCategoriesEntry(raw: string): Record<string, string> {
  const out = new Map<string, string>()
  for (let line of raw.split('\n')) {
    line = line.trim()
    if (line === '') continue
    const cut = line.indexOf(':')
    if (cut < 0) throw new Error(t('safety.err.categoriesNoColon', { line: quote(line) }))
    const label = line.slice(0, cut).trim()
    const action = line.slice(cut + 1).trim().toLowerCase()
    if (label === '') throw new Error(t('safety.err.categoriesEmptyLabel', { line: quote(line) }))
    out.set(label, action)
  }
  // fromEntries defines own properties, so a label such as "__proto__" survives.
  return Object.fromEntries(out)
}

/**
 * Parses an optional count: blank is 0 (unset), otherwise a non-negative
 * integer. label is the field's name, already translated.
 */
export function optionalCount(label: string, raw: string): number {
  raw = raw.trim()
  if (raw === '') return 0
  const n = /^[+-]?\d+$/.test(raw) ? Number(raw) : NaN
  // Past 2^53 the number sent would not be the number typed.
  if (!Number.isSafeInteger(n) || n < 0) throw new Error(t('safety.err.nonNegative', { what: label }))
  return n === 0 ? 0 : n
}

/** The desktop's int64 variant; a JS number covers both. */
export const optionalCount64 = optionalCount

/** A stored count as form text: unset (0) is blank. */
export function countText(n: number | null | undefined): string {
  return n && n > 0 ? String(n) : ''
}

// ---- profile draft ----------------------------------------------------------

/** The chain a new profile starts with. */
export const DEFAULT_CHAIN = ['source-attribution', 'affect-guard']

export function buildProfileDraft(name: string, filters: string[]): SafetyProfileDraft {
  const d: SafetyProfileDraft = { name: name.trim(), filters }
  if (d.name === '') throw new Error(t('safety.err.profileNameRequired'))
  return d
}

/** Appends filters to a chain, keeping the order and skipping ones already in it. */
export function appendFilters(chain: string[], add: string[]): string[] {
  const next = [...chain]
  for (const f of add) {
    if (f !== '' && !next.includes(f)) next.push(f)
  }
  return next
}

/** Moves one chain entry by delta positions; out-of-range moves change nothing. */
export function moveFilter(chain: string[], index: number, delta: number): string[] {
  const to = index + delta
  if (index < 0 || index >= chain.length || to < 0 || to >= chain.length) return chain
  const next = [...chain]
  const [f] = next.splice(index, 1)
  next.splice(to, 0, f)
  return next
}

// ---- classifier form --------------------------------------------------------

/** The classifier editor's fields, as typed. */
export interface ClassifierForm {
  name: string
  model: string
  /** '' is the embedded default prompt. */
  prompt: string
  mode: string
  ingress: boolean
  egress: boolean
  sources: Record<Source, boolean>
  threshold: string
  categories: string
  timeout: string
  onError: string
  onSaturated: string
  onBudget: string
  maxTokens: string
  maxInputChars: string
  maxConcurrency: string
  cacheSize: string
  cacheTTL: string
  budget: string
  refusal: string
  fallback: string
}

/** The form for a new classifier, or for editing an existing one. */
export function classifierForm(existing: SafetyClassifier | null, models: string[]): ClassifierForm {
  if (!existing) {
    return {
      name: '',
      model: models[0] ?? '',
      prompt: '',
      mode: 'enforce',
      ingress: true,
      egress: true,
      sources: { channel: true, agent: true, scheduler: false, system: false },
      threshold: '',
      categories: '',
      timeout: '',
      onError: 'block',
      onSaturated: 'block',
      onBudget: 'block',
      maxTokens: '',
      maxInputChars: '',
      maxConcurrency: '',
      cacheSize: '',
      cacheTTL: '',
      budget: '',
      refusal: '',
      fallback: '',
    }
  }
  const directions = existing.directions ?? []
  const sources = existing.sources ?? []
  // An unset list means the runtime defaults: both directions, channel + agent.
  const source = (s: Source) => (sources.length === 0 ? s === 'channel' || s === 'agent' : sources.includes(s))
  return {
    name: existing.name,
    model: existing.model ?? '',
    prompt: existing.prompt ?? '',
    mode: existing.mode || 'enforce',
    ingress: directions.length === 0 || directions.includes('ingress'),
    egress: directions.length === 0 || directions.includes('egress'),
    sources: { channel: source('channel'), agent: source('agent'), scheduler: source('scheduler'), system: source('system') },
    threshold: typeof existing.threshold === 'number' ? formatThreshold(existing.threshold) : '',
    categories: formatCategories(existing.categories),
    timeout: existing.timeout ?? '',
    onError: existing.on_error || 'block',
    onSaturated: existing.on_saturated || 'block',
    onBudget: existing.on_budget_exhausted || 'block',
    maxTokens: countText(existing.max_tokens),
    maxInputChars: countText(existing.max_input_chars),
    maxConcurrency: countText(existing.max_concurrency),
    cacheSize: countText(existing.cache?.size),
    cacheTTL: existing.cache?.ttl ?? '',
    budget: countText(existing.budget?.tokens_per_day),
    refusal: existing.refusal ?? '',
    fallback: existing.fallback ?? '',
  }
}

/**
 * Turns the form into the draft the runtime takes, checking what can be
 * checked locally in the order the form reads. Unset optional fields are left
 * out so the saved config stays as small as a hand-written one.
 */
export function buildClassifierDraft(f: ClassifierForm): SafetyClassifierDraft {
  const name = f.name.trim()
  if (name === '') throw new Error(t('safety.err.classifierNameRequired'))
  if (f.model === '') throw new Error(t('safety.err.chooseModel'))

  const directions: string[] = []
  if (f.ingress) directions.push('ingress')
  if (f.egress) directions.push('egress')
  const sources: string[] = SOURCES.filter((s) => f.sources[s])
  if (directions.length === 0) throw new Error(t('safety.err.chooseDirection'))
  if (sources.length === 0) throw new Error(t('safety.err.chooseSource'))

  const d: SafetyClassifierDraft = { name, model: f.model, mode: f.mode, directions, sources, categories: {}, cache: {}, budget: {} }
  if (f.prompt !== '') d.prompt = f.prompt

  const threshold = parseOptionalThreshold(f.threshold)
  if (threshold !== undefined) d.threshold = threshold
  d.categories = parseCategoriesEntry(f.categories)

  const timeout = f.timeout.trim()
  if (timeout !== '') d.timeout = timeout
  d.on_error = f.onError
  d.on_saturated = f.onSaturated
  d.on_budget_exhausted = f.onBudget

  const maxTokens = optionalCount(t('safety.classifier.maxTokens'), f.maxTokens)
  if (maxTokens > 0) d.max_tokens = maxTokens
  const maxInputChars = optionalCount(t('safety.classifier.maxInputChars'), f.maxInputChars)
  if (maxInputChars > 0) d.max_input_chars = maxInputChars
  const maxConcurrency = optionalCount(t('safety.classifier.maxConcurrency'), f.maxConcurrency)
  if (maxConcurrency > 0) d.max_concurrency = maxConcurrency
  const cacheSize = optionalCount(t('safety.classifier.cacheSize'), f.cacheSize)
  if (cacheSize > 0) d.cache.size = cacheSize
  const cacheTTL = f.cacheTTL.trim()
  if (cacheTTL !== '') d.cache.ttl = cacheTTL
  const budget = optionalCount64(t('safety.classifier.budget'), f.budget)
  if (budget > 0) d.budget.tokens_per_day = budget

  const refusal = f.refusal.trim()
  if (refusal !== '') d.refusal = refusal
  const fallback = f.fallback.trim()
  if (fallback !== '') d.fallback = fallback
  return d
}

/** A select's options, with the stored value kept selectable when the list no longer offers it. */
export function withCurrent(options: string[], current: string): string[] {
  return current !== '' && !options.includes(current) ? [...options, current] : options
}

// ---- custom Lua filter form -------------------------------------------------

export const LUA_FILTER_STARTER = `-- Sandboxed Lua 5.1 safety filter (string, table, math only).
-- Return a table or nil. Available on input:
--   ingress(input): input.message, input.source, input.session_id, input.annotations
--   egress(input):  input.text, input.source, input.session_id, input.annotations

function ingress(input)
  return { text = input.message }
end

function egress(input)
  local cleaned = string.gsub(input.text, "sk%-[A-Za-z0-9]+", "[REDACTED]")
  return { text = cleaned }
end
`

export interface LuaFilterForm {
  name: string
  useFile: boolean
  source: string
  file: string
  ingress: boolean
  egress: boolean
  sources: Record<Source, boolean>
  timeout: string
  instrBudget: string
  onError: string
  refusal: string
  fallback: string
}

export function luaFilterForm(existing: SafetyLuaFilter | null): LuaFilterForm {
  if (!existing) {
    return {
      name: '',
      useFile: false,
      source: LUA_FILTER_STARTER,
      file: '',
      ingress: true,
      egress: true,
      sources: { channel: true, agent: true, scheduler: false, system: false },
      timeout: '200ms',
      instrBudget: '100000',
      onError: 'pass',
      refusal: '',
      fallback: '',
    }
  }
  const directions = existing.directions ?? []
  const sources = existing.sources ?? []
  const allowSrc = (s: Source) => sources.length === 0 || sources.includes(s)
  const useFile = Boolean(existing.file && !existing.source)
  return {
    name: existing.name,
    useFile,
    source: existing.source ?? (useFile ? '' : LUA_FILTER_STARTER),
    file: existing.file ?? '',
    ingress: directions.length === 0 || directions.includes('ingress'),
    egress: directions.length === 0 || directions.includes('egress'),
    sources: {
      channel: allowSrc('channel'),
      agent: allowSrc('agent'),
      scheduler: allowSrc('scheduler'),
      system: allowSrc('system'),
    },
    timeout: existing.timeout ?? '',
    instrBudget: countText(existing.instr_budget),
    onError: existing.on_error || 'pass',
    refusal: existing.refusal ?? '',
    fallback: existing.fallback ?? '',
  }
}

export function buildLuaFilterDraft(f: LuaFilterForm): SafetyLuaFilterDraft {
  const name = f.name.trim()
  if (name === '') throw new Error(t('safety.err.luaNameRequired'))
  if (f.useFile) {
    if (f.file.trim() === '') throw new Error(t('safety.err.luaFileRequired'))
  } else if (f.source.trim() === '') {
    throw new Error(t('safety.err.luaSourceRequired'))
  }
  const directions: string[] = []
  if (f.ingress) directions.push('ingress')
  if (f.egress) directions.push('egress')
  if (directions.length === 0) throw new Error(t('safety.err.chooseDirection'))
  const sources = SOURCES.filter((s) => f.sources[s])
  if (sources.length === 0) throw new Error(t('safety.err.chooseSource'))

  const d: SafetyLuaFilterDraft = {
    name,
    directions,
    sources: sources.length === SOURCES.length ? undefined : sources,
    on_error: f.onError || 'pass',
  }
  if (f.useFile) d.file = f.file.trim()
  else d.source = f.source.replace(/\r\n?/g, '\n')

  const timeout = f.timeout.trim()
  if (timeout !== '') d.timeout = timeout
  const budget = optionalCount64(t('safety.lua.instrBudget'), f.instrBudget)
  if (budget > 0) d.instr_budget = budget
  const refusal = f.refusal.trim()
  if (refusal !== '') d.refusal = refusal
  const fallback = f.fallback.trim()
  if (fallback !== '') d.fallback = fallback
  return d
}
