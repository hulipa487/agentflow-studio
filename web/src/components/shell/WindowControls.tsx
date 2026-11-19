// The window's title bar, drawn by the page. The desktop app on Windows has no
// native title bar (it clashed with the app's own top bar), so the top bar
// drags the window, a double-click on its empty space maximises, and these
// buttons minimise, maximise and close. In a browser, and in a desktop window
// that keeps its native title bar, none of it renders.
import { useEffect, useState, type MouseEvent, type ReactNode } from 'react'
import { Minus, Square, X } from 'lucide-react'
import { t } from '@/i18n'
import { desktopWindow, useFramelessWindow } from '@/lib/desktop'
import { cx } from '@/components/ui'

/** Restore-down: two overlapping squares, as on Windows. */
function RestoreIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2" className={className} aria-hidden>
      <rect x="2.5" y="5.5" width="8" height="8" rx="1" />
      <path d="M5.5 5.5V3.5a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-2" />
    </svg>
  )
}

function ControlButton({ label, onClick, danger, children }: { label: string; onClick: () => void; danger?: boolean; children: ReactNode }) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      onClick={onClick}
      className={cx(
        'flex h-full w-[46px] items-center justify-center text-muted transition-colors',
        danger ? 'hover:bg-[#c42b1c] hover:text-white' : 'hover:bg-hover hover:text-text',
      )}
    >
      {children}
    </button>
  )
}

/** Minimise, maximise / restore and close, at the right end of a title bar. */
export function WindowControls() {
  const frameless = useFramelessWindow()
  const [maximised, setMaximised] = useState(false)
  useEffect(() => {
    if (!frameless) return
    const check = () => void desktopWindow.isMaximised().then(setMaximised)
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [frameless])
  if (!frameless) return null
  return (
    <div className="flex h-full shrink-0 self-stretch" data-window-controls>
      <ControlButton label={t('shell.window.minimise')} onClick={desktopWindow.minimise}>
        <Minus className="size-4" strokeWidth={1.4} />
      </ControlButton>
      <ControlButton label={t(maximised ? 'shell.window.restore' : 'shell.window.maximise')} onClick={desktopWindow.toggleMaximise}>
        {maximised ? <RestoreIcon className="size-[15px]" /> : <Square className="size-[13px]" strokeWidth={1.4} />}
      </ControlButton>
      <ControlButton label={t('shell.window.close')} onClick={desktopWindow.close} danger>
        <X className="size-[17px]" strokeWidth={1.3} />
      </ControlButton>
    </div>
  )
}

/**
 * Props that make an element drag the window when it is frameless: its empty
 * space drags (buttons, links and fields inside still click), and a
 * double-click on that space maximises or restores.
 */
export function useWindowDrag(): { 'data-window-drag'?: true; onDoubleClick?: (e: MouseEvent) => void } {
  const frameless = useFramelessWindow()
  if (!frameless) return {}
  return {
    'data-window-drag': true,
    onDoubleClick: (e) => {
      if ((e.target as HTMLElement).closest('button, a, input, select, textarea, [role="button"], [role="radio"], [role="menuitem"]')) return
      desktopWindow.toggleMaximise()
    },
  }
}

/** The login screen has no top bar: a strip across the top holds the drag area and the controls. */
export function LoginTitleBar({ children }: { children: ReactNode }) {
  const frameless = useFramelessWindow()
  const drag = useWindowDrag()
  if (!frameless) return <>{children}</>
  return (
    <div {...drag} className="fixed inset-x-0 top-0 z-30 flex h-11 items-center justify-end gap-2">
      {children}
      <WindowControls />
    </div>
  )
}
