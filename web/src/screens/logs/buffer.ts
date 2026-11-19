// The Logs screen's line store, ported from logsView in the desktop app
// (internal/ui/logs.go). It lives outside React on purpose: a busy runtime
// logs far faster than a component should re-render, so lines are staged here
// and folded into the view once per animation frame.

/** Bounds what the Logs screen keeps in memory. */
export const LOG_BUFFER_LINES = 5000

/** The runtime's short level names (llog.ShortName), lowest first. */
export const LOG_LEVELS: readonly string[] = ['DEV', 'DBG', 'INF', 'WRN', 'ERR']

/** The level floors the toolbar offers, in the desktop app's order. */
export const LEVEL_FLOORS: { value: string; labelKey: string }[] = [
  { value: '1', labelKey: 'logs.floorDebug' },
  { value: '2', labelKey: 'logs.floorInfo' },
  { value: '3', labelKey: 'logs.floorWarn' },
  { value: '4', labelKey: 'logs.floorError' },
  { value: '0', labelKey: 'logs.floorAll' },
]

export const DEFAULT_FLOOR = '1'

/** The index of a line's level in LOG_LEVELS; an unrecognized line counts as INF. */
export function logLevelOf(line: string): number {
  // "15:04:05 INF message k=v": the level is the second field.
  const m = /^\s*\S+\s+(\S+)/.exec(line)
  if (m) {
    const i = LOG_LEVELS.indexOf(m[1])
    if (i >= 0) return i
  }
  return 2
}

export interface LogLine {
  /** Arrival order; stable across trimming, so it keys the rendered row. */
  seq: number
  text: string
  level: number
}

export class LogBuffer {
  minLevel = Number(DEFAULT_FLOOR)
  /** Lowercased and trimmed; empty matches everything. */
  filter = ''
  paused = false
  /** Lines that arrived while paused. */
  pending = 0
  /** What the list shows: the matching lines, oldest first. */
  visible: LogLine[] = []

  private lines: LogLine[] = []
  private staged: LogLine[] = []
  /** The lines as they were when Pause was pressed; the view reads these until Resume. */
  private frozen: LogLine[] | null = null
  private seq = 0
  private dropped = 0
  private readonly cap: number

  constructor(cap = LOG_BUFFER_LINES) {
    this.cap = cap
  }

  /** How many lines are kept, shown or not. */
  get total(): number {
    return this.lines.length
  }

  /** Stages one line; it reaches the view at the next commit. */
  push(text: string): void {
    this.staged.push({ seq: ++this.seq, text, level: logLevelOf(text) })
    // Frames stop in a hidden tab, so nothing commits there; keep the backlog
    // bounded until the tab is looked at again.
    if (this.staged.length > this.cap * 2) this.staged.splice(0, this.staged.length - this.cap)
  }

  /** Folds the staged lines into the buffer and the view. False when nothing arrived. */
  commit(): boolean {
    const batch = this.staged
    if (batch.length === 0) return false
    this.staged = []
    for (const l of batch) this.lines.push(l)
    if (this.lines.length > this.cap) this.lines.splice(0, this.lines.length - this.cap)
    if (this.paused) {
      this.pending += batch.length
      return true
    }
    for (const l of batch) if (this.matches(l)) this.visible.push(l)
    const over = this.visible.length - this.cap
    if (over > 0) {
      this.visible.splice(0, over)
      this.dropped += over
    }
    return true
  }

  /**
   * Rows trimmed off the top of the view since the last call. The list uses
   * it to hold a scrolled-up reader in place while the buffer rolls over.
   */
  takeDropped(): number {
    const n = this.dropped
    this.dropped = 0
    return n
  }

  matches(l: LogLine): boolean {
    if (l.level < this.minLevel) return false
    return this.filter === '' || l.text.toLowerCase().includes(this.filter)
  }

  setFilter(q: string): void {
    this.filter = q.trim().toLowerCase()
    this.apply()
  }

  setMinLevel(level: number): void {
    this.minLevel = level
    this.apply()
  }

  /** Forgets every line (the Clear button). */
  clear(): void {
    this.lines = []
    this.staged = []
    if (this.frozen) this.frozen = []
    this.pending = 0
    this.apply()
  }

  /**
   * Starts over for a (re)connect: the runtime replays its ring on every
   * connection, so appending would show the history twice. A paused view
   * keeps what the reader stopped on.
   */
  reset(): void {
    this.lines = []
    this.staged = []
    this.pending = 0
    if (!this.paused) this.apply()
  }

  pause(): void {
    if (this.paused) return
    this.commit()
    this.frozen = this.lines.slice()
    this.paused = true
    this.pending = 0
  }

  resume(): void {
    if (!this.paused) return
    this.paused = false
    this.frozen = null
    this.pending = 0
    this.commit()
    this.apply()
  }

  /** Recomputes the view after the filter, the floor or the source changed. */
  private apply(): void {
    this.visible = (this.frozen ?? this.lines).filter((l) => this.matches(l))
    this.dropped = 0
  }
}
