// Overview: what this runtime is doing right now. Refreshes every five
// seconds while open.
import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, FlaskConical, Pencil, RotateCw, ScrollText } from 'lucide-react'
import type { Metrics, State, StateAgent, StateChannel } from '@/api/types'
import { errorMessage } from '@/api/client'
import { AsyncView, Banner, Button, Chip, IconButton, Page, Section, StatTile, StatusChip, Table } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { humanCount, humanDuration, orDash, shortID } from '@/lib/format'
import { t, useLang } from '@/i18n'
import { useClient } from '@/state/session'

const COUNTERS = [
  { name: 'agentflow_llm_calls', labelKey: 'overview.counterLlmCalls' },
  { name: 'agentflow_llm_tokens', labelKey: 'overview.counterLlmTokens' },
  { name: 'agentflow_ingress_total', labelKey: 'overview.counterIngress' },
  { name: 'agentflow_safety_drops', labelKey: 'overview.counterSafetyDrops' },
]

interface OverviewData {
  state: State
  metrics: Metrics | null
  metricsError: string
}

function persistentSummary(agents: StateAgent[]): string {
  const n = agents.filter((a) => a.persistent).length
  return n === 0 ? t('overview.noDaemons') : t('overview.daemonCount', { n })
}

function channelSummary(chs: StateChannel[]): string {
  if (chs.length === 0) return t('overview.noChannels')
  const count = new Map<string, number>()
  for (const c of chs) count.set(c.type, (count.get(c.type) ?? 0) + 1)
  return [...count.keys()]
    .sort()
    .map((k) => t('overview.channelCount', { n: count.get(k) ?? 0, type: k }))
    .join(' · ')
}

export default function Overview() {
  const client = useClient()
  const navigate = useNavigate()
  const { data, error, loading, reload } = useAsync<OverviewData>(
    async (signal) => {
      const state = await client.state(signal)
      try {
        return { state, metrics: await client.metrics(signal), metricsError: '' }
      } catch (err) {
        return { state, metrics: null, metricsError: errorMessage(err) }
      }
    },
    [client],
    { interval: 5000 },
  )
  useScreenActions({ refresh: () => reload() })
  const lang = useLang()

  const st = data?.state
  const meta = useMemo(() => {
    if (!st) return null
    const parts: string[] = []
    if (st.instance) parts.push(t('overview.metaInstance', { id: shortID(st.instance) }))
    if (st.region) parts.push(t('overview.metaRegion', { region: st.region }))
    if (st.zone) parts.push(t('overview.metaZone', { zone: st.zone }))
    if (st.topology) parts.push(t('overview.metaTopology', { topology: st.topology }))
    if ((st.instances?.length ?? 0) > 1) parts.push(t('overview.metaInstances', { n: st.instances!.length }))
    parts.push(t('overview.metaVersion', { version: orDash(st.version) }), t('overview.metaUptime', { duration: humanDuration(st.uptime_s) }))
    if (st.config_epoch) parts.push(t('overview.metaEpoch', { epoch: st.config_epoch }))
    if (st.config_path) parts.push(st.config_path)
    return parts.map((p) => <span key={p}>{p}</span>)
  }, [st, lang])

  const sessions = useMemo(
    () => [...(st?.sessions ?? [])].sort((a, b) => Number(b.busy) - Number(a.busy) || a.session_id.localeCompare(b.session_id)),
    [st],
  )
  const agents = useMemo(() => [...(st?.agents ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [st])
  const channels = st?.channels ?? []
  const instances = st?.instances ?? []
  const now = Date.now() / 1000

  return (
    <Page
      title={t('overview.title')}
      subtitle={t('overview.subtitle')}
      meta={meta}
      banner={data && error ? <Banner tone="warning">{t('overview.refreshFailed', { message: errorMessage(error) })}</Banner> : null}
      actions={
        <>
          <IconButton icon={RotateCw} label={t('common.refresh')} onClick={() => reload()} />
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!data} retry={() => reload()} loadingText={t('overview.loading')}>
        {st && (
          <div className="flex flex-col gap-3">
            {agents.length === 0 && (
              <Banner tone="accent">
                <span className="flex flex-wrap items-center gap-2">
                  <span>{t('overview.onboarding')}</span>
                  <Button size="sm" onClick={() => navigate('/models')}>{t('nav.models')}</Button>
                  <Button size="sm" variant="primary" onClick={() => navigate('/new-agent')}>{t('agents.newAgent')}</Button>
                  <Button size="sm" onClick={() => navigate('/channels')}>{t('nav.channels')}</Button>
                </span>
              </Banner>
            )}

            <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
              <StatTile
                label={t('overview.liveSessions')}
                value={st.sessions_active + st.sessions_idle}
                sub={t('overview.sessionsSub', { busy: st.sessions_active, idle: st.sessions_idle })}
                tone={st.sessions_active > 0 ? 'accent' : 'neutral'}
              />
              <StatTile label={t('overview.agents')} value={agents.length} sub={persistentSummary(agents)} />
              <StatTile label={t('overview.channels')} value={channels.length} sub={channelSummary(channels)} />
              <StatTile
                label={t('overview.credentialStore')}
                value={st.credentials_enabled ? t('common.enabled') : t('common.disabled')}
                sub="runtime.credentials"
                tone={st.credentials_enabled ? 'success' : 'warning'}
              />
            </div>

            {data.metrics ? (
              <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
                {COUNTERS.map((c) => {
                  const vals = (data.metrics!.series?.[c.name] ?? []).map((p) => p.v)
                  const delta = vals.length > 1 ? t('overview.counterDelta', { delta: humanCount(vals[vals.length - 1] - vals[0]) }) : undefined
                  return <StatTile key={c.name} label={t(c.labelKey)} value={humanCount(data.metrics!.latest?.[c.name] ?? 0)} sub={delta} spark={vals} />
                })}
              </div>
            ) : (
              <div className="rounded-card border border-line bg-surface px-3 py-2.5 text-xs text-muted">{t('overview.countersUnavailable', { message: data.metricsError })}</div>
            )}

            <div className="grid gap-3 lg:grid-cols-2">
              <Section title={t('overview.liveSessions')} subtitle={t('overview.sessionsSubtitle')} flush>
                <Table
                  rows={sessions}
                  rowKey={(s) => s.session_id}
                  empty={t('overview.sessionsEmpty')}
                  columns={[
                    { key: 'state', title: t('overview.colState'), width: 84, render: (s) => (s.busy ? <StatusChip tone="accent" pulse>{t('overview.busy')}</StatusChip> : <StatusChip>{t('overview.idle')}</StatusChip>) },
                    { key: 'agent', title: t('overview.colAgent'), width: 120, render: (s) => s.agent },
                    { key: 'session', title: t('overview.colSession'), mono: true, render: (s) => <span title={s.session_id}>{s.session_id}</span> },
                    { key: 'parent', title: t('overview.colParent'), width: 96, mono: true, render: (s) => orDash(shortID(s.parent_id)) },
                  ]}
                />
              </Section>

              <div className="flex min-w-0 flex-col gap-3">
                <Section
                  title={t('overview.agents')}
                  subtitle={t('overview.agentsSubtitle')}
                  action={
                    <Button size="sm" icon={ArrowRight} onClick={() => navigate('/agents')}>
                      {t('common.open')}
                    </Button>
                  }
                >
                  {agents.length === 0 && <div className="py-3 text-center text-sm text-muted">{t('overview.agentsEmpty')}</div>}
                  <div className="flex flex-col">
                    {agents.map((ag) => (
                      <div key={ag.name} className="flex h-8 items-center gap-2">
                        <span className="truncate text-base font-semibold">{ag.name}</span>
                        {ag.model && <Chip tone="accent">{ag.model}</Chip>}
                        {ag.persistent && <Chip tone="success">{t('overview.daemon')}</Chip>}
                        <span className="flex-1" />
                        <IconButton icon={FlaskConical} label={t('overview.chatWith', { name: ag.name })} size="sm" onClick={() => navigate('/playground?agent=' + encodeURIComponent(ag.name))} />
                        <IconButton icon={Pencil} label={t('overview.editAgent', { name: ag.name })} size="sm" onClick={() => navigate('/agents/' + encodeURIComponent(ag.name))} />
                      </div>
                    ))}
                  </div>
                </Section>

                <Section
                  title={t('overview.channels')}
                  subtitle={t('overview.channelsSubtitle')}
                  action={
                    <Button size="sm" icon={ArrowRight} onClick={() => navigate('/channels')}>
                      {t('common.open')}
                    </Button>
                  }
                >
                  {channels.length === 0 && (
                    <div className="py-3 text-center text-sm text-muted">{t('overview.channelsEmpty')}</div>
                  )}
                  <div className="flex flex-col">
                    {channels.map((ch) => (
                      <div key={ch.index} className="flex h-8 min-w-0 items-center gap-2">
                        <Chip>{ch.type}</Chip>
                        <span className="shrink-0 text-base font-semibold">{ch.name}</span>
                        <span className="min-w-0 truncate font-mono text-xs text-muted">
                          → {orDash(ch.agent)}
                          {ch.path ? ' · ' + ch.path : ''}
                          {ch.media_enabled ? ' · ' + t('overview.media') : ''}
                        </span>
                      </div>
                    ))}
                  </div>
                </Section>
              </div>
            </div>

            {data.metrics && (() => {
              const primary = new Set(COUNTERS.map((c) => c.name))
              const extraNames = Object.keys(data.metrics.series ?? {})
                .filter((name) => !primary.has(name))
                .sort()
              if (extraNames.length === 0) return null
              return (
                <Section title={t('overview.extraMetrics')} subtitle={t('overview.extraMetricsSubtitle', { n: extraNames.length })}>
                  <div className="grid grid-cols-2 gap-2.5 lg:grid-cols-3 xl:grid-cols-4">
                    {extraNames.map((name) => {
                      const vals = (data.metrics!.series?.[name] ?? []).map((p) => p.v)
                      const delta = vals.length > 1 ? t('overview.counterDelta', { delta: humanCount(vals[vals.length - 1] - vals[0]) }) : undefined
                      return (
                        <StatTile
                          key={name}
                          label={name.replace(/^agentflow_/, '').replace(/_/g, ' ')}
                          value={humanCount(data.metrics!.latest?.[name] ?? 0)}
                          sub={delta}
                          spark={vals}
                        />
                      )
                    })}
                  </div>
                </Section>
              )
            })()}

            <Section
              title={t('overview.instances')}
              subtitle={t('overview.instancesSubtitle')}
              flush
            >
              <Table
                rows={instances}
                rowKey={(i) => i.owner}
                empty={t('overview.instancesEmpty')}
                columns={[
                  { key: 'state', title: t('overview.colState'), width: 130, render: (i) => (i.self ? <StatusChip tone="accent">{t('overview.thisInstance')}</StatusChip> : <StatusChip>{t('overview.peer')}</StatusChip>) },
                  { key: 'owner', title: t('overview.colOwner'), width: 120, mono: true, render: (i) => <span title={i.owner}>{shortID(i.owner)}</span> },
                  { key: 'region', title: t('overview.colRegion'), width: 110, render: (i) => orDash(i.region) },
                  { key: 'zone', title: t('overview.colZone'), width: 100, render: (i) => orDash(i.zone) },
                  { key: 'version', title: t('overview.colVersion'), width: 100, render: (i) => orDash(i.version) },
                  {
                    key: 'epoch',
                    title: t('overview.colEpoch'),
                    mono: true,
                    render: (i) => (
                      <span className={i.config_epoch_differs ? 'text-warning' : undefined}>
                        {i.config_epoch_differs
                          ? t('overview.epochDiffers', { epoch: orDash(shortID(i.config_epoch)) })
                          : orDash(shortID(i.config_epoch))}
                      </span>
                    ),
                  },
                  { key: 'seen', title: t('overview.colLastSeen'), width: 110, render: (i) => (now - i.last_seen > 5 ? t('overview.seenAgo', { duration: humanDuration(now - i.last_seen) }) : t('overview.seenNow')) },
                ]}
              />
            </Section>
          </div>
        )}
      </AsyncView>
    </Page>
  )
}
