// The shell: a command bar (where am I, jump anywhere, which runtime), a
// compact rail, and a status bar with the runtime's identity. In the desktop
// app on Windows the command bar is also the window's title bar.
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { ArrowUpCircle, BookOpen, Check, ChevronDown, Keyboard, Lock, Moon, PanelLeft, Plus, RotateCw, Search, Settings, Sun } from 'lucide-react'
import { Dot, IconButton, Kbd, Logo, StatusChip, cx } from '@/components/ui'
import { MOD, NAV, SECTIONS, digitShortcut, navForPath, sectionKey } from '@/nav'
import { useAppearance, useResolvedMode } from '@/state/appearance'
import { useConnections } from '@/state/connections'
import { useSession } from '@/state/session'
import { useUI } from '@/state/ui'
import { useUpdater } from '@/state/updater'
import { humanDuration, shortID } from '@/lib/format'
import { t } from '@/i18n'
import { LanguageToggle } from './LanguageToggle'
import { WindowControls, useWindowDrag } from './WindowControls'
import { useDesktopShell, useDesktopVersion, useFramelessWindow } from '@/lib/desktop'

const APP_VERSION = '0.1.0'

interface Crumb {
  label: string
  to?: string
}

// Where am I: the screen, and the item open in it. The rail already says
// which section a screen is in, so the crumb does not repeat it.
function useBreadcrumb(): Crumb[] {
  const { pathname } = useLocation()
  const it = navForPath(pathname)
  if (!it) return [{ label: 'Studio' }]
  const label = t(it.labelKey)
  if (pathname === '/new-agent') return [{ label, to: it.path }, { label: t('shell.newAgent') }]
  const rest = pathname.slice(it.path.length).replace(/^\//, '')
  if (!rest || it.id === 'docs') return [{ label }]
  return [{ label, to: it.path }, { label: decodeURIComponent(rest) }]
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
          {online
            ? active || client?.displayURL || t('shell.notConnected')
            : t('shell.offlineName', { name: active || client?.displayURL || t('shell.notConnected') })}
        </span>
        <ChevronDown className="size-3.5 shrink-0 text-faint" />
      </button>
      {open && (
        <div role="menu" className="absolute right-0 top-[calc(100%+6px)] z-40 w-80 animate-pop-in rounded-card border border-line bg-float p-1 shadow-float">
          <div className="px-2 pb-1 pt-1.5 text-xs font-medium text-faint">{t('shell.connections')}</div>
          {profiles.length === 0 && <div className="px-2 py-1.5 text-xs text-muted">{t('shell.noConnections')}</div>}
          {profiles.map((p) => (
            <button key={p.name} type="button" role="menuitem" className={item} onClick={() => switchTo(p.name)}>
              <Dot tone={p.name === active ? 'success' : 'neutral'} />
              <span className="min-w-0 flex-1">
                <span className={cx('block truncate', p.name === active && 'font-semibold')}>{p.name}</span>
                <span className="block truncate font-mono text-xs text-muted">
                  {p.viaProxy ? t('shell.urlViaProxy', { url: p.url }) : p.url}
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
            <Plus className="size-4 text-muted" /> {t('shell.useAnotherConnection')}
          </button>
          <button type="button" role="menuitem" className={item} onClick={() => { setOpen(false); navigate('/settings') }}>
            <Settings className="size-4 text-muted" /> {t('shell.connectionSettings')}
          </button>
          <button type="button" role="menuitem" className={item} onClick={() => { setOpen(false); useSession.getState().lock() }}>
            <Lock className="size-4 text-muted" /> {t('shell.lockForgetToken')}
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
  const setShortcuts = useUI((s) => s.setShortcuts)
  const desktop = useDesktopShell()
  const updateStatus = useUpdater((s) => s.status)
  const dismissedVersion = useUpdater((s) => s.dismissedVersion)
  // In the desktop app on Windows this bar is also the window's title bar.
  const frameless = useFramelessWindow()
  const drag = useWindowDrag()
  return (
    <header className="shrink-0 border-b border-line bg-float" {...drag}>
      <div className={cx('flex h-11 items-center gap-2 pl-2.5', frameless ? 'pr-0' : 'pr-3')}>
        <button
          type="button"
          onClick={toggleRail}
          title={t('shell.toggleRail')}
          aria-label={t('shell.toggleRail')}
          className="group relative flex size-[30px] shrink-0 items-center justify-center rounded-ctl hover:bg-hover"
        >
          <Logo size={22} className="transition-opacity group-hover:opacity-0" />
          <PanelLeft className="absolute size-4 text-muted opacity-0 transition-opacity group-hover:opacity-100" />
        </button>
        <nav aria-label={t('shell.location')} className="flex min-w-0 items-center gap-1.5 text-base">
          {crumb.map((part, i) => {
            const last = i === crumb.length - 1
            return (
              <span key={i} className="flex min-w-0 items-center gap-1.5">
                {i > 0 && <span className="text-faint">/</span>}
                {part.to && !last ? (
                  <Link to={part.to} className="truncate text-muted transition-colors hover:text-text">
                    {part.label}
                  </Link>
                ) : (
                  <span className={cx('truncate', last ? 'font-medium text-text' : 'text-muted')}>{part.label}</span>
                )}
              </span>
            )
          })}
        </nav>
        <div className="flex-1" />
        <IconButton icon={Search} label={t('shell.withShortcut', { label: t('shell.goToHint'), key: `${MOD}+K` })} onClick={() => setPalette(true)} className="md:hidden" />
        <button
          type="button"
          onClick={() => setPalette(true)}
          className="hidden h-[30px] w-64 items-center gap-2 rounded-ctl border border-line bg-bg px-2.5 text-left hover:border-faint md:flex"
        >
          <Search className="size-3.5 shrink-0 text-faint" />
          <span className="min-w-0 flex-1 truncate text-base text-faint">{t('shell.goToHint')}</span>
          <Kbd>{MOD}+K</Kbd>
        </button>
        {desktop && updateStatus?.update_available && updateStatus.latest_version && updateStatus.latest_version !== dismissedVersion && (
          <button
            type="button"
            onClick={() => navigate('/settings')}
            className="hidden h-[30px] items-center gap-1.5 rounded-ctl border border-accent/40 bg-tone-accent px-2.5 text-xs font-medium text-accent transition hover:border-accent sm:inline-flex"
            title={t('shell.updateAvailable', { version: updateStatus.latest_version })}
          >
            <ArrowUpCircle className="h-3.5 w-3.5 shrink-0" strokeWidth={1.75} />
            <span className="truncate">{t('shell.updateAvailable', { version: updateStatus.latest_version })}</span>
          </button>
        )}
        <ConnectionMenu />
        <LanguageToggle />
        <div className="flex items-center">
          <IconButton icon={BookOpen} label={t('shell.withShortcut', { label: t('nav.docs'), key: 'F1' })} onClick={() => navigate('/docs')} />
          <IconButton icon={Keyboard} label={t('shell.withShortcut', { label: t('shell.shortcuts'), key: '?' })} onClick={() => setShortcuts(true)} />
          <IconButton icon={mode === 'light' ? Moon : Sun} label={t('shell.toggleTheme')} onClick={toggleTheme} />
          <IconButton icon={Settings} label={t('nav.settings')} onClick={() => navigate('/settings')} />
        </div>
        <WindowControls />
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
    <nav className="flex w-[200px] shrink-0 flex-col border-r border-line bg-float px-2 pb-2 pt-1" aria-label={t('shell.screens')}>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {SECTIONS.map((section) => {
          const items = NAV.filter((it) => it.section === section)
          return (
            <div key={section}>
              {section !== 'Home' && (
                <div className="px-2.5 pb-1 pt-4 text-xs font-medium text-faint">{t(sectionKey(section))}</div>
              )}
              {items.map((it) => {
                const active = it.id === activeID
                return (
                  <Link
                    key={it.id}
                    to={it.path}
                    aria-current={active ? 'page' : undefined}
                    title={
                      it.digit
                        ? t('shell.withShortcut', { label: t(it.labelKey), key: digitShortcut(it.digit) })
                        : it.id === 'docs'
                          ? t('shell.withShortcut', { label: t(it.labelKey), key: 'F1' })
                          : t(it.labelKey)
                    }
                    className={cx(
                      'relative flex h-7 items-center gap-2 rounded-ctl px-2.5 text-base transition-colors duration-100',
                      active ? 'bg-hover font-medium text-text' : 'text-muted hover:bg-hover hover:text-text',
                    )}
                  >
                    {active && <span className="absolute left-0 top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-full bg-accent" />}
                    <it.icon className={cx('size-4 shrink-0', active ? 'text-text' : 'text-faint')} strokeWidth={1.75} />
                    <span className="min-w-0 flex-1 truncate">{t(it.labelKey)}</span>
                  </Link>
                )
              })}
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
  const desktop = useDesktopShell()
  const desktopVersion = useDesktopVersion()
  const appVersion = desktop && desktopVersion ? desktopVersion : APP_VERSION
  const [retrying, setRetrying] = useState(false)
  const retry = async () => {
    if (retrying) return
    setRetrying(true)
    try {
      await useSession.getState().ping()
    } finally {
      setRetrying(false)
    }
  }
  const facts: string[] = []
  if (client) facts.push(client.viaProxy && !client.local ? t('shell.urlProxy', { url: client.displayURL }) : client.displayURL)
  if (st) {
    if (st.instance) facts.push(t('shell.instance', { id: shortID(st.instance) }))
    if (st.region) facts.push(st.region)
    if (st.topology) facts.push(st.topology)
    if (st.version) facts.push(st.version)
    facts.push(t('shell.uptime', { duration: humanDuration(st.uptime_s) }), t('shell.sessions', { busy: st.sessions_active, idle: st.sessions_idle }))
  }
  return (
    <footer className="flex h-6 shrink-0 items-center gap-3 border-t border-line bg-float px-3 text-xs text-faint">
      <span className={cx('inline-flex items-center gap-1.5', online ? 'text-success' : 'text-danger')}>
        <Dot tone={online ? 'success' : 'danger'} className="!size-1.5" />
        {online ? t('common.connected') : t('common.offline')}
      </span>
      {!online && (
        <button
          type="button"
          onClick={() => void retry()}
          disabled={retrying}
          className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs font-medium text-accent hover:bg-hover disabled:opacity-50"
        >
          <RotateCw className={cx('size-3', retrying && 'animate-spin')} />
          {t('shell.retryConnection')}
        </button>
      )}
      <span className="min-w-0 flex-1 truncate">{facts.join('  ·  ')}</span>
      <span className="shrink-0">
        {t('shell.versions', { prelude, version: appVersion })}
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
