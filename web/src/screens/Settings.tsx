// Settings: appearance, language and connection in one place, plus the keyboard
// reference and what this build is.
import { useNavigate } from 'react-router-dom'
import { BookOpen } from 'lucide-react'
import { Button, Kbd, Page, Section } from '@/components/ui'
import { APP_VERSION } from '@/components/shell/Shell'
import { orDash } from '@/lib/format'
import { useAppearance } from '@/state/appearance'
import { useCatalog, useSession } from '@/state/session'
import { AppearanceSection } from './settings/Appearance'
import { t } from '@/i18n'
import { ConnectionSection } from './settings/Connection'
import { LanguageSection } from './settings/Language'
import { ShortcutsSection } from './settings/Shortcuts'
import { Fact, themeSummary, useShownTheme } from './settings/parts'

function AboutSection() {
  const navigate = useNavigate()
  const catalog = useCatalog()
  const version = useSession((s) => s.runtime?.version ?? '')
  const mode = useAppearance((s) => s.mode)
  const palette = useAppearance((s) => s.palette)
  const shown = useShownTheme()
  return (
    <Section
      title={t('settings.about')}
      action={
        <Button size="sm" icon={BookOpen} onClick={() => navigate('/docs')}>
          {t('nav.docs')} <Kbd>F1</Kbd>
        </Button>
      }
    >
      <div className="flex flex-col">
        <Fact label={t('settings.studioVersion')}>
          <span className="font-mono text-xs">{APP_VERSION}</span>
          <span className="text-xs text-muted">{t('settings.webBuild')}</span>
        </Fact>
        <Fact label={t('settings.runtimeApi')}>
          {t('settings.adminApi')}
          <span className="font-mono text-xs text-muted">{orDash(version)}</span>
        </Fact>
        <Fact label={t('settings.preludeVersion')} hint={t('settings.preludeHint', { marker: '-- af-prelude-version' })}>
          <span className="font-mono text-xs">v{catalog.prelude_version}</span>
        </Fact>
        <Fact label={t('settings.activeTheme')}>{themeSummary(palette, mode, shown)}</Fact>
      </div>
    </Section>
  )
}

export default function Settings() {
  return (
    <Page title={t('nav.settings')} subtitle={t('settings.subtitle')}>
      <div className="flex max-w-[1080px] flex-col gap-3">
        <AppearanceSection />
        <LanguageSection />
        <ConnectionSection />
        <ShortcutsSection />
        <AboutSection />
      </div>
    </Page>
  )
}
