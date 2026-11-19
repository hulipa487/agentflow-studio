// Language: the three languages Studio speaks, each written in its own script
// so it can be found whatever language Studio is in now. The choice applies
// in place and is saved in this browser.
import { Check } from 'lucide-react'
import { Section, cx } from '@/components/ui'
import { LANGS, t, useI18n, useLang } from '@/i18n'

export function LanguageSection() {
  const lang = useLang()
  const setLang = useI18n((s) => s.setLang)
  return (
    <Section title={t('common.language')}>
      <div className="flex flex-col gap-2">
        <p className="text-xs leading-snug text-muted">{t('settings.languageHintWeb')}</p>
        <div role="radiogroup" aria-label={t('common.language')} className="flex flex-wrap gap-2">
          {LANGS.map((l) => {
            const active = l.id === lang
            return (
              <button
                key={l.id}
                type="button"
                role="radio"
                aria-checked={active}
                lang={l.id}
                onClick={() => setLang(l.id)}
                className={cx(
                  'inline-flex h-[30px] items-center gap-1.5 rounded-ctl border px-3 text-base transition-colors duration-100',
                  active ? 'border-accent bg-accent-dim/40 font-semibold text-text' : 'border-line bg-surface2 text-text hover:border-faint hover:bg-hover',
                )}
              >
                <Check className={cx('size-3.5 text-accent', !active && 'hidden')} strokeWidth={2.4} />
                {l.name}
              </button>
            )
          })}
        </div>
      </div>
    </Section>
  )
}
