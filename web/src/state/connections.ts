// Saved connection profiles. A profile is an address, how to reach it, and
// optionally its token. In the desktop app, remembered tokens are moved into the
// OS keychain (Windows Credential Manager / macOS Keychain / Secret Service).
//
// Where the token lives:
//   - "Remember" on in desktop (keychain available): OS keychain, kept out of
//     localStorage.
//   - "Remember" on in browser (or fallback): localStorage, in plain text.
//   - "Remember" off: sessionStorage only, so a reload keeps you signed in but
//     closing the tab forgets it.
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { normalizeURL, transportOf } from '@/api/client'
import { deleteDesktopSecret, getDesktopSecret, getSecretsStatus, setDesktopSecret } from '@/lib/desktop'

export interface Profile {
  name: string
  url: string
  token?: string
  /** True when the remembered token is stored in the desktop OS keychain instead of localStorage. */
  tokenInKeychain?: boolean
  forgetToken?: boolean
  viaProxy: boolean
}

interface ConnectionsState {
  profiles: Profile[]
  active: string
  /** Saves (or updates) the profile for url and makes it active. Returns its name. */
  remember: (url: string, token: string, remember: boolean, viaProxy: boolean) => string
  select: (name: string) => Profile | undefined
  /** Drops the active profile's stored token (Lock). */
  forgetActiveToken: () => void
  remove: (name: string) => void
  clearActive: () => void
}

const SESSION_KEY = 'agentflow-studio:session-tokens'
const keychainTokens = new Map<string, string>()
let keychainReady = false

export function isKeychainEnabled(): boolean {
  return keychainReady
}

export function resetKeychainStateForTests(): void {
  keychainTokens.clear()
  keychainReady = false
}

function sessionTokens(): Record<string, string> {
  try {
    return JSON.parse(sessionStorage.getItem(SESSION_KEY) || '{}')
  } catch {
    return {}
  }
}

function setSessionToken(name: string, token: string) {
  try {
    const all = sessionTokens()
    if (token) all[name] = token
    else delete all[name]
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(all))
  } catch {
    /* storage unavailable: the token simply is not kept */
  }
}

/** The token to use for a profile: the remembered one (localStorage or OS keychain), else this tab's. */
export function tokenFor(p: Profile | undefined): string {
  if (!p) return ''
  return p.token || keychainTokens.get(p.name) || sessionTokens()[p.name] || ''
}

/**
 * Checks whether the desktop OS keychain is available, loads remembered tokens
 * into memory, and migrates any legacy plaintext tokens out of localStorage.
 */
export async function hydrateDesktopTokens(): Promise<void> {
  const status = await getSecretsStatus()
  if (!status.available) {
    keychainReady = false
    return
  }
  keychainReady = true
  const current = useConnections.getState().profiles
  if (current.length === 0) return

  const snapshot = new Map(current.map((p) => [p.name, p]))
  let changed = false
  const next = await Promise.all(
    current.map(async (p) => {
      if (p.token && !p.forgetToken) {
        const ok = await setDesktopSecret(p.name, p.token)
        if (ok) {
          keychainTokens.set(p.name, p.token)
          changed = true
          return { name: p.name, url: p.url, viaProxy: p.viaProxy, forgetToken: p.forgetToken, tokenInKeychain: true }
        }
        return p
      }
      if (p.tokenInKeychain) {
        if (keychainTokens.has(p.name)) return p
        const secret = await getDesktopSecret(p.name)
        if (secret) {
          keychainTokens.set(p.name, secret)
          return p
        }
        changed = true
        return { name: p.name, url: p.url, viaProxy: p.viaProxy, forgetToken: p.forgetToken }
      }
      return p
    }),
  )

  if (changed) {
    const updatedByName = new Map(next.map((p) => [p.name, p]))
    useConnections.setState((s) => ({
      profiles: s.profiles.map((p) => (p === snapshot.get(p.name) ? (updatedByName.get(p.name) ?? p) : p)),
    }))
  }
}

export function profileName(rawURL: string): string {
  const url = normalizeURL(rawURL)
  const kind = transportOf(url)
  if (kind === 'unix') return url.slice('unix://'.length).split('/').filter(Boolean).pop() || 'socket'
  if (kind === 'npipe') return url.replace(/^n?pipe:\/\//, '').replace(/^\\\\\.\\pipe\\/, '') || 'pipe'
  try {
    return new URL(url).host || 'default'
  } catch {
    return 'default'
  }
}

function uniqueName(profiles: Profile[], rawURL: string): string {
  const taken = (n: string) => profiles.some((p) => p.name === n)
  const base = profileName(rawURL)
  if (!taken(base)) return base
  try {
    const u = new URL(normalizeURL(rawURL))
    const path = u.pathname.replace(/^\/+|\/+$/g, '')
    if (u.host && path && !taken(`${u.host}/${path}`)) return `${u.host}/${path}`
  } catch {
    /* not a URL: fall through to numbering */
  }
  for (let i = 2; ; i++) {
    const n = `${base} (${i})`
    if (!taken(n)) return n
  }
}

export const useConnections = create<ConnectionsState>()(
  persist(
    (set, get) => ({
      profiles: [],
      active: '',
      remember: (rawURL, token, remember, viaProxy) => {
        const url = normalizeURL(rawURL)
        const profiles = [...get().profiles]
        const i = profiles.findIndex((p) => p.url === url)
        const prev = i >= 0 ? profiles[i] : undefined
        const name = prev ? prev.name : uniqueName(profiles, url)

        if (remember && keychainReady) {
          keychainTokens.set(name, token)
          setSessionToken(name, '')
          const next: Profile = { name, url, viaProxy, forgetToken: false, tokenInKeychain: true }
          if (i >= 0) profiles[i] = next
          else profiles.push(next)
          set({ profiles, active: name })
          void setDesktopSecret(name, token).then((ok) => {
            if (ok || keychainTokens.get(name) !== token) return
            keychainTokens.delete(name)
            const cur = get().profiles
            if (!cur.some((p) => p.name === name && p.tokenInKeychain && !p.forgetToken)) return
            set({
              profiles: cur.map((p) =>
                p.name === name ? { name: p.name, url: p.url, viaProxy: p.viaProxy, forgetToken: false, token } : p,
              ),
            })
          })
          return name
        }

        if (!remember) {
          keychainTokens.delete(name)
          if (keychainReady || prev?.tokenInKeychain) {
            void deleteDesktopSecret(name)
          }
        } else {
          // Storing in localStorage directly instead of keychain.
          keychainTokens.delete(name)
          if (prev?.tokenInKeychain) {
            void deleteDesktopSecret(name)
          }
        }

        const next: Profile = { name, url, viaProxy, forgetToken: !remember, token: remember ? token : undefined }
        if (i >= 0) profiles[i] = next
        else profiles.push(next)
        setSessionToken(name, remember ? '' : token)
        set({ profiles, active: name })
        return name
      },
      select: (name) => {
        const p = get().profiles.find((x) => x.name === name)
        if (p) set({ active: name })
        return p
      },
      forgetActiveToken: () => {
        const { active, profiles } = get()
        const target = profiles.find((p) => p.name === active)
        keychainTokens.delete(active)
        if (active && (keychainReady || target?.tokenInKeychain)) {
          void deleteDesktopSecret(active)
        }
        setSessionToken(active, '')
        set({
          profiles: profiles.map((p) =>
            p.name === active ? { ...p, token: undefined, tokenInKeychain: undefined } : p,
          ),
        })
      },
      remove: (name) => {
        const target = get().profiles.find((p) => p.name === name)
        keychainTokens.delete(name)
        if (keychainReady || target?.tokenInKeychain) {
          void deleteDesktopSecret(name)
        }
        setSessionToken(name, '')
        const profiles = get().profiles.filter((p) => p.name !== name)
        set({ profiles, active: get().active === name ? '' : get().active })
      },
      clearActive: () => set({ active: '' }),
    }),
    { name: 'agentflow-studio:connections' },
  ),
)

/**
 * Adds connections saved by the old desktop app, on a first run with nothing
 * saved here. They always go through the local proxy, the only route the
 * desktop shell has. Returns how many were added.
 */
export function importProfiles(saved: { name?: string; url: string; token?: string; forget_token?: boolean }[], active?: string): number {
  if (useConnections.getState().profiles.length > 0) return 0
  const profiles: Profile[] = []
  for (const s of saved) {
    const url = normalizeURL(s.url)
    if (!url || profiles.some((p) => p.url === url)) continue
    const taken = s.name && !profiles.some((p) => p.name === s.name)
    const keep = !s.forget_token && !!s.token
    profiles.push({ name: taken ? s.name! : uniqueName(profiles, url), url, viaProxy: true, forgetToken: !keep, token: keep ? s.token : undefined })
  }
  if (profiles.length === 0) return 0
  useConnections.setState({ profiles, active: profiles.find((p) => p.name === active)?.name ?? '' })
  return profiles.length
}

export function activeProfile(): Profile | undefined {
  const { profiles, active } = useConnections.getState()
  return profiles.find((p) => p.name === active)
}
