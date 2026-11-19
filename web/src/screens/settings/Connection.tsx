// Connection: which runtime this tab talks to, how, what this browser has
// saved about it, and the other saved connections.
import { ArrowLeftRight, Lock, Plus, Trash2 } from 'lucide-react'
import { transportOf, type Client } from '@/api/client'
import { Button, Chip, CopyButton, Dot, IconButton, KV, Section, StatusChip } from '@/components/ui'
import { humanDuration, orDash, shortID } from '@/lib/format'
import { tokenFor, useConnections, type Profile } from '@/state/connections'
import { useClient, useSession } from '@/state/session'
import { confirm } from '@/state/ui'
import { t } from '@/i18n'
import { tr } from '@/i18n/rich'
import { Fact, FieldLabel } from './parts'

type TokenState = 'stored' | 'tab' | 'none'

function tokenState(p: Profile | undefined): TokenState {
  if (!p) return 'none'
  if (p.token) return 'stored'
  return tokenFor(p) ? 'tab' : 'none'
}

function transportLabel(url: string, viaProxy: boolean): string {
  const kind = transportOf(url)
  if (kind === 'unix') return t('settings.transportSocket')
  if (kind === 'npipe') return t('settings.transportPipe')
  return viaProxy ? t('settings.transportTcpProxy') : t('settings.transportTcpDirect')
}

function transportNote(client: Client): string {
  if (client.local) return t('settings.transportNoteLocal')
  if (client.viaProxy) return t('settings.transportNoteProxy')
  return t('settings.transportNoteDirect')
}

const TOKEN_NOTE_KEY: Record<TokenState, string> = {
  stored: 'settings.tokenNoteStored',
  tab: 'settings.tokenNoteTab',
  none: 'settings.tokenNoteNone',
}

function TokenChip({ state }: { state: TokenState }) {
  if (state === 'stored') return <Chip tone="warning">{t('settings.tokenStored')}</Chip>
  if (state === 'tab') return <Chip tone="accent">{t('settings.tokenTab')}</Chip>
  return <Chip>{t('settings.tokenNone')}</Chip>
}

/** Leaves this runtime for another saved one, exactly as the shell's connection menu does. */
async function switchTo(name: string) {
  useConnections.getState().select(name)
  useSession.getState().disconnect()
  // A profile with a token reconnects at once; without one the welcome
  // screen opens prefilled with its address.
  await useSession.getState().restore()
}

async function forget(p: Profile, isActive: boolean) {
  const ok = await confirm({
    title: t('settings.forgetTitle'),
    message: isActive ? t('settings.forgetMessageActive', { name: p.name }) : t('settings.forgetMessage', { name: p.name }),
    confirmLabel: t('settings.forget'),
    danger: true,
  })
  if (!ok) return
  useConnections.getState().remove(p.name)
  if (isActive) useSession.getState().disconnect()
}

export function ConnectionSection() {
  const client = useClient()
  const online = useSession((s) => s.online)
  const rt = useSession((s) => s.runtime)
  const profiles = useConnections((s) => s.profiles)
  const active = useConnections((s) => s.active)
  const current = profiles.find((p) => p.name === active)
  const token = tokenState(current)
  const direct = !client.viaProxy
  const corsLines = `runtime:\n  admin:\n    cors_origins: ["${location.origin}"]`

  return (
    <Section
      title={t('settings.connection')}
      subtitle={t('settings.connectionHint')}
      action={
        online ? (
          <StatusChip tone="success">{t('common.connected')}</StatusChip>
        ) : (
          <StatusChip tone="danger" pulse>
            {t('common.offline')}
          </StatusChip>
        )
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <Fact label={t('settings.endpoint')}>
            <span className="truncate font-mono text-xs" title={client.displayURL}>
              {client.displayURL}
            </span>
            <CopyButton text={client.displayURL} what={t('settings.endpointNoun')} />
          </Fact>
          <Fact label={t('settings.transport')} hint={transportNote(client)}>
            {transportLabel(client.displayURL, client.viaProxy)}
          </Fact>
          <Fact label={t('settings.adminToken')} hint={t(TOKEN_NOTE_KEY[token])}>
            <TokenChip state={token} />
          </Fact>
        </div>

        {rt && (
          <div className="grid grid-cols-2 gap-x-6 gap-y-2 rounded-ctl border border-line bg-surface2 px-3 py-2.5 md:grid-cols-4">
            <KV label={t('settings.instance')} value={<span title={rt.instance}>{orDash(shortID(rt.instance))}</span>} copy={rt.instance || undefined} />
            <KV label={t('settings.region')} value={orDash(rt.region)} />
            <KV label={t('settings.zone')} value={orDash(rt.zone)} />
            <KV label={t('settings.topology')} value={orDash(rt.topology)} />
            <KV label={t('settings.version')} value={orDash(rt.version)} />
            <KV label={t('settings.uptime')} value={humanDuration(rt.uptime_s)} />
            <KV label={t('settings.configEpoch')} value={orDash(rt.config_epoch)} />
            <KV label={t('settings.config')} value={<span title={rt.config_path}>{orDash(rt.config_path)}</span>} copy={rt.config_path || undefined} />
          </div>
        )}

        {direct && (
          <div>
            <FieldLabel>{t('settings.allowedOrigin')}</FieldLabel>
            <p className="mb-1.5 text-xs leading-snug text-muted">
              {tr('settings.allowedOriginNote', { key: 'runtime.admin.cors_origins' }, { code: (c) => <span className="font-mono">{c}</span> })}
            </p>
            <div className="flex items-start gap-2 rounded-ctl border border-line bg-bg py-2 pl-3 pr-2">
              <pre className="min-w-0 flex-1 overflow-x-auto font-mono text-xs leading-relaxed text-text">{corsLines}</pre>
              <CopyButton text={corsLines} what="cors_origins" />
            </div>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          <Button icon={Lock} onClick={() => useSession.getState().lock()} title={t('settings.lockTitle')}>
            {t('settings.lock')}
          </Button>
          <Button
            icon={Plus}
            onClick={() => {
              // Keep the saved profiles; only the active mirror is cleared so
              // the welcome screen starts blank.
              useConnections.getState().clearActive()
              useSession.getState().disconnect()
            }}
          >
            {t('shell.useAnotherConnection')}
          </Button>
          <span className="text-xs text-muted">{t('settings.lockNote')}</span>
        </div>

        <div>
          <FieldLabel>{t('settings.savedConnections')}</FieldLabel>
          {profiles.length === 0 ? (
            <div className="rounded-ctl border border-line px-3 py-3 text-xs text-muted">{t('shell.noConnections')}</div>
          ) : (
            <div className="overflow-hidden rounded-ctl border border-line">
              {profiles.map((p) => {
                const isActive = p.name === active
                return (
                  <div key={p.name} className="flex min-h-10 items-center gap-2.5 border-b border-line/70 px-3 py-1.5 last:border-b-0">
                    <Dot tone={isActive ? 'success' : 'neutral'} />
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className={'truncate text-base ' + (isActive ? 'font-semibold' : '')}>{p.name}</span>
                        {isActive && <Chip tone="success">{t('shell.activeConnection')}</Chip>}
                      </div>
                      <div className="truncate font-mono text-xs text-muted" title={p.url}>
                        {p.url}
                      </div>
                    </div>
                    <Chip>{transportLabel(p.url, p.viaProxy)}</Chip>
                    <TokenChip state={tokenState(p)} />
                    {!isActive && (
                      <Button
                        size="sm"
                        icon={ArrowLeftRight}
                        onClick={() => void switchTo(p.name)}
                        title={tokenState(p) === 'none' ? t('settings.switchNeedsToken') : t('settings.switchReconnect')}
                      >
                        {t('settings.switch')}
                      </Button>
                    )}
                    <IconButton icon={Trash2} label={t('settings.forgetName', { name: p.name })} tone="danger" onClick={() => void forget(p, isActive)} />
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </Section>
  )
}
