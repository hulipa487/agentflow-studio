// The master side of a master/detail screen: one selectable row per record,
// a title over a monospace summary line (the desktop's rail rows).
import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { cx } from '@/components/ui'

export function RailRow({
  title,
  caption,
  icon: Icon,
  active,
  onClick,
  trailing,
}: {
  title: string
  caption: ReactNode
  icon: LucideIcon
  active: boolean
  onClick: () => void
  trailing?: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? 'true' : undefined}
      className={cx(
        'flex w-full min-w-0 items-start gap-2.5 rounded-ctl px-2.5 py-1.5 text-left transition-colors duration-100',
        active ? 'bg-accent-dim' : 'hover:bg-hover active:bg-pressed',
      )}
    >
      <Icon className={cx('mt-[3px] size-3.5 shrink-0', active ? 'text-accent' : 'text-faint')} strokeWidth={2} />
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-1.5">
          <span className={cx('truncate text-base', active ? 'font-semibold text-text' : 'font-medium text-text')}>{title}</span>
          {trailing}
        </span>
        <span className="block truncate font-mono text-xs text-muted">{caption}</span>
      </span>
    </button>
  )
}

/** The quiet line a list or a detail pane shows when it has nothing to list. */
export function PaneNote({ children, lowered }: { children: ReactNode; /** Sits further down, for an empty detail pane. */ lowered?: boolean }) {
  return <div className={cx('px-3 pb-6 text-center text-sm leading-relaxed text-muted', lowered ? 'pt-12' : 'pt-6')}>{children}</div>
}
