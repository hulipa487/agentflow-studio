// Saved connection profiles, the browser counterpart of the desktop app's
// config.json (internal/appconfig). A profile is an address, how to reach it,
// and optionally its token.
//
// Where the token lives:
//   - "Remember" on:  localStorage, in plain text, like the desktop app's
//     config file. Anyone with this browser profile can read it.
//   - "Remember" off: sessionStorage only, so a reload keeps you signed in but
//     closing the tab forgets it.
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { normalizeURL, transportOf } from '@/api/client'

export interface Profile {
  name: string
  url: string
  token?: string
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

/** The token to use for a profile: the remembered one, else this tab's. */
export function tokenFor(p: Profile | undefined): string {
  if (!p) return ''
  return p.token || sessionTokens()[p.name] || ''
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
        const name = i >= 0 ? profiles[i].name : uniqueName(profiles, url)
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
        setSessionToken(active, '')
        set({ profiles: profiles.map((p) => (p.name === active ? { ...p, token: undefined } : p)) })
      },
      remove: (name) => {
        setSessionToken(name, '')
        const profiles = get().profiles.filter((p) => p.name !== name)
        set({ profiles, active: get().active === name ? '' : get().active })
      },
      clearActive: () => set({ active: '' }),
    }),
    { name: 'agentflow-studio:connections' },
  ),
)

export function activeProfile(): Profile | undefined {
  const { profiles, active } = useConnections.getState()
  return profiles.find((p) => p.name === active)
}
