// Global keyboard shortcuts. Ctrl+N and Ctrl+1…9 belong to the browser, so
// those are Alt+N and Alt+1…9, and F5 stays the browser's reload (Alt+R
// refreshes a screen). The desktop app, being the same page, uses the same.
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { modalOpen } from '@/components/ui'
import { NAV } from '@/nav'
import { useUI } from '@/state/ui'

function inEditable(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  return el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable
}

export function Shortcuts() {
  const navigate = useNavigate()
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const ui = useUI.getState()
      const mod = e.ctrlKey || e.metaKey
      const key = e.key.toLowerCase()

      if (mod && !e.altKey && (key === 'k' || (e.shiftKey && key === 'p'))) {
        e.preventDefault()
        ui.setPalette(!ui.paletteOpen)
        return
      }
      if (e.key === 'F1') {
        e.preventDefault()
        // A screen with its own reference (the builder's docs drawer) shows that instead.
        if (ui.actions.help && !modalOpen()) ui.actions.help()
        else navigate('/docs')
        return
      }
      if (mod && !e.altKey && !e.shiftKey && key === 's') {
        // Never let the browser's "Save page" dialog open over an editor.
        e.preventDefault()
        if (!modalOpen()) ui.actions.save?.()
        return
      }
      if (mod && e.key === 'Enter') {
        if (!modalOpen() && ui.actions.send) {
          e.preventDefault()
          ui.actions.send()
        }
        return
      }
      if (modalOpen()) return

      if (e.altKey && !mod && !e.shiftKey) {
        // e.code keeps working where Alt composes a different character.
        const digit = /^Digit([1-9])$/.exec(e.code)?.[1]
        if (digit) {
          const it = NAV.find((n) => n.digit === Number(digit))
          if (it) {
            e.preventDefault()
            navigate(it.path)
          }
          return
        }
        if (e.code === 'KeyN') {
          e.preventDefault()
          ui.setNewAgent(true)
          return
        }
        if (e.code === 'KeyR') {
          e.preventDefault()
          ui.actions.refresh?.()
          return
        }
      }
      if (e.key === '?' && !mod && !e.altKey && !inEditable(e.target) && !(e.target as HTMLElement | null)?.closest?.('.cm-editor')) {
        e.preventDefault()
        ui.setShortcuts(!ui.shortcutsOpen)
        return
      }
      if (e.key === 'Escape' && !inEditable(e.target) && !(e.target as HTMLElement | null)?.closest?.('.cm-editor')) {
        if (ui.actions.back) {
          e.preventDefault()
          ui.actions.back()
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate])
  return null
}
