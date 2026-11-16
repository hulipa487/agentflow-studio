// Appearance: the palette and light/dark mode. Both apply in place and are
// saved in this browser.
import { Check, Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'
import { Chip, Section, Segmented, cx } from '@/components/ui'
import { PALETTES, useAppearance, type Mode, type Palette } from '@/state/appearance'
import { FieldLabel, useShownTheme } from './parts'

/**
 * A miniature of the shell drawn in one palette. The data attributes make the
 * design tokens resolve to that palette inside this element only (see the
 * selectors in index.css), so the preview is the real colors, not a copy.
 * Spans throughout: it is rendered inside a button.
 */
function PalettePreview({ palette, theme }: { palette: Palette; theme: 'light' | 'dark' }) {
  return (
    <span data-palette={palette} data-theme={theme} aria-hidden className="block overflow-hidden rounded-ctl border border-line bg-bg">
      <span className="flag-stripe block h-[3px]" />
      <span className="flex gap-2 p-2">
        <span className="flex w-12 shrink-0 flex-col gap-1.5 rounded-md bg-float p-1.5">
          <span className="h-1.5 rounded-full bg-accent" />
          <span className="h-1.5 w-4/5 rounded-full bg-line" />
          <span className="h-1.5 w-3/5 rounded-full bg-line" />
          <span className="h-1.5 w-4/5 rounded-full bg-line" />
        </span>
        <span className="flex min-w-0 flex-1 flex-col gap-1.5 rounded-md border border-line bg-surface p-2">
          <span className="text-[11px] font-semibold leading-none text-text">assistant</span>
          <span className="font-mono text-[10px] leading-none text-muted">loops/chat.lua</span>
          <span className="mt-0.5 flex items-center gap-1">
            <span className="rounded bg-accent px-1.5 text-[9px] font-semibold leading-4 text-on-primary">Apply</span>
            <span className="rounded bg-tone-accent px-1.5 font-mono text-[9px] leading-4 text-accent">llm.chat</span>
            <span className="rounded bg-tone-success px-1.5 font-mono text-[9px] leading-4 text-success">live</span>
            <span className="rounded bg-tone-warning px-1.5 font-mono text-[9px] leading-4 text-warning">restart</span>
            <span className="rounded bg-tone-danger px-1.5 font-mono text-[9px] leading-4 text-danger">error</span>
          </span>
        </span>
      </span>
    </span>
  )
}

/** The palette's two accents and its shell color, as on the desktop app's cards. */
function Swatches({ palette, theme }: { palette: Palette; theme: 'light' | 'dark' }) {
  const dot = 'size-3.5 rounded-full border border-line'
  return (
    <span data-palette={palette} data-theme={theme} aria-hidden className="inline-flex items-center gap-1">
      <span className={cx(dot, 'bg-accent')} />
      <span className={cx(dot, 'bg-accent2')} />
      <span className={cx(dot, 'bg-bg')} />
    </span>
  )
}

const MODES: { value: Mode; label: string; icon: LucideIcon }[] = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
]

export function AppearanceSection() {
  const mode = useAppearance((s) => s.mode)
  const palette = useAppearance((s) => s.palette)
  const setMode = useAppearance((s) => s.setMode)
  const setPalette = useAppearance((s) => s.setPalette)
  const shown = useShownTheme()

  return (
    <Section
      title="Appearance"
      subtitle="Choose your Studio theme and light or dark mode. Both settings apply immediately and persist across launches."
      action={<Chip tone="accent">Saved automatically</Chip>}
    >
      <div className="flex flex-col gap-4">
        <div>
          <FieldLabel>Color theme</FieldLabel>
          <div className="grid gap-3 md:grid-cols-2">
            {PALETTES.map((p) => {
              const active = p.id === palette
              return (
                <button
                  key={p.id}
                  type="button"
                  aria-pressed={active}
                  onClick={() => setPalette(p.id)}
                  className={cx(
                    'flex min-w-0 flex-col gap-2.5 rounded-card border p-3 text-left transition-colors duration-100',
                    active ? 'border-accent bg-accent-dim/40' : 'border-line bg-surface2 hover:border-faint hover:bg-hover',
                  )}
                >
                  <PalettePreview palette={p.id} theme={shown} />
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="truncate text-[13px] font-semibold text-text">{p.title}</span>
                    <Swatches palette={p.id} theme={shown} />
                    <span className="flex-1" />
                    {active ? (
                      <span className="inline-flex h-6 shrink-0 items-center gap-1 rounded-ctl bg-accent px-2 text-xs font-semibold text-on-primary">
                        <Check className="size-3.5" strokeWidth={2.4} />
                        Active
                      </span>
                    ) : (
                      <span className="inline-flex h-6 shrink-0 items-center rounded-ctl border border-line bg-surface px-2 text-xs font-medium text-text">
                        Use {p.title}
                      </span>
                    )}
                  </span>
                  <span className="text-xs leading-snug text-muted">{p.description}</span>
                </button>
              )
            })}
          </div>
        </div>
        <div>
          <FieldLabel>Appearance mode</FieldLabel>
          <div className="flex flex-wrap items-center gap-3">
            <Segmented
              value={mode}
              onChange={setMode}
              options={MODES.map((m) => ({
                value: m.value,
                label: (
                  <span className="inline-flex items-center gap-1.5">
                    <m.icon className="size-3.5" strokeWidth={2} />
                    {m.label}
                  </span>
                ),
              }))}
            />
            <span className="text-xs text-muted">
              {mode === 'system' ? `Following the operating system: ${shown} right now.` : 'System follows the operating system’s light or dark setting.'}
            </span>
          </div>
        </div>
      </div>
    </Section>
  )
}
