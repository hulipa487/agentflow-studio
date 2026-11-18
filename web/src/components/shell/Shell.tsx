// The shell: a command bar (where am I, jump anywhere, which runtime), a
// compact rail, and a status bar with the runtime's identity. It mirrors the
// desktop app's shell so the two read as one product.
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { BookOpen, Check, ChevronDown, Lock, Moon, PanelLeft, Plus, Search, Settings, Sun } from 'lucide-react'
import { Dot, IconButton, Kbd, Logo, StatusChip, cx } from '@/components/ui'
import { MOD, NAV, SECTIONS, digitShortcut, navForPath } from '@/nav'
import { useAppearance, useResolvedMode } from '@/state/appearance'
import { useConnections } from '@/state/connections'
import { useSession } from '@/state/session'
import { useUI } from '@/state/ui'
import { humanDuration, shortID } from '@/lib/format'

const APP_VERSION = '0.1.0'

// Where am I: the screen, and the item open in it. The rail already says
// which section a screen is in, so the crumb does not repeat it.
function useBreadcrumb(): string[] {
  const { pathname } = useLocation()
  const it = navForPath(pathname)
  if (!it) return ['Studio']
  if (pathname === '/new-agent') return [it.label, 'New agent']
  const rest = pathname.slice(it.path.length).replace(/^\//, '')
  if (!rest || it.id === 'docs') return [it.label]
  return [it.label, decodeURIComponent(rest)]
}

function ConnectionMenu() {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const profiles = useConnections((s) => s.profiles)
  const active = useConnections((s) => s.active)
  const online = useSession((s) => s.online)
  const client = useSession((s) => s.client)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const switchTo = async (name: string) => {
    setOpen(false)
    if (name === active) return
    useConnections.getState().select(name)
    useSession.getState().disconnect()
    // A profile with a token reconnects at once; without one the welcome
    // screen opens prefilled with its address.
    await useSession.getState().restore()
  }

  const item = 'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-base hover:bg-hover'
  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        title={client?.displayURL}
        className="flex h-[30px] max-w-56 items-center gap-2 rounded-ctl px-2.5 hover:bg-hover"
      >
        <Dot tone={online ? 'success' : 'danger'} pulse={!online} />
        <span className="min-w-0 flex-1 truncate text-left text-sm text-muted">
          {online ? '' : 'offline · '}
          {active || client?.displayURL || 'not connected'}
        </span>
        <ChevronDown className="size-3.5 shrink-0 text-faint" />
      </button>
      {open && (
        <div role="menu" className="absolute right-0 top-[calc(100%+6px)] z-40 w-80 animate-pop-in rounded-card border border-line bg-float p-1 shadow-float">
          <div className="px-2 pb-1 pt-1.5 text-xs font-medium text-faint">Connections</div>
          {profiles.length === 0 && <div className="px-2 py-1.5 text-xs text-muted">No saved connections yet.</div>}
          {profiles.map((p) => (
            <button key={p.name} type="button" role="menuitem" className={item} onClick={() => switchTo(p.name)}>
              <Dot tone={p.name === active ? 'success' : 'neutral'} />
              <span className="min-w-0 flex-1">
                <span className={cx('block truncate', p.name === active && 'font-semibold')}>{p.name}</span>
                <span className="block truncate font-mono text-xs text-muted">
                  {p.url}
                  {p.viaProxy ? ' · via proxy' : ''}
                </span>
              </span>
              {p.name === active && <Check className="size-3.5 text-success" />}
            </button>
          ))}
          <div className="my-1 border-t border-line" />
          <button
            type="button"
            role="menuitem"
            className={item}
            onClick={() => {
              setOpen(false)
              // Keep the saved profiles; only the active mirror is cleared so
              // the welcome screen starts blank.
              useConnections.getState().clearActive()
              useSession.getState().disconnect()
            }}
          >
            <Plus className="size-4 text-muted" /> Use another connection…
          </button>
          <button type="button" role="menuitem" className={item} onClick={() => { setOpen(false); navigate('/settings') }}>
            <Settings className="size-4 text-muted" /> Connection settings
          </button>
          <button type="button" role="menuitem" className={item} onClick={() => { setOpen(false); useSession.getState().lock() }}>
            <Lock className="size-4 text-muted" /> Lock (forget the token)
          </button>
        </div>
      )}
    </div>
  )
}

function TopBar() {
  const crumb = useBreadcrumb()
  const navigate = useNavigate()
  const mode = useResolvedMode()
  const toggleTheme = useAppearance((s) => s.toggle)
  const toggleRail = useUI((s) => s.toggleRail)
  const setPalette = useUI((s) => s.setPalette)
  return (
    <header className="shrink-0 border-b border-line bg-float">
      <div className="flex h-11 items-center gap-2 pl-2.5 pr-3">
        <button
          type="button"
          onClick={toggleRail}
          title="Toggle navigation rail"
          aria-label="Toggle navigation rail"
          className="group relative flex size-[30px] shrink-0 items-center justify-center rounded-ctl hover:bg-hover"
        >
          <Logo size={22} className="transition-opacity group-hover:opacity-0" />
          <PanelLeft className="absolute size-4 text-muted opacity-0 transition-opacity group-hover:opacity-100" />
        </button>
        <nav aria-label="Location" className="flex min-w-0 items-center gap-1.5 text-base">
          {crumb.map((part, i) => (
            <span key={i} className="flex min-w-0 items-center gap-1.5">
              {i > 0 && <span className="text-faint">/</span>}
              <span className={cx('truncate', i === crumb.length - 1 ? 'text-text' : 'text-muted')}>{part}</span>
            </span>
          ))}
        </nav>
        <div className="flex-1" />
        <button
          type="button"
          onClick={() => setPalette(true)}
          className="hidden h-[30px] w-64 items-center gap-2 rounded-ctl border border-line bg-bg px-2.5 text-left hover:border-faint md:flex"
        >
          <Search className="size-3.5 shrink-0 text-faint" />
          <span className="min-w-0 flex-1 truncate text-base text-faint">Go to…</span>
          <Kbd>{MOD}+K</Kbd>
        </button>
        <ConnectionMenu />
        <div className="flex items-center">
          <IconButton icon={BookOpen} label="Documentation (F1)" onClick={() => navigate('/docs')} />
          <IconButton icon={mode === 'light' ? Moon : Sun} label="Toggle light / dark" onClick={toggleTheme} />
          <IconButton icon={Settings} label="Settings" onClick={() => navigate('/settings')} />
        </div>
      </div>
    </header>
  )
}

// The rail lists screens and nothing else: creating an agent lives on the
// Agents screen, in the command palette and on Alt+N, and each screen's
// shortcut is in its tooltip rather than on every row.
function Rail() {
  const collapsed = useUI((s) => s.railCollapsed)
  const activeID = navForPath(useLocation().pathname)?.id
  if (collapsed) return null
  return (
    <nav className="flex w-[200px] shrink-0 flex-col border-r border-line bg-float px-2 pb-2 pt-1" aria-label="Screens">
      <div className="min-h-0 flex-1 overflow-y-auto">
        {SECTIONS.map((section) => {
          const items = NAV.filter((it) => it.section === section)
          return (
            <div key={section}>
              {section !== 'Home' && (
                <div className="px-2.5 pb-1 pt-4 text-xs font-medium text-faint">{section}</div>
              )}
              {items.map((it) => (
                <Link
                  key={it.id}
                  to={it.path}
                  aria-current={it.id === activeID ? 'page' : undefined}
                  title={it.digit ? `${it.label} (${digitShortcut(it.digit)})` : it.id === 'docs' ? `${it.label} (F1)` : it.label}
                  className={cx(
                    'flex h-7 items-center gap-2 rounded-ctl px-2.5 text-base transition-colors duration-100',
                    it.id === activeID ? 'bg-hover text-text' : 'text-muted hover:bg-hover hover:text-text',
                  )}
                >
                  <it.icon className={cx('size-4 shrink-0', it.id === activeID ? 'text-text' : 'text-faint')} strokeWidth={1.75} />
                  <span className="min-w-0 flex-1 truncate">{it.label}</span>
                </Link>
              ))}
            </div>
          )
        })}
      </div>
    </nav>
  )
}

function StatusBar() {
  const online = useSession((s) => s.online)
  const client = useSession((s) => s.client)
  const st = useSession((s) => s.runtime)
  const prelude = useSession((s) => s.catalog.prelude_version)
  const facts: string[] = []
  if (client) facts.push(client.displayURL + (client.viaProxy && !client.local ? ' (proxy)' : ''))
  if (st) {
    if (st.instance) facts.push('instance ' + shortID(st.instance))
    if (st.region) facts.push(st.region)
    if (st.topology) facts.push(st.topology)
    if (st.version) facts.push(st.version)
    facts.push('up ' + humanDuration(st.uptime_s), `${st.sessions_active} busy / ${st.sessions_idle} idle`)
  }
  return (
    <footer className="flex h-6 shrink-0 items-center gap-3 border-t border-line bg-float px-3 text-xs text-faint">
      <span className={cx('inline-flex items-center gap-1.5', online ? 'text-success' : 'text-danger')}>
        <Dot tone={online ? 'success' : 'danger'} className="!size-1.5" />
        {online ? 'connected' : 'offline'}
      </span>
      <span className="min-w-0 flex-1 truncate">{facts.join('  ·  ')}</span>
      <span className="shrink-0">
        prelude v{prelude} · studio {APP_VERSION}
      </span>
    </footer>
  )
}

export function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full flex-col bg-bg">
      <TopBar />
      <div className="flex min-h-0 flex-1">
        <Rail />
        <main className="min-w-0 flex-1 overflow-hidden">{children}</main>
      </div>
      <StatusBar />
    </div>
  )
}

export { APP_VERSION, StatusChip }
