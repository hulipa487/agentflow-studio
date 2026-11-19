// Agents: every configured agent on one dense list. The row opens the
// builder; chat and delete are one click away.
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FlaskConical, Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import type { AgentRow } from '@/api/types'
import { AsyncView, Button, Chip, Chips, EmptyState, IconButton, Page, Panel, SearchBox, Section, StatusChip } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { pathTail } from '@/lib/format'
import { t } from '@/i18n'
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
      title={t('agents.title')}
      subtitle={t('agents.subtitle')}
      actions={
        <>
          {total > 0 && <span className="font-mono text-xs text-faint">{t('agents.count', { n: total })}</span>}
          <SearchBox value={filter} onChange={setFilter} placeholder={t('agents.filterPlaceholder')} className="w-56" />
          <IconButton icon={RotateCw} label={t('common.refresh')} onClick={() => reload()} />
          {total > 0 && (
            <Button variant="primary" icon={Plus} onClick={newAgent}>
              {t('agents.newAgent')}
            </Button>
          )}
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!rows} retry={() => reload()} loadingText={t('agents.loading')}>
        {total === 0 ? (
          <EmptyState
            title={t('agents.emptyTitle')}
            guidance={t('agents.emptyGuidance')}
            actionLabel={t('agents.emptyAction')}
            onAction={newAgent}
          />
        ) : (
          <div className="flex flex-col gap-3">
            {agents.length === 0 ? (
              <div className="py-8 text-center text-sm text-muted">{t('agents.noMatch')}</div>
            ) : (
              <Panel flush className="overflow-hidden">
                <div className={`${GRID} border-b border-line px-3 py-1.5 text-xs font-semibold text-faint`}>
                  <span>{t('agents.colAgent')}</span>
                  <span>{t('agents.colLoop')}</span>
                  <span>{t('agents.colCapabilities')}</span>
                  <span>{t('agents.colState')}</span>
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
                        <div className="truncate text-xs text-muted">{row.model || t('agents.defaultModel')}</div>
                      </div>
                      <div className="truncate font-mono text-sm text-muted" title={row.loop}>
                        {row.loop ? pathTail(row.loop) : t('agents.inMemoryLoop')}
                      </div>
                      <div className="min-w-0">
                        {caps.length === 0 ? <span className="font-mono text-xs text-faint">{t('agents.deploymentDefaults')}</span> : <Chips items={caps} tone="accent" max={3} />}
                      </div>
                      <div className="flex flex-wrap items-center gap-1">
                        {row.persisted ? <StatusChip tone="success">{t('agents.persisted')}</StatusChip> : <StatusChip tone="warning">{t('agents.runtimeOnly')}</StatusChip>}
                        {row.sessions > 0 && (
                          <StatusChip tone="accent" pulse>
                            {t('agents.liveSessions', { n: row.sessions })}
                          </StatusChip>
                        )}
                        {row.persistent && <Chip tone="success">{t('agents.daemon')}</Chip>}
                      </div>
                      <div className="flex items-center justify-end" onClick={(e) => e.stopPropagation()}>
                        <IconButton icon={FlaskConical} label={t('agents.chatWith', { name: row.name })} onClick={() => navigate('/playground?agent=' + encodeURIComponent(row.name))} />
                        <IconButton icon={Pencil} label={t('agents.editWhat', { name: row.name })} onClick={open} />
                        <IconButton
                          icon={Trash2}
                          label={t('agents.deleteWhat', { name: row.name })}
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
              <Section title={t('agents.spawnTitle')} subtitle={t('agents.spawnSubtitle')}>
                <Chips items={spawn.map((r) => r.name)} />
              </Section>
            )}
          </div>
        )}
      </AsyncView>
    </Page>
  )
}
