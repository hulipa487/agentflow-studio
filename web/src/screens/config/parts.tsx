// Pieces of the Config screen that carry no state of their own.
import type { ReactNode } from 'react'
import { stringify } from 'yaml'

/** A caption and one-line hint over an editor, as on the desktop app. */
export function EditorFrame({
  title,
  hint,
  trailing,
  children,
  hidden,
}: {
  title: string
  hint: ReactNode
  trailing?: ReactNode
  children: ReactNode
  /** Keeps the frame mounted but out of the layout. */
  hidden?: boolean
}) {
  return (
    <div className={hidden ? 'hidden' : 'flex min-h-0 flex-1 flex-col gap-1.5'}>
      <div className="flex min-h-6 shrink-0 items-center gap-3">
        <span className="shrink-0 font-mono text-xs font-semibold text-text">{title}</span>
        <span className="min-w-0 flex-1 truncate text-xs text-muted">{hint}</span>
        {trailing}
      </div>
      {children}
    </div>
  )
}

function pad(n: number): string {
  return n < 10 ? '0' + n : String(n)
}

/** A unix timestamp as local "2006-01-02 15:04:05": mtimes are compared to the second. */
export function stamp(unix: number): string {
  const d = new Date(unix * 1000)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function baseName(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).pop() || 'agentflow.yaml'
}

export type ViewFormat = 'json' | 'yaml'

/** The masked tree as text. Empty when the file on disk did not parse. */
export function renderView(view: unknown, format: ViewFormat): string {
  if (view === null || view === undefined) return ''
  try {
    return format === 'yaml' ? stringify(view, { lineWidth: 0 }) : JSON.stringify(view, null, 2)
  } catch {
    return ''
  }
}

/** The first "line N" a validation error names, if any (yaml errors carry one). */
export function errorLine(message: string): number | null {
  const m = /\bline (\d+)\b/.exec(message)
  if (!m) return null
  const n = Number(m[1])
  return n > 0 ? n : null
}
