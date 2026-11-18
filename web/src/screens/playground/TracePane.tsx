// Right pane: the live execution trace of the open thread.
import { memo, useMemo, useState } from 'react'
import { Copy, RotateCw } from 'lucide-react'
import type { TraceEvent } from '@/api/types'
import { Button, Dot, IconButton, Modal, Panel, Select, cx } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { copyText } from '@/lib/format'
import { toast } from '@/state/ui'
import { FILTERS, eventKey, matchesFilter, traceCost, traceDetail, traceTokens, traceTone, type TraceFilter } from './model'
import { useStickToBottom } from './useStickToBottom'

// One event as a two-line entry: sequence and kind on the first line with the
// cost on the right, the op/tool/model (and the error) beneath. The whole row
// opens the raw event.
const TraceRow = memo(function TraceRow({ ev, onOpen }: { ev: TraceEvent; onOpen: (ev: TraceEvent) => void }) {
  const tone = traceTone(ev)
  const cost = traceCost(ev)
  const detail = traceDetail(ev)
  return (
    <button
      type="button"
      onClick={() => onOpen(ev)}
      title="Show the raw event"
      className="flex w-full shrink-0 flex-col gap-0.5 rounded-[7px] px-2 py-1.5 text-left transition-colors duration-100 hover:bg-surface2 focus-visible:bg-surface2 active:bg-pressed"
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
      title={ev ? `Trace event #${ev.seq}` : ''}
      subtitle={ev?.kind}
      width={680}
      footer={
        <>
          <Button
            icon={Copy}
            onClick={async () => {
              if (await copyText(raw)) toast('event copied', 'success')
              else toast('could not reach the clipboard', 'danger')
            }}
          >
            Copy JSON
          </Button>
          <Button variant="primary" onClick={onClose}>
            Done
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
}: {
  /** null while nothing has been traced or loaded for this thread yet. */
  events: TraceEvent[] | null
  /** Changes when another thread is opened, so the list starts at its end again. */
  scope: string
  filter: TraceFilter
  onFilter: (f: TraceFilter) => void
  onRefresh: () => void
  refreshing: boolean
}) {
  const [open, setOpen] = useState<TraceEvent | null>(null)
  const shown = useMemo(() => (events ?? []).filter((ev) => matchesFilter(ev, filter)), [events, filter])
  const { ref, onScroll } = useStickToBottom<HTMLDivElement>(events, scope + '\n' + filter)

  return (
    <Panel flush className="flex h-full min-h-0 flex-col">
      <header className="flex shrink-0 items-center gap-2 border-b border-line py-2 pl-3 pr-2">
        <h2 className="text-md font-semibold leading-tight">Trace</h2>
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">{events !== null && `${traceTokens(events)} tok`}</span>
        <Select aria-label="Trace filter" className="w-[124px] shrink-0" value={filter} onChange={(v) => onFilter(v as TraceFilter)} options={[...FILTERS]} />
        <IconButton icon={RotateCw} label="Refresh trace" onClick={onRefresh} loading={refreshing} />
      </header>
      <div ref={ref} onScroll={onScroll} className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto p-1.5">
        {events !== null && shown.length === 0 && <div className="px-2 py-1.5 text-sm text-muted">No events match this filter yet.</div>}
        {shown.map((ev) => (
          <TraceRow key={eventKey(ev)} ev={ev} onOpen={setOpen} />
        ))}
      </div>
      <EventModal ev={open} onClose={() => setOpen(null)} />
    </Panel>
  )
}
