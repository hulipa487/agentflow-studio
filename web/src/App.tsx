import { Suspense, lazy, useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { Button, Loading, Logo } from '@/components/ui'
import { Overlays } from '@/components/ui/Overlays'
import { Shell } from '@/components/shell/Shell'
import { CommandPalette } from '@/components/shell/CommandPalette'
import { Shortcuts } from '@/components/shell/Shortcuts'
import { useSession } from '@/state/session'
import { useAppearance } from '@/state/appearance'
import { t, useLang } from '@/i18n'
import { LanguageToggle } from '@/components/shell/LanguageToggle'
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

export default function App() {
  const client = useSession((s) => s.client)
  const restoring = useSession((s) => s.restoring)
  const [booted, setBooted] = useState(false)
  // Subscribing keeps the document's theme attributes in step with the store.
  useAppearance((s) => s.mode)
  // Subscribing re-renders the whole tree when the language changes; t() reads
  // the store directly, so no screen has to subscribe on its own.
  useLang()

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
    return <Restoring />
  }
  if (!client) {
    return (
      <>
        <Login />
        {/* Before a connection there is no command bar to hold the switch. */}
        <LanguageToggle className="fixed right-4 top-4 z-30" />
        <Overlays />
      </>
    )
  }
  return (
    <>
      <Shell>
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
      </Shell>
      <NewAgentDialog />
      <CommandPalette />
      <Shortcuts />
      <Overlays />
    </>
  )
}
