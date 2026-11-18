// Usage: the token ledger's rollup for one UTC day, folded by tenant, agent or
// model. It is the figure quotas and agent budgets are enforced against.
import { useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, Download, RotateCw } from 'lucide-react'
import { AsyncView, Button, EmptyState, IconButton, Input, Page, Panel, Segmented, StatTile, Table, type Column } from '@/components/ui'
import { useAsync, useScreenActions, useStored } from '@/lib/hooks'
import { downloadText, humanCount, utcDay } from '@/lib/format'
import { useClient } from '@/state/session'
import { toast } from '@/state/ui'
import { GROUP_LABEL, buildTable, extraTotals, sharePercent, toCSV, type GroupBy, type UsageLine } from './usage/aggregate'

const GROUPS: GroupBy[] = ['tenant', 'agent', 'model', 'slice']

function shiftDay(day: string, days: number): string {
  const d = new Date(day + 'T00:00:00Z')
  d.setUTCDate(d.getUTCDate() + days)
  return utcDay(d)
}

function Share({ line, total }: { line: UsageLine; total: number }) {
  const frac = Math.min(Math.max(line.share, 0), 1)
  return (
    <div className="flex items-center gap-2" title={`${humanCount(line.billable)} of ${humanCount(total)} billable`}>
      <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-surface2">
        <div className="h-full rounded-full bg-accent" style={{ width: `${Math.max(frac * 100, line.billable > 0 ? 2 : 0)}%` }} />
      </div>
      <span className="w-10 shrink-0 text-right font-mono text-xs tabular-nums text-muted">{sharePercent(line.share, total)}</span>
    </div>
  )
}

export default function Usage() {
  const client = useClient()
  const [day, setDay] = useState(() => utcDay())
  const [stored, setGroupBy] = useStored<GroupBy>('agentflow-studio:usage-group', 'tenant')
  const groupBy = GROUPS.includes(stored) ? stored : 'tenant'
  const { data, error, loading, reload } = useAsync((signal) => client.usage(day, signal), [client, day])
  useScreenActions({ refresh: () => reload() })

  // The answer on screen must be the picked day's: a previous day's rollup is
  // not shown while the next one loads.
  const current = data && data.day === day ? data : undefined
  const today = utcDay()

  const pick = (next: string) => {
    // The ledger has no future; a cleared date input is not a day either.
    if (!/^\d{4}-\d{2}-\d{2}$/.test(next)) return
    setDay(next > today ? today : next)
  }

  const rows = current?.rows ?? []
  const extra = useMemo(() => extraTotals(current?.rows ?? []), [current])
  const table = useMemo(() => (current ? buildTable(current, groupBy) : { headers: [], lines: [] }), [current, groupBy])
  const total = current?.totals?.billable ?? 0

  const columns = useMemo<Column<UsageLine>[]>(() => {
    const slice = groupBy === 'slice'
    const labels: Column<UsageLine>[] = table.headers.map((h, i) => ({
      key: 'label' + i,
      title: h,
      width: slice ? [undefined, 130, 170][i] : undefined,
      // Agent names read as prose; tenant ids and model names are identifiers.
      mono: h !== 'agent',
      render: (l) => <span title={l.labels[i]}>{l.labels[i] === '' ? '—' : l.labels[i]}</span>,
    }))
    const num = (key: 'input' | 'output' | 'billable' | 'calls', width: number): Column<UsageLine> => ({
      key,
      title: key,
      width,
      align: 'right',
      mono: true,
      render: (l) => <span title={String(l[key])}>{humanCount(l[key])}</span>,
    })
    return [
      ...labels,
      { key: 'share', title: 'share', width: 150, render: (l) => <Share line={l} total={total} /> },
      num('input', 80),
      num('output', 80),
      num('billable', 84),
      num('calls', 64),
      {
        key: 'failed',
        title: 'failed',
        width: 64,
        align: 'right',
        mono: true,
        render: (l) => <span className={l.failed > 0 ? 'text-danger' : undefined}>{humanCount(l.failed)}</span>,
      },
    ]
  }, [table.headers, groupBy, total])

  const download = () => {
    const name = `agentflow-usage-${day}-${groupBy === 'slice' ? 'slices' : 'by-' + groupBy}.csv`
    downloadText(name, toCSV(table, groupBy !== 'slice'), 'text/csv')
    toast('exported to ' + name, 'success')
  }

  return (
    <Page
      title="Usage"
      subtitle="The token ledger's daily rollup (usage_daily): what every tenant, agent and model spent on a UTC day. It is the figure quotas and agent budgets are enforced against."
      meta={
        current ? (
          <span>
            {current.day} (UTC) · {rows.length} slices · {current.tenants} tenants
          </span>
        ) : null
      }
      actions={
        <>
          <IconButton icon={ChevronLeft} label="Previous day" onClick={() => setDay(shiftDay(day, -1))} />
          <div className="w-[140px]">
            <Input type="date" mono aria-label="Day (UTC)" title="Day (UTC)" value={day} max={today} onChange={(e) => pick(e.target.value)} />
          </div>
          <IconButton icon={ChevronRight} label="Next day" disabled={day >= today} onClick={() => pick(shiftDay(day, 1))} />
          <Button
            onClick={() => {
              if (day === today) reload()
              else setDay(today)
            }}
          >
            Today
          </Button>
          <IconButton icon={RotateCw} label="Refresh" onClick={() => reload()} />
        </>
      }
    >
      <AsyncView loading={loading || (!!data && !current)} error={error} hasData={!!current} retry={() => reload()} loadingText={`Reading the ledger for ${day}…`}>
        {current && (
          <div className="flex flex-col gap-3">
            <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
              <StatTile
                label="Billable"
                value={humanCount(total)}
                sub={`in ${humanCount(current.totals?.input ?? 0)} · out ${humanCount(current.totals?.output ?? 0)}`}
                tone="accent"
              />
              <StatTile label="Calls" value={humanCount(current.totals?.calls ?? 0)} sub={`${extra.failed} failed`} />
              <StatTile label="Cached input" value={humanCount(extra.cached)} sub="already counted in input" />
              <StatTile label="Reasoning" value={humanCount(extra.reasoning)} sub="already counted in output" />
            </div>

            {rows.length === 0 ? (
              <EmptyState
                title="Nothing metered on this day"
                guidance="Every LLM call through a metered agent is recorded here. Pick another day, or chat with an agent and refresh."
              />
            ) : (
              <>
                <Panel flush className="overflow-hidden">
                  <div className="flex flex-wrap items-center gap-2 border-b border-line px-3 py-2">
                    <Segmented value={groupBy} onChange={setGroupBy} options={GROUPS.map((g) => ({ value: g, label: GROUP_LABEL[g] }))} />
                    <span className="flex-1" />
                    <span className="font-mono text-xs text-faint">
                      {table.lines.length} {table.lines.length === 1 ? 'row' : 'rows'}
                    </span>
                    <Button size="sm" icon={Download} onClick={download}>
                      Download CSV
                    </Button>
                  </div>
                  <Table rows={table.lines} rowKey={(l) => l.id} columns={columns} />
                </Panel>
                <p className="text-xs leading-snug text-faint">
                  Billable = input + output, the rule quotas and budgets use. Per-call detail lives on each profile under Users when usage.events is on.
                </p>
              </>
            )}
          </div>
        )}
      </AsyncView>
    </Page>
  )
}
