// Channel helpers shared by the list and the editor: titles, the form model
// and the draft the API takes. The rules mirror build() in the desktop app's
// channels.go so both clients send the same thing.
import type { Channel, ChannelDraft } from '@/api/types'
import { splitList } from '@/lib/format'

export const CHANNEL_TYPES = ['webhook', 'telegram', 'ghhook', 'bridge', 'openai']
export const TELEGRAM_MODES = ['polling', 'webhook', 'auto']
export const OPENAI_AUTH_MODES = ['key', 'user_token', 'none']
export const JOURNAL_MODES = ['metadata', 'full', 'off']

/** The agents a channel routes to: an openai channel may expose several. */
export function channelAgents(ch: Channel): string {
  const agents = ch.agents ?? []
  return agents.length > 0 ? agents.join(', ') : ch.agent
}

export function channelTitle(ch: Channel): string {
  return ch.name ? ch.name : `${ch.type} #${ch.index + 1}`
}

/** "8 MiB" for a byte limit; exact multiples stay exact, the rest get one decimal. */
export function humanBytes(n: number): string {
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let v = n
  let u = 0
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024
    u++
  }
  return `${Number.isInteger(v) ? v : v.toFixed(1)} ${units[u]}`
}

/** What the form holds: every control's text, whatever the selected type shows. */
export interface ChannelForm {
  name: string
  type: string
  agent: string
  path: string
  mode: string
  token: string
  secret: string
  secretToken: string
  clearToken: boolean
  clearSecret: boolean
  clearSecretToken: boolean
  allowUsers: string
  timeout: string
  deliverURL: string
  async: boolean
  mediaAllow: string
  mediaMax: string
  // openai
  agents: string
  auth: string
  keys: string
  clearKeys: boolean
  journal: string
  maxRequestBytes: string
  exposeReasoning: boolean
}

export function formFor(existing: Channel | null, agents: string[]): ChannelForm {
  const form: ChannelForm = {
    name: '',
    type: 'webhook',
    agent: agents[0] ?? '',
    path: '',
    mode: 'polling',
    token: '',
    secret: '',
    secretToken: '',
    clearToken: false,
    clearSecret: false,
    clearSecretToken: false,
    allowUsers: '',
    timeout: '',
    deliverURL: '',
    async: false,
    mediaAllow: '',
    mediaMax: '',
    agents: '',
    auth: 'key',
    keys: '',
    clearKeys: false,
    journal: 'metadata',
    maxRequestBytes: '',
    exposeReasoning: false,
  }
  if (!existing) return form
  // A file may use the agent: shorthand for an openai channel's single agent.
  // Without the fallback the list would open empty and saving the form
  // unchanged would be refused for having no agents.
  const exposed = existing.agents ?? []
  const agentList = exposed.length > 0 ? exposed.join(', ') : existing.type === 'openai' ? (existing.agent ?? '') : ''
  // Optional fields are omitted on the wire when empty, hence the fallbacks.
  // A type, agent or mode the form cannot offer is left unselected so saving
  // asks for a choice instead of silently picking one.
  return {
    ...form,
    name: existing.name ?? '',
    type: CHANNEL_TYPES.includes(existing.type) ? existing.type : '',
    agent: agents.includes(existing.agent) ? existing.agent : '',
    path: existing.path ?? '',
    mode: TELEGRAM_MODES.includes(existing.mode) ? existing.mode : 'polling',
    timeout: existing.timeout ?? '',
    deliverURL: existing.deliver_url ?? '',
    allowUsers: (existing.allow_users ?? []).join(', '),
    async: existing.async ?? false,
    mediaAllow: (existing.media_allow ?? []).join(', '),
    mediaMax: existing.media_max_bytes > 0 ? String(existing.media_max_bytes) : '',
    agents: agentList,
    // An auth or journal mode the file does not state stays unselected: the
    // runtime asks for an auth mode, and a blank journal keeps the stored one.
    auth: OPENAI_AUTH_MODES.includes(existing.auth ?? '') ? (existing.auth as string) : '',
    journal: JOURNAL_MODES.includes(existing.journal ?? '') ? (existing.journal as string) : '',
    maxRequestBytes: (existing.max_request_bytes ?? 0) > 0 ? String(existing.max_request_bytes) : '',
    exposeReasoning: existing.expose_reasoning ?? false,
  }
}

/** The placeholder of the write-only openai keys field. */
export function keysPlaceholder(existing: Channel | null): string {
  const refs = existing?.key_refs ?? []
  if (refs.length > 0) return 'stored ' + refs.join(', ') + ' — leave blank to keep'
  if (existing?.has_keys) return 'leave blank to keep the stored keys'
  return '${COMPAT_API_KEY}'
}

/**
 * The placeholder of a write-only secret field: the reference when the config
 * stores one, the keep-on-blank rule when it stores a value, else what to type.
 */
export function secretPlaceholder(set: boolean, ref: string | undefined, empty: string): string {
  if (ref) return 'stored ' + ref
  if (set) return 'leave blank to keep the stored value'
  return empty
}

const INTEGER = /^[+-]?\d+$/

export type DraftResult = { draft: ChannelDraft; error?: undefined } | { draft?: undefined; error: string }

/** Checks the form and builds the draft; the runtime validates the rest. */
export function buildDraft(f: ChannelForm): DraftResult {
  const draft: ChannelDraft = { name: f.name.trim(), type: f.type, agent: f.agent }
  if (draft.name === '') return { error: 'a channel name is required' }
  if (draft.type === '') return { error: 'choose a channel type' }
  // An openai channel names its agents in a list of its own.
  if (draft.type !== 'openai' && draft.agent === '') return { error: 'choose an agent' }

  const path = f.path.trim()
  if (path) draft.path = path
  // Secrets go out as typed even when the selected type hides their field,
  // as the desktop app does; a blank one keeps the stored value.
  if (f.token) draft.token = f.token
  if (f.secret) draft.secret = f.secret
  if (f.secretToken) draft.secret_token = f.secretToken
  if (f.clearToken) draft.clear_token = true
  if (f.clearSecret) draft.clear_secret = true
  if (f.clearSecretToken) draft.clear_secret_token = true

  switch (draft.type) {
    case 'telegram': {
      draft.mode = f.mode
      const ids: number[] = []
      for (const part of splitList(f.allowUsers)) {
        const id = Number(part)
        // An id past 2^53 would be rounded on its way through JSON.
        if (!INTEGER.test(part) || !Number.isSafeInteger(id)) return { error: `allow users: "${part}" is not a numeric id` }
        ids.push(id)
      }
      if (ids.length) draft.allow_users = ids
      break
    }
    case 'webhook': {
      const timeout = f.timeout.trim()
      if (timeout) draft.timeout = timeout
      if (f.async) draft.async = true
      break
    }
    case 'bridge': {
      const deliver = f.deliverURL.trim()
      if (deliver) draft.deliver_url = deliver
      const timeout = f.timeout.trim()
      if (timeout) draft.timeout = timeout
      break
    }
    case 'openai': {
      draft.agent = ''
      const agents = splitList(f.agents)
      if (agents.length) draft.agents = agents
      if (f.auth) draft.auth = f.auth
      const keys = splitList(f.keys)
      if (keys.length) draft.keys = keys
      if (f.clearKeys) draft.clear_keys = true
      if (f.journal) draft.journal = f.journal
      if (f.exposeReasoning) draft.expose_reasoning = true
      const timeout = f.timeout.trim()
      if (timeout) draft.timeout = timeout
      const limit = f.maxRequestBytes.trim()
      if (limit !== '') {
        const n = Number(limit)
        if (!INTEGER.test(limit) || !Number.isSafeInteger(n) || n < 0) return { error: 'max request bytes must be a whole number, 0 or more' }
        if (n > 0) draft.max_request_bytes = n
      }
      break
    }
  }

  const allow = splitList(f.mediaAllow)
  if (allow.length) draft.media_allow = allow
  const max = f.mediaMax.trim()
  if (max !== '') {
    const n = Number(max)
    if (!INTEGER.test(max) || !Number.isSafeInteger(n) || n < 0) return { error: 'max bytes must be a positive integer' }
    if (n > 0) draft.media_max_bytes = n
  }
  return { draft }
}
