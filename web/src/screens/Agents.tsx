// Agents: every configured agent on one dense list. The row opens the
// builder; chat and delete are one click away.
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FlaskConical, Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import type { AgentRow } from '@/api/types'
import { AsyncView, Button, Chip, Chips, EmptyState, IconButton, Page, Panel, SearchBox, Section, StatusChip } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { pathTail, plural } from '@/lib/format'
import { useClient } from '@/state/session'
import { useUI } from '@/state/ui'
import { deleteAgentFlow } from './agentActions'

const GRID = 'grid grid-cols-[minmax(160px,1.1fr)_minmax(120px,1.4fr)_minmax(140px,1fr)_190px_96px] items-center gap-3'

export default function Agents() {
  const client = useClient()
  const navigate = useNavigate()
  const [filter, setFilter] = useState('')
  const { data: rows, error, loading, reload } = useAsync((signal) => client.agents(signal), [client])
  useScreenActions({ refresh: () => reload() })

  const { agents, spawn } = useMemo(() => {
    const q = filter.trim().toLowerCase()
    const match = (r: AgentRow) =>
      q === '' ||
      r.name.toLowerCase().includes(q) ||
      (r.model ?? '').toLowerCase().includes(q) ||
      (r.capabilities ?? []).join(' ').toLowerCase().includes(q)
    const all = (rows ?? []).filter(match)
    return { agents: all.filter((r) => !r.spawn), spawn: all.filter((r) => r.spawn) }
  }, [rows, filter])

  const total = rows?.length ?? 0
  const newAgent = () => useUI.getState().setNewAgent(true)

  return (
    <Page
      title="Agents"
      subtitle="Prototype a loop, apply it live, and keep the definition in your deployment config."
      actions={
        <>
          {total > 0 && <span className="font-mono text-xs text-faint">{plural(total, 'agent')}</span>}
          <SearchBox value={filter} onChange={setFilter} placeholder="Filter agents…" className="w-56" />
          <IconButton icon={RotateCw} label="Refresh" onClick={() => reload()} />
          {total > 0 && (
            <Button variant="primary" icon={Plus} onClick={newAgent}>
              New agent
            </Button>
          )}
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!rows} retry={() => reload()} loadingText="Loading agents…">
        {total === 0 ? (
          <EmptyState
            title="No agents yet"
            guidance="Start from a template: an echo agent needs no model."
            actionLabel="Create your first agent"
            onAction={newAgent}
          />
        ) : (
          <div className="flex flex-col gap-3">
            {agents.length === 0 ? (
              <div className="py-8 text-center text-sm text-muted">No agents match this filter.</div>
            ) : (
              <Panel flush className="overflow-hidden">
                <div className={`${GRID} border-b border-line px-3 py-1.5 text-xs font-semibold text-faint`}>
                  <span>Agent</span>
                  <span>Loop</span>
                  <span>Capabilities</span>
                  <span>State</span>
                  <span />
                </div>
                {agents.map((row) => {
                  const caps = row.capabilities ?? []
                  const open = () => navigate('/agents/' + encodeURIComponent(row.name))
                  return (
                    <div
                      key={row.name}
                      role="button"
                      tabIndex={0}
                      onClick={open}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') open()
                      }}
                      className={`${GRID} cursor-pointer border-b border-line/70 px-3 py-2 last:border-b-0 hover:bg-hover focus-visible:bg-hover`}
                    >
                      <div className="min-w-0">
                        <div className="truncate text-base font-medium">{row.name}</div>
                        <div className="truncate text-xs text-muted">{row.model || 'default model'}</div>
                      </div>
                      <div className="truncate font-mono text-sm text-muted" title={row.loop}>
                        {row.loop ? pathTail(row.loop) : 'in-memory loop'}
                      </div>
                      <div className="min-w-0">
                        {caps.length === 0 ? <span className="font-mono text-xs text-faint">deployment defaults</span> : <Chips items={caps} tone="accent" max={3} />}
                      </div>
                      <div className="flex flex-wrap items-center gap-1">
                        {row.persisted ? <StatusChip tone="success">persisted</StatusChip> : <StatusChip tone="warning">runtime only</StatusChip>}
                        {row.sessions > 0 && (
                          <StatusChip tone="accent" pulse>
                            {row.sessions} live
                          </StatusChip>
                        )}
                        {row.persistent && <Chip tone="success">daemon</Chip>}
                      </div>
                      <div className="flex items-center justify-end" onClick={(e) => e.stopPropagation()}>
                        <IconButton icon={FlaskConical} label={`Chat with ${row.name}`} onClick={() => navigate('/playground?agent=' + encodeURIComponent(row.name))} />
                        <IconButton icon={Pencil} label={`Edit ${row.name}`} onClick={open} />
                        <IconButton
                          icon={Trash2}
                          label={`Delete ${row.name}`}
                          tone="danger"
                          onClick={async () => {
                            if (await deleteAgentFlow(client, row.name)) reload(true)
                          }}
                        />
                      </div>
                    </div>
                  )
                })}
              </Panel>
            )}
            {spawn.length > 0 && (
              <Section title="Spawn profiles" subtitle="Templates used by agent.spawn; read-only in Studio.">
                <Chips items={spawn.map((r) => r.name)} />
              </Section>
            )}
          </div>
        )}
      </AsyncView>
    </Page>
  )
}
