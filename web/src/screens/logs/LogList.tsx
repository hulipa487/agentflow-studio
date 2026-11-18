// The log rows, windowed: only the lines in (and just around) the viewport
// are in the DOM, so a full 5000-line buffer scrolls like an empty one. Rows
// have a fixed height and never wrap, which is what makes the window a matter
// of arithmetic.
import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { ArrowDown } from 'lucide-react'
import { cx } from '@/components/ui'
import type { LogBuffer, LogLine } from './buffer'

const ROW = 20
const OVERSCAN = 12

const LEVEL_TEXT = ['text-faint', 'text-faint', 'text-text', 'text-warning', 'text-danger']
const LEVEL_BAR = ['bg-transparent', 'bg-transparent', 'bg-transparent', 'bg-warning', 'bg-danger']

const Row = memo(function Row({ line, onCopy }: { line: LogLine; onCopy: (text: string) => void }) {
  return (
    <button
      type="button"
      onClick={() => onCopy(line.text)}
      title={line.text}
      style={{ height: ROW }}
      className={cx('flex w-full items-stretch text-left font-mono text-sm leading-5 hover:bg-hover focus-visible:bg-hover', LEVEL_TEXT[line.level])}
    >
      <span className={cx('w-[3px] shrink-0', LEVEL_BAR[line.level])} />
      <span className="min-w-0 flex-1 truncate whitespace-pre pl-2 pr-3">{line.text}</span>
    </button>
  )
})

export function LogList({
  buffer,
  version,
  onCopy,
  empty,
}: {
  buffer: LogBuffer
  /** Changes whenever the buffer's view did. */
  version: number
  onCopy: (text: string) => void
  /** Shown in place of the rows when there are none. */
  empty: string
}) {
  const scroller = useRef<HTMLDivElement>(null)
  const [viewport, setViewport] = useState(0)
  const [top, setTop] = useState(0)
  // Following pins the view to the newest line. It is state for rendering and
  // a ref for the layout pass, which must not wait for a re-render.
  const [following, setFollowing] = useState(true)
  const follow = useRef(true)

  const rows = buffer.visible
  const n = rows.length

  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const measure = () => setViewport(el.clientHeight)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const onScroll = useCallback(() => {
    const el = scroller.current
    if (!el) return
    // Scrolling up lets go of the tail; reaching the bottom takes it again.
    // The margin is a few pixels, not a row: a trackpad scrolls in steps
    // smaller than a row, and each must be enough to let go.
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 4
    follow.current = atBottom
    setFollowing(atBottom)
    setTop(Math.floor(el.scrollTop / ROW))
  }, [])

  useLayoutEffect(() => {
    const el = scroller.current
    if (!el) return
    const dropped = buffer.takeDropped()
    if (follow.current) {
      el.scrollTop = el.scrollHeight
    } else if (dropped > 0) {
      // The buffer rolled over above a reader who scrolled up: move with the
      // lines so the ones being read stay where they are.
      el.scrollTop = Math.max(0, el.scrollTop - dropped * ROW)
      setTop(Math.floor(el.scrollTop / ROW))
    }
  }, [buffer, version, viewport])

  const jump = () => {
    const el = scroller.current
    if (!el) return
    follow.current = true
    setFollowing(true)
    el.scrollTop = el.scrollHeight
  }

  const count = Math.ceil(viewport / ROW) + 1
  const last = Math.max(0, n - count)
  // While following, the window is taken from the end rather than from the
  // scroll offset, which is only corrected after this render.
  const first = following ? last : Math.min(top, last)
  const start = Math.max(0, first - OVERSCAN)
  const end = Math.min(n, first + count + OVERSCAN)

  return (
    <div className="relative min-h-0 flex-1">
      <div ref={scroller} onScroll={onScroll} role="log" aria-live="off" aria-label="Runtime log" className="absolute inset-0 overflow-x-hidden overflow-y-auto">
        <div className="relative" style={{ height: n * ROW }}>
          <div className="absolute inset-x-0 top-0" style={{ transform: `translateY(${start * ROW}px)` }}>
            {rows.slice(start, end).map((line) => (
              <Row key={line.seq} line={line} onCopy={onCopy} />
            ))}
          </div>
        </div>
      </div>
      {n === 0 && <div className="pointer-events-none absolute inset-0 flex items-center justify-center px-6 text-center text-sm text-muted">{empty}</div>}
      {!following && n > 0 && (
        <button
          type="button"
          onClick={jump}
          className="absolute bottom-3 right-4 inline-flex h-7 animate-pop-in items-center gap-1.5 rounded-full border border-line bg-float px-3 text-xs font-medium text-text shadow-float hover:bg-hover"
        >
          <ArrowDown className="size-3.5 text-accent" strokeWidth={2.2} />
          Jump to latest
        </button>
      )}
    </div>
  )
}
