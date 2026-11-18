// The card every source tab shares: a title with its actions, a one-line
// hint, and an editor that takes the remaining height.
import type { ReactNode } from 'react'

export function EditorFrame({
  title,
  hint,
  actions,
  children,
  footer,
}: {
  title: string
  hint: ReactNode
  actions?: ReactNode
  children: ReactNode
  /** Sits under the editor (the loop's issue list). */
  footer?: ReactNode
}) {
  return (
    <section className="flex min-h-0 flex-1 flex-col rounded-card border border-line bg-surface">
      <header className="shrink-0 px-3 pb-2 pt-2.5">
        <div className="flex min-h-[26px] items-center gap-2">
          <h3 className="min-w-0 flex-1 truncate text-md font-semibold leading-tight">{title}</h3>
          {actions && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
        </div>
        <p className="mt-0.5 text-xs leading-snug text-muted">{hint}</p>
      </header>
      <div className="flex min-h-0 flex-1 flex-col px-3 pb-3">{children}</div>
      {footer}
    </section>
  )
}
