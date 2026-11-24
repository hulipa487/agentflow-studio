// Wire types for AgentFlow's /admin/api surface; the JSON keys are the
// runtime's. Lists can be null (a Go nil slice), and fields marked
// optional are ones the runtime omits when empty.

/**
 * A file's modification stamp, echoed back on a write so the runtime can
 * refuse it if the file changed in between. Channels and safety send it in
 * nanoseconds, which is beyond what a JavaScript number holds exactly, so the
 * client keeps it as the digits it received. Treat it as opaque.
 */
export type Mtime = string

export interface TemplateSetting {
  key: string
  label: string
  description?: string
  type: 'bool' | 'number' | 'string' | 'select'
  default: unknown
  min?: number
  max?: number
  placeholder?: string
  options?: string[]
}

export interface Template {
  name: string
  title: string
  description: string
  capabilities?: string[]
  memory?: string
  instructions?: string
  extras?: Record<string, unknown>
  settings?: TemplateSetting[]
  loop: string
}

export interface Catalog {
  models: string[] | null
  tools: string[] | null
  capabilities: string[] | null
  default_capabilities: string[] | null
  safety_profiles: string[] | null
  memory_profiles: string[] | null
  shell_profiles?: string[] | null
  ssh_profiles?: string[] | null
  environments?: string[] | null
  packages?: string[] | null
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
  ssh?: string
  /** The memory profile the agent names; "conversational" is the built-in one. */
  memory?: string
  /** An inline memory profile, as the config spells it. The builder keeps it as it is. */
  memory_inline?: Record<string, unknown> | null
  skills: string[] | null
  capabilities: string[] | null
  can_contact: string[] | null
  persistent: boolean
  credentials: string[] | null
  inherit_user?: boolean | null
  instructions_prompt?: string
  budget?: Record<string, unknown> | null
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

export interface MediaPart {
  type: string
  text?: string
  mime?: string
  data?: string
  url?: string
  handle?: string
  name?: string
  size?: number
}

export interface Reply {
  text: string
  status: string
  /** Media attachments sent with the reply (typically handle-only descriptors). */
  attachments?: MediaPart[]
}

export interface ChatResult {
  thread: string
  session: string
  replies: Reply[] | null
  events: number
  timed_out: boolean
}

/** Runtime-owned redacted approval prompt streamed when a gated action needs confirmation. */
export interface ApprovalPrompt {
  id: string
  session_key: string
  agent: string
  action: string
  destination?: string
  description?: string
  args_preview?: Record<string, unknown>
  expires_at: number
}

export type ApprovalStatus =
  | 'pending'
  | 'approved'
  | 'denied'
  | 'claimed'
  | 'expired'
  | 'succeeded'
  | 'failed'
  | 'uncertain'

export interface ApprovalRecord extends ApprovalPrompt {
  status: ApprovalStatus
  created_at: number
  decided_at?: number
  decided_by?: string
}

/** One SSE event of a playground turn: trace, reply, approval, error or done. */
export interface ChatEvent {
  type: string
  trace?: TraceEvent
  reply?: Reply
  approval?: ApprovalPrompt
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

export interface LoopTestAssertion {
  kind: string
  index: number
  expected: string
  actual?: string
  ok: boolean
  err?: string
}

export interface LoopTestResult {
  ok: boolean
  error?: string
  agent?: string
  loop?: string
  model?: string
  duration_ms: number
  sends: string[]
  ops: string[]
  assertions: LoopTestAssertion[]
  events: TraceEvent[]
}

export interface LoopTestRequest {
  yaml: string
  agent?: string
  loop_source?: string
  instructions?: string
  model?: string
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
  attachments?: MediaPart[]
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
  stream?: boolean | null
  thinking: string
  server_tools: string[] | null
  key_source?: string
  credential?: string
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
  stream?: boolean | null
  thinking: string
  server_tools: string[]
  key_source?: string
  credential?: string
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
  script_filters?: string[] | null
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

export interface SafetyScriptFilterDraft {
  name: string
  source?: string
  file?: string
  directions?: string[]
  sources?: string[]
  timeout?: string
  instr_budget?: number
  on_error?: string
  refusal?: string
  fallback?: string
}

export interface SafetyScriptFilter extends SafetyScriptFilterDraft {
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
  script_filters?: SafetyScriptFilter[] | null
  models: string[] | null
  prompts: string[] | null
  note: string
}

export interface SafetyProfileDraft {
  name: string
  filters: string[]
}

export interface SafetyValidateRequest {
  kind: 'profile' | 'classifier' | 'script_filter'
  creating: boolean
  profile?: SafetyProfileDraft
  classifier?: SafetyClassifierDraft
  script_filter?: SafetyScriptFilterDraft
}

export interface MemoryStoreDraft {
  name: string
  backend: string
  table?: string
  retention?: string
  window?: number
  requires?: string[]
  scope?: string
  pool?: string
  locality_ok?: boolean
}

export interface MemoryProfileDraft {
  name: string
  stores: MemoryStoreDraft[]
  write?: string[]
  recall?: string
  embed_model?: string
  rerank_model?: string
  oversample?: number
}

export interface MemoryProfile extends MemoryProfileDraft {
  agents: string[] | null
  spawn_profiles: string[] | null
  live: boolean
}

/** A backend draft. url and api_key are write only: empty keeps the stored value. */
export interface MemoryBackendDraft {
  name: string
  provider: string
  path?: string
  url?: string
  clear_url?: boolean
  api_key?: string
  clear_api_key?: boolean
  collection?: string
  distance?: string
  timeout?: string
  dim?: number
  vector_scan_limit?: number
  max_entries?: number
}

/** A backend as the runtime shows it: secrets only as has_*, a reference, or a masked URL. */
export interface MemoryBackend {
  name: string
  provider: string
  path?: string
  collection?: string
  distance?: string
  timeout?: string
  dim?: number
  vector_scan_limit?: number
  max_entries?: number
  has_url: boolean
  url_ref?: string
  url_display?: string
  has_api_key: boolean
  api_key_ref?: string
  other_keys: string[] | null
  profiles: string[] | null
  agents: string[] | null
  /** The built-in main_db on the runtime store; not in the file. */
  implicit: boolean
  live: boolean
}

export interface MemoryList {
  ok: boolean
  restart_required: boolean
  saved_note: string
  mode: string
  path: string
  editable: boolean
  mtime?: Mtime
  providers: string[] | null
  backends: MemoryBackend[] | null
  profiles: MemoryProfile[] | null
  /** Agents on the built-in conversational profile. */
  builtin_agents: string[] | null
  models: string[] | null
  rerank_models: string[] | null
  write_plugins: string[] | null
  recall_plugins: string[] | null
  requires: string[] | null
  note: string
}

export interface MemoryValidateRequest {
  kind: 'profile' | 'backend'
  creating: boolean
  profile?: MemoryProfileDraft
  backend?: MemoryBackendDraft
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

/** The raw config text (single file or selected config-directory fragment). */
export interface ConfigFile {
  ok: boolean
  error: string
  mode?: string
  file?: string
  files?: string[]
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

// ---- shared & built-in TypeScript/JS plugins --------------------------------

export interface BuiltinPlugin {
  name: string
  kind: 'support' | 'route'
  summary: string
  shadowed: boolean
  path?: string
  mtime?: Mtime
  source: string
  builtin_source: string
}

export interface PackageFile {
  name: string
  path: string
  mtime?: Mtime
  source: string
}

export interface ScriptPackage {
  name: string
  dir: string
  managed: boolean
  files: PackageFile[] | null
}

export interface PluginsList {
  ok: boolean
  mode: string
  editable: boolean
  config_path?: string
  plugin_dir?: string
  plugin_dir_configured: boolean
  prelude_version: number
  builtins: BuiltinPlugin[] | null
  packages: ScriptPackage[] | null
  note?: string
}

export interface PluginValidateRequest {
  kind: 'builtin' | 'package'
  name: string
  file?: string
  source: string
}

export interface PluginValidateResult {
  ok: boolean
  errors: string[] | null
}

export interface TelegramBotInfo {
  bot_id: number
  username: string
  first_name?: string
  can_join_groups: boolean
  can_read_all_group_messages: boolean
  privacy_mode_enabled: boolean
}

export type StudioEventOutcome = 'answered' | 'ignored' | 'dropped' | 'failed' | 'received'

export interface StudioEvent {
  id: string
  ts: number
  direction: string
  status: string
  channel: string
  chat: string
  sender: string
  user_uuid?: string
  agent: string
  session_id?: string
  type?: string
  text?: string
  text_bytes?: number
  attachment_bytes?: number
  err?: string
  has_trace: boolean
  outcome: StudioEventOutcome
}

export interface StudioEventFilter {
  channel?: string
  agent?: string
  chat?: string
  session?: string
  direction?: string
  status?: string
  limit?: number
}

export interface SessionTraceResponse {
  ok: boolean
  session: string
  events: TraceEvent[]
}

/** The runtime's billing rule: input + output. */
export function billable(t: Pick<UsageTotals, 'input' | 'output'>): number {
  return (t.input || 0) + (t.output || 0)
}
