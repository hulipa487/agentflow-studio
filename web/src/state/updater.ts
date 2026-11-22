// Desktop app update state: tracks GitHub release checks, user auto-check
// preference, and dismissed update badges across views (Shell + Settings).
import { create } from 'zustand'
import {
  applyDesktopUpdate,
  checkForUpdates,
  getUpdateStatus,
  type UpdateStatus,
} from '@/lib/desktop'

const AUTO_KEY = 'agentflow-studio:auto-update'
const DISMISS_KEY = 'agentflow-studio:dismissed-update'

function readAuto(): boolean {
  try {
    return localStorage.getItem(AUTO_KEY) !== '0'
  } catch {
    return true
  }
}

function readDismissed(): string {
  try {
    return localStorage.getItem(DISMISS_KEY) ?? ''
  } catch {
    return ''
  }
}

interface UpdaterState {
  status: UpdateStatus | null
  checking: boolean
  applying: boolean
  error: string
  autoCheck: boolean
  dismissedVersion: string
  loadStatus: () => Promise<void>
  check: (silent?: boolean) => Promise<UpdateStatus | null>
  apply: () => Promise<boolean>
  setAutoCheck: (v: boolean) => void
  dismiss: (ver: string) => void
}

export const useUpdater = create<UpdaterState>((set, get) => ({
  status: null,
  checking: false,
  applying: false,
  error: '',
  autoCheck: readAuto(),
  dismissedVersion: readDismissed(),

  loadStatus: async () => {
    try {
      const status = await getUpdateStatus()
      set({ status, error: status.error ?? '' })
    } catch {
      /* not in desktop shell */
    }
  },

  check: async (silent = false) => {
    if (get().checking || get().applying) return get().status
    set({ checking: true, ...(silent ? {} : { error: '' }) })
    try {
      const status = await checkForUpdates()
      set({ status, error: status.error ?? '', checking: false })
      return status
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ checking: false, ...(silent ? {} : { error: msg }) })
      return null
    }
  },

  apply: async () => {
    if (get().applying) return false
    set({ applying: true, error: '' })
    try {
      await applyDesktopUpdate()
      return true
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ applying: false, error: msg })
      return false
    }
  },

  setAutoCheck: (v) => {
    try {
      localStorage.setItem(AUTO_KEY, v ? '1' : '0')
    } catch {
      /* ignore storage errors */
    }
    set({ autoCheck: v })
  },

  dismiss: (ver) => {
    try {
      localStorage.setItem(DISMISS_KEY, ver)
    } catch {
      /* ignore storage errors */
    }
    set({ dismissedVersion: ver })
  },
}))
