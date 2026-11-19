// The Playground's three-pane workspace. The kit's Split is two panes with one
// pixel-sized side; nesting two of them lets the chat collapse to nothing in a
// narrow window. Here the side panes hold their dragged widths while there is
// room and give way down to a floor before the chat does, and the whole row
// scrolls sideways once even the floors do not fit.
import { useRef, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react'
import { t } from '@/i18n'
import { useStored } from '@/lib/hooks'

const LEFT = { min: 200, max: 420, initial: 240 }
const RIGHT = { min: 260, max: 640, initial: 360 }
const CENTER_MIN = 340
const HANDLE = 12

function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(Math.max(lo, hi), v))
}

function Handle({ onDrag, onNudge, onReset }: { onDrag: (clientX: number) => void; onNudge: (dx: number) => void; onReset: () => void }) {
  const dragging = useRef(false)
  const end = (e: PointerEvent<HTMLDivElement>) => {
    dragging.current = false
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId)
  }
  return (
    <div
      role="separator"
      aria-orientation="vertical"
      tabIndex={0}
      title={t('common.resizeHint')}
      onPointerDown={(e) => {
        e.preventDefault()
        // Capture keeps the drag alive when the pointer leaves the 12px strip.
        e.currentTarget.setPointerCapture(e.pointerId)
        dragging.current = true
      }}
      onPointerMove={(e) => {
        if (dragging.current) onDrag(e.clientX)
      }}
      onPointerUp={end}
      onPointerCancel={end}
      onDoubleClick={onReset}
      onKeyDown={(e: KeyboardEvent<HTMLDivElement>) => {
        if (e.key === 'ArrowLeft') onNudge(-16)
        else if (e.key === 'ArrowRight') onNudge(16)
        else return
        e.preventDefault()
      }}
      className="group relative cursor-col-resize touch-none select-none rounded outline-none focus-visible:bg-accent/15"
    >
      <span className="absolute inset-y-2 left-1/2 w-px -translate-x-1/2 rounded-full bg-transparent transition-colors group-hover:bg-accent/60 group-active:bg-accent" />
    </div>
  )
}

export function Panes({ left, center, right }: { left: ReactNode; center: ReactNode; right: ReactNode }) {
  // Same key space and encoding as the kit's Split, so the sizes survive reloads.
  const [l, setL] = useStored<number>('agentflow-studio:split:playground.left', LEFT.initial)
  const [r, setR] = useStored<number>('agentflow-studio:split:playground.trace', RIGHT.initial)
  const root = useRef<HTMLDivElement>(null)
  const leftEl = useRef<HTMLDivElement>(null)
  const rightEl = useRef<HTMLDivElement>(null)

  const lw = clamp(Number(l) || LEFT.initial, LEFT.min, LEFT.max)
  const rw = clamp(Number(r) || RIGHT.initial, RIGHT.min, RIGHT.max)

  // A side may only grow into space the chat can spare at the other side's
  // current width, so the stored size never runs ahead of what is drawn.
  const room = (other: HTMLDivElement | null) => (root.current?.clientWidth ?? 0) - 2 * HANDLE - CENTER_MIN - (other?.offsetWidth ?? 0)
  const sizeLeft = (v: number) => setL(Math.round(clamp(v, LEFT.min, Math.min(LEFT.max, room(rightEl.current)))))
  const sizeRight = (v: number) => setR(Math.round(clamp(v, RIGHT.min, Math.min(RIGHT.max, room(leftEl.current)))))

  return (
    <div className="min-h-0 flex-1 overflow-x-auto overflow-y-hidden">
      <div
        ref={root}
        className="grid h-full min-h-0 grid-rows-[minmax(0,1fr)]"
        style={{
          minWidth: LEFT.min + CENTER_MIN + RIGHT.min + 2 * HANDLE,
          gridTemplateColumns: `minmax(${LEFT.min}px, ${lw}px) ${HANDLE}px minmax(${CENTER_MIN}px, 1fr) ${HANDLE}px minmax(${RIGHT.min}px, ${rw}px)`,
        }}
      >
        <div ref={leftEl} className="flex min-h-0 min-w-0 flex-col">
          {left}
        </div>
        <Handle
          onDrag={(x) => root.current && sizeLeft(x - root.current.getBoundingClientRect().left - HANDLE / 2)}
          onNudge={(dx) => sizeLeft((leftEl.current?.offsetWidth ?? lw) + dx)}
          onReset={() => setL(LEFT.initial)}
        />
        <div className="flex min-h-0 min-w-0 flex-col">{center}</div>
        <Handle
          onDrag={(x) => root.current && sizeRight(root.current.getBoundingClientRect().right - x - HANDLE / 2)}
          onNudge={(dx) => sizeRight((rightEl.current?.offsetWidth ?? rw) - dx)}
          onReset={() => setR(RIGHT.initial)}
        />
        <div ref={rightEl} className="flex min-h-0 min-w-0 flex-col">
          {right}
        </div>
      </div>
    </div>
  )
}
