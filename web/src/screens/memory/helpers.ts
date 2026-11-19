// Pure helpers for the Memory screen, shared with the desktop Studio's
// internal/ui/memory.go so both apps read and build drafts the same way.
// Builders throw an Error whose message is the text the form shows.
import type { MemoryBackend, MemoryBackendDraft, MemoryProfile, MemoryProfileDraft, MemoryStoreDraft } from '@/api/types'
import { splitList } from '@/lib/format'

/** Which config keys each provider reads; the editor shows only those fields. */
export const PROVIDER_KEYS: Record<string, string[]> = {
  sqlite: ['path', 'vector_scan_limit'],
  postgres: ['url', 'dim'],
  pgvector: ['url', 'dim'],
  redis: ['url'],
  redisvector: ['url', 'dim'],
  qdrant: ['url', 'api_key', 'collection', 'dim', 'distance', 'timeout'],
  volatile: ['max_entries'],
}

/** One line on what each provider is for, shown under the picker. */
export const PROVIDER_HINTS: Record<string, string> = {
  sqlite: 'A local SQLite file: key-value, full-text and brute-force vector search. No server needed.',
  postgres: 'A PostgreSQL server: key-value and full-text search, shared by every instance.',
  pgvector: 'PostgreSQL with the pgvector extension: everything postgres does, plus indexed vector search.',
  redis: 'A Redis server: key-value only, fast and shared.',
  redisvector: 'Redis Stack: key-value plus vector search.',
  qdrant: 'A Qdrant server: vector search at scale.',
  volatile: 'Process memory, lost on restart and shared by every session. Bound it with max entries or a store retention.',
}

export const QDRANT_DISTANCES = ['Cosine', 'Euclid', 'Dot', 'Manhattan']
export const STORE_SCOPES = [
  { value: '', label: 'per user (default)' },
  { value: 'agent', label: 'whole agent' },
]

export function providerHas(provider: string, key: string): boolean {
  return (PROVIDER_KEYS[provider] ?? []).includes(key)
}

/** Where a backend points, in one line and without secrets. */
export function backendTarget(b: MemoryBackend): string {
  if (b.provider === 'sqlite') return b.path || './data/agentflow.db'
  if (b.provider === 'volatile') return b.max_entries ? `max ${b.max_entries} entries` : 'unbounded'
  const where = b.url_ref || b.url_display || (b.has_url ? 'URL set' : 'no URL')
  return b.provider === 'qdrant' && b.collection ? `${where} · ${b.collection}` : where
}

/** The "used by" line of a backend row. */
export function backendUsedBy(b: MemoryBackend, builtinAgents: string[]): string {
  const parts: string[] = []
  const profiles = (b.profiles ?? [])
    .filter((p) => p !== 'conversational' || builtinAgents.length > 0)
    .map((p) => (p === 'conversational' ? `conversational (built-in: ${builtinAgents.join(', ')})` : p))
  if (profiles.length > 0) parts.push('profiles: ' + profiles.join(', '))
  // Agents whose stores are on the backend, through a named or inline
  // profile; those on the built-in profile are already named above.
  const agents = (b.agents ?? []).filter((a) => !builtinAgents.includes(a))
  if (agents.length > 0) parts.push('agents: ' + agents.join(', '))
  return parts.length > 0 ? parts.join(' · ') : 'unused'
}

export function backendInUse(b: MemoryBackend, builtinAgents: string[]): boolean {
  return backendUsedBy(b, builtinAgents) !== 'unused'
}

/** The "used by" line of a profile row. */
export function profileUsedBy(p: Pick<MemoryProfile, 'agents' | 'spawn_profiles'>): string {
  const parts: string[] = []
  if ((p.agents ?? []).length > 0) parts.push('agents: ' + (p.agents ?? []).join(', '))
  if ((p.spawn_profiles ?? []).length > 0) parts.push('spawn: ' + (p.spawn_profiles ?? []).join(', '))
  return parts.length > 0 ? parts.join(' · ') : 'not used by an agent yet'
}

export function profileInUse(p: Pick<MemoryProfile, 'agents' | 'spawn_profiles'>): boolean {
  return (p.agents ?? []).length > 0 || (p.spawn_profiles ?? []).length > 0
}

/** One store in a line: "dialogue → main_db.dialogue · 30d · window 200". */
export function storeSummary(s: MemoryStoreDraft): string {
  const parts = [`${s.backend}${s.table ? '.' + s.table : ''}`]
  if (s.retention) parts.push(s.retention)
  if (s.window) parts.push(`window ${s.window}`)
  if ((s.requires ?? []).length > 0) parts.push((s.requires ?? []).join('+'))
  if (s.pool) parts.push(`pool ${s.pool}`)
  else if (s.scope === 'agent') parts.push('whole agent')
  return parts.join(' · ')
}

/** A whole number field: '' is 0 (unset); anything else must be a non-negative integer. */
export function parseCount(text: string, label: string): number {
  const t = text.trim()
  if (t === '') return 0
  if (!/^\d+$/.test(t)) throw new Error(`${label} must be a whole number`)
  const n = Number(t)
  if (!Number.isSafeInteger(n)) throw new Error(`${label} is too large`)
  return n
}

export function countText(n: number | undefined): string {
  return n ? String(n) : ''
}

// ---- backends ---------------------------------------------------------------

export interface BackendForm {
  name: string
  provider: string
  path: string
  /** Write only: empty keeps the stored value. */
  url: string
  clearUrl: boolean
  apiKey: string
  clearApiKey: boolean
  collection: string
  distance: string
  timeout: string
  dim: string
  vectorScanLimit: string
  maxEntries: string
}

export function backendForm(b: MemoryBackend | null): BackendForm {
  return {
    name: b?.name ?? '',
    provider: b?.provider ?? 'sqlite',
    path: b?.path ?? '',
    url: '',
    clearUrl: false,
    apiKey: '',
    clearApiKey: false,
    collection: b?.collection ?? '',
    distance: b?.distance ?? '',
    timeout: b?.timeout ?? '',
    dim: countText(b?.dim),
    vectorScanLimit: countText(b?.vector_scan_limit),
    maxEntries: countText(b?.max_entries),
  }
}

/** The draft a backend form sends; only the provider's own keys are included. */
export function buildBackendDraft(f: BackendForm): MemoryBackendDraft {
  const d: MemoryBackendDraft = { name: f.name.trim(), provider: f.provider }
  const has = (k: string) => providerHas(f.provider, k)
  if (has('path') && f.path.trim()) d.path = f.path.trim()
  if (has('url')) {
    if (f.url.trim()) d.url = f.url.trim()
    else if (f.clearUrl) d.clear_url = true
  }
  if (has('api_key')) {
    if (f.apiKey.trim()) d.api_key = f.apiKey.trim()
    else if (f.clearApiKey) d.clear_api_key = true
  }
  if (has('collection') && f.collection.trim()) d.collection = f.collection.trim()
  if (has('distance') && f.distance.trim()) d.distance = f.distance.trim()
  if (has('timeout') && f.timeout.trim()) d.timeout = f.timeout.trim()
  if (has('dim')) {
    const n = parseCount(f.dim, 'Dimensions')
    if (n) d.dim = n
  }
  if (has('vector_scan_limit')) {
    const n = parseCount(f.vectorScanLimit, 'Vector scan limit')
    if (n) d.vector_scan_limit = n
  }
  if (has('max_entries')) {
    const n = parseCount(f.maxEntries, 'Max entries')
    if (n) d.max_entries = n
  }
  return d
}

// ---- profiles ---------------------------------------------------------------

export interface StoreRow {
  name: string
  backend: string
  table: string
  retention: string
  window: string
  requires: string[]
  scope: string
  pool: string
  localityOk: boolean
}

export interface ProfileForm {
  name: string
  stores: StoreRow[]
  write: string[]
  recall: string
  embedModel: string
  rerankModel: string
  oversample: string
}

export function storeRow(s: MemoryStoreDraft | null, backend = ''): StoreRow {
  return {
    name: s?.name ?? '',
    backend: s?.backend ?? backend,
    table: s?.table ?? '',
    retention: s?.retention ?? '',
    window: countText(s?.window),
    requires: [...(s?.requires ?? [])],
    scope: s?.scope ?? '',
    pool: s?.pool ?? '',
    localityOk: s?.locality_ok ?? false,
  }
}

/**
 * A new profile starts like the built-in conversational one: a recent
 * dialogue store and a searchable facts store, on the first backend.
 */
export function profileForm(p: MemoryProfile | null, backends: string[]): ProfileForm {
  if (p) {
    return {
      name: p.name,
      stores: (p.stores ?? []).map((s) => storeRow(s)),
      write: [...(p.write ?? [])],
      recall: p.recall ?? '',
      embedModel: p.embed_model ?? '',
      rerankModel: p.rerank_model ?? '',
      oversample: countText(p.oversample),
    }
  }
  const backend = backends.includes('main_db') ? 'main_db' : (backends[0] ?? '')
  return {
    name: '',
    stores: [
      { ...storeRow(null, backend), name: 'dialogue', table: 'dialogue', retention: '30d', window: '1000', requires: ['kv'] },
      { ...storeRow(null, backend), name: 'facts', table: 'facts', retention: 'forever', requires: ['kv', 'text_search'] },
    ],
    write: ['plugin:routing_table'],
    recall: 'plugin:recency',
    embedModel: '',
    rerankModel: '',
    oversample: '',
  }
}

export function buildProfileDraft(f: ProfileForm): MemoryProfileDraft {
  const stores = f.stores.map((r, i) => {
    const label = r.name.trim() ? `Store "${r.name.trim()}"` : `Store ${i + 1}`
    const s: MemoryStoreDraft = { name: r.name.trim(), backend: r.backend.trim() }
    if (r.table.trim()) s.table = r.table.trim()
    if (r.retention.trim()) s.retention = r.retention.trim()
    const w = parseCount(r.window, `${label}: window`)
    if (w) s.window = w
    if (r.requires.length > 0) s.requires = [...r.requires]
    if (r.pool.trim()) s.pool = r.pool.trim()
    else if (r.scope) s.scope = r.scope
    if (r.localityOk) s.locality_ok = true
    return s
  })
  const d: MemoryProfileDraft = { name: f.name.trim(), stores }
  const write = f.write.flatMap((w) => splitList(w))
  if (write.length > 0) d.write = write
  if (f.recall.trim()) d.recall = f.recall.trim()
  if (f.embedModel.trim()) d.embed_model = f.embedModel.trim()
  if (f.rerankModel.trim()) d.rerank_model = f.rerankModel.trim()
  const o = parseCount(f.oversample, 'Oversample')
  if (o) d.oversample = o
  return d
}

/** Store names a profile's recall plugin reads, so the editor can say which ones matter. */
export function recallStores(recall: string): string[] {
  if (recall === 'plugin:semantic') return ['semantic', 'dialogue']
  if (recall === 'plugin:recency' || recall === '') return ['dialogue', 'facts']
  return []
}
