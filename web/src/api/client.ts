// The admin API client. One instance talks to one AgentFlow runtime, either
// straight from the browser (the runtime must list this origin in
// runtime.admin.cors_origins) or through Studio's local proxy, which also
// reaches Unix sockets and named pipes. Every request carries the bearer
// token explicitly; there are no cookies.
import { t } from '@/i18n'
import type * as T from './types'

/** An HTTP or transport failure. status 0 means the request got no response. */
export class ApiError extends Error {
  status: number
  /** True when the browser blocked or could not make the request at all. */
  network: boolean
  constructor(status: number, message: string, network = false) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.network = network
  }
}

export type Transport = 'http' | 'unix' | 'npipe'

export function transportOf(raw: string): Transport {
  const s = raw.trim()
  if (s.startsWith('unix://')) return 'unix'
  if (s.startsWith('npipe://') || s.startsWith('pipe://') || s.startsWith('\\\\.\\pipe\\')) return 'npipe'
  return 'http'
}

/** Normalizes what the operator typed: adds http://, trims the trailing slash. */
export function normalizeURL(raw: string): string {
  let s = raw.trim()
  if (transportOf(s) !== 'http') return s
  if (s !== '' && !s.includes('://')) s = 'http://' + s
  return s.replace(/\/+$/, '')
}

export const PROXY_PREFIX = '/__af'

export interface ClientOptions {
  url: string
  token: string
  /** Route through Studio's local proxy instead of calling the runtime directly. */
  viaProxy: boolean
}

type Body = unknown

// Nanosecond mtimes exceed 2^53, and the runtime compares them for equality,
// so they must survive the round trip digit for digit: they are quoted before
// JSON.parse sees them and unquoted again on the way out.
const MTIME_IN = /"mtime":\s*(\d{16,})/g
const MTIME_OUT = /"mtime":"(\d+)"/g

export function parseBody<R>(text: string): R {
  return JSON.parse(text.replace(MTIME_IN, '"mtime":"$1"')) as R
}

export function encodeBody(body: Body): string {
  return JSON.stringify(body).replace(MTIME_OUT, '"mtime":$1')
}

/** How long a plain request may take before it is given up on. */
const DEFAULT_TIMEOUT_MS = 60_000

const enc = encodeURIComponent

function query(params: Record<string, string | number | boolean | undefined>): string {
  const parts: string[] = []
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '') continue
    parts.push(`${enc(k)}=${enc(String(v))}`)
  }
  return parts.length ? '?' + parts.join('&') : ''
}

async function errorFromResponse(res: Response): Promise<ApiError> {
  const text = (await res.text().catch(() => '')).slice(0, 1 << 20)
  let msg = ''
  try {
    const body = JSON.parse(text)
    if (typeof body === 'string') msg = body.trim()
    else if (body && typeof body === 'object') {
      if (typeof body.error === 'string') msg = body.error
      if (!msg && Array.isArray(body.errors) && body.errors.length) msg = body.errors.join('; ')
    }
  } catch {
    /* not JSON */
  }
  if (!msg) msg = text.trim()
  if (!msg) msg = `${res.status} ${res.statusText}`.trim()
  if (res.status === 401) msg = t('connection.tokenRejected', { message: msg })
  return new ApiError(res.status, msg)
}

export class Client {
  /** What the operator entered, including unix:// or npipe://. */
  readonly displayURL: string
  readonly token: string
  readonly viaProxy: boolean

  constructor(opts: ClientOptions) {
    this.displayURL = normalizeURL(opts.url)
    this.token = opts.token.trim()
    this.viaProxy = opts.viaProxy || transportOf(this.displayURL) !== 'http'
  }

  /** True when the admin API is reached over a local socket or pipe. */
  get local(): boolean {
    return transportOf(this.displayURL) !== 'http'
  }

  private target(path: string): { url: string; headers: Record<string, string> } {
    const headers: Record<string, string> = {}
    if (this.token) headers['Authorization'] = 'Bearer ' + this.token
    if (this.viaProxy) {
      headers['X-AF-Target'] = this.displayURL
      return { url: PROXY_PREFIX + path, headers }
    }
    return { url: this.displayURL + path, headers }
  }

  private unreachable(err: unknown): ApiError {
    const why = err instanceof Error ? err.message : String(err)
    // A browser reports CORS refusals and a dead host identically.
    const msg = this.viaProxy
      ? t('connection.cannotReach', { url: this.displayURL, error: why })
      : t('connection.cannotReachCors', { url: this.displayURL, error: why, origin: location.origin })
    return new ApiError(0, msg, true)
  }

  /** Low-level fetch with auth; rejects with ApiError on a non-2xx answer. */
  async raw(method: string, path: string, body?: Body, signal?: AbortSignal, accept?: string): Promise<Response> {
    if (!this.displayURL) throw new ApiError(0, t('connection.urlRequired'))
    const { url, headers } = this.target(path)
    const init: RequestInit = { method, headers, signal, cache: 'no-store', credentials: 'omit' }
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json'
      init.body = encodeBody(body)
    }
    if (accept) headers['Accept'] = accept
    let res: Response
    try {
      res = await fetch(url, init)
    } catch (err) {
      if (signal?.aborted) throw err
      throw this.unreachable(err)
    }
    if (!res.ok) throw await errorFromResponse(res)
    return res
  }

  private async json<R>(method: string, path: string, body?: Body, signal?: AbortSignal, timeoutMs = DEFAULT_TIMEOUT_MS): Promise<R> {
    // A runtime that accepts the connection and then never answers would
    // otherwise leave a spinner up for good.
    const ctl = new AbortController()
    let timedOut = false
    const timer = setTimeout(() => {
      timedOut = true
      ctl.abort()
    }, timeoutMs)
    const onAbort = () => ctl.abort()
    if (signal?.aborted) ctl.abort()
    else signal?.addEventListener('abort', onAbort, { once: true })
    try {
      const res = await this.raw(method, path, body, ctl.signal)
      const text = await res.text()
      if (!text) return undefined as R
      try {
        return parseBody<R>(text)
      } catch {
        throw new ApiError(res.status, t('connection.notJSON'))
      }
    } catch (err) {
      if (timedOut) throw new ApiError(0, t('connection.timeout', { url: this.displayURL, s: Math.round(timeoutMs / 1000) }), true)
      throw err
    } finally {
      clearTimeout(timer)
      signal?.removeEventListener('abort', onAbort)
    }
  }

  // ---- connection -----------------------------------------------------------

  /** Verifies connectivity and the token. */
  ping(signal?: AbortSignal): Promise<void> {
    return this.json('GET', '/admin/api/state', undefined, signal)
  }

  state(signal?: AbortSignal): Promise<T.State> {
    return this.json('GET', '/admin/api/state', undefined, signal)
  }

  async catalog(signal?: AbortSignal): Promise<T.Catalog> {
    const r = await this.json<{ catalog: T.Catalog }>('GET', '/admin/api/studio/catalog', undefined, signal)
    return r.catalog
  }

  metrics(signal?: AbortSignal): Promise<T.Metrics> {
    return this.json('GET', '/admin/api/metrics', undefined, signal)
  }

  // ---- agents ---------------------------------------------------------------

  async agents(signal?: AbortSignal): Promise<T.AgentRow[]> {
    const r = await this.json<{ agents: T.AgentRow[] | null }>('GET', '/admin/api/studio/agents', undefined, signal)
    return r.agents ?? []
  }

  agent(name: string, signal?: AbortSignal): Promise<T.AgentDetail> {
    return this.json('GET', '/admin/api/studio/agents/' + enc(name), undefined, signal)
  }

  validateAgent(req: T.ValidateRequest, signal?: AbortSignal): Promise<T.ValidationResult> {
    return this.json('POST', '/admin/api/studio/agents/validate', req, signal)
  }

  putAgent(name: string, req: T.AgentRequest, signal?: AbortSignal): Promise<T.ApplyResult> {
    return this.json('PUT', '/admin/api/studio/agents/' + enc(name), req, signal)
  }

  deleteAgent(name: string, persist: boolean, force: boolean): Promise<void> {
    return this.json('DELETE', `/admin/api/studio/agents/${enc(name)}${query({ persist, force })}`)
  }

  // ---- playground -----------------------------------------------------------

  async threads(agent: string, signal?: AbortSignal): Promise<T.Thread[]> {
    const r = await this.json<{ threads: T.Thread[] | null }>(
      'GET',
      '/admin/api/studio/threads' + query({ agent }),
      undefined,
      signal,
    )
    return r.threads ?? []
  }

  async trace(agent: string, thread: string, signal?: AbortSignal): Promise<T.TraceEvent[]> {
    const r = await this.json<{ events: T.TraceEvent[] | null }>(
      'GET',
      `/admin/api/studio/trace/${enc(agent)}/${enc(thread)}`,
      undefined,
      signal,
    )
    return r.events ?? []
  }

  stopThread(agent: string, thread: string, signal?: AbortSignal): Promise<void> {
    return this.json('POST', `/admin/api/studio/threads/${enc(thread)}/stop${query({ agent })}`, {}, signal, 10_000)
  }

  /** Runs an agentflow loop-test YAML scenario with deterministic mocks. */
  async runLoopTest(req: T.LoopTestRequest, signal?: AbortSignal): Promise<T.LoopTestResult> {
    const r = await this.json<{ result: T.LoopTestResult }>('POST', '/admin/api/studio/loop/test', req, signal, 35_000)
    return r.result
  }

  /** Fetches a media blob by handle through the authenticated Studio endpoint. */
  async mediaBlob(handle: string, signal?: AbortSignal): Promise<Blob> {
    const res = await this.raw('GET', `/admin/api/studio/media/${enc(handle)}`, undefined, signal)
    return res.blob()
  }

  /**
   * Runs one playground turn and calls onEvent for every SSE event (trace,
   * reply, error, done). Resolves when the stream ends; abort the signal to
   * stop listening.
   */
  async chat(req: T.ChatRequest, onEvent: (ev: T.ChatEvent) => void, signal?: AbortSignal): Promise<void> {
    const res = await this.raw('POST', '/admin/api/studio/chat', req, signal, 'text/event-stream')
    await readSSE(res, (event, data) => {
      let ev: T.ChatEvent
      try {
        ev = JSON.parse(data) as T.ChatEvent
      } catch {
        throw new ApiError(0, t('connection.badEvent'))
      }
      if (!ev.type) ev.type = event
      onEvent(ev)
    })
  }

  // ---- channels -------------------------------------------------------------

  channels(signal?: AbortSignal): Promise<T.ChannelList> {
    return this.json('GET', '/admin/api/studio/channels', undefined, signal)
  }

  /** Dry-runs one draft; index -1 means a new channel. Returns the errors. */
  async validateChannel(index: number, channel: T.ChannelDraft): Promise<string[]> {
    const r = await this.json<{ ok: boolean; errors: string[] | null }>(
      'POST',
      '/admin/api/studio/channels/validate',
      { index, channel },
    )
    return r.errors ?? []
  }

  /** Creates (index < 0) or replaces a channel and persists the set. */
  saveChannel(index: number, channel: T.ChannelDraft, mtime: T.Mtime | undefined): Promise<T.ChannelList> {
    const body = { channel, mtime }
    if (index >= 0) return this.json('PUT', `/admin/api/studio/channels/${index}`, body)
    return this.json('POST', '/admin/api/studio/channels', body)
  }

  deleteChannel(index: number, mtime: T.Mtime | undefined): Promise<T.ChannelList> {
    return this.json('DELETE', `/admin/api/studio/channels/${index}${query({ mtime })}`)
  }

  // ---- models ---------------------------------------------------------------

  async models(signal?: AbortSignal): Promise<T.ModelList> {
    const r = await this.json<T.ModelList>('GET', '/admin/api/models', undefined, signal)
    return { models: r.models ?? [], file_error: r.file_error ?? '' }
  }

  putModel(name: string, draft: T.ModelDraft): Promise<void> {
    return this.json('PUT', '/admin/api/models/' + enc(name), draft)
  }

  deleteModel(name: string): Promise<void> {
    return this.json('DELETE', '/admin/api/models/' + enc(name))
  }

  testModel(name: string): Promise<T.ModelTest> {
    // A real one-token call: allow for the model's own timeout and retries.
    return this.json('POST', `/admin/api/models/${enc(name)}/test`, {}, undefined, 180_000)
  }

  persistModels(): Promise<void> {
    return this.json('POST', '/admin/api/models/persist', {})
  }

  revertModels(): Promise<void> {
    return this.json('POST', '/admin/api/models/revert', {})
  }

  // ---- safety ---------------------------------------------------------------

  safety(signal?: AbortSignal): Promise<T.SafetyList> {
    return this.json('GET', '/admin/api/studio/safety', undefined, signal)
  }

  async validateSafety(req: T.SafetyValidateRequest): Promise<string[]> {
    const r = await this.json<{ ok: boolean; errors: string[] | null }>(
      'POST',
      '/admin/api/studio/safety/validate',
      req,
    )
    return r.errors ?? []
  }

  saveSafetyProfile(name: string, create: boolean, profile: T.SafetyProfileDraft, mtime: T.Mtime | undefined): Promise<T.SafetyList> {
    const body = { profile, mtime }
    if (create) return this.json('POST', '/admin/api/studio/safety/profiles', body)
    return this.json('PUT', '/admin/api/studio/safety/profiles/' + enc(name), body)
  }

  deleteSafetyProfile(name: string, mtime: T.Mtime | undefined): Promise<T.SafetyList> {
    return this.json('DELETE', `/admin/api/studio/safety/profiles/${enc(name)}${query({ mtime })}`)
  }

  saveSafetyClassifier(
    name: string,
    create: boolean,
    classifier: T.SafetyClassifierDraft,
    mtime: T.Mtime | undefined,
  ): Promise<T.SafetyList> {
    const body = { classifier, mtime }
    if (create) return this.json('POST', '/admin/api/studio/safety/classifiers', body)
    return this.json('PUT', '/admin/api/studio/safety/classifiers/' + enc(name), body)
  }

  deleteSafetyClassifier(name: string, mtime: T.Mtime | undefined): Promise<T.SafetyList> {
    return this.json('DELETE', `/admin/api/studio/safety/classifiers/${enc(name)}${query({ mtime })}`)
  }

  saveSafetyLuaFilter(
    name: string,
    create: boolean,
    luaFilter: T.SafetyLuaFilterDraft,
    mtime: T.Mtime | undefined,
  ): Promise<T.SafetyList> {
    const body = { lua_filter: luaFilter, mtime }
    if (create) return this.json('POST', '/admin/api/studio/safety/lua-filters', body)
    return this.json('PUT', '/admin/api/studio/safety/lua-filters/' + enc(name), body)
  }

  deleteSafetyLuaFilter(name: string, mtime: T.Mtime | undefined): Promise<T.SafetyList> {
    return this.json('DELETE', `/admin/api/studio/safety/lua-filters/${enc(name)}${query({ mtime })}`)
  }

  // ---- memory ---------------------------------------------------------------

  memory(signal?: AbortSignal): Promise<T.MemoryList> {
    return this.json('GET', '/admin/api/studio/memory', undefined, signal)
  }

  async validateMemory(req: T.MemoryValidateRequest): Promise<string[]> {
    const r = await this.json<{ ok: boolean; errors: string[] | null }>('POST', '/admin/api/studio/memory/validate', req)
    return r.errors ?? []
  }

  saveMemoryProfile(name: string, create: boolean, profile: T.MemoryProfileDraft, mtime: T.Mtime | undefined): Promise<T.MemoryList> {
    const body = { profile, mtime }
    if (create) return this.json('POST', '/admin/api/studio/memory/profiles', body)
    return this.json('PUT', '/admin/api/studio/memory/profiles/' + enc(name), body)
  }

  deleteMemoryProfile(name: string, mtime: T.Mtime | undefined): Promise<T.MemoryList> {
    return this.json('DELETE', `/admin/api/studio/memory/profiles/${enc(name)}${query({ mtime })}`)
  }

  saveMemoryBackend(name: string, create: boolean, backend: T.MemoryBackendDraft, mtime: T.Mtime | undefined): Promise<T.MemoryList> {
    const body = { backend, mtime }
    if (create) return this.json('POST', '/admin/api/studio/memory/backends', body)
    return this.json('PUT', '/admin/api/studio/memory/backends/' + enc(name), body)
  }

  deleteMemoryBackend(name: string, mtime: T.Mtime | undefined): Promise<T.MemoryList> {
    return this.json('DELETE', `/admin/api/studio/memory/backends/${enc(name)}${query({ mtime })}`)
  }

  // ---- credentials ----------------------------------------------------------

  /** Lists one bucket; an empty user is the engine-wide bucket. */
  credentials(user: string, signal?: AbortSignal): Promise<T.CredentialList> {
    return this.json('GET', '/admin/api/credentials' + query({ user }), undefined, signal)
  }

  putCredential(draft: T.CredentialDraft): Promise<T.CredentialList> {
    return this.json('POST', '/admin/api/credentials', draft)
  }

  deleteCredential(user: string, service: string): Promise<T.CredentialList> {
    return this.json('DELETE', '/admin/api/credentials' + query({ service, user }))
  }

  // ---- logs -----------------------------------------------------------------

  /**
   * Streams the runtime's log tail: the ring is replayed first, then live
   * lines arrive until the signal aborts or the runtime closes the stream.
   * onOpen fires once the runtime has accepted the stream, which is the only
   * way to tell "connected" from "connecting" on a runtime with nothing to say.
   */
  async logs(onLine: (line: string) => void, signal?: AbortSignal, onOpen?: () => void): Promise<void> {
    const res = await this.raw('GET', '/admin/api/logs', undefined, signal, 'text/event-stream')
    onOpen?.()
    await readSSE(res, (_event, data) => onLine(data))
  }

  // ---- raw config -----------------------------------------------------------

  config(signal?: AbortSignal, file?: string): Promise<T.ConfigFile> {
    return this.json('GET', '/admin/api/config' + query({ file }), undefined, signal)
  }

  /** Boot-equivalent validation; a refusal is ok=false, not a thrown error. */
  validateConfig(raw: string, file?: string): Promise<T.ConfigResult> {
    return this.json('POST', '/admin/api/config/validate', file ? { raw, file } : { raw })
  }

  /** Validates, backs up to .bak and atomically replaces the file. 409 = changed on disk. */
  saveConfig(raw: string, mtime: number, file?: string): Promise<T.ConfigResult> {
    return this.json('POST', '/admin/api/config/save', file ? { raw, mtime, file } : { raw, mtime })
  }

  // ---- users ----------------------------------------------------------------

  users(signal?: AbortSignal): Promise<T.UserList> {
    return this.json('GET', '/admin/api/users', undefined, signal)
  }

  user(id: string, signal?: AbortSignal): Promise<T.UserDetail> {
    return this.json('GET', '/admin/api/users/' + enc(id), undefined, signal)
  }

  /** Sets the daily token limit (0 inherits the default). */
  async setUserLimit(id: string, tokensPerDay: number): Promise<T.Profile> {
    const r = await this.json<{ profile: T.Profile }>('POST', `/admin/api/users/${enc(id)}/limit`, {
      tokens_per_day: tokensPerDay,
    })
    return r.profile
  }

  async setUserSettings(id: string, settings: T.UserSettings): Promise<T.Profile> {
    const r = await this.json<{ profile: T.Profile }>('PUT', `/admin/api/users/${enc(id)}/settings`, settings)
    return r.profile
  }

  /** Detaches one channel handle; the runtime refuses the last one. */
  async unlinkIdentity(id: string, identityID: string): Promise<T.Profile> {
    const r = await this.json<{ profile: T.Profile }>('POST', `/admin/api/users/${enc(id)}/unlink`, {
      identity_id: identityID,
    })
    return r.profile
  }

  issueInvite(): Promise<T.Invite> {
    return this.json('POST', '/admin/api/users/invites', {})
  }

  /** Everything held for a profile, as pretty-printed JSON text. */
  async exportUser(id: string): Promise<string> {
    const res = await this.raw('GET', `/admin/api/users/${enc(id)}/export`)
    const text = await res.text()
    try {
      return JSON.stringify(JSON.parse(text), null, 2)
    } catch {
      return text
    }
  }

  /** Full-fidelity regional migration bundle (profile, memory, files, credentials). */
  async exportUserBundle(id: string): Promise<string> {
    const res = await this.raw('GET', `/admin/api/users/${enc(id)}/bundle`)
    return res.text()
  }

  /** Imports a previously exported migration bundle into this instance. */
  importUserBundle(bundle: unknown): Promise<{ ok: boolean; user_id?: string }> {
    return this.json('POST', '/admin/api/users/import', bundle)
  }

  /** Purges a profile. The runtime requires ?confirm=<id>. */
  deleteUser(id: string): Promise<void> {
    return this.json('DELETE', `/admin/api/users/${enc(id)}${query({ confirm: id })}`)
  }

  // ---- usage ----------------------------------------------------------------

  /** The token rollup for one UTC day (YYYY-MM-DD; empty = today). */
  usage(day: string, signal?: AbortSignal): Promise<T.UsageDay> {
    return this.json('GET', '/admin/api/usage' + query({ day }), undefined, signal)
  }

  // ---- tenancy --------------------------------------------------------------

  async groups(signal?: AbortSignal): Promise<T.Group[]> {
    const r = await this.json<{ groups: T.Group[] | null }>('GET', '/admin/api/groups', undefined, signal)
    return r.groups ?? []
  }

  group(id: string, signal?: AbortSignal): Promise<T.GroupDetail> {
    return this.json('GET', '/admin/api/groups/' + enc(id), undefined, signal)
  }

  /** Creates a group whose master is the creator's personal uuid. */
  async createGroup(name: string, creator: string): Promise<T.Group> {
    const r = await this.json<{ group: T.Group }>('POST', '/admin/api/groups', { name, creator })
    return r.group
  }

  addMember(group: string, user: string): Promise<void> {
    return this.json('POST', `/admin/api/groups/${enc(group)}/members`, { user })
  }

  removeMember(group: string, user: string): Promise<void> {
    return this.json('DELETE', `/admin/api/groups/${enc(group)}/members/${enc(user)}`)
  }

  /** Hands the group from the current master (from) to a member. */
  setMaster(group: string, from: string, user: string): Promise<void> {
    return this.json('PUT', `/admin/api/groups/${enc(group)}/master`, { from, user })
  }

  /** Ends access now; by must be the current master. */
  disbandGroup(group: string, by: string): Promise<void> {
    return this.json('DELETE', '/admin/api/groups/' + enc(group), { by })
  }

  async pools(signal?: AbortSignal): Promise<T.Pool[]> {
    const r = await this.json<{ pools: T.Pool[] | null }>('GET', '/admin/api/pools', undefined, signal)
    return r.pools ?? []
  }

  async createPool(draft: T.PoolDraft): Promise<T.Pool> {
    const r = await this.json<{ pool: T.Pool }>('POST', '/admin/api/pools', draft)
    return r.pool
  }

  async grants(pool: string, signal?: AbortSignal): Promise<T.Grant[]> {
    const r = await this.json<{ grants: T.Grant[] | null }>(
      'GET',
      `/admin/api/pools/${enc(pool)}/grants`,
      undefined,
      signal,
    )
    return r.grants ?? []
  }

  /** Gives agent access to the pool for tenant ("" = every member) in mode "rw" or "r". */
  setGrant(pool: string, agent: string, tenant: string, mode: string): Promise<void> {
    return this.json('PUT', `/admin/api/pools/${enc(pool)}/grants`, { agent, tenant, mode })
  }

  removeGrant(pool: string, agent: string, tenant: string): Promise<void> {
    // tenant is sent even when empty: "" means the every-member grant.
    return this.json('DELETE', `/admin/api/pools/${enc(pool)}/grants?agent=${enc(agent)}&tenant=${enc(tenant)}`)
  }

  // ---- shared & built-in Lua plugins ---------------------------------------

  plugins(signal?: AbortSignal): Promise<T.PluginsList> {
    return this.json('GET', '/admin/api/studio/plugins', undefined, signal)
  }

  async validatePlugin(req: T.PluginValidateRequest): Promise<string[]> {
    const r = await this.json<T.PluginValidateResult>('POST', '/admin/api/studio/plugins/validate', req)
    return r.errors ?? []
  }

  saveBuiltinPlugin(name: string, source: string, mtime?: T.Mtime): Promise<T.PluginsList> {
    return this.json('PUT', `/admin/api/studio/plugins/builtins/${enc(name)}`, { source, mtime: mtime ?? ('0' as T.Mtime) })
  }

  resetBuiltinPlugin(name: string, mtime?: T.Mtime): Promise<T.PluginsList> {
    return this.json('DELETE', `/admin/api/studio/plugins/builtins/${enc(name)}${query({ mtime })}`)
  }

  createLuaPackage(name: string, file?: string, source?: string): Promise<T.PluginsList> {
    return this.json('POST', '/admin/api/studio/plugins/packages', { name, file, source })
  }

  deleteLuaPackage(name: string): Promise<T.PluginsList> {
    return this.json('DELETE', `/admin/api/studio/plugins/packages/${enc(name)}`)
  }

  savePackageFile(pkg: string, file: string, source: string, mtime?: T.Mtime): Promise<T.PluginsList> {
    return this.json('PUT', `/admin/api/studio/plugins/packages/${enc(pkg)}/files/${enc(file)}`, {
      source,
      mtime: mtime ?? ('0' as T.Mtime),
    })
  }

  deletePackageFile(pkg: string, file: string, mtime?: T.Mtime): Promise<T.PluginsList> {
    return this.json('DELETE', `/admin/api/studio/plugins/packages/${enc(pkg)}/files/${enc(file)}${query({ mtime })}`)
  }
}

/**
 * Reads a text/event-stream body and calls onEvent(event, data) per event.
 * EventSource cannot send an Authorization header, so streams are read from
 * fetch directly. Comment lines (keepalives) are skipped.
 */
export async function readSSE(res: Response, onEvent: (event: string, data: string) => void): Promise<void> {
  if (!res.body) return
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  let event = ''
  let data: string[] = []
  const dispatch = () => {
    if (data.length > 0) onEvent(event, data.join('\n'))
    event = ''
    data = []
  }
  const feed = (line: string) => {
    if (line.endsWith('\r')) line = line.slice(0, -1)
    if (line === '') dispatch()
    else if (line.startsWith(':')) return
    else if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''))
  }
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      let nl: number
      while ((nl = buf.indexOf('\n')) >= 0) {
        feed(buf.slice(0, nl))
        buf = buf.slice(nl + 1)
      }
    }
    if (buf !== '') feed(buf)
    dispatch()
  } finally {
    reader.releaseLock()
  }
}

/**
 * Asks this origin whether Studio's local proxy is serving it, and which
 * shell serves the page: "wails" for the desktop app, empty for a browser.
 */
export async function probeProxy(): Promise<{ available: boolean; platform: string; shell: string; frameless: boolean; version: string }> {
  const none = { available: false, platform: '', shell: '', frameless: false, version: '' }
  try {
    const res = await fetch(PROXY_PREFIX + '/__ping', { cache: 'no-store' })
    if (!res.ok) return none
    const body = await res.json()
    return {
      available: body?.proxy === true,
      platform: String(body?.platform ?? ''),
      shell: String(body?.shell ?? ''),
      // The desktop app without a native title bar: the page draws the window controls.
      frameless: body?.frameless === true,
      version: String(body?.version ?? ''),
    }
  } catch {
    return none
  }
}

/**
 * Reports whether err is the runtime saying a subsystem is switched off
 * (503), and turns the message into guidance.
 */
export function featureOff(err: unknown): string | null {
  if (!(err instanceof ApiError) || err.status !== 503) return null
  let msg = err.message
  if (msg.includes('identity')) {
    msg = t('common.featureOffIdentity', { message: msg })
  } else if (msg.includes('tenancy')) {
    msg = t('common.featureOffTenancy', { message: msg })
  }
  return msg
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}
