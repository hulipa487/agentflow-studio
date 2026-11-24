// Appearance: the palette and light/dark mode. Both apply in place and are
// saved in this browser.
import { Check, Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'
import { Chip, Section, Segmented, cx } from '@/components/ui'
import { PALETTES, useAppearance, type Mode, type Palette } from '@/state/appearance'
import { t } from '@/i18n'
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
          <span className="text-xs font-semibold leading-none text-text">assistant</span>
          <span className="font-mono text-xs leading-none text-muted">loops/chat.ts</span>
          <span className="mt-0.5 flex items-center gap-1">
            <span className="rounded bg-accent px-1.5 text-xs font-semibold leading-4 text-on-primary">{t('common.apply')}</span>
            <span className="rounded bg-tone-accent px-1.5 font-mono text-xs leading-4 text-accent">llm.chat</span>
            <span className="rounded bg-tone-success px-1.5 font-mono text-xs leading-4 text-success">{t('settings.previewLive')}</span>
            <span className="rounded bg-tone-warning px-1.5 font-mono text-xs leading-4 text-warning">{t('settings.previewRestart')}</span>
            <span className="rounded bg-tone-danger px-1.5 font-mono text-xs leading-4 text-danger">{t('settings.previewError')}</span>
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

const MODES: { value: Mode; labelKey: string; icon: LucideIcon }[] = [
  { value: 'light', labelKey: 'settings.modeLight', icon: Sun },
  { value: 'dark', labelKey: 'settings.modeDark', icon: Moon },
  { value: 'system', labelKey: 'settings.modeSystem', icon: Monitor },
]

export function AppearanceSection() {
  const mode = useAppearance((s) => s.mode)
  const palette = useAppearance((s) => s.palette)
  const setMode = useAppearance((s) => s.setMode)
  const setPalette = useAppearance((s) => s.setPalette)
  const shown = useShownTheme()

  return (
    <Section
      title={t('settings.appearance')}
      subtitle={t('settings.appearanceHint')}
      action={<Chip tone="accent">{t('settings.savedAutomatically')}</Chip>}
    >
      <div className="flex flex-col gap-4">
        <div>
          <FieldLabel>{t('settings.colorTheme')}</FieldLabel>
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
                    <span className="truncate text-base font-semibold text-text">{p.title}</span>
                    <Swatches palette={p.id} theme={shown} />
                    <span className="flex-1" />
                    {active ? (
                      <span className="inline-flex h-6 shrink-0 items-center gap-1 rounded-ctl bg-accent px-2 text-xs font-semibold text-on-primary">
                        <Check className="size-3.5" strokeWidth={2.4} />
                        {t('settings.active')}
                      </span>
                    ) : (
                      <span className="inline-flex h-6 shrink-0 items-center rounded-ctl border border-line bg-surface px-2 text-xs font-medium text-text">
                        {t('settings.usePalette', { name: p.title })}
                      </span>
                    )}
                  </span>
                  <span className="text-xs leading-snug text-muted">{t(p.descriptionKey)}</span>
                </button>
              )
            })}
          </div>
        </div>
        <div>
          <FieldLabel>{t('settings.appearanceMode')}</FieldLabel>
          <div className="flex flex-wrap items-center gap-3">
            <Segmented
              value={mode}
              onChange={setMode}
              options={MODES.map((m) => ({
                value: m.value,
                label: (
                  <span className="inline-flex items-center gap-1.5">
                    <m.icon className="size-3.5" strokeWidth={2} />
                    {t(m.labelKey)}
                  </span>
                ),
              }))}
            />
            <span className="text-xs text-muted">
              {mode === 'system' ? t('settings.followingSystem', { mode: shown }) : t('settings.systemFollows')}
            </span>
          </div>
        </div>
      </div>
    </Section>
  )
}
