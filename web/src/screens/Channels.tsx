// Channels: the gateway's entry points (webhook, Telegram, GitHub webhook,
// bridge, OpenAI-compatible API). Saves go to the deployment config and bind at the next runtime
// start, so every row says whether it is live or waiting for a restart.
import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { FolderOpen, Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { Channel, ChannelList } from '@/api/types'
import { AsyncView, Banner, Button, Chip, CopyButton, EmptyState, IconButton, Page, Panel, StatusChip } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { openFolder, useDesktopShell } from '@/lib/desktop'
import { useClient } from '@/state/session'
import { confirm, info, toast } from '@/state/ui'
import { t } from '@/i18n'
import { ChannelEditor } from './channels/ChannelEditor'
import { QQ_DEFAULT_SCENES, channelAgents, channelTitle, humanBytes } from './channels/model'
import { useAlive } from './channels/useAlive'

interface ChannelsData {
  list: ChannelList
  /** Agents a channel can target: spawn profiles are templates, not routes. */
  agents: string[]
}

const GRID = 'grid grid-cols-[minmax(180px,1.2fr)_minmax(110px,0.8fr)_minmax(160px,1.5fr)_150px_64px] items-center gap-3'

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <span className="inline-flex min-w-0 max-w-full items-center gap-1.5">
      <span className="shrink-0 text-xs font-semibold text-faint">{label}</span>
      {children}
    </span>
  )
}

function Value({ children, title }: { children: ReactNode; title?: string }) {
  return (
    <span className="truncate font-mono text-sm text-muted" title={title}>
      {children}
    </span>
  )
}

/** Whether a secret is stored, plus its ${ENV} / cred: reference. Never a value. */
function SecretChip({ set, secretRef }: { set: boolean; secretRef?: string }) {
  return (
    <Chip tone={set ? 'success' : 'warning'}>
      {secretRef
        ? t(set ? 'channels.secret.configuredRef' : 'channels.secret.notSetRef', { ref: secretRef })
        : t(set ? 'channels.secret.configured' : 'channels.secret.notSet')}
    </Chip>
  )
}

function ChannelFacts({ ch }: { ch: Channel }) {
  const facts: ReactNode[] = []
  const users = ch.allow_users ?? []
  const media = ch.media_allow ?? []
  const timeout = ch.timeout ? (
    <Fact key="timeout" label={t('channels.fact.timeout')}>
      <Value>{ch.timeout}</Value>
    </Fact>
  ) : null
  switch (ch.type) {
    case 'telegram':
      facts.push(
        <Fact key="mode" label={t('channels.fact.mode')}>
          <Value>{ch.mode || 'polling'}</Value>
        </Fact>,
      )
      if (users.length > 0) {
        facts.push(
          <Fact key="users" label={t('channels.fact.allowUsers')}>
            <Value title={users.join(', ')}>{users.join(', ')}</Value>
          </Fact>,
        )
      }
      facts.push(
        <Fact key="token" label={t('channels.fact.token')}>
          <SecretChip set={ch.has_token} secretRef={ch.token_ref} />
        </Fact>,
        <Fact key="secret_token" label={t('channels.fact.webhookSecret')}>
          <SecretChip set={ch.has_secret_token} secretRef={ch.secret_token_ref} />
        </Fact>,
      )
      break
    case 'qq':
      facts.push(
        <Fact key="mode" label={t('channels.fact.mode')}>
          <Value>{ch.mode || 'websocket'}</Value>
        </Fact>,
        <Fact key="app" label={t('channels.fact.appId')}>
          <Value>{ch.app_id || '—'}</Value>
        </Fact>,
        <Fact key="scenes" label={t('channels.fact.scenes')}>
          <Value>{(ch.scenes ?? []).length > 0 ? (ch.scenes ?? []).join(', ') : QQ_DEFAULT_SCENES.join(', ')}</Value>
        </Fact>,
        <Fact key="secret" label={t('channels.fact.appSecret')}>
          <SecretChip set={ch.has_secret} secretRef={ch.secret_ref} />
        </Fact>,
      )
      if (ch.markdown) {
        facts.push(
          <Fact key="md" label={t('channels.fact.replies')}>
            <Value>{t('channels.fact.repliesMarkdown')}</Value>
          </Fact>,
        )
      }
      if (ch.api_base) {
        facts.push(
          <Fact key="api" label={t('channels.fact.apiBase')}>
            <Value title={ch.api_base}>{ch.api_base}</Value>
          </Fact>,
        )
      }
      break
    case 'email':
      facts.push(
        <Fact key="mode" label={t('channels.fact.mode')}>
          <Value>{ch.mode || 'idle'}</Value>
        </Fact>,
        <Fact key="imap" label={t('channels.fact.imap')}>
          <Value title={`${ch.imap_host || '—'}:${ch.imap_port || 993} (${ch.mailbox || 'INBOX'})`}>
            {`${ch.imap_host || '—'}:${ch.imap_port || 993}`}
          </Value>
        </Fact>,
        <Fact key="smtp" label={t('channels.fact.smtp')}>
          <Value title={`${ch.smtp_host || '—'}:${ch.smtp_port || 465}`}>
            {`${ch.smtp_host || '—'}:${ch.smtp_port || 465}`}
          </Value>
        </Fact>,
        <Fact key="username" label={t('channels.fact.username')}>
          <Value title={ch.username || '—'}>{ch.username || '—'}</Value>
        </Fact>,
        <Fact key="password" label={t('channels.fact.password')}>
          <SecretChip set={ch.has_password ?? false} secretRef={ch.password_ref ?? ''} />
        </Fact>,
        <Fact key="mark_seen" label={t('channels.fact.markSeen')}>
          <Value>{ch.mark_seen === false ? t('channels.fact.markSeenOff') : t('channels.fact.markSeenOn')}</Value>
        </Fact>,
      )
      if ((ch.allow_from ?? []).length > 0) {
        facts.push(
          <Fact key="allow_from" label={t('channels.fact.allowFrom')}>
            <Value title={(ch.allow_from ?? []).join(', ')}>{(ch.allow_from ?? []).join(', ')}</Value>
          </Fact>,
        )
      }
      if ((ch.allow_domains ?? []).length > 0) {
        facts.push(
          <Fact key="allow_domains" label={t('channels.fact.allowDomains')}>
            <Value title={(ch.allow_domains ?? []).join(', ')}>{(ch.allow_domains ?? []).join(', ')}</Value>
          </Fact>,
        )
      }
      break
    case 'ghhook':
      facts.push(
        <Fact key="secret" label={t('channels.fact.secret')}>
          <SecretChip set={ch.has_secret} secretRef={ch.secret_ref} />
        </Fact>,
      )
      break
    case 'bridge':
      if (ch.deliver_url) {
        facts.push(
          <Fact key="deliver" label={t('channels.fact.deliver')}>
            <Value title={ch.deliver_url}>{ch.deliver_url}</Value>
          </Fact>,
        )
      }
      facts.push(
        <Fact key="secret" label={t('channels.fact.inboundSecret')}>
          <SecretChip set={ch.has_secret} secretRef={ch.secret_ref} />
        </Fact>,
        <Fact key="token" label={t('channels.fact.outboundToken')}>
          <SecretChip set={ch.has_token} secretRef={ch.token_ref} />
        </Fact>,
        timeout,
      )
      break
    case 'webhook':
      facts.push(timeout)
      if (ch.async) {
        facts.push(
          <Fact key="mode" label={t('channels.fact.mode')}>
            <Value>{t('channels.fact.asyncJob')}</Value>
          </Fact>,
        )
      }
      break
    case 'openai': {
      const refs = (ch.key_refs ?? []).join(', ')
      facts.push(
        <Fact key="auth" label={t('channels.fact.auth')}>
          <Value>{ch.auth || '—'}</Value>
        </Fact>,
      )
      if (ch.auth === 'key') {
        facts.push(
          <Fact key="keys" label={t('channels.fact.keys')}>
            <SecretChip set={ch.has_keys ?? false} secretRef={refs} />
          </Fact>,
        )
      }
      facts.push(
        <Fact key="journal" label={t('channels.fact.journal')}>
          <Value>{ch.journal || 'metadata'}</Value>
        </Fact>,
        timeout,
      )
      break
    }
  }
  // The audit mode applies to every channel type. An openai row always shows
  // it (above); any other type shows it when it is not the default.
  if (ch.type !== 'openai' && ch.journal && ch.journal !== 'full') {
    facts.push(
      <Fact key="journal" label={t('channels.fact.journal')}>
        <Value>{ch.journal}</Value>
      </Fact>,
    )
  }
  if (media.length > 0) {
    facts.push(
      <Fact key="media" label={t('channels.fact.media')}>
        <Value title={media.join(', ')}>
          {ch.media_max_bytes > 0 ? t('channels.fact.mediaMax', { types: media.join(', '), size: humanBytes(ch.media_max_bytes) }) : media.join(', ')}
        </Value>
      </Fact>,
    )
  }
  const shown = facts.filter(Boolean)
  if (shown.length === 0) return null
  return <div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1">{shown}</div>
}

export default function Channels() {
  const client = useClient()
  const navigate = useNavigate()
  const alive = useAlive()
  const [editing, setEditing] = useState<{ existing: Channel | null } | null>(null)
  const [problem, setProblem] = useState('')
  const [deleting, setDeleting] = useState<number | null>(null)

  const { data, error, loading, refreshing, reload, setData } = useAsync<ChannelsData>(
    async (signal) => {
      // The agent list only feeds the editor's picker; without it the screen
      // still lists channels and the editor explains what is missing.
      const agents = client.agents(signal).then(
        (rows) => rows.filter((r) => !r.spawn).map((r) => r.name),
        () => [] as string[],
      )
      try {
        return { list: await client.channels(signal), agents: await agents }
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          throw new ApiError(404, t('channels.apiMissing'))
        }
        throw err
      }
    },
    [client],
  )

  const refresh = () => {
    setProblem('')
    reload()
  }
  useScreenActions({ refresh })

  // A failed reload drops the list, and the editor with it; without this the
  // dialog would pop back up, emptied, once a retry succeeds.
  useEffect(() => {
    if (!data) setEditing(null)
  }, [data])

  const list = data?.list
  const channels = list?.channels ?? []
  // The desktop app can show the config's folder in the file manager.
  const desktop = useDesktopShell()
  const showFolder = async (path: string) => {
    try {
      const r = await openFolder(path)
      if (r.ok) return
      const key = r.problem === 'missing' ? 'shell.folderMissing' : r.problem === 'notdir' ? 'shell.folderNotDir' : 'shell.folderOpenFailed'
      toast(t(key, { dir: r.dir, message: r.ok === false && r.error ? r.error : r.dir }), 'danger')
    } catch (err) {
      toast(t('shell.folderOpenFailed', { message: errorMessage(err) }), 'danger')
    }
  }
  const editable = list?.editable ?? false
  const pending = channels.filter((ch) => !ch.live).length

  const applyList = (next: ChannelList) => setData((prev) => ({ list: next, agents: prev?.agents ?? [] }))

  const openEditor = (existing: Channel | null) => {
    if (!data) return
    if (!data.list.editable) {
      void info({ title: t('channels.readOnlyTitle'), message: data.list.note || t('channels.readOnlyMessage') })
      return
    }
    if (data.agents.length === 0) {
      void confirm({
        title: t('channels.needAgentTitle'),
        message: t('channels.needAgentMessage'),
        confirmLabel: t('channels.goToAgents'),
      }).then((ok) => {
        if (ok && alive.current) navigate('/agents')
      })
      return
    }
    setEditing({ existing })
  }

  const remove = async (ch: Channel) => {
    if (!data) return
    const mtime = data.list.mtime
    const ok = await confirm({
      title: t('channels.deleteTitle'),
      message: t('channels.deleteMessage', { name: channelTitle(ch) }),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok || !alive.current) return
    setDeleting(ch.index)
    try {
      const next = await client.deleteChannel(ch.index, mtime)
      if (!alive.current) return
      applyList(next)
      setProblem('')
      toast(
        next.saved_note || (next.restart_required ? t('channels.deleted') : t('channels.deletedLive')),
        next.restart_required ? 'warning' : 'success',
      )
    } catch (err) {
      if (!alive.current) return
      if (err instanceof ApiError && err.status === 409) {
        // The index may point at another channel now, so the delete is not
        // retried: the operator picks the row again from the fresh list.
        const again = await confirm({
          title: t('channels.fileChangedTitle'),
          message: t('channels.fileChangedMessage', { message: errorMessage(err) }),
          confirmLabel: t('channels.reload'),
        })
        if (again && alive.current) refresh()
      } else {
        setProblem(errorMessage(err))
      }
    } finally {
      if (alive.current) setDeleting(null)
    }
  }

  const banners: ReactNode[] = []
  if (list && !editable) {
    banners.push(
      <Banner key="readonly" tone="warning">
        {list.note || t('channels.readOnlyMessage')}
      </Banner>,
    )
  }
  // A write's own note also covers what the per-row state cannot show: a
  // secret-only edit leaves the row "live" although it needs a restart too.
  if (list?.saved_note) {
    banners.push(
      <Banner key="saved" tone={list.restart_required ? 'warning' : 'success'}>
        {list.saved_note}
      </Banner>,
    )
  }
  if (data && error) {
    banners.push(
      <Banner key="refresh" tone="warning">
        {t('channels.refreshFailed', { message: errorMessage(error) })}
      </Banner>,
    )
  }
  if (problem) {
    banners.push(
      <Banner key="problem" tone="danger" onClose={() => setProblem('')}>
        {problem}
      </Banner>,
    )
  }

  return (
    <Page
      title={t('channels.title')}
      subtitle={t('channels.subtitle')}
      meta={
        list ? (
          <>
            <span>{list.mode === 'configdir' ? t('channels.modeConfigDir') : t('channels.modeSingleFile')}</span>
            {list.path && (
              <span className="inline-flex min-w-0 items-center gap-1">
                <span className="truncate" title={list.path}>
                  {list.path}
                </span>
                <CopyButton text={list.path} what={t('channels.configPath')} />
                {desktop && <IconButton icon={FolderOpen} size="sm" label={t('channels.openConfigFolder')} onClick={() => void showFolder(list.path!)} />}
              </span>
            )}
            {editable && list.note && <span>{list.note}</span>}
          </>
        ) : null
      }
      banner={banners.length > 0 ? banners : null}
      actions={
        <>
          {list && (
            <>
              <span className="font-mono text-xs text-faint">{t('channels.count', { n: channels.length })}</span>
              {!editable && <Chip tone="warning">{t('channels.readOnlyChip')}</Chip>}
              <StatusChip tone={pending > 0 ? 'warning' : 'success'}>
                {pending === 0 ? t('channels.allLive') : t('channels.pendingRestart', { n: pending })}
              </StatusChip>
            </>
          )}
          <IconButton icon={RotateCw} label={t('common.refresh')} onClick={refresh} loading={refreshing} />
          {/* While the list is empty the empty state carries this action. */}
          {channels.length > 0 && (
            <Button variant="primary" icon={Plus} disabled={!editable} onClick={() => openEditor(null)}>
              {t('channels.newChannel')}
            </Button>
          )}
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!data} retry={refresh} loadingText={t('channels.loading')}>
        {channels.length === 0 ? (
          <EmptyState
            title={t('channels.emptyTitle')}
            guidance={t('channels.emptyGuidance')}
            actionLabel={t('channels.emptyAction')}
            onAction={() => openEditor(null)}
          />
        ) : (
          <Panel flush className="overflow-x-auto">
            <div className="min-w-[760px]">
              <div className={`${GRID} border-b border-line px-3 py-1.5 text-xs font-semibold text-faint`}>
                <span>{t('channels.col.channel')}</span>
                <span>{t('channels.col.agent')}</span>
                <span>{t('channels.col.route')}</span>
                <span>{t('channels.col.state')}</span>
                <span />
              </div>
              {channels.map((ch) => {
                const title = channelTitle(ch)
                const routed = channelAgents(ch)
                return (
                  <div key={ch.index} className="border-b border-line/70 px-3 py-2 last:border-b-0">
                    <div className={GRID}>
                      <div className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-base font-semibold" title={title}>
                          {title}
                        </span>
                        <Chip tone="accent">{ch.type}</Chip>
                        {(ch.media_allow ?? []).length > 0 && <Chip>{t('channels.chip.media')}</Chip>}
                      </div>
                      <div className="truncate font-mono text-xs" title={routed}>
                        {routed || '—'}
                      </div>
                      <div className="truncate font-mono text-sm text-muted" title={ch.path}>
                        {ch.path || <span className="text-faint">{t('channels.driverDefault')}</span>}
                      </div>
                      <div>{ch.live ? <StatusChip tone="success">{t('channels.chip.live')}</StatusChip> : <StatusChip tone="warning">{t('channels.chip.restartRequired')}</StatusChip>}</div>
                      <div className="flex items-center justify-end">
                        <IconButton icon={Pencil} label={t('channels.editNamed', { name: title })} onClick={() => openEditor(ch)} />
                        <IconButton
                          icon={Trash2}
                          label={t('channels.deleteNamed', { name: title })}
                          tone="danger"
                          loading={deleting === ch.index}
                          disabled={deleting !== null}
                          onClick={() => remove(ch)}
                        />
                      </div>
                    </div>
                    <ChannelFacts ch={ch} />
                  </div>
                )
              })}
            </div>
          </Panel>
        )}
      </AsyncView>
      {editing && data && (
        <ChannelEditor
          // A fresh form per channel: reopening never shows the last one's text.
          key={editing.existing ? editing.existing.index : 'new'}
          existing={editing.existing}
          agents={data.agents}
          mtime={data.list.mtime}
          reloading={refreshing}
          onClose={() => setEditing(null)}
          onSaved={(next) => {
            applyList(next)
            setEditing(null)
            setProblem('')
            toast(
              next.saved_note || (next.restart_required ? t('channels.saved') : t('channels.savedLive')),
              next.restart_required ? 'warning' : 'success',
            )
          }}
          onReload={() => {
            if (editing.existing) {
              setEditing(null)
              toast(t('channels.listReloadedReopen'), 'warning')
              refresh()
            } else {
              // Quiet: a failed re-read must not drop the list and, with it,
              // the form the operator is still filling in.
              reload(true)
            }
          }}
        />
      )}
    </Page>
  )
}
