// Settings: appearance and connection in one place, plus the keyboard
// reference and what this build is.
import { useNavigate } from 'react-router-dom'
import { BookOpen } from 'lucide-react'
import { Button, Kbd, Page, Section } from '@/components/ui'
import { APP_VERSION } from '@/components/shell/Shell'
import { orDash } from '@/lib/format'
import { useAppearance } from '@/state/appearance'
import { useCatalog, useSession } from '@/state/session'
import { AppearanceSection } from './settings/Appearance'
import { ConnectionSection } from './settings/Connection'
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
      title="About"
      action={
        <Button size="sm" icon={BookOpen} onClick={() => navigate('/docs')}>
          Documentation <Kbd>F1</Kbd>
        </Button>
      }
    >
      <div className="flex flex-col">
        <Fact label="Studio version">
          <span className="font-mono text-xs">{APP_VERSION}</span>
          <span className="text-xs text-muted">web</span>
        </Fact>
        <Fact label="Runtime API">
          AgentFlow admin API
          <span className="font-mono text-xs text-muted">{orDash(version)}</span>
        </Fact>
        <Fact label="Prelude version" hint="The Lua-facing API version of the connected runtime. Every loop declares the one it targets (-- af-prelude-version) and the runtime refuses any other.">
          <span className="font-mono text-xs">v{catalog.prelude_version}</span>
        </Fact>
        <Fact label="Active theme">{themeSummary(palette, mode, shown)}</Fact>
      </div>
    </Section>
  )
}

export default function Settings() {
  return (
    <Page title="Settings" subtitle="Customize Studio appearance, theme palette, and runtime connection.">
      <div className="flex max-w-[1080px] flex-col gap-3">
        <AppearanceSection />
        <ConnectionSection />
        <ShortcutsSection />
        <AboutSection />
      </div>
    </Page>
  )
}
