// One chat session with an agent: the open thread, its transcript and trace,
// and the turn in flight. The Playground and the builder's run panel both
// draw it; this hook owns the stream, the stop, and the rule that a late
// answer from an earlier turn, thread or agent is never applied.
import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, errorMessage, type Client } from '@/api/client'
import type { ApprovalPrompt, ChatEvent, MediaPart, Reply, Thread, TraceEvent } from '@/api/types'
import { t } from '@/i18n'
import { newThreadID, shortID } from '@/lib/format'
import { toastError } from '@/state/ui'
import { READY, appendEvent, joinReply, messageID, transcriptFromTrace, type Message, type Status } from './model'

type WireReply = Reply & { attachments?: MediaPart[] | null }

function replyNote(r: WireReply): string {
  const notes: string[] = []
  if (r.status && r.status !== 'delivered') notes.push(r.status)
  return notes.join(' · ')
}

export interface ChatSession {
  agent: string
  thread: string
  status: Status
  messages: Message[]
  /** null while nothing has been traced or loaded for this thread yet. */
  events: TraceEvent[] | null
  /** The open agent's recent threads, or null until they load. */
  threads: Thread[] | null
  sending: boolean
  stopping: boolean
  tracing: boolean
  /** Moves to another agent on a new thread. */
  switchAgent: (name: string) => void
  newThread: () => void
  openThread: (id: string) => Promise<void>
  refreshTrace: () => Promise<void>
  loadThreads: () => void
  stop: () => Promise<void>
  /** Starts a turn; false when one is already running or both text and attachments are blank. */
  send: (text: string, actAs?: string, attachments?: MediaPart[]) => boolean
  /** Approves or denies an inline runtime approval prompt. */
  decideApproval: (id: string, approve: boolean) => Promise<void>
  /** True while a turn is in flight, readable before the next render. */
  busy: () => boolean
}

export function useChatSession(client: Client, initialAgent: string, opts: { threads?: boolean } = {}): ChatSession {
  const listThreads = opts.threads ?? true
  const [agent, setAgent] = useState(initialAgent)
  const [thread, setThread] = useState(newThreadID)
  const [status, setStatus] = useState<Status>(READY)
  const [messages, setMessages] = useState<Message[]>([])
  const [events, setEvents] = useState<TraceEvent[] | null>(null)
  const [threads, setThreads] = useState<{
    agent: string
    rows: Thread[]
  } | null>(null)
  const [sending, setSending] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [tracing, setTracing] = useState(false)

  // What callbacks compare against: the session they started in.
  const session = useRef({ agent, thread })
  // Bumped whenever the session changes or a new turn starts, so a late answer
  // from an earlier turn, thread or agent is never applied.
  const epoch = useRef(0)
  const alive = useRef(true)
  // Mirrors `sending` for the Ctrl/Cmd+Enter path, which can fire again before a re-render.
  const inFlight = useRef(false)
  const chatCtl = useRef<AbortController | null>(null)
  const traceCtl = useRef<AbortController | null>(null)
  const threadsCtl = useRef<AbortController | null>(null)

  const loadThreadsFor = useCallback(
    (forAgent: string) => {
      if (!listThreads || forAgent === '') return
      threadsCtl.current?.abort()
      const c = new AbortController()
      threadsCtl.current = c
      client.threads(forAgent, c.signal).then(
        (rows) => {
          if (!c.signal.aborted) setThreads({ agent: forAgent, rows })
        },
        () => {
          // As on the desktop: a failed refresh keeps the list that is showing.
        },
      )
    },
    [client, listThreads],
  )
  const loadThreads = useCallback(() => loadThreadsFor(session.current.agent), [loadThreadsFor])

  /** Moves to another agent or thread: drops the running stream and clears chat and trace. */
  const enter = useCallback((nextAgent: string, nextThread: string, nextStatus: Status) => {
    epoch.current++
    chatCtl.current?.abort()
    chatCtl.current = null
    traceCtl.current?.abort()
    traceCtl.current = null
    session.current = { agent: nextAgent, thread: nextThread }
    inFlight.current = false
    setAgent(nextAgent)
    setThread(nextThread)
    setSending(false)
    setTracing(false)
    setMessages([])
    setEvents(null)
    setStatus(nextStatus)
  }, [])

  const switchAgent = useCallback(
    (name: string) => {
      if (name === '' || name === session.current.agent) return
      enter(name, newThreadID(), READY)
      setThreads(null)
      loadThreadsFor(name)
    },
    [enter, loadThreadsFor],
  )

  useEffect(() => {
    alive.current = true
    loadThreadsFor(session.current.agent)
    return () => {
      alive.current = false
      epoch.current++
      chatCtl.current?.abort()
      traceCtl.current?.abort()
      threadsCtl.current?.abort()
    }
  }, [loadThreadsFor])

  const newThread = useCallback(() => {
    const id = newThreadID()
    enter(session.current.agent, id, {
      key: 'playground.statusNewThread',
      params: { id: shortID(id) },
      tone: 'accent',
    })
    loadThreadsFor(session.current.agent)
  }, [enter, loadThreadsFor])

  /** Fetches the thread's whole trace ring. Resolves null if superseded or failed. */
  const fetchTrace = useCallback(async (): Promise<TraceEvent[] | null> => {
    const { agent: a, thread: th } = session.current
    if (a === '') return null
    traceCtl.current?.abort()
    const c = new AbortController()
    traceCtl.current = c
    setTracing(true)
    try {
      const evs = await client.trace(a, th, c.signal)
      return c.signal.aborted ? null : evs
    } catch (err) {
      if (!c.signal.aborted) toastError(err)
      return null
    } finally {
      if (!c.signal.aborted && alive.current) setTracing(false)
    }
  }, [client])

  const openThread = useCallback(
    async (id: string) => {
      enter(session.current.agent, id, {
        key: 'playground.statusThread',
        params: { id: shortID(id) },
        tone: 'accent',
      })
      const evs = await fetchTrace()
      if (!evs) return
      setEvents(evs)
      // This tab never saw the thread's turns, so the only transcript on offer
      // is what the trace recorded.
      setMessages(transcriptFromTrace(evs))
      loadThreadsFor(session.current.agent)
    },
    [enter, fetchTrace, loadThreadsFor],
  )

  const refreshTrace = useCallback(async () => {
    const evs = await fetchTrace()
    if (evs) setEvents(evs)
  }, [fetchTrace])

  const stop = useCallback(async () => {
    const mine = session.current
    if (mine.agent === '') return
    // Aborting first lets the turn report "turn canceled" before the stop lands.
    chatCtl.current?.abort()
    setStatus({ key: 'playground.statusStopping', tone: 'warning' })
    setStopping(true)
    try {
      await client.stopThread(mine.agent, mine.thread)
      if (alive.current && session.current === mine) setStatus({ key: 'playground.statusStopRequested', tone: 'warning' })
    } catch (err) {
      if (alive.current) {
        toastError(err)
        if (session.current === mine) setStatus({ key: 'playground.statusError', tone: 'danger' })
      }
    } finally {
      if (alive.current) setStopping(false)
    }
  }, [client])

  const send = (text: string, actAs = '', attachments?: MediaPart[]): boolean => {
    const { agent: a, thread: th } = session.current
    const atts = attachments && attachments.length > 0 ? attachments : undefined
    if (inFlight.current || a === '' || (text.trim() === '' && !atts)) return false
    inFlight.current = true
    setSending(true)
    setStatus({ key: 'playground.statusRunning', tone: 'accent' })
    // The trace shows this turn alone while it streams; refresh brings the thread's ring back.
    setEvents(null)

    const now = new Date()
    const asked: Message = { id: messageID(), role: 'you', text, at: now, attachments: atts }
    const pending = messageID()
    setMessages((m) => [...m, asked, { id: pending, role: 'agent', text: '', at: now, pending: true }])

    const mine = ++epoch.current
    const live = () => alive.current && epoch.current === mine
    const ctl = new AbortController()
    chatCtl.current = ctl
    const acting = actAs.trim()

    const complain = (text: string, note?: string) => {
      const bubble: Message = {
        id: messageID(),
        role: 'error',
        text,
        at: new Date(),
        note,
      }
      setMessages((m) => [...m, bubble])
    }
    const fail = (statusText: string, bubble: string) => {
      setStatus({ key: 'playground.statusErrorWith', params: { message: statusText }, tone: 'danger' })
      complain(bubble)
    }

    let sawEvent = false
    let sawDone = false
    // True once a loop failure has its bubble (from the trace entry), so the
    // error event that ends the stream does not say the same thing twice.
    let loopFailed = false
    // True once the stream itself decided the status (an error, a timeout), so
    // a clean end does not overwrite it with "done".
    let settled = false

    const onEvent = (ev: ChatEvent) => {
      if (!live()) return
      sawEvent = true
      switch (ev.type) {
        case 'trace':
          if (ev.trace) {
            const tr = ev.trace
            setEvents((cur) => appendEvent(cur, tr))
            // A failed loop sends no reply; say so where one was expected.
            // The runtime then ends the stream with an error event (older
            // runtimes send nothing more and run out the chat timeout).
            if (tr.kind === 'error' && tr.err) {
              loopFailed = true
              complain(tr.err, t('playground.loopError'))
            }
          }
          break
        case 'reply':
          if (ev.reply) {
            const reply: WireReply = ev.reply
            const note = replyNote(reply)
            const atts = reply.attachments?.length ? reply.attachments : undefined
            setMessages((m) =>
              m.map((x) =>
                x.id === pending
                  ? {
                      ...x,
                      text: joinReply(x.text, reply.text ?? ''),
                      note: note || x.note,
                      attachments: atts ? [...(x.attachments ?? []), ...atts] : x.attachments,
                    }
                  : x,
              ),
            )
          }
          break
        case 'approval':
          if (ev.approval) {
            const ap: ApprovalPrompt = ev.approval
            setMessages((m) => {
              if (m.some((x) => x.approval?.prompt.id === ap.id)) return m
              const entry: Message = {
                id: messageID(),
                role: 'approval',
                text: ap.action,
                at: new Date(),
                approval: { prompt: ap, state: 'pending' },
              }
              const idx = m.findIndex((x) => x.id === pending)
              if (idx >= 0) return [...m.slice(0, idx), entry, ...m.slice(idx)]
              return [...m, entry]
            })
          }
          break
        case 'error': {
          // The last word of a turn that failed: the message could not be
          // handed to the agent, or its loop failed while answering.
          const msg = ev.error ?? ''
          settled = true
          if (loopFailed) setStatus({ key: 'playground.statusErrorWith', params: { message: msg }, tone: 'danger' })
          else fail(msg, msg || t('playground.noErrorMessage'))
          break
        }
        case 'done':
          sawDone = true
          if (ev.done?.timed_out) {
            settled = true
            setStatus({
              key: 'playground.statusTimedOut',
              tone: 'warning',
            })
          }
          break
      }
    }

    const finish = (err: unknown) => {
      // A switch to another agent or thread already reset the screen.
      if (!live()) return
      if (chatCtl.current === ctl) chatCtl.current = null
      inFlight.current = false
      setSending(false)
      setMessages((m) => m.map((x) => (x.id === pending ? { ...x, pending: false } : x)))
      if (ctl.signal.aborted) {
        setStatus({ key: 'playground.statusCanceled', tone: 'warning' })
      } else if (err instanceof ApiError) {
        // Refused or unreachable before the stream began (unknown agent, 4xx, no route).
        fail(err.message, err.message)
      } else if (err) {
        // The connection broke mid-stream. The runtime only stops streaming; the loop goes on.
        const reason = errorMessage(err)
        fail(t('playground.streamError', { message: reason }), t('playground.streamBroken', { message: reason }))
      } else if (!sawEvent) {
        // Older runtimes answer 200 and close the stream when they refuse a
        // turn before delivery (current ones answer 400 with the reason); the
        // one refusal a non-empty message can meet there is an act-as id that
        // is not a known profile.
        fail(t('playground.noTurn'), acting ? t('playground.noTurnActAs', { user: acting }) : t('playground.noTurnBubble'))
      } else if (!sawDone && !settled) {
        setStatus({
          key: 'playground.statusStreamEnded',
          tone: 'warning',
        })
      } else if (!settled) {
        setStatus({ key: 'playground.statusDone', tone: 'success' })
      }
      loadThreadsFor(a)
    }

    client
      .chat(
        {
          agent: a,
          thread: th,
          text,
          ...(atts ? { attachments: atts } : {}),
          ...(acting ? { act_as_user: acting } : {}),
        },
        onEvent,
        ctl.signal,
      )
      .then(() => finish(undefined), finish)
    return true
  }

  const decideApproval = useCallback(
    async (id: string, approve: boolean) => {
      setMessages((m) =>
        m.map((x) =>
          x.approval?.prompt.id === id
            ? { ...x, approval: { ...x.approval, state: 'submitting', error: undefined } }
            : x,
        ),
      )
      try {
        const rec = await client.decideStudioApproval(id, approve)
        if (!alive.current) return
        const nextState = rec.status === 'approved' ? 'approved' : 'denied'
        setMessages((m) =>
          m.map((x) =>
            x.approval?.prompt.id === id
              ? { ...x, approval: { ...x.approval, state: nextState, error: undefined } }
              : x,
          ),
        )
      } catch (err) {
        toastError(err)
        if (!alive.current) return
        const msg = errorMessage(err)
        setMessages((m) =>
          m.map((x) =>
            x.approval?.prompt.id === id
              ? { ...x, approval: { ...x.approval, state: 'error', error: msg } }
              : x,
          ),
        )
      }
    },
    [client],
  )

  return {
    agent,
    thread,
    status,
    messages,
    events,
    threads: threads && threads.agent === agent ? threads.rows : null,
    sending,
    stopping,
    tracing,
    switchAgent,
    newThread,
    openThread,
    refreshTrace,
    loadThreads,
    stop,
    send,
    decideApproval,
    busy: () => inFlight.current,
  }
}
