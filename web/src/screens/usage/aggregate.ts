// The Usage screen's arithmetic, ported from the desktop Studio
// (internal/ui/usage.go): totals the tiles show, and the ledger rows folded by
// tenant, agent or model.
import { billable, type UsageDay, type UsageRow } from '@/api/types'

export type GroupBy = 'tenant' | 'agent' | 'model' | 'slice'

export const GROUP_LABEL: Record<GroupBy, string> = {
  tenant: 'By tenant',
  agent: 'By agent',
  model: 'By model',
  slice: 'Every slice',
}

/** One table row: a group of ledger rows, or a single (tenant, agent, model) slice. */
export interface UsageLine {
  id: string
  /** One label per leading column: the group key, or tenant / agent / model. */
  labels: string[]
  input: number
  output: number
  billable: number
  calls: number
  failed: number
  /** How many ledger rows were folded into this line. */
  slices: number
  /** This line's part of the day's billable tokens, 0..1. */
  share: number
}

export interface UsageTable {
  /** Titles of the leading label columns. */
  headers: string[]
  lines: UsageLine[]
}

/** The day's figures the rollup's own totals leave out. */
export function extraTotals(rows: UsageRow[]): { cached: number; reasoning: number; failed: number } {
  let cached = 0
  let reasoning = 0
  let failed = 0
  for (const r of rows) {
    cached += r.cached || 0
    reasoning += r.reasoning || 0
    failed += r.failed || 0
  }
  return { cached, reasoning, failed }
}

// Calls made outside any tenant (operator chat, daemons) carry an empty id.
export function tenantLabel(id: string): string {
  return id === '' || id == null ? '(no tenant)' : id
}

// Largest spender first. The desktop sorts on billable alone; the label breaks
// ties here so equal rows do not swap places between refreshes.
function byBillable(a: UsageLine, b: UsageLine): number {
  if (a.billable !== b.billable) return b.billable - a.billable
  const x = a.labels.join('\u0000')
  const y = b.labels.join('\u0000')
  return x < y ? -1 : x > y ? 1 : 0
}

export function buildTable(day: UsageDay, groupBy: GroupBy): UsageTable {
  const rows = day.rows ?? []
  const total = day.totals?.billable ?? 0
  const share = (n: number) => (total > 0 ? n / total : 0)

  if (groupBy === 'slice') {
    const lines = rows.map<UsageLine>((r, i) => ({
      id: String(i),
      labels: [tenantLabel(r.user_id), r.agent ?? '', r.model ?? ''],
      input: r.input || 0,
      output: r.output || 0,
      billable: billable(r),
      calls: r.calls || 0,
      failed: r.failed || 0,
      slices: 1,
      share: share(billable(r)),
    }))
    return { headers: ['tenant', 'agent', 'model'], lines: lines.sort(byBillable) }
  }

  const keyOf = (r: UsageRow): string =>
    groupBy === 'agent' ? (r.agent ?? '') : groupBy === 'model' ? (r.model ?? '') : tenantLabel(r.user_id)
  const groups = new Map<string, UsageLine>()
  for (const r of rows) {
    const k = keyOf(r)
    let g = groups.get(k)
    if (!g) {
      g = { id: k, labels: [k], input: 0, output: 0, billable: 0, calls: 0, failed: 0, slices: 0, share: 0 }
      groups.set(k, g)
    }
    g.input += r.input || 0
    g.output += r.output || 0
    g.calls += r.calls || 0
    g.failed += r.failed || 0
    g.slices++
  }
  const lines = [...groups.values()]
  for (const g of lines) {
    g.billable = g.input + g.output
    g.share = share(g.billable)
  }
  return { headers: [groupBy], lines: lines.sort(byBillable) }
}

export function sharePercent(share: number, total: number): string {
  if (total <= 0) return '—'
  const pct = share * 100
  if (pct > 0 && pct < 0.1) return '<0.1%'
  return (pct >= 10 ? pct.toFixed(0) : pct.toFixed(1)) + '%'
}

function csvCell(v: string | number): string {
  const s = String(v)
  return /[",\r\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s
}

/** The visible rows as CSV, with exact token counts rather than the rounded ones on screen. */
export function toCSV(table: UsageTable, grouped: boolean): string {
  const head = [...table.headers, ...(grouped ? ['slices'] : []), 'share_pct', 'input', 'output', 'billable', 'calls', 'failed']
  const out = [head.map(csvCell).join(',')]
  for (const l of table.lines) {
    const cells: (string | number)[] = [...l.labels]
    if (grouped) cells.push(l.slices)
    cells.push((l.share * 100).toFixed(2), l.input, l.output, l.billable, l.calls, l.failed)
    out.push(cells.map(csvCell).join(','))
  }
  return out.join('\r\n') + '\r\n'
}
