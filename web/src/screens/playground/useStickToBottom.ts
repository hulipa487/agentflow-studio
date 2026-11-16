import { useCallback, useLayoutEffect, useRef } from 'react'

/**
 * Keeps a scroll container pinned to its end as content arrives, the way the
 * desktop app's ScrollToBottom does, but lets go once the reader scrolls up
 * and picks up again when they return to the end.
 *
 * `content` is whatever changes when rows are added; a new `scope` (another
 * thread, another filter) always starts pinned.
 */
export function useStickToBottom<E extends HTMLElement>(content: unknown, scope: unknown) {
  const ref = useRef<E>(null)
  const pinned = useRef(true)

  const onScroll = useCallback(() => {
    const el = ref.current
    if (el) pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
  }, [])

  /** Pins again regardless of where the reader is (their own message was just added). */
  const pin = useCallback(() => {
    pinned.current = true
  }, [])

  useLayoutEffect(() => {
    pinned.current = true
  }, [scope])

  useLayoutEffect(() => {
    const el = ref.current
    if (el && pinned.current) el.scrollTop = el.scrollHeight
  }, [content, scope])

  return { ref, onScroll, pin }
}
