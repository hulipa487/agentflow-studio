// Usage: the token ledger's rollup for one UTC day, folded by tenant, agent or
// model. It is the figure quotas and agent budgets are enforced against.
import { useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, Download, RotateCw } from 'lucide-react'
import { AsyncView, Button, EmptyState, IconButton, Input, Page, Panel, Segmented, StatTile, Table, type Column } from '@/components/ui'
import { t, useLang } from '@/i18n'
import { useAsync, useScreenActions, useStored } from '@/lib/hooks'
import { downloadText, humanCount, utcDay } from '@/lib/format'
import { useClient } from '@/state/session'
import { toast } from '@/state/ui'
import { COLUMN_TITLE_KEY, GROUP_LABEL_KEY, buildTable, extraTotals, sharePercent, toCSV, type GroupBy, type UsageLine } from './usage/aggregate'

const GROUPS: GroupBy[] = ['tenant', 'agent', 'model', 'slice']

function shiftDay(day: string, days: number): string {
  const d = new Date(day + 'T00:00:00Z')
  d.setUTCDate(d.getUTCDate() + days)
  return utcDay(d)
}

function Share({ line, total }: { line: UsageLine; total: number }) {
  const frac = Math.min(Math.max(line.share, 0), 1)
  return (
    <div className="flex items-center gap-2" title={t('usage.shareTitle', { part: humanCount(line.billable), total: humanCount(total) })}>
      <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-surface2">
        <div className="h-full rounded-full bg-accent" style={{ width: `${Math.max(frac * 100, line.billable > 0 ? 2 : 0)}%` }} />
      </div>
      <span className="w-10 shrink-0 text-right font-mono text-xs tabular-nums text-muted">{sharePercent(line.share, total)}</span>
    </div>
  )
}

export default function Usage() {
  const client = useClient()
  const lang = useLang()
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
  // The no-tenant label is translated text, so the table follows the language.
  const table = useMemo(() => (current ? buildTable(current, groupBy) : { headers: [], lines: [] }), [current, groupBy, lang])
  const total = current?.totals?.billable ?? 0

  const columns = useMemo<Column<UsageLine>[]>(() => {
    const slice = groupBy === 'slice'
    const labels: Column<UsageLine>[] = table.headers.map((h, i) => ({
      key: 'label' + i,
      title: t(COLUMN_TITLE_KEY[h] ?? h),
      width: slice ? [undefined, 130, 170][i] : undefined,
      // Agent names read as prose; tenant ids and model names are identifiers.
      mono: h !== 'agent',
      render: (l) => <span title={l.labels[i]}>{l.labels[i] === '' ? '—' : l.labels[i]}</span>,
    }))
    const num = (key: 'input' | 'output' | 'billable' | 'calls', width: number): Column<UsageLine> => ({
      key,
      title: t('usage.col.' + key),
      width,
      align: 'right',
      mono: true,
      render: (l) => <span title={String(l[key])}>{humanCount(l[key])}</span>,
    })
    return [
      ...labels,
      { key: 'share', title: t('usage.col.share'), width: 150, render: (l) => <Share line={l} total={total} /> },
      num('input', 80),
      num('output', 80),
      num('billable', 84),
      num('calls', 64),
      {
        key: 'failed',
        title: t('usage.col.failed'),
        width: 64,
        align: 'right',
        mono: true,
        render: (l) => <span className={l.failed > 0 ? 'text-danger' : undefined}>{humanCount(l.failed)}</span>,
      },
    ]
  }, [table.headers, groupBy, total, lang])

  const download = () => {
    const name = `agentflow-usage-${day}-${groupBy === 'slice' ? 'slices' : 'by-' + groupBy}.csv`
    downloadText(name, toCSV(table, groupBy !== 'slice'), 'text/csv')
    toast(t('usage.exported', { name }), 'success')
  }

  return (
    <Page
      title={t('usage.title')}
      subtitle={t('usage.subtitle')}
      meta={
        current ? (
          <span>{t('usage.meta', { day: current.day, slices: rows.length, tenants: current.tenants })}</span>
        ) : null
      }
      actions={
        <>
          <IconButton icon={ChevronLeft} label={t('usage.prevDay')} onClick={() => setDay(shiftDay(day, -1))} />
          <div className="w-[140px]">
            <Input type="date" mono aria-label={t('usage.day')} title={t('usage.day')} value={day} max={today} onChange={(e) => pick(e.target.value)} />
          </div>
          <IconButton icon={ChevronRight} label={t('usage.nextDay')} disabled={day >= today} onClick={() => pick(shiftDay(day, 1))} />
          <Button
            onClick={() => {
              if (day === today) reload()
              else setDay(today)
            }}
          >
            {t('usage.today')}
          </Button>
          <IconButton icon={RotateCw} label={t('common.refresh')} onClick={() => reload()} />
        </>
      }
    >
      <AsyncView loading={loading || (!!data && !current)} error={error} hasData={!!current} retry={() => reload()} loadingText={t('usage.loading', { day })}>
        {current && (
          <div className="flex flex-col gap-3">
            <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
              <StatTile
                label={t('usage.billable')}
                value={humanCount(total)}
                sub={t('usage.inOut', { input: humanCount(current.totals?.input ?? 0), output: humanCount(current.totals?.output ?? 0) })}
                tone="accent"
              />
              <StatTile label={t('usage.calls')} value={humanCount(current.totals?.calls ?? 0)} sub={t('usage.failedCount', { n: extra.failed })} />
              <StatTile label={t('usage.cachedInput')} value={humanCount(extra.cached)} sub={t('usage.inInput')} />
              <StatTile label={t('usage.reasoning')} value={humanCount(extra.reasoning)} sub={t('usage.inOutput')} />
            </div>

            {rows.length === 0 ? (
              <EmptyState
                title={t('usage.emptyTitle')}
                guidance={t('usage.emptyGuidance')}
              />
            ) : (
              <>
                <Panel flush className="overflow-hidden">
                  <div className="flex flex-wrap items-center gap-2 border-b border-line px-3 py-2">
                    <Segmented value={groupBy} onChange={setGroupBy} options={GROUPS.map((g) => ({ value: g, label: t(GROUP_LABEL_KEY[g]) }))} />
                    <span className="flex-1" />
                    <span className="font-mono text-xs text-faint">{t('usage.rows', { n: table.lines.length })}</span>
                    <Button size="sm" icon={Download} onClick={download}>
                      {t('usage.downloadCsv')}
                    </Button>
                  </div>
                  <Table rows={table.lines} rowKey={(l) => l.id} columns={columns} />
                </Panel>
                <p className="text-xs leading-snug text-faint">
                  {t('usage.footnote')}
                </p>
              </>
            )}
          </div>
        )}
      </AsyncView>
    </Page>
  )
}
