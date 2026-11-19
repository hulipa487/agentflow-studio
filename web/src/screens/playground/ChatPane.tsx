// Center pane: the thread header with its status, the transcript, the composer.
import { forwardRef, useImperativeHandle, useRef, type ReactNode } from 'react'
import { Send, Square } from 'lucide-react'
import { Button, CopyButton, Dot, IconButton, Panel, cx, toneText } from '@/components/ui'
import { t } from '@/i18n'
import { clockTime, shortID } from '@/lib/format'
import { MOD } from '@/nav'
import { statusText, type Message, type Status } from './model'
import { useStickToBottom } from './useStickToBottom'

const ROLE_LABEL_KEY = { you: 'playground.roleYou', agent: 'playground.roleAgent', error: 'playground.roleError' } as const

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
          mine ? 'border-line bg-surface2' : failed ? 'border-danger/40 bg-surface' : 'border-line bg-surface',
        )}
      >
        <div className="flex items-center gap-3">
          <span className={cx('text-xs font-medium', failed ? 'text-danger' : 'text-muted')}>
            {/* A traced turn.start is whatever reached the session, not necessarily typed here. */}
            {mine && m.fromTrace ? t('playground.roleInbound') : t(ROLE_LABEL_KEY[m.role])}
          </span>
          {m.note && <span className="min-w-0 truncate font-mono text-xs text-faint">{m.note}</span>}
          <span className="ml-auto font-mono text-xs text-faint">{clockTime(m.at)}</span>
        </div>
        {/* Replies are untrusted model output: always plain text, never markup. */}
        <div className={cx('mt-0.5 select-text whitespace-pre-wrap break-words text-base leading-relaxed', failed && 'text-danger', waiting && 'text-muted')}>
          {waiting ? m.pending ? <span className="animate-pulse">…</span> : t('playground.noReply') : m.text}
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
    /** Extra header buttons, before Stop. */
    actions?: ReactNode
    /** The line beside Send; defaults to the send shortcut. */
    hint?: string
    /** What the empty transcript suggests. */
    emptyHint?: string
  }
>(function ChatPane({ agent, thread, status, messages, sending, stopping, draft, onDraft, onSend, onStop, actions, hint, emptyHint }, handle) {
  const { ref, onScroll, pin } = useStickToBottom<HTMLDivElement>(messages, agent + '\n' + thread)
  const input = useRef<HTMLTextAreaElement>(null)
  useImperativeHandle(handle, () => ({ pinToEnd: pin, focusComposer: () => input.current?.focus() }), [pin])

  const rebuilt = messages.some((m) => m.fromTrace)
  const shownStatus = statusText(status)

  return (
    <Panel flush className="flex h-full min-h-0 flex-col">
      <header className="flex shrink-0 items-center gap-2.5 border-b border-line px-3 py-2">
        <div className="min-w-[96px] flex-1">
          <div className="truncate text-base font-semibold leading-tight">{agent}</div>
          <div className="flex min-w-0 items-center gap-0.5">
            <span className="truncate font-mono text-xs text-muted" title={thread}>
              {t('playground.threadHeader', { id: shortID(thread) })}
            </span>
            <CopyButton text={thread} what={t('playground.threadId')} />
          </div>
        </div>
        <div className="flex min-w-0 shrink items-center gap-2" role="status" aria-live="polite">
          <Dot tone={status.tone} pulse={sending} className="shrink-0" />
          <span
            className={cx('min-w-0 max-w-[220px] truncate text-sm', status.tone === 'accent' || status.tone === 'neutral' ? 'text-text' : toneText(status.tone))}
            title={shownStatus}
          >
            {shownStatus}
          </span>
        </div>
        {actions}
        <IconButton icon={Square} label={t('playground.stopSession')} tone="danger" onClick={onStop} loading={stopping} />
      </header>

      <div ref={ref} onScroll={onScroll} className="flex min-h-0 flex-1 flex-col overflow-y-auto py-3">
        {messages.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-1 px-6 py-8 text-center">
            <div className="text-base font-medium">{t('playground.emptyTitle')}</div>
            <div className="text-sm text-muted">{emptyHint ?? t('playground.emptyHint')}</div>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {rebuilt && (
              <div className="px-4 pb-1 text-center text-xs leading-snug text-faint">
                {t('playground.rebuiltNote')}
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
            aria-label={t('playground.messageLabel')}
            placeholder={t('playground.messagePlaceholder')}
            className="block w-full resize-none bg-transparent px-2.5 pt-2 text-base leading-relaxed text-text placeholder:text-faint focus:outline-none"
          />
          <div className="flex items-center gap-2 py-1 pl-2.5 pr-1">
            <span className="min-w-0 flex-1 truncate text-xs text-faint">{hint ?? t('playground.sendHint', { mod: MOD })}</span>
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
              {t('common.send')}
            </Button>
          </div>
        </div>
      </div>
    </Panel>
  )
})
