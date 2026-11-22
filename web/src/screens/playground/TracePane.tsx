// Right pane: the live execution trace of the open thread, with interactive
// step-by-step time-travel scrubbing across recorded events.
import { memo, useEffect, useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, Copy, FlaskConical, Pause, Pin, Play, RotateCw, SkipForward } from 'lucide-react'
import type { TraceEvent } from '@/api/types'
import { Button, Dot, IconButton, Modal, Panel, Select, cx } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { t, useLang } from '@/i18n'
import { copyText } from '@/lib/format'
import { toast } from '@/state/ui'
import { FILTERS, FILTER_LABEL_KEY, eventKey, matchesFilter, traceCost, traceDetail, traceTokens, traceTone, type TraceFilter } from './model'
import { sliceTraceAt } from './replay'
import { useStickToBottom } from './useStickToBottom'

// One event as a two-line entry: sequence and kind on the first line with the
// cost on the right, the op/tool/model (and the error) beneath. The whole row
// opens the raw event.
const TraceRow = memo(function TraceRow({
  ev,
  state = 'normal',
  onOpen,
}: {
  ev: TraceEvent
  state?: 'normal' | 'active' | 'future'
  onOpen: (ev: TraceEvent) => void
}) {
  // memo() skips the tree's re-render on a language change; this subscribes.
  useLang()
  const tone = traceTone(ev)
  const cost = traceCost(ev)
  const detail = traceDetail(ev)
  return (
    <button
      type="button"
      onClick={() => onOpen(ev)}
      title={t('playground.showRawEvent')}
      className={cx(
        'flex w-full shrink-0 flex-col gap-0.5 rounded-[7px] border px-2 py-1.5 text-left transition-colors duration-100 hover:bg-surface2 focus-visible:bg-surface2 active:bg-pressed',
        state === 'active' ? 'border-accent/50 bg-tone-accent/70' : 'border-transparent',
        state === 'future' && 'opacity-40',
      )}
    >
      <span className="flex w-full min-w-0 items-center gap-2">
        <Dot tone={tone} />
        <span className="shrink-0 font-mono text-xs text-faint">#{ev.seq}</span>
        <span className={cx('min-w-0 truncate font-mono text-sm font-bold', tone === 'neutral' ? 'text-muted' : tone === 'danger' ? 'text-danger' : 'text-text')}>
          {ev.kind}
        </span>
        {cost !== '' && <span className="ml-auto shrink-0 pl-2 font-mono text-xs tabular-nums text-muted">{cost}</span>}
      </span>
      {detail !== '' && <span className="w-full truncate whitespace-pre pl-4 font-mono text-xs text-muted">{detail}</span>}
      {ev.err && <span className="w-full whitespace-pre-wrap break-words pl-4 font-mono text-xs text-danger">{ev.err}</span>}
    </button>
  )
})

function EventModal({ ev, onClose }: { ev: TraceEvent | null; onClose: () => void }) {
  // The event exactly as the runtime sent it, including fields the typed
  // model does not name (a safety verdict, say).
  const raw = useMemo(() => (ev ? JSON.stringify(ev, null, 2) : ''), [ev])
  return (
    <Modal
      open={ev !== null}
      onClose={onClose}
      title={ev ? t('playground.eventTitle', { seq: ev.seq }) : ''}
      subtitle={ev?.kind}
      width={680}
      footer={
        <>
          <Button
            icon={Copy}
            onClick={async () => {
              if (await copyText(raw)) toast(t('playground.eventCopied'), 'success')
              else toast(t('common.copyFailed'), 'danger')
            }}
          >
            {t('playground.copyJson')}
          </Button>
          <Button variant="primary" onClick={onClose}>
            {t('common.done')}
          </Button>
        </>
      }
    >
      <CodeEditor value={raw} language="json" readOnly lineWrapping className="h-[400px]" />
    </Modal>
  )
}

export function TracePane({
  events,
  scope,
  filter,
  onFilter,
  onRefresh,
  refreshing,
  embedded = false,
  onPinRun,
  onExportScenario,
}: {
  /** null while nothing has been traced or loaded for this thread yet. */
  events: TraceEvent[] | null
  /** Changes when another thread is opened, so the list starts at its end again. */
  scope: string
  filter: TraceFilter
  onFilter: (f: TraceFilter) => void
  onRefresh: () => void
  refreshing: boolean
  embedded?: boolean
  onPinRun?: () => void
  onExportScenario?: () => void
}) {
  const [open, setOpen] = useState<TraceEvent | null>(null)
  const [scrubStep, setScrubStep] = useState<number | null>(null)
  const [playing, setPlaying] = useState(false)

  useEffect(() => {
    setScrubStep(null)
    setPlaying(false)
  }, [scope])

  const total = events?.length ?? 0
  useEffect(() => {
    if (!playing || total === 0) return
    const timer = setInterval(() => {
      setScrubStep((cur) => {
        const next = cur === null ? 0 : cur + 1
        if (next >= total - 1) {
          setPlaying(false)
          return total - 1
        }
        return next
      })
    }, 450)
    return () => clearInterval(timer)
  }, [playing, total])

  const activeStep = total === 0 ? null : scrubStep === null ? total - 1 : Math.min(scrubStep, total - 1)
  const isScrubbing = scrubStep !== null && activeStep !== null
  const slice = useMemo(() => (events && activeStep !== null ? sliceTraceAt(events, activeStep) : null), [events, activeStep])
  const cutoffKey = isScrubbing && slice?.current ? eventKey(slice.current) : ''

  const shown = useMemo(() => (events ?? []).filter((ev) => matchesFilter(ev, filter)), [events, filter])
  const activeKeySet = useMemo(() => {
    if (!isScrubbing || !slice) return null
    return new Set(slice.visibleEvents.map((ev) => eventKey(ev)))
  }, [isScrubbing, slice])

  const { ref, onScroll } = useStickToBottom<HTMLDivElement>(events, scope + '\n' + filter)

  const togglePlay = () => {
    if (total === 0) return
    if (playing) {
      setPlaying(false)
      return
    }
    if (scrubStep === null || scrubStep >= total - 1) {
      setScrubStep(0)
    }
    setPlaying(true)
  }

  const content = (
    <>
      <header className="flex shrink-0 items-center gap-1.5 border-b border-line py-2 pl-3 pr-2">
        {!embedded && <h2 className="text-md font-semibold leading-tight">{t('playground.trace')}</h2>}
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">{events !== null && t('playground.traceTokens', { n: traceTokens(events) })}</span>
        <Select
          aria-label={t('playground.traceFilter')}
          className="w-[116px] shrink-0"
          value={filter}
          onChange={(v) => onFilter(v as TraceFilter)}
          options={FILTERS.map((f) => ({ value: f, label: t(FILTER_LABEL_KEY[f]) }))}
        />
        {onPinRun && total > 0 && <IconButton icon={Pin} label={t('playground.pinRun')} onClick={onPinRun} />}
        {onExportScenario && total > 0 && <IconButton icon={FlaskConical} label={t('playground.exportScenario')} onClick={onExportScenario} />}
        <IconButton icon={RotateCw} label={t('playground.refreshTrace')} onClick={onRefresh} loading={refreshing} />
      </header>

      {total > 0 && slice && activeStep !== null && (
        <div className="flex shrink-0 flex-col gap-1.5 border-b border-line bg-bg/60 px-2.5 py-2">
          <div className="flex items-center gap-1">
            <IconButton
              icon={ChevronLeft}
              label={t('playground.timeTravelPrev')}
              disabled={activeStep <= 0}
              onClick={() => {
                setPlaying(false)
                setScrubStep(Math.max(0, activeStep - 1))
              }}
            />
            <IconButton icon={playing ? Pause : Play} label={playing ? t('playground.timeTravelPause') : t('playground.timeTravelPlay')} onClick={togglePlay} />
            <IconButton
              icon={ChevronRight}
              label={t('playground.timeTravelNext')}
              disabled={activeStep >= total - 1}
              onClick={() => {
                setPlaying(false)
                setScrubStep(Math.min(total - 1, activeStep + 1))
              }}
            />
            <input
              type="range"
              aria-label={t('playground.timeTravelStep', { step: activeStep + 1, total })}
              min={0}
              max={total - 1}
              value={activeStep}
              onChange={(e) => {
                setPlaying(false)
                setScrubStep(Number(e.target.value))
              }}
              className="mx-1 h-1.5 min-w-0 flex-1 cursor-pointer accent-accent"
            />
            <IconButton
              icon={SkipForward}
              label={t('playground.timeTravelLive')}
              disabled={!isScrubbing && !playing}
              onClick={() => {
                setPlaying(false)
                setScrubStep(null)
              }}
            />
          </div>
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-0.5 font-mono text-xs text-muted">
            <span className={cx('font-semibold', isScrubbing && 'text-accent')}>{t('playground.timeTravelStep', { step: activeStep + 1, total })}</span>
            <span>{t('playground.timeTravelElapsed', { ms: slice.elapsedMs })}</span>
            {slice.tokensIn + slice.tokensOut > 0 && <span>{t('playground.costTokens', { in: slice.tokensIn, out: slice.tokensOut })}</span>}
            <span>{t('playground.timeTravelOps', { n: slice.opsSoFar })}</span>
            {slice.errorCount > 0 && <span className="text-danger">{t('playground.timeTravelErrors', { n: slice.errorCount })}</span>}
          </div>
        </div>
      )}

      <div ref={ref} onScroll={onScroll} className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-1.5">
        {events !== null && shown.length === 0 && <div className="px-2 py-1.5 text-sm text-muted">{t('playground.noEventsMatch')}</div>}
        {shown.map((ev) => {
          const k = eventKey(ev)
          let state: 'normal' | 'active' | 'future' = 'normal'
          if (isScrubbing && activeKeySet) {
            if (k === cutoffKey) state = 'active'
            else if (!activeKeySet.has(k)) state = 'future'
          }
          return <TraceRow key={k} ev={ev} state={state} onOpen={setOpen} />
        })}
      </div>
      <EventModal ev={open} onClose={() => setOpen(null)} />
    </>
  )

  if (embedded) return <div className="flex h-full min-h-0 flex-col">{content}</div>
  return (
    <Panel flush className="flex h-full min-h-0 flex-col">
      {content}
    </Panel>
  )
}
