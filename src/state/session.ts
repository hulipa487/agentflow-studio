// The live connection: the API client, the builder catalog, and the
// heartbeat's view of the runtime. Nothing here is persisted; see
// connections.ts for what is saved.
import { create } from 'zustand'
import { Client, errorMessage } from '@/api/client'
import type { Catalog, State } from '@/api/types'
import { activeProfile, tokenFor, useConnections } from './connections'
import { toast } from './ui'

const EMPTY_CATALOG: Catalog = {
  models: [],
  tools: [],
  capabilities: [],
  default_capabilities: [],
  safety_profiles: [],
  memory_profiles: [],
  prompt_keys: [],
  prelude_version: 1,
  templates: [],
}

export interface ConnectStep {
  (text: string): void
}

interface SessionState {
  client: Client | null
  catalog: Catalog
  /** The heartbeat's last verdict. */
  online: boolean
  /** The latest /admin/api/state snapshot from the heartbeat. */
  runtime: State | null
  /** True while a remembered connection is being restored on load. */
  restoring: boolean
  /** Why the last automatic reconnect failed; the welcome screen shows it once. */
  restoreError: string

  connect: (
    url: string,
    token: string,
    opts: { remember: boolean; viaProxy: boolean; signal?: AbortSignal; onStep?: ConnectStep },
  ) => Promise<void>
  /** Reconnects with the active saved profile, if it has a token. */
  restore: () => Promise<boolean>
  /** Forgets the token and returns to the welcome screen. */
  lock: () => void
  /** Returns to the welcome screen keeping every saved profile. */
  disconnect: () => void
  reloadCatalog: () => Promise<void>
}

let heartbeat: ReturnType<typeof setInterval> | null = null

function stopHeartbeat() {
  if (heartbeat) clearInterval(heartbeat)
  heartbeat = null
}

function startHeartbeat() {
  stopHeartbeat()
  const beat = async () => {
    const { client, online } = useSession.getState()
    if (!client) return
    const ctl = new AbortController()
    const timer = setTimeout(() => ctl.abort(), 8000)
    try {
      const runtime = await client.state(ctl.signal)
      if (useSession.getState().client !== client) return
      useSession.setState({ online: true, runtime })
      if (!online) toast('runtime reachable again', 'success')
    } catch (err) {
      if (useSession.getState().client !== client) return
      useSession.setState({ online: false })
      if (online) toast('runtime unreachable: ' + errorMessage(err), 'danger')
    } finally {
      clearTimeout(timer)
    }
  }
  heartbeat = setInterval(beat, 15000)
}

export const useSession = create<SessionState>((set, get) => ({
  client: null,
  catalog: EMPTY_CATALOG,
  online: false,
  runtime: null,
  restoring: false,
  restoreError: '',

  connect: async (url, token, { remember, viaProxy, signal, onStep }) => {
    const client = new Client({ url, token, viaProxy })
    onStep?.(`Contacting ${client.displayURL}…`)
    // The state call is the ping: it proves the address and the token.
    const runtime = await client.state(signal)
    onStep?.('Loading the catalog…')
    const catalog = await client.catalog(signal)
    // Only a connection that worked is saved, so a wrong URL or token never
    // becomes next launch's default.
    useConnections.getState().remember(client.displayURL, client.token, remember, client.viaProxy)
    set({ client, catalog: { ...EMPTY_CATALOG, ...catalog }, online: true, runtime })
    startHeartbeat()
  },

  restore: async () => {
    const p = activeProfile()
    const token = tokenFor(p)
    if (!p || !token) return false
    set({ restoring: true, restoreError: '' })
    try {
      await get().connect(p.url, token, { remember: !p.forgetToken, viaProxy: p.viaProxy })
      return true
    } catch (err) {
      // Falling back to the welcome screen without a word would look like the
      // saved connection had been lost.
      set({ restoreError: `Could not reconnect to ${p.name}: ${errorMessage(err)}` })
      return false
    } finally {
      set({ restoring: false })
    }
  },

  lock: () => {
    stopHeartbeat()
    useConnections.getState().forgetActiveToken()
    set({ client: null, online: false, runtime: null })
  },

  disconnect: () => {
    stopHeartbeat()
    set({ client: null, online: false, runtime: null })
  },

  reloadCatalog: async () => {
    const client = get().client
    if (!client) return
    try {
      const catalog = await client.catalog()
      if (get().client === client) set({ catalog: { ...EMPTY_CATALOG, ...catalog } })
    } catch {
      /* the next builder open retries */
    }
  },
}))

/** The connected client. Screens render only while connected, so this is non-null there. */
export function useClient(): Client {
  const client = useSession((s) => s.client)
  if (!client) throw new Error('not connected')
  return client
}

export function useCatalog(): Catalog {
  return useSession((s) => s.catalog)
}
