// The live Playground: one agent, one thread, one turn at a time. This holds
// the session logic ported from playground.go (send, stop, open, refresh);
// the three panes only draw it.
import { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { ApiError, errorMessage, type Client } from '@/api/client'
import type { ChatEvent, Reply, Thread, TraceEvent } from '@/api/types'
import { Button, Page } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { newThreadID, shortID } from '@/lib/format'
import { toastError } from '@/state/ui'
import { ChatPane, type ChatPaneHandle } from './ChatPane'
import { Panes } from './Panes'
import { ThreadsPane } from './ThreadsPane'
import { TracePane } from './TracePane'
import { READY, appendEvent, joinReply, messageID, transcriptFromTrace, type Message, type Status, type TraceFilter } from './model'

export const TITLE = 'Playground'
export const SUBTITLE = 'Chat with an agent turn by turn. Each thread is its own session, with every operation traced.'

// The typed Reply names only text and status; the runtime also sends any
// attachments, which a text transcript cannot show.
type WireReply = Reply & { attachments?: unknown[] | null }

function replyNote(r: WireReply): string {
  const notes: string[] = []
  if (r.status && r.status !== 'delivered') notes.push(r.status)
  const n = r.attachments?.length ?? 0
  if (n > 0) notes.push(`${n} attachment(s) not shown`)
  return notes.join(' · ')
}

export function Workspace({ client, agents, reloadAgents }: { client: Client; agents: string[]; reloadAgents: () => void }) {
  const [params, setParams] = useSearchParams()
  const wanted = params.get('agent') ?? ''

  const [agent, setAgent] = useState(() => (agents.includes(wanted) ? wanted : agents[0]))
  const [thread, setThread] = useState(newThreadID)
  const [status, setStatus] = useState<Status>(READY)
  const [messages, setMessages] = useState<Message[]>([])
  const [events, setEvents] = useState<TraceEvent[] | null>(null)
  const [threads, setThreads] = useState<{ agent: string; rows: Thread[] } | null>(null)
  const [sending, setSending] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [tracing, setTracing] = useState(false)
  const [filter, setFilter] = useState<TraceFilter>('All events')
  const [actAs, setActAs] = useState('')
  const [draft, setDraft] = useState('')

  // What callbacks compare against: the session they started in.
  const session = useRef({ agent, thread })
  // Bumped whenever the session changes or a new turn starts, so a late answer
  // from an earlier turn, thread or agent is never applied.
  const epoch = useRef(0)
  const alive = useRef(true)
  // Mirrors `sending` for the Ctrl/Cmd+Enter path, which can fire again before a re-render.
  const busy = useRef(false)
  const chatCtl = useRef<AbortController | null>(null)
  const traceCtl = useRef<AbortController | null>(null)
  const threadsCtl = useRef<AbortController | null>(null)
  const chat = useRef<ChatPaneHandle>(null)

  // A runtime without the identity layer just offers no suggestions.
  const users = useAsync(
    (signal) =>
      client.users(signal).then(
        (l) => (l.users ?? []).map((u) => u.user_id),
        () => [] as string[],
      ),
    [client],
  )

  const loadThreads = useCallback(
    (forAgent: string) => {
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
    [client],
  )

  /** Moves to another agent or thread: drops the running stream and clears chat and trace. */
  const enter = useCallback((nextAgent: string, nextThread: string, nextStatus: Status) => {
    epoch.current++
    chatCtl.current?.abort()
    chatCtl.current = null
    traceCtl.current?.abort()
    traceCtl.current = null
    session.current = { agent: nextAgent, thread: nextThread }
    busy.current = false
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
      loadThreads(name)
    },
    [enter, loadThreads],
  )

  useEffect(() => {
    alive.current = true
    loadThreads(session.current.agent)
    return () => {
      alive.current = false
      epoch.current++
      chatCtl.current?.abort()
      traceCtl.current?.abort()
      threadsCtl.current?.abort()
    }
  }, [loadThreads])

  // The address bar can change under the open screen (a "chat with" link, the
  // back button); follow it like a pick from the select.
  useEffect(() => {
    if (agents.includes(wanted)) switchAgent(wanted)
  }, [wanted, agents, switchAgent])

  // A refresh can find the open agent deleted: move to one that still exists.
  useEffect(() => {
    if (agents.includes(session.current.agent)) return
    switchAgent(agents[0])
    setParams({ agent: agents[0] }, { replace: true })
  }, [agents, switchAgent, setParams])

  const pickAgent = (name: string) => {
    switchAgent(name)
    setParams({ agent: name }, { replace: true })
  }

  const newThread = () => {
    const id = newThreadID()
    enter(session.current.agent, id, { text: 'new thread ' + shortID(id), tone: 'accent' })
    loadThreads(session.current.agent)
    chat.current?.focusComposer()
  }

  /** Fetches the thread's whole trace ring. Resolves null if superseded or failed. */
  const fetchTrace = async (): Promise<TraceEvent[] | null> => {
    const { agent: a, thread: t } = session.current
    traceCtl.current?.abort()
    const c = new AbortController()
    traceCtl.current = c
    setTracing(true)
    try {
      const evs = await client.trace(a, t, c.signal)
      return c.signal.aborted ? null : evs
    } catch (err) {
      if (!c.signal.aborted) toastError(err)
      return null
    } finally {
      if (!c.signal.aborted && alive.current) setTracing(false)
    }
  }

  const openThread = async (id: string) => {
    enter(session.current.agent, id, { text: 'thread ' + shortID(id), tone: 'accent' })
    const evs = await fetchTrace()
    if (!evs) return
    setEvents(evs)
    // This tab never saw the thread's turns, so the only transcript on offer
    // is what the trace recorded.
    setMessages(transcriptFromTrace(evs))
    loadThreads(session.current.agent)
  }

  const refreshTrace = async () => {
    const evs = await fetchTrace()
    if (evs) setEvents(evs)
  }

  const stop = async () => {
    const mine = session.current
    // Aborting first lets the turn report "turn canceled" before the stop lands.
    chatCtl.current?.abort()
    setStatus({ text: 'stopping…', tone: 'warning' })
    setStopping(true)
    try {
      await client.stopThread(mine.agent, mine.thread)
      if (alive.current && session.current === mine) setStatus({ text: 'stop requested', tone: 'warning' })
    } catch (err) {
      if (alive.current) {
        toastError(err)
        if (session.current === mine) setStatus({ text: 'error', tone: 'danger' })
      }
    } finally {
      if (alive.current) setStopping(false)
    }
  }

  const send = () => {
    const text = draft
    const { agent: a, thread: t } = session.current
    if (busy.current || a === '' || text.trim() === '') return
    busy.current = true
    setDraft('')
    setSending(true)
    setStatus({ text: 'running…', tone: 'accent' })
    // The trace shows this turn alone while it streams; refresh brings the thread's ring back.
    setEvents(null)

    const now = new Date()
    const asked: Message = { id: messageID(), role: 'you', text, at: now }
    const pending = messageID()
    setMessages((m) => [...m, asked, { id: pending, role: 'agent', text: '', at: now, pending: true }])
    chat.current?.pinToEnd()

    const mine = ++epoch.current
    const live = () => alive.current && epoch.current === mine
    const ctl = new AbortController()
    chatCtl.current = ctl
    const acting = actAs.trim()

    const complain = (text: string, note?: string) => {
      const bubble: Message = { id: messageID(), role: 'error', text, at: new Date(), note }
      setMessages((m) => [...m, bubble])
    }
    const fail = (statusText: string, bubble: string) => {
      setStatus({ text: 'error: ' + statusText, tone: 'danger' })
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
              complain(tr.err, 'loop error')
            }
          }
          break
        case 'reply':
          if (ev.reply) {
            const reply: WireReply = ev.reply
            const note = replyNote(reply)
            setMessages((m) => m.map((x) => (x.id === pending ? { ...x, text: joinReply(x.text, reply.text ?? ''), note: note || x.note } : x)))
          }
          break
        case 'error': {
          // The last word of a turn that failed: the message could not be
          // handed to the agent, or its loop failed while answering.
          const msg = ev.error ?? ''
          settled = true
          if (loopFailed) setStatus({ text: 'error: ' + msg, tone: 'danger' })
          else fail(msg, msg || 'The runtime reported an error without a message.')
          break
        }
        case 'done':
          sawDone = true
          if (ev.done?.timed_out) {
            settled = true
            setStatus({ text: 'turn timed out; the loop is still running', tone: 'warning' })
          }
          break
      }
    }

    const finish = (err: unknown) => {
      // A switch to another agent or thread already reset the screen.
      if (!live()) return
      if (chatCtl.current === ctl) chatCtl.current = null
      busy.current = false
      setSending(false)
      setMessages((m) => m.map((x) => (x.id === pending ? { ...x, pending: false } : x)))
      if (ctl.signal.aborted) {
        setStatus({ text: 'turn canceled', tone: 'warning' })
      } else if (err instanceof ApiError) {
        // Refused or unreachable before the stream began (unknown agent, 4xx, no route).
        fail(err.message, err.message)
      } else if (err) {
        // The connection broke mid-stream. The runtime only stops streaming; the loop goes on.
        const msg = 'stream: ' + errorMessage(err)
        fail(msg, msg + '\n\nThe turn may still be running on the runtime. Refresh the trace to see how far it got.')
      } else if (!sawEvent) {
        // Older runtimes answer 200 and close the stream when they refuse a
        // turn before delivery (current ones answer 400 with the reason); the
        // one refusal a non-empty message can meet there is an act-as id that
        // is not a known profile.
        const msg = 'the runtime closed the stream without running a turn'
        fail(msg, acting ? `${msg}.\n\nAct as user "${acting}" is probably not a known profile.` : msg + '.')
      } else if (!sawDone && !settled) {
        setStatus({ text: 'stream ended before the turn finished', tone: 'warning' })
      } else if (!settled) {
        setStatus({ text: 'done', tone: 'success' })
      }
      loadThreads(a)
    }

    client
      .chat({ agent: a, thread: t, text, ...(acting ? { act_as_user: acting } : {}) }, onEvent, ctl.signal)
      .then(() => finish(undefined), finish)
  }

  useScreenActions({
    send,
    refresh: () => {
      // An agent created since this screen opened joins the picker.
      reloadAgents()
      loadThreads(session.current.agent)
      void refreshTrace()
    },
  })

  return (
    <Page
      title={TITLE}
      subtitle={SUBTITLE}
      scroll={false}
      actions={
        <Button variant="primary" icon={Plus} onClick={newThread}>
          New thread
        </Button>
      }
    >
      <Panes
        left={
          <ThreadsPane
            agents={agents}
            agent={agent}
            onAgent={pickAgent}
            threads={threads && threads.agent === agent ? threads.rows : null}
            thread={thread}
            onOpen={(id) => void openThread(id)}
            actAs={actAs}
            onActAs={setActAs}
            userIDs={users.data ?? []}
          />
        }
        center={
          <ChatPane
            ref={chat}
            agent={agent}
            thread={thread}
            status={status}
            messages={messages}
            sending={sending}
            stopping={stopping}
            draft={draft}
            onDraft={setDraft}
            onSend={send}
            onStop={() => void stop()}
          />
        }
        right={<TracePane events={events} scope={agent + '\n' + thread} filter={filter} onFilter={setFilter} onRefresh={() => void refreshTrace()} refreshing={tracing} />}
      />
    </Page>
  )
}
