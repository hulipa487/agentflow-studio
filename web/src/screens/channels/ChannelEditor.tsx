// The add / edit dialog for one gateway channel. Fields follow the selected
// type; secrets are write-only (blank keeps the stored value, the checkbox
// removes it). Validate dry-runs the draft, Save persists it.
import { useState } from 'react'
import { Check, RotateCw } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { Channel, ChannelList, QQBotInfo, TelegramBotInfo } from '@/api/types'
import { Banner, Button, Checkbox, Chip, Field, Input, Modal, Select, StatusChip, type Tone } from '@/components/ui'
import { useClient } from '@/state/session'
import { t } from '@/i18n'
import {
  CHANNEL_TYPES,
  JOURNAL_MODES,
  OPENAI_AUTH_MODES,
  QQ_MODES,
  QQ_SCENES,
  TELEGRAM_MODES,
  buildDraft,
  channelTitle,
  formFor,
  keysPlaceholder,
  secretPlaceholder,
  type ChannelForm,
} from './model'
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
  const [busy, setBusy] = useState<'validate' | 'save' | 'test' | null>(null)
  const [botInfo, setBotInfo] = useState<TelegramBotInfo | null>(null)
  const [qqInfo, setQQInfo] = useState<QQBotInfo | null>(null)

  const set = <K extends keyof ChannelForm>(key: K, value: ChannelForm[K]) => setForm((f) => ({ ...f, [key]: value }))
  const text = (key: 'name' | 'path' | 'allowUsers' | 'timeout' | 'deliverURL' | 'mediaAllow' | 'mediaMax' | 'agents' | 'maxRequestBytes' | 'appID' | 'apiBase') => ({
    value: form[key],
    onChange: (e: { target: { value: string } }) => set(key, e.target.value),
  })

  const index = existing ? existing.index : -1
  const hasToken = existing?.has_token ?? false
  const hasSecret = existing?.has_secret ?? false
  const hasSecretToken = existing?.has_secret_token ?? false
  const hasKeys = existing?.has_keys ?? false
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
      setNotice(errors.length > 0 ? { tone: 'danger', text: errors.join('\n') } : { tone: 'success', text: t('channels.valid') })
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

  const testTelegram = async () => {
    if (busy) return
    setBusy('test')
    setNotice(null)
    try {
      const info = await client.testTelegramToken({
        index: existing?.index,
        token: form.token.trim() || undefined,
        api_base: existing?.api_base || undefined,
      })
      if (!alive.current) return
      setBotInfo(info)
    } catch (err) {
      if (!alive.current) return
      setBotInfo(null)
      setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy(null)
    }
  }

  const testQQ = async () => {
    if (busy) return
    setBusy('test')
    setNotice(null)
    try {
      const info = await client.testQQCredentials({
        index: existing?.index,
        app_id: form.appID.trim() || undefined,
        secret: form.secret.trim() || undefined,
        api_base: form.apiBase.trim() || existing?.api_base || undefined,
      })
      if (!alive.current) return
      setQQInfo(info)
    } catch (err) {
      if (!alive.current) return
      setQQInfo(null)
      setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy(null)
    }
  }

  const reloadAfterConflict = () => {
    setConflict(false)
    // A new channel is appended, so the form survives the reload. An edit is
    // addressed by index, which the hand edit may have shifted: the list
    // closes this dialog rather than let it save over the wrong channel.
    setNotice(existing ? null : { tone: 'accent', text: t('channels.reloadedSaveAgain') })
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
      clearLabel={t('channels.clearSecret')}
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
      clearLabel={t('channels.clearToken')}
    />
  )

  const bridge = form.type === 'bridge'
  const openai = form.type === 'openai'
  const qq = form.type === 'qq'
  const toggleScene = (id: string, on: boolean) =>
    set('scenes', on ? [...form.scenes.filter((s) => s !== id), id] : form.scenes.filter((s) => s !== id))

  return (
    <Modal
      open
      // Stays open while a request is in flight; the answer decides what happens next.
      onClose={() => {
        if (!busy) onClose()
      }}
      title={existing ? t('channels.editNamed', { name: channelTitle(existing) }) : t('channels.newChannel')}
      subtitle={t('channels.editorSubtitle')}
      width={660}
      footer={
        <>
          <Button onClick={onClose} disabled={busy !== null}>
            {t('common.cancel')}
          </Button>
          <Button icon={Check} onClick={validate} loading={busy === 'validate'} disabled={busy !== null || reloading}>
            {t('channels.validate')}
          </Button>
          <Button variant="primary" onClick={save} loading={busy === 'save'} disabled={busy !== null || reloading}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {notice && <Banner tone={notice.tone}>{notice.text}</Banner>}
        {conflict && (
          <div className="flex items-center gap-3">
            <Button icon={RotateCw} onClick={reloadAfterConflict}>
              {t('channels.reloadChannels')}
            </Button>
            <span className="text-xs text-muted">
              {existing ? t('channels.conflictEdit') : t('channels.conflictNew')}
            </span>
          </div>
        )}

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label={t('common.name')}>
            <Input placeholder="telegram-main" {...text('name')} />
          </Field>
          <Field label={t('channels.field.type')}>
            <Select value={form.type} onChange={(v) => set('type', v)} options={CHANNEL_TYPES} placeholder={form.type === '' ? t('channels.chooseType') : undefined} />
          </Field>
          {openai ? (
            <Field label={t('channels.field.agentsExposed')} hint={t('channels.hint.agentsExposed')}>
              <Input mono placeholder="coder, helper" {...text('agents')} />
            </Field>
          ) : (
            <Field label={t('channels.field.agent')} hint={staleAgent && form.agent === '' ? t('channels.hint.staleAgent', { name: staleAgent }) : undefined}>
              <Select value={form.agent} onChange={(v) => set('agent', v)} options={agents} mono placeholder={form.agent === '' ? t('channels.chooseAgent') : undefined} />
            </Field>
          )}
          {!(qq && form.qqMode === 'websocket') && (
            <Field
              label={t('channels.field.path')}
              hint={
                bridge
                  ? t('channels.hint.pathBridge')
                  : qq
                    ? t('channels.hint.pathQQ')
                    : t('channels.hint.pathDefault')
              }
            >
              <Input mono placeholder={bridge ? t('channels.placeholder.pathBridge') : qq ? t('channels.placeholder.pathQQ') : t('channels.placeholder.pathDefault')} {...text('path')} />
            </Field>
          )}
        </div>

        {form.type === 'telegram' && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label={t('channels.field.telegramMode')} hint={t('channels.hint.telegramMode')}>
              <Select value={form.mode} onChange={(v) => set('mode', v)} options={TELEGRAM_MODES} />
            </Field>
            <Field label={t('channels.field.allowedUsers')} hint={t('channels.hint.allowedUsers')}>
              <Input mono placeholder="123456789, 987654321" {...text('allowUsers')} />
            </Field>
            {tokenField(t('channels.field.botToken'), t('channels.placeholder.botToken'))}
            <SecretField
              label={t('channels.field.webhookSecretToken')}
              value={form.secretToken}
              onChange={(v) => set('secretToken', v)}
              placeholder={secretPlaceholder(hasSecretToken, existing?.secret_token_ref, t('channels.placeholder.telegramWebhookSecret'))}
              stored={hasSecretToken}
              clear={form.clearSecretToken}
              onClear={(v) => set('clearSecretToken', v)}
              clearLabel={t('channels.clearWebhookSecret')}
            />
            <div className="flex flex-col gap-2 rounded-ctl border border-line bg-surface2 p-2.5 sm:col-span-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-xs text-muted">
                  {(form.mode === '' || form.mode === 'polling')
                    ? t('channels.telegram.pollingLiveHint')
                    : t('channels.telegram.verifyHint')}
                </div>
                <Button
                  size="sm"
                  onClick={testTelegram}
                  loading={busy === 'test'}
                  disabled={busy !== null || reloading || (form.token.trim() === '' && !hasToken)}
                >
                  {t('channels.telegram.testToken')}
                </Button>
              </div>
              {botInfo && (
                <div className="flex flex-col gap-1.5 border-t border-line/60 pt-2 text-xs">
                  <div className="flex flex-wrap items-center gap-2">
                    <Chip tone="accent">@{botInfo.username}</Chip>
                    <span className="font-mono text-faint">#{botInfo.bot_id}</span>
                    <StatusChip tone={botInfo.privacy_mode_enabled ? 'warning' : 'success'}>
                      {botInfo.privacy_mode_enabled
                        ? t('channels.telegram.privacyOn')
                        : t('channels.telegram.privacyOff')}
                    </StatusChip>
                  </div>
                  <div className="text-muted">
                    {botInfo.privacy_mode_enabled
                      ? t('channels.telegram.privacyOnDesc')
                      : t('channels.telegram.privacyOffDesc')}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {qq && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label={t('channels.field.appId')} hint={t('channels.hint.appId')}>
              <Input mono placeholder="102xxxxxx" {...text('appID')} />
            </Field>
            {secretField(t('channels.field.appSecret'), t('channels.placeholder.appSecret'), t('channels.hint.appSecret'))}
            <Field
              label={t('channels.field.eventDelivery')}
              hint={
                form.qqMode === 'webhook'
                  ? t('channels.hint.qqWebhook')
                  : t('channels.hint.qqWebsocket')
              }
            >
              <Select value={form.qqMode} onChange={(v) => set('qqMode', v)} options={QQ_MODES} />
            </Field>
            <Field label={t('channels.field.apiBase')} hint={t('channels.hint.apiBase')}>
              <Input mono placeholder="https://api.bot.qq.com" {...text('apiBase')} />
            </Field>
            <Field label={t('channels.field.scenes')} hint={t('channels.hint.scenes')} className="sm:col-span-2">
              <div className="flex flex-wrap gap-x-4 gap-y-1.5">
                {QQ_SCENES.map((s) => (
                  <Checkbox key={s.id} checked={form.scenes.includes(s.id)} onChange={(v) => toggleScene(s.id, v)} label={t(s.labelKey)} />
                ))}
              </div>
            </Field>
            <Field
              label={t('channels.field.replies')}
              hint={t('channels.hint.replies')}
              className="sm:col-span-2"
            >
              <Checkbox checked={form.markdown} onChange={(v) => set('markdown', v)} label={t('channels.markdownReplies')} />
            </Field>
            <div className="flex flex-col gap-2 rounded-ctl border border-line bg-surface2 p-2.5 sm:col-span-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-xs text-muted">
                  {(form.qqMode === '' || form.qqMode === 'websocket')
                    ? t('channels.qq.websocketLiveHint')
                    : t('channels.qq.verifyHint')}
                </div>
                <Button
                  size="sm"
                  onClick={testQQ}
                  loading={busy === 'test'}
                  disabled={busy !== null || reloading || form.appID.trim() === '' || (form.secret.trim() === '' && !hasSecret)}
                >
                  {t('channels.qq.testCredentials')}
                </Button>
              </div>
              <div className="text-xs text-faint">{t('channels.qq.groupAllHint')}</div>
              {qqInfo && (
                <div className="flex flex-col gap-1.5 border-t border-line/60 pt-2 text-xs">
                  <div className="flex flex-wrap items-center gap-2">
                    {qqInfo.username && <Chip tone="accent">{qqInfo.username}</Chip>}
                    <span className="font-mono text-faint">AppID {qqInfo.app_id}</span>
                    <StatusChip tone="success">{t('channels.qq.connected')}</StatusChip>
                    {typeof qqInfo.shards === 'number' && qqInfo.shards > 0 && (
                      <Chip>{t('channels.qq.shards', { n: qqInfo.shards })}</Chip>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {form.type === 'ghhook' && secretField(t('channels.field.webhookSecret'), t('channels.placeholder.githubSecret'), t('channels.hint.githubSecret'))}

        {form.type === 'webhook' && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label={t('channels.field.replyTimeout')} hint={t('channels.hint.replyTimeout')}>
              <Input mono placeholder="55s" {...text('timeout')} />
            </Field>
            {/* Full width: the kit truncates a checkbox label that does not fit. */}
            <Checkbox className="sm:col-span-2" checked={form.async} onChange={(v) => set('async', v)} label={t('channels.asyncReply')} />
          </div>
        )}

        {bridge && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label={t('channels.field.deliverUrl')} hint={t('channels.hint.deliverUrl')}>
              <Input mono placeholder="https://adapter.example.com/deliver" {...text('deliverURL')} />
            </Field>
            <Field label={t('channels.field.deliveryTimeout')} hint={t('channels.hint.deliveryTimeout')}>
              <Input mono placeholder="10s" {...text('timeout')} />
            </Field>
            {secretField(t('channels.field.inboundSecret'), t('channels.placeholder.inboundSecret'), t('channels.hint.inboundSecret'))}
            {tokenField(t('channels.field.outboundToken'), t('channels.placeholder.outboundToken'), t('channels.hint.outboundToken'))}
          </div>
        )}

        {openai && (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field
              label={t('channels.field.auth')}
              hint={
                form.auth === 'user_token'
                  ? t('channels.hint.authUserToken')
                  : form.auth === 'none'
                    ? t('channels.hint.authNone')
                    : undefined
              }
            >
              <Select value={form.auth} onChange={(v) => set('auth', v)} options={OPENAI_AUTH_MODES} placeholder={form.auth === '' ? t('channels.chooseAuth') : undefined} />
            </Field>
            {form.auth === 'key' ? (
              <SecretField
                label={t('channels.field.apiKeys')}
                hint={t('channels.hint.apiKeys')}
                value={form.keys}
                onChange={(v) => set('keys', v)}
                placeholder={keysPlaceholder(existing)}
                stored={hasKeys}
                clear={form.clearKeys}
                onClear={(v) => set('clearKeys', v)}
                clearLabel={t('channels.clearKeys')}
              />
            ) : (
              <div className="hidden sm:block" />
            )}
            <Field label={t('channels.field.journal')} hint={t('channels.hint.journal')}>
              <Select value={form.journal} onChange={(v) => set('journal', v)} options={JOURNAL_MODES} placeholder={form.journal === '' ? t('channels.placeholder.journalDefault') : undefined} />
            </Field>
            <Field label={t('channels.field.requestTimeout')} hint={t('channels.hint.requestTimeout')}>
              <Input mono placeholder="600s" {...text('timeout')} />
            </Field>
            <Field label={t('channels.field.maxRequestBytes')} hint={t('channels.hint.maxRequestBytes')}>
              <Input mono inputMode="numeric" placeholder="33554432" {...text('maxRequestBytes')} />
            </Field>
            <Checkbox
              className="sm:col-span-2"
              checked={form.exposeReasoning}
              onChange={(v) => set('exposeReasoning', v)}
              label={t('channels.exposeReasoning')}
              hint={t('channels.hint.exposeReasoning')}
            />
          </div>
        )}

        <div className="mt-1 border-t border-line pt-3">
          <h3 className="mb-2 text-base font-semibold">{t('channels.mediaHeading')}</h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label={t('channels.field.mediaAllow')} hint={t('channels.hint.mediaAllow')}>
              <Input mono placeholder="image/*, application/pdf" {...text('mediaAllow')} />
            </Field>
            <Field label={t('channels.field.mediaMax')} hint={t('channels.hint.mediaMax')}>
              <Input mono inputMode="numeric" placeholder="8388608" {...text('mediaMax')} />
            </Field>
          </div>
        </div>
      </div>
    </Modal>
  )
}
