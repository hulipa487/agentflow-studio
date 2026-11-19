// Channels: the gateway's entry points (webhook, Telegram, GitHub webhook,
// bridge, OpenAI-compatible API). Saves go to the deployment config and bind at the next runtime
// start, so every row says whether it is live or waiting for a restart.
import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { Channel, ChannelList } from '@/api/types'
import { AsyncView, Banner, Button, Chip, CopyButton, EmptyState, IconButton, Page, Panel, StatusChip } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { plural } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, info, toast } from '@/state/ui'
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
      {set ? 'configured' : 'not set'}
      {secretRef ? ' · ' + secretRef : ''}
    </Chip>
  )
}

function ChannelFacts({ ch }: { ch: Channel }) {
  const facts: ReactNode[] = []
  const users = ch.allow_users ?? []
  const media = ch.media_allow ?? []
  const timeout = ch.timeout ? (
    <Fact key="timeout" label="timeout">
      <Value>{ch.timeout}</Value>
    </Fact>
  ) : null
  switch (ch.type) {
    case 'telegram':
      facts.push(
        <Fact key="mode" label="mode">
          <Value>{ch.mode || 'polling'}</Value>
        </Fact>,
      )
      if (users.length > 0) {
        facts.push(
          <Fact key="users" label="allow users">
            <Value title={users.join(', ')}>{users.join(', ')}</Value>
          </Fact>,
        )
      }
      facts.push(
        <Fact key="token" label="token">
          <SecretChip set={ch.has_token} secretRef={ch.token_ref} />
        </Fact>,
        <Fact key="secret_token" label="webhook secret">
          <SecretChip set={ch.has_secret_token} secretRef={ch.secret_token_ref} />
        </Fact>,
      )
      break
    case 'qq':
      facts.push(
        <Fact key="mode" label="mode">
          <Value>{ch.mode || 'websocket'}</Value>
        </Fact>,
        <Fact key="app" label="app id">
          <Value>{ch.app_id || '—'}</Value>
        </Fact>,
        <Fact key="scenes" label="scenes">
          <Value>{(ch.scenes ?? []).length > 0 ? (ch.scenes ?? []).join(', ') : QQ_DEFAULT_SCENES.join(', ')}</Value>
        </Fact>,
        <Fact key="secret" label="app secret">
          <SecretChip set={ch.has_secret} secretRef={ch.secret_ref} />
        </Fact>,
      )
      if (ch.markdown) {
        facts.push(
          <Fact key="md" label="replies">
            <Value>markdown</Value>
          </Fact>,
        )
      }
      if (ch.api_base) {
        facts.push(
          <Fact key="api" label="api base">
            <Value title={ch.api_base}>{ch.api_base}</Value>
          </Fact>,
        )
      }
      break
    case 'ghhook':
      facts.push(
        <Fact key="secret" label="secret">
          <SecretChip set={ch.has_secret} secretRef={ch.secret_ref} />
        </Fact>,
      )
      break
    case 'bridge':
      if (ch.deliver_url) {
        facts.push(
          <Fact key="deliver" label="deliver">
            <Value title={ch.deliver_url}>{ch.deliver_url}</Value>
          </Fact>,
        )
      }
      facts.push(
        <Fact key="secret" label="inbound secret">
          <SecretChip set={ch.has_secret} secretRef={ch.secret_ref} />
        </Fact>,
        <Fact key="token" label="outbound token">
          <SecretChip set={ch.has_token} secretRef={ch.token_ref} />
        </Fact>,
        timeout,
      )
      break
    case 'webhook':
      facts.push(timeout)
      if (ch.async) {
        facts.push(
          <Fact key="mode" label="mode">
            <Value>async · job id</Value>
          </Fact>,
        )
      }
      break
    case 'openai': {
      const refs = (ch.key_refs ?? []).join(', ')
      facts.push(
        <Fact key="auth" label="auth">
          <Value>{ch.auth || '—'}</Value>
        </Fact>,
      )
      if (ch.auth === 'key') {
        facts.push(
          <Fact key="keys" label="keys">
            <SecretChip set={ch.has_keys ?? false} secretRef={refs} />
          </Fact>,
        )
      }
      facts.push(
        <Fact key="journal" label="journal">
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
      <Fact key="journal" label="journal">
        <Value>{ch.journal}</Value>
      </Fact>,
    )
  }
  if (media.length > 0) {
    facts.push(
      <Fact key="media" label="media">
        <Value title={media.join(', ')}>
          {media.join(', ')}
          {ch.media_max_bytes > 0 ? ` · max ${humanBytes(ch.media_max_bytes)}` : ''}
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
          throw new ApiError(404, 'This runtime does not expose the Studio channels API yet; rebuild and restart AgentFlow, then reopen Channels.')
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
  const editable = list?.editable ?? false
  const pending = channels.filter((ch) => !ch.live).length

  const applyList = (next: ChannelList) => setData((prev) => ({ list: next, agents: prev?.agents ?? [] }))

  const openEditor = (existing: Channel | null) => {
    if (!data) return
    if (!data.list.editable) {
      void info({ title: 'Channels are read-only', message: data.list.note || "This deployment's channels cannot be edited from Studio." })
      return
    }
    if (data.agents.length === 0) {
      void confirm({
        title: 'Create an agent first',
        message: 'A channel must route messages to an existing agent. Create an agent, then add the channel.',
        confirmLabel: 'Go to Agents',
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
      title: 'Delete channel',
      message: `Delete "${channelTitle(ch)}" from the deployment config?\n\nThe running instance keeps it until the next restart.`,
      confirmLabel: 'Delete',
      danger: true,
    })
    if (!ok || !alive.current) return
    setDeleting(ch.index)
    try {
      const next = await client.deleteChannel(ch.index, mtime)
      if (!alive.current) return
      applyList(next)
      setProblem('')
      toast('channel deleted · restart required', 'warning')
    } catch (err) {
      if (!alive.current) return
      if (err instanceof ApiError && err.status === 409) {
        // The index may point at another channel now, so the delete is not
        // retried: the operator picks the row again from the fresh list.
        const again = await confirm({
          title: 'Channels file changed',
          message: `${errorMessage(err)}\n\nNothing was deleted. Reload the channel list?`,
          confirmLabel: 'Reload',
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
        {list.note || "This deployment's channels cannot be edited from Studio."}
      </Banner>,
    )
  }
  // A write's own note also covers what the per-row state cannot show: a
  // secret-only edit leaves the row "live" although it needs a restart too.
  if (list?.saved_note) {
    banners.push(
      <Banner key="saved" tone="warning">
        {list.saved_note}
      </Banner>,
    )
  }
  if (data && error) {
    banners.push(
      <Banner key="refresh" tone="warning">
        refresh failed: {errorMessage(error)}
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
      title="Channels"
      subtitle="Entry points that route messages into agents. Saves go to the deployment config and bind at the next runtime start."
      meta={
        list ? (
          <>
            <span>{list.mode === 'configdir' ? 'config directory' : 'single config file'}</span>
            {list.path && (
              <span className="inline-flex min-w-0 items-center gap-1">
                <span className="truncate" title={list.path}>
                  {list.path}
                </span>
                <CopyButton text={list.path} what="config path" />
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
              <span className="font-mono text-xs text-faint">{plural(channels.length, 'channel')}</span>
              {!editable && <Chip tone="warning">read-only</Chip>}
              <StatusChip tone={pending > 0 ? 'warning' : 'success'}>
                {pending === 0 ? 'all changes live' : `${pending === 1 ? '1 change' : `${pending} changes`} · restart required`}
              </StatusChip>
            </>
          )}
          <IconButton icon={RotateCw} label="Refresh" onClick={refresh} loading={refreshing} />
          {/* While the list is empty the empty state carries this action. */}
          {channels.length > 0 && (
            <Button variant="primary" icon={Plus} disabled={!editable} onClick={() => openEditor(null)}>
              New channel
            </Button>
          )}
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!data} retry={refresh} loadingText="Loading channels…">
        {channels.length === 0 ? (
          <EmptyState
            title="No channels yet"
            guidance="Create a channel to route messages into an agent."
            actionLabel="Create a channel"
            onAction={() => openEditor(null)}
          />
        ) : (
          <Panel flush className="overflow-x-auto">
            <div className="min-w-[760px]">
              <div className={`${GRID} border-b border-line px-3 py-1.5 text-xs font-semibold text-faint`}>
                <span>Channel</span>
                <span>Agent</span>
                <span>Route</span>
                <span>State</span>
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
                        {(ch.media_allow ?? []).length > 0 && <Chip>media</Chip>}
                      </div>
                      <div className="truncate font-mono text-xs" title={routed}>
                        {routed || '—'}
                      </div>
                      <div className="truncate font-mono text-sm text-muted" title={ch.path}>
                        {ch.path || <span className="text-faint">driver default</span>}
                      </div>
                      <div>{ch.live ? <StatusChip tone="success">live</StatusChip> : <StatusChip tone="warning">restart required</StatusChip>}</div>
                      <div className="flex items-center justify-end">
                        <IconButton icon={Pencil} label={`Edit ${title}`} onClick={() => openEditor(ch)} />
                        <IconButton
                          icon={Trash2}
                          label={`Delete ${title}`}
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
            toast('channel saved · restart required', 'warning')
          }}
          onReload={() => {
            if (editing.existing) {
              setEditing(null)
              toast('channel list reloaded · reopen the channel to edit it', 'warning')
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
