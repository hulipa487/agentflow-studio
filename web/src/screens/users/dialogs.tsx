// The Users screen's two modals: the per-person model and instruction layer,
// and the registration invite that is shown exactly once.
import { useMemo, useState } from 'react'
import { Save } from 'lucide-react'
import { errorMessage } from '@/api/client'
import type { Invite, Profile } from '@/api/types'
import { Banner, Button, CopyButton, Field, Modal, Select, Textarea } from '@/components/ui'
import { t } from '@/i18n'
import { orDash } from '@/lib/format'
import { useCatalog, useClient } from '@/state/session'
import { useAlive } from './helpers'

/** Mount it to open it: the form starts from the profile it was given. */
export function SettingsDialog({ profile, onClose, onSaved }: { profile: Profile; onClose: () => void; onSaved: () => void }) {
  const client = useClient()
  const catalog = useCatalog()
  const alive = useAlive()
  const [model, setModel] = useState(profile.model ?? '')
  const [instructions, setInstructions] = useState(profile.instructions_append ?? '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // A model that has since left the registry stays selectable, so opening the
  // dialog never silently changes what the profile runs on.
  const models = useMemo(() => {
    const list = [...(catalog.models ?? [])]
    if (profile.model && !list.includes(profile.model)) list.push(profile.model)
    return list
  }, [catalog.models, profile.model])

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      // Both fields are always sent: an empty string clears the override,
      // which is what "(agent default)" and an emptied layer mean.
      await client.setUserSettings(profile.user_id, { model, instructions_append: instructions })
    } catch (err) {
      if (alive.current) {
        setError(errorMessage(err))
        setSaving(false)
      }
      return
    }
    onSaved()
  }

  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!saving}
      width={620}
      title={t('users.settings.title')}
      subtitle={t('users.settings.subtitle')}
      footer={
        <>
          <Button onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" icon={Save} loading={saving} onClick={save}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Field label={t('users.settings.model')} hint={t('users.settings.modelHint')}>
          <Select value={model} onChange={setModel} options={models} placeholder={t('users.settings.agentDefault')} mono disabled={saving} />
        </Field>
        <Field label={t('users.settings.instructions')} hint={t('users.settings.instructionsHint')}>
          <Textarea mono rows={8} value={instructions} onChange={(e) => setInstructions(e.target.value)} disabled={saving} />
        </Field>
        <Banner tone="danger">{error}</Banner>
      </div>
    </Modal>
  )
}

export function InviteDialog({ invite, onClose }: { invite: Invite | null; onClose: () => void }) {
  if (!invite) return null
  return (
    <Modal
      open
      onClose={onClose}
      width={520}
      title={t('users.inviteDialog.title')}
      subtitle={t('users.inviteDialog.subtitle')}
      footer={
        <Button variant="primary" className="min-w-24" onClick={onClose} autoFocus>
          {t('common.done')}
        </Button>
      }
    >
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2 rounded-card border border-line bg-bg py-2 pl-3 pr-2">
          <code className="min-w-0 flex-1 select-all break-all font-mono text-md tracking-wide text-text">{invite.invite}</code>
          <CopyButton text={invite.invite} what={t('users.whatInviteCode')} size="md" />
        </div>
        <p className="break-words text-xs leading-snug text-faint">
          {t('users.inviteDialog.singleUse', { expires: orDash(invite.expires_in), hint: invite.hint ?? '' })}
        </p>
        <p className="text-xs leading-snug text-faint">{t('users.inviteDialog.registrationNote')}</p>
      </div>
    </Modal>
  )
}
