import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { cx } from './index'

/**
 * A two-pane horizontal split with a draggable divider. `size` is the width of
 * the fixed side in pixels; the other side takes the rest. The chosen size is
 * remembered per `storageKey`. `collapsed` hides the fixed side and the divider
 * without unmounting it, so whatever it holds keeps its state; `minOther` is
 * the width the flexible side keeps however wide the fixed side was dragged.
 */
export function Split({
  left,
  right,
  fixed = 'right',
  initial = 320,
  min = 220,
  max = 720,
  storageKey,
  className,
  collapsed = false,
  minOther = 240,
}: {
  left: ReactNode
  right: ReactNode
  /** Which pane keeps a pixel width. */
  fixed?: 'left' | 'right'
  initial?: number
  min?: number
  max?: number
  storageKey?: string
  className?: string
  collapsed?: boolean
  minOther?: number
}) {
  const [size, setSize] = useState<number>(() => {
    if (storageKey) {
      try {
        const v = Number(localStorage.getItem('agentflow-studio:split:' + storageKey))
        if (v >= min && v <= max) return v
      } catch {
        /* default */
      }
    }
    return initial
  })
  const root = useRef<HTMLDivElement>(null)
  const dragging = useRef(false)
  // The room the fixed side may take in the current window: a stored size
  // from a wide window must not squeeze the other side in a narrow one.
  const [room, setRoom] = useState(Infinity)
  useEffect(() => {
    const el = root.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => setRoom(el.clientWidth - 12 - minOther))
    ro.observe(el)
    return () => ro.disconnect()
  }, [minOther])
  const shown = Math.max(min, Math.min(size, room))

  const onMove = useCallback(
    (e: PointerEvent) => {
      if (!dragging.current || !root.current) return
      const rect = root.current.getBoundingClientRect()
      const raw = fixed === 'left' ? e.clientX - rect.left : rect.right - e.clientX
      setSize(Math.max(min, Math.min(max, Math.min(raw, rect.width - 12 - minOther))))
    },
    [fixed, min, max, minOther],
  )

  useEffect(() => {
    const stop = () => {
      if (!dragging.current) return
      dragging.current = false
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', stop)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', stop)
    }
  }, [onMove])

  useEffect(() => {
    if (!storageKey) return
    try {
      localStorage.setItem('agentflow-studio:split:' + storageKey, String(Math.round(size)))
    } catch {
      /* not persisted */
    }
  }, [size, storageKey])

  const handle = (
    <div
      role="separator"
      aria-orientation="vertical"
      onPointerDown={(e) => {
        e.preventDefault()
        dragging.current = true
        document.body.style.cursor = 'col-resize'
        document.body.style.userSelect = 'none'
      }}
      onDoubleClick={() => setSize(initial)}
      className={cx('group relative w-3 shrink-0 cursor-col-resize', collapsed && 'hidden')}
      title="Drag to resize · double-click to reset"
    >
      <span className="absolute inset-y-2 left-1/2 w-px -translate-x-1/2 rounded-full bg-transparent transition-colors group-hover:bg-accent/60" />
    </div>
  )

  return (
    <div ref={root} className={cx('flex min-h-0 min-w-0 flex-1', className)}>
      <div className={cx('flex min-h-0 min-w-0 flex-col', collapsed && fixed === 'left' && 'hidden')} style={fixed === 'left' ? { width: shown, flexShrink: 0 } : { flex: 1 }}>
        {left}
      </div>
      {handle}
      <div className={cx('flex min-h-0 min-w-0 flex-col', collapsed && fixed === 'right' && 'hidden')} style={fixed === 'right' ? { width: shown, flexShrink: 0 } : { flex: 1 }}>
        {right}
      </div>
    </div>
  )
}
