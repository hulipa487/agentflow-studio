// Desktop app update state: tracks GitHub release checks, user auto-check /
// auto-install preferences, and dismissed update badges across views (Shell + Settings).
import { create } from 'zustand'
import { t } from '@/i18n'
import {
  applyDesktopUpdate,
  checkForUpdates,
  getUpdateStatus,
  type UpdateStatus,
} from '@/lib/desktop'
import { toast } from '@/state/ui'

const AUTO_KEY = 'agentflow-studio:auto-update'
const AUTO_INSTALL_KEY = 'agentflow-studio:auto-install'
const DISMISS_KEY = 'agentflow-studio:dismissed-update'
export const AUTO_CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000

function readAuto(): boolean {
  try {
    return localStorage.getItem(AUTO_KEY) !== '0'
  } catch {
    return true
  }
}

function readAutoInstall(): boolean {
  try {
    return localStorage.getItem(AUTO_INSTALL_KEY) === '1'
  } catch {
    return false
  }
}

function writeStorage(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* ignore storage errors */
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
  autoInstall: boolean
  autoAttemptedVersion: string
  dismissedVersion: string
  loadStatus: () => Promise<void>
  check: (silent?: boolean) => Promise<UpdateStatus | null>
  apply: () => Promise<boolean>
  setAutoCheck: (v: boolean) => void
  setAutoInstall: (v: boolean) => void
  dismiss: (ver: string) => void
}

export const useUpdater = create<UpdaterState>((set, get) => {
  const maybeAutoInstall = (status: UpdateStatus | null): void => {
    if (!status) return
    const { autoInstall, autoAttemptedVersion, applying } = get()
    if (
      !autoInstall ||
      !status.update_available ||
      !status.can_self_update ||
      status.is_dev ||
      !status.latest_version ||
      applying ||
      autoAttemptedVersion === status.latest_version
    ) {
      return
    }
    set({ autoAttemptedVersion: status.latest_version })
    toast(t('settings.updateAutoInstalling', { version: status.latest_version }), 'accent')
    void get().apply()
  }

  return {
    status: null,
    checking: false,
    applying: false,
    error: '',
    autoCheck: readAuto(),
    autoInstall: readAutoInstall(),
    autoAttemptedVersion: '',
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
        maybeAutoInstall(status)
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
      writeStorage(AUTO_KEY, v ? '1' : '0')
      if (!v && get().autoInstall) {
        writeStorage(AUTO_INSTALL_KEY, '0')
        set({ autoCheck: false, autoInstall: false })
        return
      }
      set({ autoCheck: v })
    },

    setAutoInstall: (v) => {
      writeStorage(AUTO_INSTALL_KEY, v ? '1' : '0')
      if (v && !get().autoCheck) {
        writeStorage(AUTO_KEY, '1')
        set({ autoInstall: true, autoCheck: true })
      } else {
        set({ autoInstall: v })
      }
      if (!v) return
      const cur = get().status
      if (cur?.update_available && cur.can_self_update && !cur.is_dev) {
        maybeAutoInstall(cur)
      } else if (!cur?.checked_at) {
        void get().check(true)
      }
    },

    dismiss: (ver) => {
      writeStorage(DISMISS_KEY, ver)
      set({ dismissedVersion: ver })
    },
  }
})

let autoLoopTimer: ReturnType<typeof setInterval> | null = null

export function startAutoUpdateLoop(intervalMs = AUTO_CHECK_INTERVAL_MS): void {
  const st = useUpdater.getState()
  if (st.autoCheck) {
    void st.check(true)
  }
  if (autoLoopTimer !== null) return
  autoLoopTimer = setInterval(() => {
    const cur = useUpdater.getState()
    if (cur.autoCheck) {
      void cur.check(true)
    }
  }, intervalMs)
}

export function stopAutoUpdateLoop(): void {
  if (autoLoopTimer !== null) {
    clearInterval(autoLoopTimer)
    autoLoopTimer = null
  }
}
