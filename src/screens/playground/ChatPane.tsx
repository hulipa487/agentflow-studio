// Center pane: the thread header with its status, the transcript, the composer.
import { forwardRef, useImperativeHandle, useRef } from 'react'
import { Send, Square } from 'lucide-react'
import { Avatar, Button, CopyButton, Dot, IconButton, Logo, Panel, cx, toneText } from '@/components/ui'
import { clockTime, initial, shortID } from '@/lib/format'
import { MOD } from '@/nav'
import type { Message, Status } from './model'
import { useStickToBottom } from './useStickToBottom'

const ROLE_LABEL = { you: 'YOU', agent: 'AGENT', error: 'ERROR' } as const

function Bubble({ m }: { m: Message }) {
  const mine = m.role === 'you'
  const failed = m.role === 'error'
  // An attachment-only reply has no text but is not a missing reply.
  const waiting = m.role === 'agent' && m.text === '' && !m.note
  return (
    <div className={cx('flex px-2', mine ? 'justify-end' : 'justify-start')}>
      <div
        className={cx(
          'min-w-[120px] max-w-[min(100%,560px)] rounded-card border px-2.5 py-2',
          mine ? 'border-accent/70 bg-accent-dim' : failed ? 'border-danger/50 bg-tone-danger' : 'border-line bg-surface2',
        )}
      >
        <div className="flex items-center gap-3">
          <span className={cx('font-mono text-[10px] font-bold', mine ? 'text-accent' : failed ? 'text-danger' : 'text-muted')}>
            {/* A traced turn.start is whatever reached the session, not necessarily typed here. */}
            {mine && m.fromTrace ? 'INBOUND' : ROLE_LABEL[m.role]}
          </span>
          {m.note && <span className="min-w-0 truncate font-mono text-[10px] text-faint">{m.note}</span>}
          <span className="ml-auto font-mono text-[10px] text-faint">{clockTime(m.at)}</span>
        </div>
        {/* Replies are untrusted model output: always plain text, never markup. */}
        <div className={cx('mt-0.5 select-text whitespace-pre-wrap break-words text-[13px] leading-relaxed', failed && 'text-danger', waiting && 'text-muted')}>
          {waiting ? m.pending ? <span className="animate-pulse">…</span> : 'No reply.' : m.text}
        </div>
      </div>
    </div>
  )
}

export interface ChatPaneHandle {
  /** Brings the newest message into view even if the reader had scrolled up. */
  pinToEnd: () => void
  focusComposer: () => void
}

export const ChatPane = forwardRef<
  ChatPaneHandle,
  {
    agent: string
    thread: string
    status: Status
    messages: Message[]
    sending: boolean
    stopping: boolean
    draft: string
    onDraft: (v: string) => void
    onSend: () => void
    onStop: () => void
  }
>(function ChatPane({ agent, thread, status, messages, sending, stopping, draft, onDraft, onSend, onStop }, handle) {
  const { ref, onScroll, pin } = useStickToBottom<HTMLDivElement>(messages, agent + '\n' + thread)
  const input = useRef<HTMLTextAreaElement>(null)
  useImperativeHandle(handle, () => ({ pinToEnd: pin, focusComposer: () => input.current?.focus() }), [pin])

  const rebuilt = messages.some((m) => m.fromTrace)

  return (
    <Panel flush className="flex h-full min-h-0 flex-col">
      <header className="flex shrink-0 items-center gap-2.5 border-b border-line px-3 py-2">
        <Avatar letter={initial(agent)} size={34} />
        <div className="min-w-[96px] flex-1">
          <div className="truncate text-[14px] font-semibold leading-tight">{agent}</div>
          <div className="flex min-w-0 items-center gap-0.5">
            <span className="truncate font-mono text-[11px] text-muted" title={thread}>
              thread · {shortID(thread)}
            </span>
            <CopyButton text={thread} what="thread id" />
          </div>
        </div>
        <div className="flex min-w-0 shrink items-center gap-2" role="status" aria-live="polite">
          <Dot tone={status.tone} pulse={sending} className="shrink-0" />
          <span
            className={cx('min-w-0 max-w-[220px] truncate text-[11.5px]', status.tone === 'accent' || status.tone === 'neutral' ? 'text-text' : toneText(status.tone))}
            title={status.text}
          >
            {status.text}
          </span>
        </div>
        <IconButton icon={Square} label="Stop session" tone="danger" onClick={onStop} loading={stopping} />
      </header>

      <div ref={ref} onScroll={onScroll} className="flex min-h-0 flex-1 flex-col overflow-y-auto py-3">
        {messages.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-1 px-6 py-8 text-center">
            <Logo size={48} />
            <div className="mt-1 text-[13px] font-semibold">Say hello</div>
            <div className="text-[12.5px] text-muted">The turn streams into the trace panel on the right.</div>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {rebuilt && (
              <div className="px-4 pb-1 text-center text-[11px] leading-snug text-faint">
                Earlier messages are rebuilt from this thread&apos;s trace payloads; anything the trace ring has dropped is not shown.
              </div>
            )}
            {messages.map((m) => (
              <Bubble key={m.id} m={m} />
            ))}
          </div>
        )}
      </div>

      <div className="shrink-0 border-t border-line p-2.5">
        <div className="rounded-ctl border border-line bg-bg transition-colors duration-100 focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/20 hover:border-faint focus-within:hover:border-accent">
          <textarea
            ref={input}
            value={draft}
            onChange={(e) => onDraft(e.target.value)}
            rows={3}
            spellCheck={false}
            aria-label="Message the agent"
            placeholder="Message the agent…"
            className="block w-full resize-none bg-transparent px-2.5 pt-2 text-[13px] leading-relaxed text-text placeholder:text-faint focus:outline-none"
          />
          <div className="flex items-center gap-2 py-1 pl-2.5 pr-1">
            <span className="min-w-0 flex-1 truncate text-[11px] text-faint">{MOD}+Enter sends · Enter adds a newline</span>
            <Button
              variant="primary"
              icon={Send}
              className="w-[92px]"
              disabled={sending || draft.trim() === ''}
              onClick={() => {
                onSend()
                input.current?.focus()
              }}
            >
              Send
            </Button>
          </div>
        </div>
      </div>
    </Panel>
  )
})
