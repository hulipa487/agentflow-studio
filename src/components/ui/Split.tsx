import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { cx } from './index'

/**
 * A two-pane horizontal split with a draggable divider. `size` is the width of
 * the fixed side in pixels; the other side takes the rest. The chosen size is
 * remembered per `storageKey`.
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

  const onMove = useCallback(
    (e: PointerEvent) => {
      if (!dragging.current || !root.current) return
      const rect = root.current.getBoundingClientRect()
      const raw = fixed === 'left' ? e.clientX - rect.left : rect.right - e.clientX
      setSize(Math.max(min, Math.min(max, Math.min(raw, rect.width - 240))))
    },
    [fixed, min, max],
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
      className="group relative w-3 shrink-0 cursor-col-resize"
      title="Drag to resize · double-click to reset"
    >
      <span className="absolute inset-y-2 left-1/2 w-px -translate-x-1/2 rounded-full bg-transparent transition-colors group-hover:bg-accent/60" />
    </div>
  )

  return (
    <div ref={root} className={cx('flex min-h-0 min-w-0 flex-1', className)}>
      <div className="flex min-h-0 min-w-0 flex-col" style={fixed === 'left' ? { width: size, flexShrink: 0 } : { flex: 1 }}>
        {left}
      </div>
      {handle}
      <div className="flex min-h-0 min-w-0 flex-col" style={fixed === 'right' ? { width: size, flexShrink: 0 } : { flex: 1 }}>
        {right}
      </div>
    </div>
  )
}
