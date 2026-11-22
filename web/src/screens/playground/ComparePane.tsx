// Compare pane: side-by-side latency, token, output, and aligned trace diff
// across pinned live threads and Replay & Eval runs.
import { useMemo } from 'react'
import { Chip, EmptyState, Select, cx } from '@/components/ui'
import { t } from '@/i18n'
import { compareRuns, type PlaygroundRun, type RunOpDiffRow } from './replay'

function formatDelta(v: number, unit = ''): string {
  if (v === 0) return `0${unit}`
  return `${v > 0 ? '+' : ''}${v}${unit}`
}

function stepLabel(ev?: { kind: string; op?: string; tool?: string; model?: string; err?: string; duration_ms?: number }): string {
  if (!ev) return '—'
  const parts = [ev.op || ev.kind, ev.tool, ev.model].filter(Boolean)
  if ((ev.duration_ms ?? 0) > 0) parts.push(`${ev.duration_ms}ms`)
  return parts.join(' · ')
}

const DIFF_BADGE: Record<RunOpDiffRow['status'], { labelKey: string; cls: string }> = {
  same: { labelKey: 'playground.compareSame', cls: 'text-muted bg-surface2' },
  changed: { labelKey: 'playground.compareChanged', cls: 'text-warning bg-tone-warning' },
  added: { labelKey: 'playground.compareAdded', cls: 'text-success bg-tone-success' },
  removed: { labelKey: 'playground.compareRemoved', cls: 'text-danger bg-tone-danger' },
}

export function ComparePane({
  runs,
  baseId,
  candId,
  onSelectBase,
  onSelectCand,
}: {
  runs: PlaygroundRun[]
  baseId: string
  candId: string
  onSelectBase: (id: string) => void
  onSelectCand: (id: string) => void
}) {
  const baseRun = runs.find((r) => r.id === baseId) ?? runs[1] ?? runs[0]
  const candRun = runs.find((r) => r.id === candId) ?? runs[0]

  const comparison = useMemo(() => {
    if (!baseRun || !candRun || runs.length < 2) return null
    return compareRuns(baseRun, candRun)
  }, [baseRun, candRun, runs.length])

  if (runs.length < 2 || !comparison || !baseRun || !candRun) {
    return (
      <div className="flex h-full min-h-0 flex-col p-3">
        <EmptyState title={t('playground.compareTimeline')} guidance={t('playground.compareNeedTwo')} />
      </div>
    )
  }

  const options = runs.map((r) => ({
    value: r.id,
    label: `[${r.source === 'live' ? t('playground.compareSourceLive') : t('playground.compareSourceReplay')}] ${r.label}`,
  }))

  const metrics = [
    {
      label: t('playground.compareStatus'),
      a: comparison.base.ok ? t('playground.comparePass') : t('playground.compareFail'),
      b: comparison.cand.ok ? t('playground.comparePass') : t('playground.compareFail'),
      delta: '',
      tone: comparison.cand.ok ? 'text-success' : 'text-danger',
    },
    {
      label: t('playground.compareDuration'),
      a: `${comparison.base.durationMs}ms`,
      b: `${comparison.cand.durationMs}ms`,
      delta: formatDelta(comparison.delta.durationMs, 'ms'),
      tone: comparison.delta.durationMs <= 0 ? 'text-success' : 'text-warning',
    },
    {
      label: t('playground.compareTokens'),
      a: `${comparison.base.tokensIn}→${comparison.base.tokensOut}`,
      b: `${comparison.cand.tokensIn}→${comparison.cand.tokensOut}`,
      delta: formatDelta(comparison.delta.tokensIn + comparison.delta.tokensOut - 0),
      tone: comparison.delta.tokensIn + comparison.delta.tokensOut <= 0 ? 'text-success' : 'text-warning',
    },
    {
      label: t('playground.compareOps'),
      a: String(comparison.base.opCount),
      b: String(comparison.cand.opCount),
      delta: formatDelta(comparison.delta.opCount),
      tone: 'text-muted',
    },
    {
      label: t('playground.compareErrors'),
      a: String(comparison.base.errorCount),
      b: String(comparison.cand.errorCount),
      delta: formatDelta(comparison.delta.errorCount),
      tone: comparison.delta.errorCount <= 0 ? 'text-muted' : 'text-danger',
    },
  ]

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto p-2.5">
      <div className="grid shrink-0 grid-cols-2 gap-2 border-b border-line pb-2.5">
        <label className="flex min-w-0 flex-col gap-1">
          <span className="text-xs font-semibold text-faint">{t('playground.compareBaseline')}</span>
          <Select aria-label={t('playground.compareBaseline')} value={baseRun.id} onChange={onSelectBase} options={options} />
        </label>
        <label className="flex min-w-0 flex-col gap-1">
          <span className="text-xs font-semibold text-faint">{t('playground.compareCandidate')}</span>
          <Select aria-label={t('playground.compareCandidate')} value={candRun.id} onChange={onSelectCand} options={options} />
        </label>
      </div>

      <div className="mt-2.5 grid shrink-0 grid-cols-2 gap-1.5 sm:grid-cols-3">
        {metrics.map((m) => (
          <div key={m.label} className="rounded-ctl border border-line bg-bg/60 px-2 py-1.5">
            <div className="text-[10px] font-medium uppercase tracking-wide text-faint">{m.label}</div>
            <div className="mt-0.5 flex items-baseline justify-between gap-1 font-mono text-xs">
              <span className="truncate text-text">{`${m.a} → ${m.b}`}</span>
              {m.delta !== '' && <span className={cx('shrink-0 font-semibold', m.tone)}>{m.delta}</span>}
            </div>
          </div>
        ))}
      </div>

      {comparison.replyDiff.length > 0 && (
        <div className="mt-3 flex flex-col gap-1.5">
          <div className="text-xs font-semibold text-faint">{t('playground.compareReplies')}</div>
          {comparison.replyDiff.map((row) => (
            <div
              key={row.index}
              className={cx('grid grid-cols-2 gap-2 rounded-ctl border p-2 text-xs', row.same ? 'border-line bg-bg/50' : 'border-warning/40 bg-tone-warning/40')}
            >
              <pre className="whitespace-pre-wrap break-words font-sans text-muted">{row.base || '—'}</pre>
              <pre className="whitespace-pre-wrap break-words font-sans text-text">{row.cand || '—'}</pre>
            </div>
          ))}
        </div>
      )}

      <div className="mt-3 flex flex-col gap-1">
        <div className="text-xs font-semibold text-faint">{t('playground.compareTimeline')}</div>
        {comparison.opDiff.map((row) => {
          const badge = DIFF_BADGE[row.status]
          return (
            <div key={row.index} className="flex flex-col gap-1 rounded-ctl border border-line bg-bg/50 px-2 py-1.5">
              <div className="flex items-center justify-between gap-2">
                <span className="font-mono text-[10px] text-faint">#{row.index + 1}</span>
                <Chip className={badge.cls}>{t(badge.labelKey)}</Chip>
              </div>
              <div className="grid grid-cols-2 gap-2 font-mono text-xs">
                <span className="truncate text-muted" title={stepLabel(row.base)}>
                  {stepLabel(row.base)}
                </span>
                <span className="truncate text-text" title={stepLabel(row.cand)}>
                  {stepLabel(row.cand)}
                </span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
