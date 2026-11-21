import { Suspense, lazy, useEffect, useState, type ReactNode } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { ArrowLeft, Moon, Sun } from 'lucide-react'
import { Button, IconButton, Loading, Logo, cx } from '@/components/ui'
import { ErrorBoundary } from '@/components/ui/ErrorBoundary'
import { Overlays } from '@/components/ui/Overlays'
import { Shell } from '@/components/shell/Shell'
import { CommandPalette } from '@/components/shell/CommandPalette'
import { Shortcuts } from '@/components/shell/Shortcuts'
import { ShortcutsModal } from '@/components/shell/ShortcutsModal'
import { useSession } from '@/state/session'
import { useAppearance, useResolvedMode } from '@/state/appearance'
import { t, useLang } from '@/i18n'
import { LanguageToggle } from '@/components/shell/LanguageToggle'
import { LoginTitleBar, WindowControls, useWindowDrag } from '@/components/shell/WindowControls'
import { useFramelessWindow } from '@/lib/desktop'
import Login from '@/screens/Login'
import Overview from '@/screens/Overview'
import Agents from '@/screens/Agents'
import { NewAgentDialog } from '@/screens/NewAgentDialog'

// The heavier screens load on first visit; the editors alone are most of the
// bundle.
const Builder = lazy(() => import('@/screens/Builder'))
const Playground = lazy(() => import('@/screens/Playground'))
const Channels = lazy(() => import('@/screens/Channels'))
const Models = lazy(() => import('@/screens/Models'))
const Memory = lazy(() => import('@/screens/Memory'))
const Credentials = lazy(() => import('@/screens/Credentials'))
const Safety = lazy(() => import('@/screens/Safety'))
const Users = lazy(() => import('@/screens/Users'))
const Tenancy = lazy(() => import('@/screens/Tenancy'))
const Usage = lazy(() => import('@/screens/Usage'))
const Logs = lazy(() => import('@/screens/Logs'))
const Config = lazy(() => import('@/screens/Config'))
const Docs = lazy(() => import('@/screens/Docs'))
const Settings = lazy(() => import('@/screens/Settings'))

function Restoring() {
  const step = useSession((s) => s.restoreStep)
  const active = useSession((s) => s.restoring)
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4">
      <Logo size={56} />
      <Loading text={step || t('shell.reconnecting')} className="py-0" />
      {/* Cancel opens the welcome screen, prefilled with this connection. */}
      {active && <Button onClick={() => useSession.getState().cancelRestore()}>{t('common.cancel')}</Button>}
    </div>
  )
}

function OfflineDocsShell({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const mode = useResolvedMode()
  const toggleTheme = useAppearance((s) => s.toggle)
  const frameless = useFramelessWindow()
  const drag = useWindowDrag()
  return (
    <div className="flex h-full flex-col bg-bg">
      <header className="shrink-0 border-b border-line bg-float" {...drag}>
        <div className={cx('flex h-11 items-center gap-2 pl-3', frameless ? 'pr-0' : 'pr-3')}>
          <Logo size={22} />
          <span className="text-base font-semibold text-text">Studio</span>
          <span className="text-faint">/</span>
          <span className="min-w-0 flex-1 truncate text-base text-muted">{t('docs.title')}</span>
          <Button size="sm" variant="primary" icon={ArrowLeft} onClick={() => navigate('/')}>
            {t('login.backToConnect')}
          </Button>
          <LanguageToggle />
          <IconButton icon={mode === 'light' ? Moon : Sun} label={t('shell.toggleTheme')} onClick={toggleTheme} />
          <WindowControls />
        </div>
      </header>
      <main className="min-h-0 flex-1 overflow-hidden">{children}</main>
    </div>
  )
}

export default function App() {
  const client = useSession((s) => s.client)
  const restoring = useSession((s) => s.restoring)
  const location = useLocation()
  const navigate = useNavigate()
  const [booted, setBooted] = useState(false)
  // Subscribing keeps the document's theme attributes in step with the store.
  useAppearance((s) => s.mode)
  // Subscribing re-renders the whole tree when the language changes; t() reads
  // the store directly, so no screen has to subscribe on its own.
  useLang()
  // The desktop window without a native title bar (Windows): see LoginTitleBar.
  const frameless = useFramelessWindow()

  // Reconnect at once when a URL and token were remembered.
  useEffect(() => {
    let live = true
    useSession
      .getState()
      .restore()
      .finally(() => {
        if (live) setBooted(true)
      })
    return () => {
      live = false
    }
  }, [])

  // Also while switching to another saved connection: the welcome screen
  // mounts once the attempt is over, so it can say why one failed.
  if (!booted || (restoring && !client)) {
    return (
      <>
        <Restoring />
        {/* A reconnect can take a while: the window can still be moved and closed. */}
        <LoginTitleBar>{null}</LoginTitleBar>
      </>
    )
  }
  if (!client) {
    return (
      <>
        <ErrorBoundary resetKey={location.pathname} onHome={() => navigate('/')}>
          <Suspense fallback={<Loading />}>
            <Routes>
              <Route path="/docs" element={<OfflineDocsShell><Docs /></OfflineDocsShell>} />
              <Route path="/docs/:slug" element={<OfflineDocsShell><Docs /></OfflineDocsShell>} />
              <Route
                path="*"
                element={
                  <>
                    <Login />
                    {/* Before a connection there is no command bar to hold the switch.
                        In the frameless desktop window it sits in the title strip. */}
                    <LoginTitleBar>
                      <LanguageToggle className={frameless ? undefined : 'fixed right-4 top-4 z-30'} />
                    </LoginTitleBar>
                  </>
                }
              />
            </Routes>
          </Suspense>
        </ErrorBoundary>
        <Overlays />
      </>
    )
  }
  return (
    <>
      <Shell>
        <ErrorBoundary resetKey={location.pathname} onHome={() => navigate('/overview')}>
          <Suspense fallback={<Loading />}>
            {/* Keyed by connection: switching runtimes remounts every screen. */}
            <Routes key={client.displayURL}>
              <Route path="/" element={<Navigate to="/overview" replace />} />
              <Route path="/overview" element={<Overview />} />
              <Route path="/agents" element={<Agents />} />
              {/* Creating has its own path so an agent may be called anything, "new" included. */}
              <Route path="/new-agent" element={<Builder />} />
              <Route path="/agents/:name" element={<Builder />} />
              <Route path="/playground" element={<Playground />} />
              <Route path="/channels" element={<Channels />} />
              <Route path="/models" element={<Models />} />
              <Route path="/memory" element={<Memory />} />
              <Route path="/credentials" element={<Credentials />} />
              <Route path="/safety" element={<Safety />} />
              <Route path="/users" element={<Users />} />
              <Route path="/users/:id" element={<Users />} />
              <Route path="/tenancy" element={<Tenancy />} />
              <Route path="/usage" element={<Usage />} />
              <Route path="/logs" element={<Logs />} />
              <Route path="/config" element={<Config />} />
              <Route path="/docs" element={<Docs />} />
              <Route path="/docs/:slug" element={<Docs />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="*" element={<Navigate to="/overview" replace />} />
            </Routes>
          </Suspense>
        </ErrorBoundary>
      </Shell>
      <NewAgentDialog />
      <CommandPalette />
      <Shortcuts />
      <ShortcutsModal />
      <Overlays />
    </>
  )
}
