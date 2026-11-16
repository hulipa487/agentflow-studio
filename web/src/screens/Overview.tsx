// Overview: what this runtime is doing right now. Refreshes every five
// seconds while open.
import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, FlaskConical, Pencil, Plus, RotateCw, ScrollText } from 'lucide-react'
import type { Metrics, State, StateAgent, StateChannel } from '@/api/types'
import { errorMessage } from '@/api/client'
import { AsyncView, Banner, Button, Chip, IconButton, Page, Section, StatTile, StatusChip, Table } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { humanCount, humanDuration, orDash, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { useUI } from '@/state/ui'

const COUNTERS = [
  { name: 'agentflow_llm_calls', label: 'LLM calls' },
  { name: 'agentflow_llm_tokens', label: 'LLM tokens' },
  { name: 'agentflow_ingress_total', label: 'Inbound messages' },
  { name: 'agentflow_safety_drops', label: 'Safety drops' },
]

interface OverviewData {
  state: State
  metrics: Metrics | null
  metricsError: string
}

function persistentSummary(agents: StateAgent[]): string {
  const n = agents.filter((a) => a.persistent).length
  return n === 0 ? 'none run as daemons' : `${n} daemon`
}

function channelSummary(chs: StateChannel[]): string {
  if (chs.length === 0) return 'no front door yet'
  const count = new Map<string, number>()
  for (const c of chs) count.set(c.type, (count.get(c.type) ?? 0) + 1)
  return [...count.keys()]
    .sort()
    .map((k) => `${count.get(k)} ${k}`)
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

  const st = data?.state
  const meta = useMemo(() => {
    if (!st) return null
    const parts: string[] = []
    if (st.instance) parts.push('instance ' + shortID(st.instance))
    if (st.region) parts.push('region ' + st.region)
    if (st.zone) parts.push('zone ' + st.zone)
    if (st.topology) parts.push('topology ' + st.topology)
    if ((st.instances?.length ?? 0) > 1) parts.push(`${st.instances!.length} instances`)
    parts.push('version ' + orDash(st.version), 'up ' + humanDuration(st.uptime_s))
    if (st.config_epoch) parts.push('epoch ' + st.config_epoch)
    if (st.config_path) parts.push(st.config_path)
    return parts.map((p) => <span key={p}>{p}</span>)
  }, [st])

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
      title="Overview"
      subtitle="What this runtime is doing right now. Refreshes every 5 seconds while open."
      meta={meta}
      banner={data && error ? <Banner tone="warning">refresh failed: {errorMessage(error)}</Banner> : null}
      actions={
        <>
          <Button icon={ScrollText} onClick={() => navigate('/logs')}>
            Logs
          </Button>
          <Button icon={FlaskConical} onClick={() => navigate('/playground')}>
            Playground
          </Button>
          <Button variant="primary" icon={Plus} onClick={() => useUI.getState().setNewAgent(true)}>
            New agent
          </Button>
          <IconButton icon={RotateCw} label="Refresh" onClick={() => reload()} />
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!data} retry={() => reload()} loadingText="Contacting the runtime…">
        {st && (
          <div className="flex flex-col gap-3">
            <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
              <StatTile
                label="Live sessions"
                value={st.sessions_active + st.sessions_idle}
                sub={`${st.sessions_active} busy · ${st.sessions_idle} idle`}
                tone={st.sessions_active > 0 ? 'accent' : 'neutral'}
              />
              <StatTile label="Agents" value={agents.length} sub={persistentSummary(agents)} />
              <StatTile label="Channels" value={channels.length} sub={channelSummary(channels)} />
              <StatTile
                label="Credential store"
                value={st.credentials_enabled ? 'enabled' : 'disabled'}
                sub="runtime.credentials"
                tone={st.credentials_enabled ? 'success' : 'warning'}
              />
            </div>

            {data.metrics ? (
              <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
                {COUNTERS.map((c) => {
                  const vals = (data.metrics!.series?.[c.name] ?? []).map((p) => p.v)
                  const delta = vals.length > 1 ? `+${humanCount(vals[vals.length - 1] - vals[0])} in the sampled window` : undefined
                  return <StatTile key={c.name} label={c.label} value={humanCount(data.metrics!.latest?.[c.name] ?? 0)} sub={delta} spark={vals} />
                })}
              </div>
            ) : (
              <div className="rounded-card border border-line bg-surface px-3 py-2.5 text-xs text-muted">Counters unavailable: {data.metricsError}</div>
            )}

            <div className="grid gap-3 lg:grid-cols-2">
              <Section title="Live sessions" subtitle="One actor per (agent, session key). Busy means a turn is running." flush>
                <Table
                  rows={sessions}
                  rowKey={(s) => s.session_id}
                  empty="No live sessions. A session starts on its agent's first message."
                  columns={[
                    { key: 'state', title: 'State', width: 84, render: (s) => (s.busy ? <StatusChip tone="accent" pulse>busy</StatusChip> : <StatusChip>idle</StatusChip>) },
                    { key: 'agent', title: 'Agent', width: 120, render: (s) => s.agent },
                    { key: 'session', title: 'Session', mono: true, render: (s) => <span title={s.session_id}>{s.session_id}</span> },
                    { key: 'parent', title: 'Parent', width: 96, mono: true, render: (s) => orDash(shortID(s.parent_id)) },
                  ]}
                />
              </Section>

              <div className="flex min-w-0 flex-col gap-3">
                <Section
                  title="Agents"
                  subtitle="Configured at boot; Studio-applied agents appear under Agents."
                  action={
                    <Button size="sm" icon={ArrowRight} onClick={() => navigate('/agents')}>
                      Open
                    </Button>
                  }
                >
                  {agents.length === 0 && <div className="py-3 text-center text-[12.5px] text-muted">No agents in the config. Create one from Agents.</div>}
                  <div className="flex flex-col">
                    {agents.map((ag) => (
                      <div key={ag.name} className="flex h-8 items-center gap-2">
                        <span className="truncate text-[13px] font-semibold">{ag.name}</span>
                        {ag.model && <Chip tone="accent">{ag.model}</Chip>}
                        {ag.persistent && <Chip tone="success">daemon</Chip>}
                        <span className="flex-1" />
                        <IconButton icon={FlaskConical} label={`Chat with ${ag.name}`} size="sm" onClick={() => navigate('/playground?agent=' + encodeURIComponent(ag.name))} />
                        <IconButton icon={Pencil} label={`Edit ${ag.name}`} size="sm" onClick={() => navigate('/agents/' + encodeURIComponent(ag.name))} />
                      </div>
                    ))}
                  </div>
                </Section>

                <Section
                  title="Channels"
                  subtitle="Bound when the runtime started."
                  action={
                    <Button size="sm" icon={ArrowRight} onClick={() => navigate('/channels')}>
                      Open
                    </Button>
                  }
                >
                  {channels.length === 0 && (
                    <div className="py-3 text-center text-[12.5px] text-muted">No channels. Add one from Channels; it binds at the next start.</div>
                  )}
                  <div className="flex flex-col">
                    {channels.map((ch) => (
                      <div key={ch.index} className="flex h-8 min-w-0 items-center gap-2">
                        <Chip>{ch.type}</Chip>
                        <span className="shrink-0 text-[13px] font-semibold">{ch.name}</span>
                        <span className="min-w-0 truncate font-mono text-[11px] text-muted">
                          → {orDash(ch.agent)}
                          {ch.path ? ' · ' + ch.path : ''}
                          {ch.media_enabled ? ' · media' : ''}
                        </span>
                      </div>
                    ))}
                  </div>
                </Section>
              </div>
            </div>

            <Section
              title="Instances"
              subtitle="Every process currently heartbeating on this runtime store. An epoch that differs means two instances run different configs."
              flush
            >
              <Table
                rows={instances}
                rowKey={(i) => i.owner}
                empty="No live instances reported. This runtime may predate the instance registry."
                columns={[
                  { key: 'state', title: 'State', width: 130, render: (i) => (i.self ? <StatusChip tone="accent">this instance</StatusChip> : <StatusChip>peer</StatusChip>) },
                  { key: 'owner', title: 'Owner', width: 120, mono: true, render: (i) => <span title={i.owner}>{shortID(i.owner)}</span> },
                  { key: 'region', title: 'Region', width: 110, render: (i) => orDash(i.region) },
                  { key: 'zone', title: 'Zone', width: 100, render: (i) => orDash(i.zone) },
                  { key: 'version', title: 'Version', width: 100, render: (i) => orDash(i.version) },
                  {
                    key: 'epoch',
                    title: 'Config epoch',
                    mono: true,
                    render: (i) => (
                      <span className={i.config_epoch_differs ? 'text-warning' : undefined}>
                        {orDash(shortID(i.config_epoch))}
                        {i.config_epoch_differs ? ' (differs)' : ''}
                      </span>
                    ),
                  },
                  { key: 'seen', title: 'Last seen', width: 110, render: (i) => (now - i.last_seen > 5 ? humanDuration(now - i.last_seen) + ' ago' : 'now') },
                ]}
              />
            </Section>
          </div>
        )}
      </AsyncView>
    </Page>
  )
}
