// Pure helpers for the Credentials screen, ported from the desktop Studio
// (internal/ui/credentials.go).
import type { Credential, CredentialDraft, CredentialList } from '@/api/types'
import { t } from '@/i18n'

/** The kinds the editor offers; the store accepts any label. */
export const KINDS = ['api_key', 'token', 'password', 'oauth', 'generic']

export const DEFAULT_HEADER = 'Authorization'
export const DEFAULT_SCHEME = 'Bearer'

/** The engine-wide bucket is the empty user. */
export function bucketLabel(user: string): string {
  return user === '' ? t('credentials.engineWide') : user
}

/** The bucket as it reads inside a sentence. */
export function bucketPhrase(user: string): string {
  return user === '' ? t('credentials.bucketEngineWide') : t('credentials.bucketUser', { user })
}

/**
 * The selectable buckets: engine-wide first, then the user UUIDs that hold a
 * credential, plus the selected one, so a hand-entered UUID stays selectable
 * before it holds anything.
 */
export function bucketOptions(users: string[] | null | undefined, selected: string): string[] {
  const all = new Set<string>(['', ...(users ?? []), selected])
  return [...all].sort((a, b) => {
    if (a === '') return b === '' ? 0 : -1
    if (b === '') return 1
    return a < b ? -1 : a > b ? 1 : 0
  })
}

/** How the runtime injects the secret into a request; the value is never shown. */
export function injectionShape(c: Pick<Credential, 'header' | 'scheme'>): string {
  const token = `<${t('credentials.secretToken')}>`
  if (c.header && c.scheme) return `${c.header}: ${c.scheme} ${token}`
  if (c.header) return `${c.header}: ${token}`
  return `${DEFAULT_HEADER}: ${DEFAULT_SCHEME} ${token}`
}

/** The credential editor's fields, as typed. The secret is never prefilled. */
export interface CredentialForm {
  service: string
  kind: string
  secret: string
  header: string
  scheme: string
}

export function credentialForm(existing: Credential | null): CredentialForm {
  if (!existing) return { service: '', kind: 'api_key', secret: '', header: DEFAULT_HEADER, scheme: DEFAULT_SCHEME }
  return {
    service: existing.service,
    kind: existing.kind || 'api_key',
    secret: '',
    header: existing.header || DEFAULT_HEADER,
    scheme: existing.scheme ?? '',
  }
}

/**
 * Turns the form into the draft the runtime takes. A blank secret on an edit
 * is sent as blank: the runtime then keeps the encrypted value and updates
 * only the metadata. The secret is sent exactly as typed, untrimmed.
 */
export function buildCredentialDraft(f: CredentialForm, user: string, creating: boolean): CredentialDraft {
  const d: CredentialDraft = {
    user,
    service: f.service.trim(),
    kind: f.kind,
    secret: f.secret,
    header: f.header.trim(),
    scheme: f.scheme.trim(),
  }
  if (d.service === '') throw new Error(t('credentials.error.serviceRequired'))
  if (creating && d.secret === '') throw new Error(t('credentials.error.secretRequired'))
  return d
}

/** What the screen shows when the runtime has no credential store. */
export function disabledList(note: string): CredentialList {
  return { ok: true, enabled: false, bucket: '', users: [''], credentials: [], note }
}

/**
 * The key of the note for a runtime that answers 503. The list holds the key,
 * not its text, so the note follows a language change; credentialNote() reads it.
 */
export const DISABLED_NOTE_KEY = 'credentials.disabledNote'

/** A list's note as shown: Studio's own note translated, the runtime's as it came. */
export function credentialNote(list: Pick<CredentialList, 'note'>): string {
  return list.note === DISABLED_NOTE_KEY ? t(DISABLED_NOTE_KEY) : (list.note ?? '')
}
