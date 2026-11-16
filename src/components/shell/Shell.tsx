// The shell: a command bar (where am I, jump anywhere, which runtime), a
// compact rail, and a status bar with the runtime's identity. It mirrors the
// desktop app's shell so the two read as one product.
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { BookOpen, Check, ChevronDown, Lock, Moon, PanelLeft, Plus, Search, Settings, Sun } from 'lucide-react'
import { Dot, IconButton, Kbd, Logo, StatusChip, cx } from '@/components/ui'
import { ALT, MOD, NAV, SECTIONS, digitShortcut, navForPath } from '@/nav'
import { useAppearance, useResolvedMode } from '@/state/appearance'
import { useConnections } from '@/state/connections'
import { useSession } from '@/state/session'
import { useUI } from '@/state/ui'
import { humanDuration, shortID } from '@/lib/format'

const APP_VERSION = '0.1.0'

function useBreadcrumb(): string {
  const { pathname } = useLocation()
  const it = navForPath(pathname)
  if (!it) return 'Studio'
  if (pathname === '/new-agent') return `${it.section} / ${it.label} / new`
  const rest = pathname.slice(it.path.length).replace(/^\//, '')
  const base = `${it.section} / ${it.label}`
  if (!rest || it.id === 'docs') return base
  return `${base} / ${decodeURIComponent(rest)}`
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

  const item = 'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] hover:bg-hover'
  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        title={client?.displayURL}
        className="flex h-[30px] w-48 items-center gap-2 rounded-ctl border border-line bg-surface px-2.5 hover:bg-hover"
      >
        <Dot tone={online ? 'success' : 'danger'} pulse={!online} />
        <span className="min-w-0 flex-1 truncate text-left font-mono text-[11px] text-text">
          {online ? '' : 'offline · '}
          {active || client?.displayURL || 'not connected'}
        </span>
        <ChevronDown className="size-3.5 shrink-0 text-faint" />
      </button>
      {open && (
        <div role="menu" className="absolute right-0 top-[calc(100%+6px)] z-40 w-80 animate-pop-in rounded-card border border-line bg-float p-1 shadow-float">
          <div className="px-2 pb-1 pt-1.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-faint">Connections</div>
          {profiles.length === 0 && <div className="px-2 py-1.5 text-xs text-muted">No saved connections yet.</div>}
          {profiles.map((p) => (
            <button key={p.name} type="button" role="menuitem" className={item} onClick={() => switchTo(p.name)}>
              <Dot tone={p.name === active ? 'success' : 'neutral'} />
              <span className="min-w-0 flex-1">
                <span className={cx('block truncate', p.name === active && 'font-semibold')}>{p.name}</span>
                <span className="block truncate font-mono text-[11px] text-muted">
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
    <header className="shrink-0 bg-float">
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
        <span className="min-w-0 truncate font-mono text-[13px] text-text">{crumb}</span>
        <div className="flex-1" />
        <button
          type="button"
          onClick={() => setPalette(true)}
          className="hidden h-[30px] w-[300px] items-center gap-2 rounded-ctl border border-line bg-bg px-2.5 text-left hover:border-faint md:flex"
        >
          <Search className="size-3.5 shrink-0 text-faint" />
          <span className="min-w-0 flex-1 truncate text-[13px] text-faint">Search screens, agents, actions…</span>
          <Kbd>{MOD}+K</Kbd>
        </button>
        <ConnectionMenu />
        <div className="flex items-center">
          <IconButton icon={BookOpen} label="Documentation (F1)" onClick={() => navigate('/docs')} />
          <IconButton icon={mode === 'light' ? Moon : Sun} label="Toggle light / dark" onClick={toggleTheme} />
          <IconButton icon={Settings} label="Settings" onClick={() => navigate('/settings')} />
        </div>
      </div>
      <div className="flag-stripe h-0.5" />
    </header>
  )
}

function Rail() {
  const collapsed = useUI((s) => s.railCollapsed)
  const setNewAgent = useUI((s) => s.setNewAgent)
  const activeID = navForPath(useLocation().pathname)?.id
  if (collapsed) return null
  return (
    <nav className="flex w-[216px] shrink-0 flex-col border-r border-line bg-float px-2 pb-2 pt-3" aria-label="Screens">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => setNewAgent(true)}
          className="flex h-[30px] flex-1 items-center justify-center gap-1.5 rounded-ctl bg-accent text-[13px] font-semibold text-on-primary hover:brightness-110"
        >
          <Plus className="size-3.5" strokeWidth={2.4} /> New agent
        </button>
        <Kbd>{ALT}+N</Kbd>
      </div>
      <div className="mt-1 min-h-0 flex-1 overflow-y-auto">
        {SECTIONS.map((section) => {
          const items = NAV.filter((it) => it.section === section)
          return (
            <div key={section}>
              {section !== 'Home' && (
                <div className="px-3 pb-1 pt-3 text-[10px] font-bold uppercase tracking-[0.07em] text-faint">{section}</div>
              )}
              {items.map((it) => (
                <Link
                  key={it.id}
                  to={it.path}
                  aria-current={it.id === activeID ? 'page' : undefined}
                  className={cx(
                    'group relative flex h-[30px] items-center gap-2 rounded-[7px] pl-3 pr-2 text-[13px] transition-colors duration-100',
                    it.id === activeID ? 'bg-surface2 font-semibold text-text' : 'text-muted hover:bg-hover hover:text-text',
                  )}
                >
                  {it.id === activeID && <span className="absolute left-0 top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-full bg-gradient-to-b from-accent to-accent2" />}
                  <it.icon className={cx('size-[15px] shrink-0', it.id === activeID && 'text-accent')} strokeWidth={1.9} />
                  <span className="min-w-0 flex-1 truncate">{it.label}</span>
                  {it.digit ? <Kbd>{digitShortcut(it.digit)}</Kbd> : it.id === 'docs' ? <Kbd>F1</Kbd> : null}
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
  const hint = (key: string, what: string) => (
    <span className="hidden items-center gap-1 lg:inline-flex">
      <Kbd>{key}</Kbd>
      <span>{what}</span>
    </span>
  )
  return (
    <footer className="flex h-6 shrink-0 items-center gap-3 border-t border-line bg-float px-3 font-mono text-[11px] text-faint">
      <span className={cx('inline-flex items-center gap-1.5', online ? 'text-success' : 'text-danger')}>
        <Dot tone={online ? 'success' : 'danger'} className="!size-1.5" />
        {online ? 'connected' : 'offline'}
      </span>
      <span className="min-w-0 flex-1 truncate text-muted">{facts.join('  ·  ')}</span>
      {hint(`${MOD}+K`, 'commands')}
      {hint('F1', 'docs')}
      {hint(`${ALT}+R`, 'refresh')}
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
