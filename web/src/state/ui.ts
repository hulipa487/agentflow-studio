// Shell-level UI state: toasts, modal prompts, the command palette, the rail,
// and the keyboard actions the current screen has registered.
import { create } from 'zustand'
import type { ReactNode } from 'react'

export type Tone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger'

export interface Toast {
  id: number
  text: string
  tone: Tone
}

export interface ConfirmOptions {
  title: string
  message: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
  /** When set, the confirm button stays disabled until this exact text is typed. */
  typeToConfirm?: string
}

export interface PromptOptions {
  title: string
  subtitle?: string
  label: string
  placeholder?: string
  initial?: string
  okLabel?: string
  multiline?: boolean
  /** Returns an error message, or null/undefined when the value is acceptable. */
  validate?: (value: string) => string | null | undefined
  /** Suggestions offered as the operator types (a datalist). */
  options?: string[]
}

export interface InfoOptions {
  title: string
  message: ReactNode
}

type Dialog =
  | { kind: 'confirm'; opts: ConfirmOptions; resolve: (ok: boolean) => void }
  | { kind: 'prompt'; opts: PromptOptions; resolve: (value: string | null) => void }
  | { kind: 'info'; opts: InfoOptions; resolve: () => void }

/** Keyboard actions a screen can claim while it is mounted. */
export interface ScreenActions {
  /** Ctrl/Cmd+S */
  save?: () => void
  /** Ctrl/Cmd+Enter */
  send?: () => void
  /** Esc (when no dialog or field is in the way) */
  back?: () => void
  /** Alt+R and the shell's refresh command */
  refresh?: () => void
}

interface UIState {
  toasts: Toast[]
  dialog: Dialog | null
  paletteOpen: boolean
  newAgentOpen: boolean
  railCollapsed: boolean
  actions: ScreenActions
  setPalette: (open: boolean) => void
  setNewAgent: (open: boolean) => void
  toggleRail: () => void
  dismissToast: (id: number) => void
  closeDialog: () => void
}

let nextToast = 1

const RAIL_KEY = 'agentflow-studio:rail-collapsed'

export const useUI = create<UIState>((set) => ({
  toasts: [],
  dialog: null,
  paletteOpen: false,
  newAgentOpen: false,
  railCollapsed: (() => {
    try {
      return localStorage.getItem(RAIL_KEY) === '1'
    } catch {
      return false
    }
  })(),
  actions: {},
  setPalette: (paletteOpen) => set({ paletteOpen }),
  setNewAgent: (newAgentOpen) => set({ newAgentOpen }),
  toggleRail: () =>
    set((s) => {
      try {
        localStorage.setItem(RAIL_KEY, s.railCollapsed ? '0' : '1')
      } catch {
        /* not persisted */
      }
      return { railCollapsed: !s.railCollapsed }
    }),
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  closeDialog: () => set({ dialog: null }),
}))

/** Shows a transient status message in the corner. */
export function toast(text: string, tone: Tone = 'neutral'): void {
  if (!text) return
  const id = nextToast++
  useUI.setState((s) => ({ toasts: [...s.toasts.slice(-3), { id, text, tone }] }))
  setTimeout(() => useUI.getState().dismissToast(id), tone === 'danger' ? 6000 : 2600)
}

export function toastError(err: unknown): void {
  toast('Error: ' + (err instanceof Error ? err.message : String(err)), 'danger')
}

/** Asks a yes/no question in a modal. Resolves false on cancel or Esc. */
export function confirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    useUI.setState({
      dialog: {
        kind: 'confirm',
        opts,
        resolve: (ok) => {
          useUI.setState({ dialog: null })
          resolve(ok)
        },
      },
    })
  })
}

/** Asks for one value in a modal. Resolves null on cancel. */
export function prompt(opts: PromptOptions): Promise<string | null> {
  return new Promise((resolve) => {
    useUI.setState({
      dialog: {
        kind: 'prompt',
        opts,
        resolve: (value) => {
          useUI.setState({ dialog: null })
          resolve(value)
        },
      },
    })
  })
}

/** Shows a message with a single Done button. */
export function info(opts: InfoOptions): Promise<void> {
  return new Promise((resolve) => {
    useUI.setState({
      dialog: {
        kind: 'info',
        opts,
        resolve: () => {
          useUI.setState({ dialog: null })
          resolve()
        },
      },
    })
  })
}

export function requireNonEmpty(what: string) {
  return (v: string) => (v.trim() === '' ? `${what} is required` : null)
}
