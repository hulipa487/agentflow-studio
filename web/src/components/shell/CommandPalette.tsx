// Go to (Ctrl/Cmd+K): one searchable list over every screen, every agent
// ("Edit echo", "Chat with assistant") and the global actions.
import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { Bot, FlaskConical, Keyboard, Lock, Moon, PanelLeft, Plus, RotateCw, Search, Send, type LucideIcon } from 'lucide-react'
import { Kbd, cx } from '@/components/ui'
import { ALT, NAV, digitShortcut } from '@/nav'
import { useAppearance } from '@/state/appearance'
import { useSession } from '@/state/session'
import { useUI } from '@/state/ui'
import { t, useLang } from '@/i18n'

interface Command {
  title: string
  hint: string
  shortcut?: string
  icon: LucideIcon
  run: () => void
}

/** Ranks a command against the query; lower is better, -1 is no match. */
export function paletteMatch(c: Pick<Command, 'title' | 'hint'>, q: string): number {
  if (q === '') return 0
  const title = c.title.toLowerCase()
  const hint = c.hint.toLowerCase()
  // "Go to Users" matches "use" by the screen's name, in any language.
  const goTo = t('shell.palette.goTo', { screen: '' }).toLowerCase()
  const name = title.startsWith(goTo) ? title.slice(goTo.length) : title
  if (name.startsWith(q)) return 0
  if (title.includes(q)) return 1
  if (hint.includes(q)) return 2
  for (const w of q.split(/\s+/).filter(Boolean)) {
    if (!title.includes(w) && !hint.includes(w)) return -1
  }
  return 3
}

export function CommandPalette() {
  const open = useUI((s) => s.paletteOpen)
  const setOpen = useUI((s) => s.setPalette)
  const navigate = useNavigate()
  const client = useSession((s) => s.client)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(0)
  const [agents, setAgents] = useState<string[]>([])
  const list = useRef<HTMLDivElement>(null)
  const lang = useLang()

  useEffect(() => {
    if (!open) return
    setQuery('')
    setSelected(0)
    let live = true
    client
      ?.agents()
      .then((rows) => {
        if (live) setAgents(rows.filter((r) => !r.spawn).map((r) => r.name).sort())
      })
      .catch(() => {})
    return () => {
      live = false
    }
  }, [open, client])

  const commands = useMemo<Command[]>(() => {
    const out: Command[] = NAV.map((it) => ({
      title: t('shell.palette.goTo', { screen: t(it.labelKey) }),
      hint: t(it.hintKey),
      shortcut: it.digit ? digitShortcut(it.digit) : it.id === 'docs' ? 'F1' : undefined,
      icon: it.icon,
      run: () => navigate(it.path),
    }))
    for (const name of agents) {
      out.push({ title: t('shell.palette.editAgent', { name }), hint: t('shell.palette.editAgentHint', { name }), icon: Bot, run: () => navigate('/agents/' + encodeURIComponent(name)) })
      out.push({ title: t('shell.palette.chatWith', { name }), hint: t('shell.palette.chatWithHint', { name }), icon: FlaskConical, run: () => navigate('/playground?agent=' + encodeURIComponent(name)) })
    }
    out.push(
      { title: t('shell.newAgent'), hint: t('shell.palette.newAgentHint'), shortcut: `${ALT}+N`, icon: Plus, run: () => useUI.getState().setNewAgent(true) },
      { title: t('shell.palette.refresh'), hint: t('shell.palette.refreshHint'), shortcut: `${ALT}+R`, icon: RotateCw, run: () => useUI.getState().actions.refresh?.() },
      { title: t('shell.toggleTheme'), hint: t('shell.palette.toggleThemeHint'), icon: Moon, run: () => useAppearance.getState().toggle() },
      { title: t('shell.toggleRail'), hint: t('shell.palette.toggleRailHint'), icon: PanelLeft, run: () => useUI.getState().toggleRail() },
      { title: t('shell.palette.shortcuts'), hint: t('shell.palette.shortcutsHint'), shortcut: '?', icon: Keyboard, run: () => useUI.getState().setShortcuts(true) },
      { title: t('shell.palette.invite'), hint: t('shell.palette.inviteHint'), icon: Send, run: () => navigate('/users?invite=1') },
      { title: t('shell.palette.lock'), hint: t('shell.palette.lockHint'), icon: Lock, run: () => useSession.getState().lock() },
    )
    return out
    // lang is a dependency: the titles and hints are translated text.
  }, [agents, navigate, lang])

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase()
    return commands
      .map((c, i) => ({ c, rank: paletteMatch(c, q), i }))
      .filter((x) => x.rank >= 0)
      .sort((a, b) => a.rank - b.rank || a.i - b.i)
      .map((x) => x.c)
  }, [commands, query])

  useEffect(() => {
    setSelected(0)
  }, [query])

  useEffect(() => {
    list.current?.querySelector<HTMLElement>(`[data-index="${selected}"]`)?.scrollIntoView({ block: 'nearest' })
  }, [selected])

  if (!open) return null
  const run = (c: Command | undefined) => {
    if (!c) return
    setOpen(false)
    c.run()
  }
  return createPortal(
    <div
      data-modal
      className="fixed inset-0 z-50 flex animate-fade-in items-start justify-center bg-black/55 px-4 pt-[12vh] backdrop-blur-[2px]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) setOpen(false)
      }}
    >
      <div role="dialog" aria-label={t('shell.palette.title')} className="flex max-h-[70vh] w-[600px] max-w-full animate-pop-in flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-float">
        <div className="flex items-center gap-2 border-b border-line px-3.5">
          <Search className="size-4 shrink-0 text-faint" />
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('shell.palette.placeholder')}
            spellCheck={false}
            className="h-11 min-w-0 flex-1 bg-transparent text-md text-text placeholder:text-faint focus:outline-none"
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') {
                e.preventDefault()
                setSelected((s) => Math.min(s + 1, shown.length - 1))
              } else if (e.key === 'ArrowUp') {
                e.preventDefault()
                setSelected((s) => Math.max(s - 1, 0))
              } else if (e.key === 'Enter') {
                e.preventDefault()
                run(shown[selected])
              } else if (e.key === 'Escape') {
                e.preventDefault()
                e.stopPropagation()
                setOpen(false)
              }
            }}
          />
          <Kbd>Esc</Kbd>
        </div>
        <div ref={list} className="min-h-0 flex-1 overflow-y-auto p-1.5">
          {shown.length === 0 && <div className="px-3 py-6 text-center text-sm text-muted">{t('shell.palette.noMatchQuery', { query })}</div>}
          {shown.map((c, i) => (
            <button
              key={c.title}
              type="button"
              data-index={i}
              onMouseMove={() => setSelected(i)}
              onClick={() => run(c)}
              className={cx('relative flex w-full items-center gap-2.5 rounded-[7px] px-3 py-1.5 text-left', i === selected ? 'bg-surface2' : '')}
            >
              {i === selected && <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-full bg-accent" />}
              <c.icon className={cx('size-4 shrink-0', i === selected ? 'text-accent' : 'text-muted')} strokeWidth={1.9} />
              <span className="min-w-0 flex-1">
                <span className={cx('block truncate text-base', i === selected ? 'font-semibold text-text' : 'text-text')}>{c.title}</span>
                <span className="block truncate text-xs text-muted">{c.hint}</span>
              </span>
              {c.shortcut && <Kbd>{c.shortcut}</Kbd>}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-3 border-t border-line px-3.5 py-1.5 font-mono text-xs text-faint">
          <span className="inline-flex items-center gap-1"><Kbd>↑</Kbd><Kbd>↓</Kbd> {t('shell.palette.move')}</span>
          <span className="inline-flex items-center gap-1"><Kbd>Enter</Kbd> {t('shell.palette.run')}</span>
          <span className="ml-auto">{t('shell.palette.count', { shown: shown.length, total: commands.length })}</span>
        </div>
      </div>
    </div>,
    document.body,
  )
}
