// Small layout pieces shared by the Settings sections.
import { useEffect, useState, type ReactNode } from 'react'
import { PALETTES, useAppearance, type Mode } from '@/state/appearance'

/** A two-column fact line: a fixed caption column and a value that fills the rest. */
export function Fact({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[130px_minmax(0,1fr)] items-start gap-x-2 py-1">
      <span className="pt-0.5 text-xs font-medium text-muted">{label}</span>
      <div className="min-w-0">
        <div className="flex min-h-5 min-w-0 items-center gap-1.5 text-base">{children}</div>
        {hint && <p className="mt-0.5 text-xs leading-snug text-muted">{hint}</p>}
      </div>
    </div>
  )
}

export function FieldLabel({ children }: { children: ReactNode }) {
  return <div className="mb-1.5 text-xs font-medium text-muted">{children}</div>
}

const lightQuery = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: light)') : null

/**
 * Light or dark as it is on screen right now. Unlike the store's resolver
 * this re-renders when the OS flips while the mode is "system".
 */
export function useShownTheme(): 'light' | 'dark' {
  const mode = useAppearance((s) => s.mode)
  const [systemLight, setSystemLight] = useState(lightQuery?.matches ?? false)
  useEffect(() => {
    if (!lightQuery) return
    const onChange = () => setSystemLight(lightQuery.matches)
    lightQuery.addEventListener('change', onChange)
    return () => lightQuery.removeEventListener('change', onChange)
  }, [])
  if (mode === 'light' || mode === 'dark') return mode
  return systemLight ? 'light' : 'dark'
}

/** "Trans Pride · Dark mode", as the desktop app's About section words it. */
export function themeSummary(palette: string, mode: Mode, shown: 'light' | 'dark'): string {
  const title = PALETTES.find((p) => p.id === palette)?.title ?? palette
  const label = shown === 'light' ? 'Light mode' : 'Dark mode'
  return `${title} · ${label}${mode === 'system' ? ' (following the system)' : ''}`
}
