import { useCallback, useEffect, useRef, useState } from 'react'
import { useBlocker } from 'react-router-dom'
import { confirm, useUI, type ConfirmOptions, type ScreenActions } from '@/state/ui'

export interface AsyncState<T> {
  data: T | undefined
  error: unknown
  /** True until the first result (or failure) arrives, and during a visible reload. */
  loading: boolean
  /** True while any request is in flight, including background refreshes. */
  refreshing: boolean
  /** Runs the call again. quiet keeps the current data on screen meanwhile. */
  reload: (quiet?: boolean) => void
  /** Replaces the data locally, e.g. with the list a mutation returned. */
  setData: (next: T | ((prev: T | undefined) => T)) => void
}

/**
 * Runs an async call and tracks its result: the pattern every screen uses to
 * load from the runtime. The call receives an AbortSignal that fires when the
 * component unmounts or a newer call supersedes this one, so navigating away
 * never applies a stale answer.
 *
 * With `interval` it polls quietly; a failed background refresh keeps the last
 * good data and only reports through `error`.
 */
export function useAsync<T>(
  call: (signal: AbortSignal) => Promise<T>,
  deps: unknown[],
  opts: { interval?: number; enabled?: boolean } = {},
): AsyncState<T> {
  const { interval, enabled = true } = opts
  const [data, setDataState] = useState<T | undefined>(undefined)
  const [error, setError] = useState<unknown>(undefined)
  const [loading, setLoading] = useState(enabled)
  const [refreshing, setRefreshing] = useState(false)
  const callRef = useRef(call)
  callRef.current = call
  const ctl = useRef<AbortController | null>(null)
  const hasData = useRef(false)

  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const run = useCallback((quiet = false) => {
    // A reload asked for after the screen is gone (a late mutation callback)
    // must not start a request whose answer has nowhere to go.
    if (!mounted.current) return
    ctl.current?.abort()
    const c = new AbortController()
    ctl.current = c
    if (!quiet) setLoading(true)
    setRefreshing(true)
    callRef
      .current(c.signal)
      .then((v) => {
        if (c.signal.aborted) return
        hasData.current = true
        setDataState(v)
        setError(undefined)
      })
      .catch((err) => {
        if (c.signal.aborted) return
        setError(err)
        if (!quiet) {
          hasData.current = false
          setDataState(undefined)
        }
      })
      .finally(() => {
        if (c.signal.aborted) return
        setLoading(false)
        setRefreshing(false)
      })
  }, [])

  useEffect(() => {
    if (!enabled) {
      setLoading(false)
      return
    }
    hasData.current = false
    run(false)
    let timer: ReturnType<typeof setInterval> | undefined
    if (interval && interval > 0) {
      timer = setInterval(() => {
        // Skip a tick while the tab is hidden: nobody is looking.
        if (document.visibilityState === 'visible') run(hasData.current)
      }, interval)
    }
    return () => {
      if (timer) clearInterval(timer)
      ctl.current?.abort()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, enabled, interval])

  const setData = useCallback((next: T | ((prev: T | undefined) => T)) => {
    hasData.current = true
    setError(undefined)
    setDataState((prev) => (typeof next === 'function' ? (next as (p: T | undefined) => T)(prev) : next))
  }, [])

  return { data, error, loading, refreshing, reload: run, setData }
}

/**
 * Claims the shell's keyboard actions for the mounted screen: Ctrl/Cmd+S
 * (save), Ctrl/Cmd+Enter (send), Esc (back) and Alt+R (refresh). The latest
 * closures are always used; they are released on unmount.
 */
export function useScreenActions(actions: ScreenActions): void {
  const ref = useRef(actions)
  ref.current = actions
  useEffect(() => {
    const proxy: ScreenActions = {
      save: () => ref.current.save?.(),
      send: () => ref.current.send?.(),
      back: () => ref.current.back?.(),
      refresh: () => ref.current.refresh?.(),
    }
    // Only advertise what the screen actually handles.
    const claimed: ScreenActions = {}
    if (ref.current.save) claimed.save = proxy.save
    if (ref.current.send) claimed.send = proxy.send
    if (ref.current.back) claimed.back = proxy.back
    if (ref.current.refresh) claimed.refresh = proxy.refresh
    useUI.setState({ actions: claimed })
    return () => {
      if (useUI.getState().actions === claimed) useUI.setState({ actions: {} })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [!!actions.save, !!actions.send, !!actions.back, !!actions.refresh])
}

/** Navigate with this state to pass a screen's unsaved-edits guard (its own save or delete flow). */
export const SKIP_GUARD = { skipGuard: true } as const

/**
 * Protects unsaved edits: the browser warns before the tab closes or reloads,
 * and any in-app navigation away (rail, palette, shortcuts, links, the Back
 * button) waits for a "discard?" confirmation. A screen's own navigation after
 * a save or delete passes `{ state: SKIP_GUARD }`.
 */
export function useUnsavedGuard(dirty: boolean, opts?: Partial<ConfirmOptions>): void {
  useEffect(() => {
    if (!dirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault()
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [dirty])

  const blocker = useBlocker(({ currentLocation, nextLocation }) => {
    if (!dirty) return false
    if ((nextLocation.state as { skipGuard?: boolean } | null)?.skipGuard) return false
    return currentLocation.pathname !== nextLocation.pathname || currentLocation.search !== nextLocation.search
  })
  const optsRef = useRef(opts)
  optsRef.current = opts
  useEffect(() => {
    if (blocker.state !== 'blocked') return
    let live = true
    void confirm({
      title: 'Discard changes?',
      message: 'There are unsaved edits on this screen. Leave without saving them?',
      confirmLabel: 'Discard',
      danger: true,
      ...optsRef.current,
    }).then((ok) => {
      if (!live) return
      if (ok) blocker.proceed()
      else blocker.reset()
    })
    return () => {
      live = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [blocker.state])
}

export function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return v
}

/** A value mirrored to localStorage (per-viewer conveniences only). */
export function useStored<T>(key: string, initial: T): [T, (v: T) => void] {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key)
      return raw === null ? initial : (JSON.parse(raw) as T)
    } catch {
      return initial
    }
  })
  const set = useCallback(
    (v: T) => {
      setValue(v)
      try {
        localStorage.setItem(key, JSON.stringify(v))
      } catch {
        /* not persisted */
      }
    },
    [key],
  )
  return [value, set]
}
