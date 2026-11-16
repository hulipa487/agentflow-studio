// Formatting helpers, ported from the desktop Studio (internal/ui/screen.go
// and widgets.go) so both apps print numbers, durations and ids the same way.

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
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${m}m`
  if (seconds < 48 * 3600) return `${h}h ${m % 60}m`
  return `${Math.floor(h / 24)}d ${h % 24}h`
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
  const t = new Date(unix * 1000)
  const d = (Date.now() - t.getTime()) / 1000
  if (d < 0) return localDateTime(t)
  if (d < 60) return 'just now'
  if (d < 3600) return `${Math.floor(d / 60)}m ago`
  if (d < 86400) return `${Math.floor(d / 3600)}h ago`
  if (d < 7 * 86400) return `${Math.floor(d / 86400)}d ago`
  return localDate(t)
}

export function plural(n: number, noun: string): string {
  return n === 1 ? `1 ${noun}` : `${n} ${noun}s`
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

/** Saves text as a file from the browser. */
export function downloadText(filename: string, text: string, type = 'application/json'): void {
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
