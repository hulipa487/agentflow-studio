// Formatting helpers, ported from the desktop Studio (internal/ui/screen.go
// and widgets.go) so both apps print numbers, durations and ids the same way.
import { t } from '@/i18n'
import { inDesktopShell, saveWithDialog } from './desktop'
import { toast } from '@/state/ui'

export function humanCount(n: number): string {
  const neg = n < 0
  if (neg) n = -n
  let s: string
  if (n >= 1_000_000_000) s = (n / 1e9).toFixed(1) + 'B'
  else if (n >= 1_000_000) s = (n / 1e6).toFixed(1) + 'M'
  else if (n >= 10_000) s = (n / 1e3).toFixed(1) + 'k'
  else s = String(Math.trunc(n))
  s = s.replace('.0', '')
  return neg ? '-' + s : s
}

export function humanDuration(seconds: number): string {
  seconds = Math.max(0, Math.trunc(seconds))
  const m = Math.floor(seconds / 60)
  const h = Math.floor(seconds / 3600)
  if (seconds < 60) return t('common.durSeconds', { s: seconds })
  if (seconds < 3600) return t('common.durMinutes', { m })
  if (seconds < 48 * 3600) return t('common.durHours', { h, m: m % 60 })
  return t('common.durDays', { d: Math.floor(h / 24), h: h % 24 })
}

function pad(n: number): string {
  return n < 10 ? '0' + n : String(n)
}

export function localDate(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function localDateTime(d: Date): string {
  return `${localDate(d)} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function clockTime(d: Date): string {
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** A unix timestamp (seconds) as "3m ago", or a date beyond a week. */
export function relTime(unix: number): string {
  if (!unix || unix <= 0) return '—'
  const when = new Date(unix * 1000)
  const d = (Date.now() - when.getTime()) / 1000
  if (d < 0) return localDateTime(when)
  if (d < 60) return t('common.justNow')
  if (d < 3600) return t('common.minutesAgo', { n: Math.floor(d / 60) })
  if (d < 86400) return t('common.hoursAgo', { n: Math.floor(d / 3600) })
  if (d < 7 * 86400) return t('common.daysAgo', { n: Math.floor(d / 86400) })
  return localDate(when)
}

export function orDash(s: string | null | undefined): string {
  return s && s.trim() !== '' ? s : '—'
}

/**
 * The tail of a path: its last `keep` segments behind an ellipsis, so a long
 * absolute path still shows the part that tells files apart.
 */
export function pathTail(path: string, keep = 2): string {
  const parts = path.split('/').filter(Boolean)
  if (parts.length <= keep + 1) return path
  return '…/' + parts.slice(-keep).join('/')
}

export function shortID(s: string | null | undefined): string {
  if (!s) return ''
  return s.length <= 8 ? s : s.slice(0, 8)
}

/** Splits a comma- or newline-separated list, trimming and dropping blanks. */
export function splitList(s: string): string[] {
  return s
    .split(/[,\n]/)
    .map((p) => p.trim())
    .filter(Boolean)
}

export function newThreadID(): string {
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

/** Today's date in UTC as YYYY-MM-DD (the usage ledger's day). */
export function utcDay(d = new Date()): string {
  return d.toISOString().slice(0, 10)
}

export function initial(name: string | null | undefined): string {
  return name && name.length > 0 ? name[0].toUpperCase() : '?'
}

/**
 * Saves text as a file: a download in a browser, a save dialog in the desktop
 * app (whose webview would otherwise drop the file in the home folder unasked).
 * Resolves to the file's name once it is saved, or null when the operator
 * cancelled the dialog or saving failed (a failure is reported here).
 */
export async function downloadText(filename: string, text: string, type = 'application/json'): Promise<string | null> {
  if (!(await inDesktopShell())) {
    browserDownload(filename, text, type)
    return filename
  }
  try {
    const r = await saveWithDialog(filename, text)
    if (r.ok) return r.name
    if (!r.cancelled) toast(t('common.saveFailed', { message: r.error ?? '' }), 'danger')
  } catch (err) {
    toast(t('common.saveFailed', { message: err instanceof Error ? err.message : String(err) }), 'danger')
  }
  return null
}

function browserDownload(filename: string, text: string, type: string): void {
  const url = URL.createObjectURL(new Blob([text], { type }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    // Clipboard API needs a secure context; fall back for plain-http LAN hosts.
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    let ok = false
    try {
      ok = document.execCommand('copy')
    } catch {
      ok = false
    }
    ta.remove()
    return ok
  }
}
