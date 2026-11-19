// Light/dark mode and palette. Saved in localStorage; "system" follows the OS
// until the operator picks a side, like the desktop app's first launch.
import { useEffect, useState } from 'react'
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type Mode = 'system' | 'light' | 'dark'
export type Palette = 'trans' | 'obsidian'

// The titles are proper names and stay as they are in every language; the
// descriptions are catalogue keys.
export const PALETTES: { id: Palette; title: string; descriptionKey: string }[] = [
  { id: 'trans', title: 'Trans Pride', descriptionKey: 'settings.paletteTransDescription' },
  { id: 'obsidian', title: 'Obsidian', descriptionKey: 'settings.paletteObsidianDescription' },
]

interface AppearanceState {
  mode: Mode
  palette: Palette
  setMode: (m: Mode) => void
  setPalette: (p: Palette) => void
  toggle: () => void
}

const media = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: light)') : null

export function resolveMode(mode: Mode): 'light' | 'dark' {
  if (mode === 'light' || mode === 'dark') return mode
  return media?.matches ? 'light' : 'dark'
}

function apply(mode: Mode, palette: Palette) {
  const root = document.documentElement
  root.dataset.theme = resolveMode(mode)
  root.dataset.palette = palette
}

export const useAppearance = create<AppearanceState>()(
  persist(
    (set, get) => ({
      mode: 'system',
      palette: 'trans',
      setMode: (mode) => {
        set({ mode })
        apply(mode, get().palette)
      },
      setPalette: (palette) => {
        set({ palette })
        apply(get().mode, palette)
      },
      toggle: () => {
        const next = resolveMode(get().mode) === 'light' ? 'dark' : 'light'
        set({ mode: next })
        apply(next, get().palette)
      },
    }),
    {
      name: 'agentflow-studio:appearance',
      partialize: (s) => ({ mode: s.mode, palette: s.palette }),
      onRehydrateStorage: () => (state) => {
        if (state) apply(state.mode, state.palette)
      },
    },
  ),
)

media?.addEventListener('change', () => {
  const { mode, palette } = useAppearance.getState()
  if (mode === 'system') apply(mode, palette)
})

/** The resolved light/dark value, re-rendering when it changes (the OS flipping included). */
export function useResolvedMode(): 'light' | 'dark' {
  const mode = useAppearance((s) => s.mode)
  const [, bump] = useState(0)
  useEffect(() => {
    if (mode !== 'system' || !media) return
    const onChange = () => bump((n) => n + 1)
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [mode])
  return resolveMode(mode)
}
