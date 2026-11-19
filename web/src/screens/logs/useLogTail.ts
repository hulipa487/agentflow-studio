// Keeps the log stream open for as long as the Logs screen is mounted:
// connect, replay, follow, and reconnect with the desktop app's backoff
// (1s doubling to 16s) when the stream ends.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { errorMessage, type Client } from '@/api/client'
import type { Params } from '@/i18n'
import type { Tone } from '@/state/ui'
import { LogBuffer } from './buffer'

export interface TailStatus {
  /** A message key, read when drawn so a language change re-reads it. */
  key: string
  params?: Params
  tone: Tone
  /** True while lines can arrive. */
  live: boolean
}

const BACKOFF_START = 1000
const BACKOFF_CEILING = 15000
/** A stream that held this long was healthy; its end is not part of a failing streak. */
const HEALTHY_AFTER = 30000

/**
 * client.logs() with one addition: `open` fires when the runtime has accepted
 * the stream, before the first line. Without it "connected" could only be
 * inferred from a line arriving, and a quiet runtime (-log-level error) may
 * have none to replay.
 */
async function streamLogs(client: Client, on: { open: () => void; line: (line: string) => void }, signal: AbortSignal): Promise<void> {
  await client.logs(on.line, signal, on.open)
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const done = () => {
      clearTimeout(timer)
      signal.removeEventListener('abort', done)
      resolve()
    }
    const timer = setTimeout(done, ms)
    signal.addEventListener('abort', done)
  })
}

export function useLogTail(client: Client) {
  const buffer = useMemo(() => new LogBuffer(), [client])
  // The buffer is mutable; this counter is what tells React it changed.
  const [version, setVersion] = useState(0)
  const [status, setStatus] = useState<TailStatus>({ key: 'logs.connecting', tone: 'neutral', live: false })
  /** Why the last attempt failed; cleared once a stream opens. */
  const [error, setError] = useState<unknown>(undefined)
  const [generation, setGeneration] = useState(0)
  const frame = useRef(0)

  const refresh = useCallback(() => setVersion((v) => v + 1), [])

  useEffect(() => {
    const ctl = new AbortController()
    const { signal } = ctl

    // One render per frame however many lines arrived in it.
    const schedule = () => {
      if (frame.current) return
      frame.current = requestAnimationFrame(() => {
        frame.current = 0
        if (buffer.commit()) refresh()
      })
    }

    const run = async () => {
      let backoff = BACKOFF_START
      while (!signal.aborted) {
        setStatus({ key: 'logs.connecting', tone: 'neutral', live: false })
        let openedAt = 0
        let failure: unknown
        try {
          await streamLogs(
            client,
            {
              open: () => {
                if (signal.aborted) return
                openedAt = Date.now()
                // Every (re)connect replays the runtime's ring, so start from
                // it rather than appending a second copy of the history.
                buffer.reset()
                refresh()
                setError(undefined)
                setStatus({ key: 'logs.streaming', params: { url: client.displayURL }, tone: 'success', live: true })
              },
              line: (line) => {
                if (signal.aborted) return
                buffer.push(line)
                schedule()
              },
            },
            signal,
          )
        } catch (err) {
          failure = err
        }
        if (signal.aborted) return
        if (openedAt > 0 && Date.now() - openedAt >= HEALTHY_AFTER) backoff = BACKOFF_START
        if (failure !== undefined) setError(failure)
        const seconds = backoff / 1000
        setStatus({
          key: failure !== undefined ? 'logs.reconnecting' : 'logs.endedReconnecting',
          params: failure !== undefined ? { message: errorMessage(failure), seconds } : { seconds },
          tone: failure !== undefined ? 'danger' : 'warning',
          live: false,
        })
        await sleep(backoff, signal)
        if (backoff < BACKOFF_CEILING) backoff *= 2
      }
    }
    void run()

    return () => {
      ctl.abort()
      cancelAnimationFrame(frame.current)
      frame.current = 0
    }
  }, [client, buffer, generation, refresh])

  /** Drops what is on screen and reconnects (the screen's refresh action). */
  const restart = useCallback(() => {
    buffer.clear()
    refresh()
    setGeneration((g) => g + 1)
  }, [buffer, refresh])

  return { buffer, version, status, error, refresh, restart }
}
