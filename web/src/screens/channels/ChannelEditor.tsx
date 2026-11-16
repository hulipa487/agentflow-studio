// The add / edit dialog for one gateway channel. Fields follow the selected
// type; secrets are write-only (blank keeps the stored value, the checkbox
// removes it). Validate dry-runs the draft, Save persists it.
import { useState } from 'react'
import { Check, RotateCw } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { Channel, ChannelList } from '@/api/types'
import { Banner, Button, Checkbox, Field, Input, Modal, Select, type Tone } from '@/components/ui'
import { useClient } from '@/state/session'
import { CHANNEL_TYPES, TELEGRAM_MODES, buildDraft, channelTitle, formFor, secretPlaceholder, type ChannelForm } from './model'
import { useAlive } from './useAlive'

function SecretField({
  label,
  hint,
  value,
  onChange,
  placeholder,
  stored,
  clear,
  onClear,
  clearLabel,
}: {
  label: string
  hint?: string
  value: string
  onChange: (v: string) => void
  placeholder: string
  /** A value is stored, so offering to remove it makes sense. */
  stored: boolean
  clear: boolean
  onClear: (v: boolean) => void
  clearLabel: string
}) {
  return (
    <Field label={label} hint={hint}>
      {/* new-password keeps a password manager from filling in an unrelated login. */}
      <Input type="password" autoComplete="new-password" mono value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
      {stored && <Checkbox checked={clear} onChange={onClear} label={clearLabel} />}
    </Field>
  )
}

export function ChannelEditor({
  existing,
  agents,
  mtime,
  reloading,
  onClose,
  onSaved,
  onReload,
}: {
  /** null adds a channel. */
  existing: Channel | null
  agents: string[]
  /** The list's mtime, passed back untouched so the runtime can detect a hand edit. */
  mtime: ChannelList['mtime']
  /** True while the list is being re-read after a conflict. */
  reloading: boolean
  onClose: () => void
  onSaved: (list: ChannelList) => void
  /** Re-reads the list after the file changed on disk. */
  onReload: () => void
}) {
  const client = useClient()
  const alive = useAlive()
  const [form, setForm] = useState<ChannelForm>(() => formFor(existing, agents))
  const [notice, setNotice] = useState<{ tone: Tone; text: string } | null>(null)
  const [conflict, setConflict] = useState(false)
  const [busy, setBusy] = useState<'validate' | 'save' | null>(null)

  const set = <K extends keyof ChannelForm>(key: K, value: ChannelForm[K]) => setForm((f) => ({ ...f, [key]: value }))
  const text = (key: 'name' | 'path' | 'allowUsers' | 'timeout' | 'deliverURL' | 'mediaAllow' | 'mediaMax') => ({
    value: form[key],
    onChange: (e: { target: { value: string } }) => set(key, e.target.value),
  })

  const index = existing ? existing.index : -1
  const hasToken = existing?.has_token ?? false
  const hasSecret = existing?.has_secret ?? false
  const hasSecretToken = existing?.has_secret_token ?? false
  const staleAgent = existing && existing.agent && !agents.includes(existing.agent) ? existing.agent : ''

  const draftOrReport = () => {
    const r = buildDraft(form)
    if (r.error !== undefined) {
      setNotice({ tone: 'danger', text: r.error })
      return null
    }
    return r.draft
  }

  const validate = async () => {
    if (busy) return
    const draft = draftOrReport()
    if (!draft) return
    setBusy('validate')
    try {
      const errors = await client.validateChannel(index, draft)
      if (!alive.current) return
      setNotice(errors.length > 0 ? { tone: 'danger', text: errors.join('\n') } : { tone: 'success', text: 'Valid. Restart the runtime to bind it.' })
    } catch (err) {
      if (alive.current) setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy(null)
    }
  }

  const save = async () => {
    if (busy) return
    const draft = draftOrReport()
    if (!draft) return
    setBusy('save')
    try {
      const list = await client.saveChannel(index, draft, mtime)
      if (alive.current) onSaved(list)
    } catch (err) {
      if (!alive.current) return
      setNotice({ tone: 'danger', text: errorMessage(err) })
      setConflict(err instanceof ApiError && err.status === 409)
      setBusy(null)
    }
  }

  const reloadAfterConflict = () => {
    setConflict(false)
    // A new channel is appended, so the form survives the reload. An edit is
    // addressed by index, which the hand edit may have shifted: the list
    // closes this dialog rather than let it save over the wrong channel.
    setNotice(existing ? null : { tone: 'accent', text: 'Channel list reloaded. Save again to add the channel.' })
    onReload()
  }

  const secretField = (label: string, empty: string, hint?: string) => (
    <SecretField
      label={label}
      hint={hint}
      value={form.secret}
      onChange={(v) => set('secret', v)}
      placeholder={secretPlaceholder(hasSecret, existing?.secret_ref, empty)}
      stored={hasSecret}
      clear={form.clearSecret}
      onClear={(v) => set('clearSecret', v)}
      clearLabel="Remove the stored secret"
    />
  )
  const tokenField = (label: string, empty: string, hint?: string) => (
    <SecretField
      label={label}
      hint={hint}
      value={form.token}
      onChange={(v) => set('token', v)}
      placeholder={secretPlaceholder(hasToken, existing?.token_ref, empty)}
      stored={hasToken}
      clear={form.clearToken}
      onClear={(v) => set('clearToken', v)}
      clearLabel="Remove the stored token"
    />
  )

  const bridge = form.type === 'bridge'

  return (
    <Modal
      open
      // Stays open while a request is in flight; the answer decides what happens next.
      onClose={() => {
        if (!busy) onClose()
      }}
      title={existing ? 'Edit ' + channelTitle(existing) : 'New channel'}
      subtitle="Route inbound messages into an agent. Changes take effect on the next runtime start."
      width={660}
      footer={
        <>
          <Button onClick={onClose} disabled={busy !== null}>
            Cancel
          </Button>
          <Button icon={Check} onClick={validate} loading={busy === 'validate'} disabled={busy !== null || reloading}>
            Validate
          </Button>
          <Button variant="primary" onClick={save} loading={busy === 'save'} disabled={busy !== null || reloading}>
            Save
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {notice && <Banner tone={notice.tone}>{notice.text}</Banner>}
        {conflict && (
          <div className="flex items-center gap-3">
            <Button icon={RotateCw} onClick={reloadAfterConflict}>
              Reload channels
            </Button>
            <span className="text-xs text-muted">
              {existing ? 'Reloading closes this dialog: reopen the channel to edit the version on disk.' : 'Nothing was saved. Reload the list, then save again.'}
            </span>
          </div>
        )}

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label="Name">
            <Input placeholder="telegram-main" {...text('name')} />
          </Field>
          <Field label="Type">
            <Select value={form.type} onChange={(v) => set('type', v)} options={CHANNEL_TYPES} placeholder={form.type === '' ? 'choose a channel type' : undefined} />
          </Field>
          <Field label="Agent" hint={staleAgent && form.agent === '' ? `Agent "${staleAgent}" no longer exists. Choose the agent this channel routes to.` : undefined}>
            <Select value={form.agent} onChange={(v) => set('agent', v)} options={agents} mono placeholder={form.agent === '' ? 'choose an agent' : undefined} />
          </Field>
          <Field label="Path" hint={bridge ? 'Path is required for a bridge and must end with /.' : 'Optional route on the shared listener. An explicit path must end with /.'}>
            <Input mono placeholder={bridge ? 'e.g. /bridge/' : 'optional · e.g. /webhook/'} {...text('path')} />
          </Field>
        </div>

        {form.type === 'telegram' && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Telegram mode" hint="Webhook and auto modes need gateway.public_url. Polling ignores the path.">
              <Select value={form.mode} onChange={(v) => set('mode', v)} options={TELEGRAM_MODES} />
            </Field>
            <Field label="Allowed Telegram user IDs" hint="Comma-separated numeric IDs; empty accepts anyone the bot can see.">
              <Input mono placeholder="123456789, 987654321" {...text('allowUsers')} />
            </Field>
            {tokenField('Bot token', 'bot token')}
            <SecretField
              label="Webhook secret token"
              value={form.secretToken}
              onChange={(v) => set('secretToken', v)}
              placeholder={secretPlaceholder(hasSecretToken, existing?.secret_token_ref, 'telegram webhook secret (optional)')}
              stored={hasSecretToken}
              clear={form.clearSecretToken}
              onClear={(v) => set('clearSecretToken', v)}
              clearLabel="Remove the stored webhook secret"
            />
          </div>
        )}

        {form.type === 'ghhook' && secretField('Webhook secret', 'GitHub webhook secret', 'Required. The HMAC signature is the only thing authenticating a delivery.')}

        {form.type === 'webhook' && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Reply timeout" hint="Default 55s. Raise it for pipelines that outrun a single turn.">
              <Input mono placeholder="55s" {...text('timeout')} />
            </Field>
            {/* Full width: the kit truncates a checkbox label that does not fit. */}
            <Checkbox className="sm:col-span-2" checked={form.async} onChange={(v) => set('async', v)} label="Async (202 + job id instead of a blocking reply)" />
          </div>
        )}

        {bridge && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Deliver URL" hint="The adapter receives neutral replies here as JSON. Absolute http(s) URL.">
              <Input mono placeholder="https://adapter.example.com/deliver" {...text('deliverURL')} />
            </Field>
            <Field label="Delivery timeout" hint="Default 10s. The adapter must answer each delivery within it.">
              <Input mono placeholder="10s" {...text('timeout')} />
            </Field>
            {secretField('Inbound HMAC secret', 'inbound HMAC secret', 'Required. The adapter signs each inbound body with X-Agentflow-Signature.')}
            {tokenField('Outbound bearer token', 'bearer token (optional)', 'Optional. Sent as Authorization: Bearer when delivering to the adapter.')}
          </div>
        )}

        <div className="mt-1 border-t border-line pt-3">
          <h3 className="mb-2 text-[13px] font-semibold">Inbound media</h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Allowed MIME patterns" hint="Comma-separated, e.g. image/*, application/pdf. Empty disables attachments.">
              <Input mono placeholder="image/*, application/pdf" {...text('mediaAllow')} />
            </Field>
            <Field label="Max bytes" hint="Defaults to 8 MiB when media is enabled.">
              <Input mono inputMode="numeric" placeholder="8388608" {...text('mediaMax')} />
            </Field>
          </div>
        </div>
      </div>
    </Modal>
  )
}
