// The language switch: EN / 简体 / 繁體. Each language is written in its own
// script whatever the current one is, so it can be found by someone who cannot
// read the language Studio happens to be in.
import { cx } from '@/components/ui'
import { LANGS, t, useI18n, useLang } from '@/i18n'

export function LanguageToggle({ className }: { className?: string }) {
  const lang = useLang()
  const setLang = useI18n((s) => s.setLang)
  return (
    <div
      role="radiogroup"
      aria-label={t('common.language')}
      className={cx('flex h-[30px] shrink-0 items-center gap-0.5 rounded-ctl border border-line bg-bg p-0.5', className)}
    >
      {LANGS.map((l) => (
        <button
          key={l.id}
          type="button"
          role="radio"
          aria-checked={l.id === lang}
          lang={l.id}
          title={l.name}
          onClick={() => setLang(l.id)}
          className={cx(
            'h-full min-w-9 whitespace-nowrap rounded-[5px] px-2 text-xs transition-colors duration-100',
            l.id === lang ? 'bg-hover font-semibold text-text' : 'text-muted hover:text-text',
          )}
        >
          {l.label}
        </button>
      ))}
    </div>
  )
}
