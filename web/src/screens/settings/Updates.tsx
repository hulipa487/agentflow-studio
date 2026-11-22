// Updates: desktop-only self-updater UI backed by /__af/__desktop/update/*.
import { useEffect, type ReactNode } from 'react'
import { Download, ExternalLink, RefreshCw } from 'lucide-react'
import { Banner, Button, Checkbox, Chip, Section } from '@/components/ui'
import { useDesktopShell, openExternalURL } from '@/lib/desktop'
import { t } from '@/i18n'
import { useUpdater } from '@/state/updater'
import { confirm, toast } from '@/state/ui'
import { Fact } from './parts'

export function UpdatesSection() {
  const desktop = useDesktopShell()
  const { status, checking, applying, error, autoCheck, loadStatus, check, apply, setAutoCheck } = useUpdater()

  useEffect(() => {
    if (desktop) void loadStatus()
  }, [desktop, loadStatus])

  if (!desktop) return null

  const onInstall = async () => {
    if (!status?.latest_version) return
    const ok = await confirm({
      title: t('settings.updateConfirmTitle', { version: status.latest_version }),
      message: t('settings.updateConfirmMessage', { version: status.latest_version }),
      confirmLabel: t('settings.updateInstall'),
    })
    if (!ok) return
    const launched = await apply()
    if (!launched) return
    toast(t('settings.updateInstalling'), 'accent')
  }

  let badge: ReactNode
  if (status?.update_available && status.latest_version) {
    badge = <Chip tone="accent">{t('settings.updateAvailable', { version: status.latest_version })}</Chip>
  } else if (status?.is_dev && status.latest_version) {
    badge = <Chip tone="warning">{t('settings.updateDevBuild', { version: status.latest_version })}</Chip>
  } else if (status?.checked_at) {
    badge = <Chip tone="success">{t('settings.updateUpToDate')}</Chip>
  } else {
    badge = <Chip>{t('settings.updateNotChecked')}</Chip>
  }

  const blockedReason =
    !status?.can_self_update && status?.managed_reason === 'readonly'
      ? t('settings.updateReasonReadonly', { platform: status.platform })
      : !status?.can_self_update && status?.managed_reason === 'missing_asset'
        ? t('settings.updateReasonMissingAsset', { platform: status.platform, arch: status.arch })
        : ''

  return (
    <Section
      title={t('settings.updates')}
      subtitle={t('settings.updatesSubtitle')}
      action={
        <Button size="sm" icon={RefreshCw} loading={checking} disabled={applying} onClick={() => void check(false)}>
          {checking ? t('settings.updateChecking') : t('settings.updateCheck')}
        </Button>
      }
    >
      <div className="flex flex-col gap-3">
        {error && <Banner tone="danger">{error}</Banner>}
        {blockedReason && status?.update_available && <Banner tone="warning">{blockedReason}</Banner>}

        <div className="flex flex-col">
          <Fact label={t('settings.updateCurrent')}>
            <span className="font-mono text-xs">{status?.current_version || '—'}</span>
          </Fact>
          <Fact label={t('settings.updatePlatform')}>
            <span className="font-mono text-xs">
              {status ? `${status.platform}/${status.arch}` : '—'}
            </span>
          </Fact>
          {status?.latest_version && (
            <Fact label={t('settings.updateLatest')}>
              <span className="font-mono text-xs">{status.latest_version}</span>
            </Fact>
          )}
          <Fact label={t('settings.updateStatus')}>
            {badge}
            {status?.checked_at && (
              <span className="text-xs text-muted">
                {t('settings.updateLastChecked', { time: new Date(status.checked_at).toLocaleTimeString() })}
              </span>
            )}
          </Fact>
        </div>

        <Checkbox checked={autoCheck} onChange={setAutoCheck} label={t('settings.updateAutoCheck')} />

        {status?.update_available && status.release_notes && (
          <div className="rounded-ctl border border-line bg-bg/60 p-2.5">
            <div className="mb-1 text-xs font-semibold text-faint">{t('settings.updateNotes')}</div>
            <pre className="max-h-36 overflow-y-auto whitespace-pre-wrap font-sans text-xs leading-relaxed text-muted">
              {status.release_notes}
            </pre>
          </div>
        )}

        {(status?.update_available || (status?.latest_version && status.release_url)) && (
          <div className="flex flex-wrap items-center gap-2">
            {status.update_available && status.can_self_update && (
              <Button variant="primary" icon={Download} loading={applying} disabled={checking} onClick={() => void onInstall()}>
                {applying ? t('settings.updateInstalling') : t('settings.updateInstall')}
              </Button>
            )}
            {status.release_url && (
              <Button icon={ExternalLink} onClick={() => void openExternalURL(status.release_url!)}>
                {t('settings.updateOpenRelease')}
              </Button>
            )}
          </div>
        )}
      </div>
    </Section>
  )
}
