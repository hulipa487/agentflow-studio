// Channel helpers shared by the list and the editor: titles, the form model
// and the draft the API takes. The rules mirror build() in the desktop app's
// channels.go so both clients send the same thing.
import type { Channel, ChannelDraft } from '@/api/types'
import { splitList } from '@/lib/format'

export const CHANNEL_TYPES = ['webhook', 'telegram', 'ghhook', 'bridge']
export const TELEGRAM_MODES = ['polling', 'webhook', 'auto']

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
  }
  if (!existing) return form
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
  }
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
  if (draft.agent === '') return { error: 'choose an agent' }

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
