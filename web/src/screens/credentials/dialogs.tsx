// The Credentials screen's two dialogs: the add/edit form and the "open a
// user bucket" picker. Neither ever holds a stored secret: the API does not
// return one, and the form's own field is dropped when the dialog closes.
import { useEffect, useRef, useState, type RefObject } from 'react'
import { Upload } from 'lucide-react'
import { errorMessage, type Client } from '@/api/client'
import type { Credential, CredentialDraft, CredentialList } from '@/api/types'
import { Banner, Button, Chip, ComboInput, Field, Input, Modal, Select } from '@/components/ui'
import { t } from '@/i18n'
import { KINDS, bucketPhrase, buildCredentialDraft, credentialForm, type CredentialForm } from './helpers'

/** True until the component unmounts, so a late answer is not applied. */
export function useMounted(): RefObject<boolean> {
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  return mounted
}

export function CredentialEditor({
  client,
  bucket,
  existing,
  onClose,
  onSaved,
}: {
  client: Client
  /** The bucket the credential is written to; '' is engine-wide. */
  bucket: string
  /** Null adds a credential. */
  existing: Credential | null
  onClose: () => void
  onSaved: (list: CredentialList, draft: CredentialDraft) => void
}) {
  const creating = existing === null
  const mounted = useMounted()
  const [form, setForm] = useState<CredentialForm>(() => credentialForm(existing))
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const set = (key: keyof CredentialForm) => (e: { target: { value: string } }) => setForm((f) => ({ ...f, [key]: e.target.value }))

  const kinds = existing && existing.kind !== '' && !KINDS.includes(existing.kind) ? [...KINDS, existing.kind] : KINDS

  const save = async () => {
    if (saving) return
    let draft: CredentialDraft
    try {
      draft = buildCredentialDraft(form, bucket, creating)
    } catch (err) {
      setError(errorMessage(err))
      return
    }
    setSaving(true)
    try {
      const list = await client.putCredential(draft)
      if (mounted.current) onSaved(list, draft)
    } catch (err) {
      if (mounted.current) setError(errorMessage(err))
    } finally {
      if (mounted.current) setSaving(false)
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!saving}
      width={600}
      title={creating ? t('credentials.newCredential') : t('credentials.editNamed', { name: existing.service })}
      subtitle={t('credentials.editorSubtitle')}
      footer={
        <>
          <Button onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" icon={Upload} loading={saving} onClick={() => void save()}>
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </>
      }
    >
      {error && (
        <Banner tone="danger" className="mb-3">
          {error}
        </Banner>
      )}
      <div className="flex flex-col gap-4">
        <div className="flex items-center gap-2 text-xs text-muted">
          <span>{t('credentials.bucket')}</span>
          <Chip tone={bucket === '' ? 'neutral' : 'accent'}>{bucketPhrase(bucket)}</Chip>
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label={t('credentials.field.service')}>
            <Input mono value={form.service} onChange={set('service')} placeholder="github_token" disabled={!creating} autoFocus={creating} />
          </Field>
          <Field label={t('credentials.field.kind')}>
            <Select mono value={form.kind} onChange={(kind) => setForm((f) => ({ ...f, kind }))} options={kinds} />
          </Field>
        </div>
        <Field
          label={t('credentials.field.secret')}
          hint={t('credentials.hint.secret')}
        >
          {/* A password field with no reveal control; new-password keeps the browser from filling a saved login into it. */}
          <Input
            mono
            type="password"
            autoComplete="new-password"
            data-1p-ignore
            data-lpignore="true"
            value={form.secret}
            onChange={set('secret')}
            placeholder={creating ? t('credentials.placeholder.secretNew') : t('credentials.placeholder.secretKeep')}
            autoFocus={!creating}
          />
        </Field>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label={t('credentials.field.header')}>
            <Input mono value={form.header} onChange={set('header')} />
          </Field>
          <Field label={t('credentials.field.scheme')}>
            <Input mono value={form.scheme} onChange={set('scheme')} />
          </Field>
        </div>
        <p className="-mt-2 text-xs leading-snug text-faint">
          {t('credentials.hint.sentAs')}
        </p>
      </div>
    </Modal>
  )
}

/** Lets an operator address a user UUID before it holds a credential. */
export function BucketDialog({ client, onClose, onOpen }: { client: Client; onClose: () => void; onOpen: (user: string) => void }) {
  const [value, setValue] = useState('')
  const [error, setError] = useState('')
  const [ids, setIDs] = useState<string[]>([])

  // Suggestions only: a runtime without the identity layer yields none and
  // the field stays free text.
  useEffect(() => {
    const ctl = new AbortController()
    client
      .users(ctl.signal)
      .then((l) => {
        if (!ctl.signal.aborted) setIDs((l.users ?? []).map((u) => u.user_id))
      })
      .catch(() => {})
    return () => ctl.abort()
  }, [client])

  const choose = () => {
    const user = value.trim()
    if (user === '') {
      setError(t('credentials.bucketDialog.error'))
      return
    }
    onOpen(user)
  }

  return (
    <Modal
      open
      onClose={onClose}
      width={520}
      title={t('credentials.bucketDialog.title')}
      subtitle={t('credentials.bucketDialog.subtitle')}
      footer={
        <>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
          <Button variant="primary" onClick={choose}>
            {t('credentials.bucketDialog.open')}
          </Button>
        </>
      }
    >
      <Field label={t('credentials.bucketDialog.field')} hint={t('credentials.bucketDialog.hint')} error={error || undefined}>
        <ComboInput
          mono
          autoFocus
          options={ids}
          value={value}
          invalid={error !== ''}
          placeholder={t('credentials.bucketDialog.placeholder')}
          onChange={(e) => {
            setValue(e.target.value)
            setError('')
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              choose()
            }
          }}
        />
      </Field>
    </Modal>
  )
}
