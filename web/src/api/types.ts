// Wire types for AgentFlow's /admin/api surface. They mirror the desktop
// Studio's internal/api (types.go, operate.go) field for field; the JSON keys
// are the runtime's. Lists can be null (a Go nil slice), and fields marked
// optional are ones the runtime omits when empty.

/**
 * A file's modification stamp, echoed back on a write so the runtime can
 * refuse it if the file changed in between. Channels and safety send it in
 * nanoseconds, which is beyond what a JavaScript number holds exactly, so the
 * client keeps it as the digits it received. Treat it as opaque.
 */
export type Mtime = string

export interface Template {
  name: string
  title: string
  description: string
  loop: string
}

export interface Catalog {
  models: string[] | null
  tools: string[] | null
  capabilities: string[] | null
  default_capabilities: string[] | null
  safety_profiles: string[] | null
  memory_profiles: string[] | null
  prompt_keys: string[] | null
  prelude_version: number
  templates: Template[] | null
}

export interface AgentRow {
  name: string
  model: string
  loop: string
  spawn: boolean
  persistent: boolean
  capabilities: string[] | null
  persisted: boolean
  sessions: number
}

export interface AgentJSON {
  loop: string
  model: string
  history_budget: number
  safety: string
  shell?: string
  skills: string[] | null
  capabilities: string[] | null
  can_contact: string[] | null
  persistent: boolean
  credentials: string[] | null
  inherit_user?: boolean | null
  extras?: Record<string, unknown> | null
}

export interface AgentFiles {
  mode: string
  profile_path: string
  loop_path: string
  instructions_path: string
  managed: boolean
  mtime?: number
}

export interface AgentDetail {
  ok: boolean
  name: string
  source: string
  agent_yaml: string
  agent: AgentJSON
  loop: string
  instructions: string
  files: AgentFiles
  persisted: boolean
  spawn: boolean
  persistent: boolean
  model: string
  capabilities: string[] | null
  skills: string[] | null
  sessions: number
}

export interface TraceEvent {
  seq: number
  ts: string
  agent: string
  session: string
  thread?: string
  turn?: string
  kind: string
  op?: string
  tool?: string
  model?: string
  ok: boolean
  err?: string
  duration_ms?: number
  tokens_in?: number
  tokens_out?: number
  tokens_reasoning?: number
  tool_calls?: number
  messages?: number
  text?: string
  /** The safety decision attached to this event, when there was one. */
  safety?: unknown
}

export interface Reply {
  text: string
  status: string
  /** Media the agent sent with the reply; the playground only counts them. */
  attachments?: unknown[]
}

export interface ChatResult {
  thread: string
  session: string
  replies: Reply[] | null
  events: number
  timed_out: boolean
}

/** One SSE event of a playground turn: trace, reply, error or done. */
export interface ChatEvent {
  type: string
  trace?: TraceEvent
  reply?: Reply
  done?: ChatResult
  error?: string
}

export interface Thread {
  agent: string
  thread: string
  session: string
  last: string
  events: number
}

export interface ValidationResult {
  ok: boolean
  errors: string[] | null
  warnings: string[] | null
}

export interface ApplyResult {
  ok: boolean
  applied: boolean
  persisted?: boolean
  files?: AgentFiles
  note?: string
  persist_error?: string
}

export interface ChatRequest {
  agent: string
  thread: string
  text: string
  act_as_user?: string
}

export interface AgentRequest {
  agent_yaml: string
  loop: string
  instructions: string
  apply: boolean
  persist: boolean
  force: boolean
  expect_mtime: number
}

export interface ValidateRequest {
  name: string
  agent_yaml: string
  loop: string
  instructions: string
}

/** A gateway channel. Secrets are write-only: has_* says one is stored. */
export interface Channel {
  index: number
  name: string
  type: string
  agent: string
  /** The agents an openai channel exposes as model ids. */
  agents?: string[]
  mode: string
  path: string
  timeout: string
  async: boolean
  deliver_url: string
  app_id?: string
  scenes?: string[]
  api_base?: string
  markdown?: boolean
  /** openai: how callers authenticate (key, user_token or none). */
  auth?: string
  max_request_bytes?: number
  expose_reasoning?: boolean
  /** What the audit journal keeps (full, metadata or off); empty is the type's default. */
  journal?: string
  allow_users: number[] | null
  media_allow: string[] | null
  media_max_bytes: number
  has_token: boolean
  has_secret: boolean
  has_secret_token: boolean
  has_keys?: boolean
  token_ref: string
  secret_ref: string
  secret_token_ref: string
  /** The ${ENV} / cred: references among an openai channel's keys. */
  key_refs?: string[]
  live: boolean
}

export interface ChannelList {
  ok: boolean
  restart_required: boolean
  saved_note: string
  mode: string
  path: string
  editable: boolean
  mtime?: Mtime
  channels: Channel[] | null
  note: string
}

/** Editable channel shape. An empty secret keeps the stored one; clear_* removes it. */
export interface ChannelDraft {
  name: string
  type: string
  agent: string
  agents?: string[]
  mode?: string
  path?: string
  token?: string
  secret?: string
  secret_token?: string
  clear_token?: boolean
  clear_secret?: boolean
  clear_secret_token?: boolean
  auth?: string
  /** openai shared keys. Empty keeps the stored ones; clear_keys removes them. */
  keys?: string[]
  clear_keys?: boolean
  max_request_bytes?: number
  expose_reasoning?: boolean
  journal?: string
  allow_users?: number[]
  timeout?: string
  async?: boolean
  deliver_url?: string
  app_id?: string
  scenes?: string[]
  api_base?: string
  markdown?: boolean
  media_allow?: string[]
  media_max_bytes?: number
}

export interface Model {
  name: string
  provider: string
  model: string
  base_url: string
  timeout: string
  retry: number
  max_tokens: number
  thinking: string
  server_tools: string[] | null
  has_key: boolean
  key_fingerprint: string
  in_runtime: boolean
  in_file: boolean
  drift: boolean
}

/** Editable model shape. An empty api_key on an existing model keeps the stored key. */
export interface ModelDraft {
  provider: string
  model: string
  api_key: string
  base_url: string
  timeout: string
  retry: number
  max_tokens: number
  thinking: string
  server_tools: string[]
}

export interface ModelTest {
  ok: boolean
  latency_ms: number
  class: string
  error: string
}

export interface ModelList {
  models: Model[] | null
  file_error?: string
}

export interface SafetyProfile {
  name: string
  filters: string[] | null
  agents: string[] | null
  spawn_profiles: string[] | null
  classifiers: string[] | null
  live: boolean
}

export interface SafetyClassifierCache {
  size?: number
  ttl?: string
}

export interface SafetyClassifierBudget {
  tokens_per_day?: number
}

export interface SafetyClassifierDraft {
  name: string
  model: string
  prompt?: string
  mode?: string
  directions?: string[]
  sources?: string[]
  /** Omitted keeps the runtime default (0.7). */
  threshold?: number
  categories: Record<string, string>
  timeout?: string
  on_error?: string
  on_saturated?: string
  on_budget_exhausted?: string
  max_tokens?: number
  max_input_chars?: number
  max_concurrency?: number
  cache: SafetyClassifierCache
  budget: SafetyClassifierBudget
  refusal?: string
  fallback?: string
}

export interface SafetyClassifier extends SafetyClassifierDraft {
  used_by: string[] | null
  live: boolean
}

export interface SafetyList {
  ok: boolean
  restart_required: boolean
  saved_note: string
  mode: string
  path: string
  editable: boolean
  mtime?: Mtime
  builtin_filters: string[] | null
  profiles: SafetyProfile[] | null
  classifiers: SafetyClassifier[] | null
  models: string[] | null
  prompts: string[] | null
  note: string
}

export interface SafetyProfileDraft {
  name: string
  filters: string[]
}

export interface SafetyValidateRequest {
  kind: 'profile' | 'classifier'
  creating: boolean
  profile?: SafetyProfileDraft
  classifier?: SafetyClassifierDraft
}

/** One stored secret's metadata. The value is never sent. */
export interface Credential {
  service: string
  kind: string
  header?: string
  scheme?: string
  fingerprint?: string
  created_at: number
  updated_at: number
}

export interface CredentialList {
  ok: boolean
  enabled: boolean
  bucket: string
  users: string[] | null
  credentials: Credential[] | null
  note: string
}

export interface CredentialDraft {
  user: string
  service: string
  kind: string
  secret: string
  header: string
  scheme: string
}

// ---- operator side ----------------------------------------------------------

export interface StateAgent {
  name: string
  model: string
  loop: string
  persistent: boolean
  singleton: boolean
}

export interface StateChannel {
  name: string
  type: string
  agent: string
  path: string
  media_enabled: boolean
  index: number
}

export interface StateSession {
  session_id: string
  agent: string
  parent_id: string
  busy: boolean
}

export interface StateInstance {
  owner: string
  region: string
  zone: string
  version: string
  topology: string
  config_epoch: string
  config_epoch_differs: boolean
  started_at: number
  last_seen: number
  self: boolean
}

export interface State {
  ok: boolean
  version: string
  started_at: number
  uptime_s: number
  config_path: string
  config_epoch: string
  instance: string
  region: string
  topology: string
  zone: string
  instances: StateInstance[] | null
  agents: StateAgent[] | null
  channels: StateChannel[] | null
  sessions: StateSession[] | null
  sessions_active: number
  sessions_idle: number
  credentials_enabled: boolean
}

export interface MetricPoint {
  at: number
  v: number
}

export interface Metrics {
  ok: boolean
  series: Record<string, MetricPoint[]> | null
  latest: Record<string, number> | null
}

/** The raw config text. ok is false (with error) for a config-directory instance. */
export interface ConfigFile {
  ok: boolean
  error: string
  path: string
  raw: string
  mtime: number
  view: unknown
}

export interface ConfigResult {
  ok: boolean
  error: string
  mtime: number
  note: string
}

export interface Quota {
  used: number
  limit: number
  in_flight: number
}

export interface UserRow {
  user_id: string
  display_name: string
  email: string
  tokens_per_day: number
  created_at: number
  updated_at: number
  identities: number
  quota: Quota | null
}

export interface UserList {
  day: string
  users: UserRow[] | null
}

export interface Identity {
  id: string
  user_id: string
  channel: string
  native_from: string
  reply_to: string
  username?: string
  name?: string
  trust: string
  deliverable: boolean
  linkable: boolean
  first_seen: number
  last_seen: number
}

export interface Profile {
  user_id: string
  display_name: string
  email: string
  tokens_per_day: number
  model?: string
  instructions_append?: string
  created_at: number
  updated_at: number
  identities: Identity[] | null
}

export interface UsageTotals {
  input: number
  output: number
  cached: number
  cache_write: number
  reasoning: number
  calls: number
  failed: number
}

export interface UsageEvent extends UsageTotals {
  ts: number
  kind: string
  model: string
  agent: string
  ok: boolean
}

export interface JournalEntry {
  id: string
  ts: number
  direction: string
  status: string
  channel: string
  chat: string
  sender: string
  user_uuid: string
  agent?: string
  session_id: string
  type: string
  text?: string
  err?: string
}

export interface UserDetail {
  profile: Profile
  day: string
  usage_today: UsageTotals | null
  recent_calls: UsageEvent[] | null
  audit: JournalEntry[] | null
  quota: Quota | null
  projects: string[] | null
}

export interface Invite {
  invite: string
  expires_in: string
  hint: string
}

/** undefined leaves a field unchanged; an empty string clears it. */
export interface UserSettings {
  model?: string
  instructions_append?: string
}

export interface UsageRow extends UsageTotals {
  user_id: string
  day: string
  agent: string
  model: string
}

export interface UsageDay {
  ok: boolean
  day: string
  rows: UsageRow[] | null
  totals: { input: number; output: number; billable: number; calls: number }
  tenants: number
}

export interface Group {
  uuid: string
  name: string
  master: string
  created_at: number
  disbanded_at?: number
}

export interface Membership {
  group: string
  user: string
  joined_at: number
}

export interface GroupDetail {
  group: Group
  members: Membership[] | null
}

export interface Pool {
  uuid: string
  name: string
  owner: { tenant?: string; group?: string }
  retention_days: number
  created_at: number
  created_by: string
}

export interface Grant {
  pool: string
  agent: string
  tenant: string
  mode: string
}

export interface PoolDraft {
  name: string
  creator: string
  owner_tenant?: string
  owner_group?: string
  retention_days: number
}

/** The runtime's billing rule: input + output. */
export function billable(t: Pick<UsageTotals, 'input' | 'output'>): number {
  return (t.input || 0) + (t.output || 0)
}
