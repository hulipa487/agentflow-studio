import { useEffect, useRef } from 'react'

/**
 * False once the component has unmounted, so a mutation that resolves after
 * the operator navigated away does not set state or raise a dialog.
 */
export function useAlive(): { readonly current: boolean } {
  const alive = useRef(true)
  useEffect(() => {
    // Set on mount as well: StrictMode runs the cleanup once before the real mount.
    alive.current = true
    return () => {
      alive.current = false
    }
  }, [])
  return alive
}
