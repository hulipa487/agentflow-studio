// The desktop shell. In the desktop app (desktop-wails) this web app is shown
// in the system webview, and the shell's Go side answers under the local
// proxy's prefix with the things a browser tab cannot do: a save dialog,
// opening a folder in the file manager, the old desktop app's saved
// connections, and the window colour behind the page. In a browser none of
// it exists and every function here falls back or does nothing.
import { PROXY_PREFIX, probeProxy } from '@/api/client'
import { useEffect, useState } from 'react'
import { useI18n, type Lang } from '@/i18n'
import { useAppearance } from '@/state/appearance'
import { importProfiles } from '@/state/connections'

const DESKTOP = PROXY_PREFIX + '/__desktop/'

let shell: Promise<boolean> | null = null

/** Whether this page runs inside the desktop shell. Asked once, then cached. */
export function inDesktopShell(): Promise<boolean> {
  shell ??= probeProxy().then((p) => p.shell === 'wails')
  return shell
}

/** inDesktopShell as React state: false until the answer arrives. */
export function useDesktopShell(): boolean {
  const [yes, setYes] = useState(false)
  useEffect(() => {
    let live = true
    void inDesktopShell().then((v) => live && setYes(v))
    return () => {
      live = false
    }
  }, [])
  return yes
}

async function call<T>(action: string, body?: unknown): Promise<T> {
  const res = await fetch(DESKTOP + action, {
    method: body === undefined ? 'GET' : 'POST',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
  })
  const out = (await res.json()) as T & { error?: string }
  if (!res.ok) throw new Error(out.error || `desktop ${action}: HTTP ${res.status}`)
  return out
}

export type SaveResult = { ok: true; path: string; name: string } | { ok: false; cancelled?: boolean; error?: string }

/** Shows the save dialog and writes text where the operator chose. */
export function saveWithDialog(name: string, content: string): Promise<SaveResult> {
  return call<SaveResult>('save', { name, content })
}

export type OpenFolderResult = { ok: true; dir: string } | { ok: false; problem: 'missing' | 'notdir' | 'failed'; dir: string; error?: string }

/** Shows the folder holding path (or path itself, if a folder) in the file manager. */
export function openFolder(path: string): Promise<OpenFolderResult> {
  return call<OpenFolderResult>('open-folder', { path })
}

export interface DesktopConfig {
  found: boolean
  profiles: { name: string; url: string; token?: string; forget_token?: boolean }[]
  active?: string
  theme?: string
  palette?: string
  language?: string
}

/** The connections and preferences the old (Fyne) desktop app saved, if any. */
export function oldDesktopConfig(): Promise<DesktopConfig> {
  return call<DesktopConfig>('fyne-config')
}

// The window shows its own colour wherever the page has not painted yet: the
// first frame, and the edge a resize uncovers. Keep it the page's background.
function syncBackground() {
  let last = ''
  const send = () => {
    const bg = getComputedStyle(document.body).backgroundColor
    const m = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(bg)
    if (!m || bg === last) return
    last = bg
    void call('background', { r: Number(m[1]), g: Number(m[2]), b: Number(m[3]) }).catch(() => {})
  }
  // The theme is applied as data-theme / data-palette on <html>; the new
  // colours are computed on the next frame.
  new MutationObserver(() => requestAnimationFrame(send)).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme', 'data-palette', 'class'],
  })
  requestAnimationFrame(send)
}

const IMPORTED_KEY = 'agentflow-studio:desktop-imported'
// Where this app keeps its own choices (src/i18n, src/state/appearance).
const LANGUAGE_KEY = 'agentflow-studio:language'
const APPEARANCE_KEY = 'agentflow-studio:appearance'
const LANGS: readonly string[] = ['en', 'zh-Hans', 'zh-Hant']
const isLang = (v: unknown): v is Lang => typeof v === 'string' && LANGS.includes(v)

function stored(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

/**
 * On the first run of the desktop shell, carries over what the old (Fyne)
 * desktop app saved: its connections, and its language and theme where this
 * app has none of its own yet. Runs once; later changes are this app's.
 */
async function importOldDesktopApp(): Promise<void> {
  if (stored(IMPORTED_KEY)) return
  let cfg: DesktopConfig
  try {
    cfg = await oldDesktopConfig()
  } catch {
    return
  }
  try {
    localStorage.setItem(IMPORTED_KEY, '1')
  } catch {
    return // storage is unavailable: nothing could be kept anyway
  }
  if (!cfg.found) return
  importProfiles(cfg.profiles, cfg.active)
  if (!stored(LANGUAGE_KEY) && isLang(cfg.language)) useI18n.getState().setLang(cfg.language)
  if (!stored(APPEARANCE_KEY)) {
    if (cfg.theme === 'light' || cfg.theme === 'dark') useAppearance.getState().setMode(cfg.theme)
    if (cfg.palette === 'trans' || cfg.palette === 'obsidian') useAppearance.getState().setPalette(cfg.palette)
  }
}

/** Starts what the shell needs from the page; does nothing in a browser. */
export function startDesktopBridge(): void {
  void inDesktopShell().then((yes) => {
    if (!yes) return
    syncBackground()
    void importOldDesktopApp()
  })
}
