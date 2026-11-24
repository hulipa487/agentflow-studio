// Center pane: the thread header with its status, the transcript, the composer.
import { forwardRef, useEffect, useImperativeHandle, useRef, useState, type ReactNode } from 'react'
import { Check, Download, Paperclip, Send, ShieldAlert, Square, X } from 'lucide-react'
import type { MediaPart } from '@/api/types'
import { Button, CopyButton, Dot, IconButton, Panel, cx, toneText } from '@/components/ui'
import { t } from '@/i18n'
import { clockTime, shortID } from '@/lib/format'
import { MOD } from '@/nav'
import { statusText, type Message, type Status } from './model'
import { useStickToBottom } from './useStickToBottom'

const ROLE_LABEL_KEY = {
  you: 'playground.roleYou',
  agent: 'playground.roleAgent',
  approval: 'playground.roleApproval',
  error: 'playground.roleError',
} as const

type MediaLoader = (handle: string, signal?: AbortSignal) => Promise<Blob>

const SAFE_IMAGE_MIMES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])

function isSafeHttpURL(raw?: string): boolean {
  if (!raw) return false
  return raw.startsWith('https://') || raw.startsWith('http://')
}

function formatBytes(n?: number): string {
  if (!n || n <= 0) return ''
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`
  return `${(n / (1024 * 1024)).toFixed(1)} MiB`
}

function AttachmentView({ part, loadMedia }: { part: MediaPart; loadMedia?: MediaLoader }) {
  const [blobURL, setBlobURL] = useState<string | null>(null)
  const [blobMIME, setBlobMIME] = useState<string>(part.mime ?? '')
  const [err, setErr] = useState<string | null>(null)

  const handle = part.handle
  const mime = (blobMIME || part.mime || '').toLowerCase()
  const isImage = SAFE_IMAGE_MIMES.has(mime) || (part.type === 'image' && mime === '')
  const isAudio = mime.startsWith('audio/') || part.type === 'audio'

  useEffect(() => {
    if (!handle || !loadMedia) return
    const ctl = new AbortController()
    let objectURL: string | null = null
    setErr(null)
    loadMedia(handle, ctl.signal).then(
      (blob) => {
        if (ctl.signal.aborted) return
        objectURL = URL.createObjectURL(blob)
        setBlobURL(objectURL)
        if (blob.type) setBlobMIME(blob.type)
      },
      (e) => {
        if (!ctl.signal.aborted) setErr(e instanceof Error ? e.message : String(e))
      },
    )
    return () => {
      ctl.abort()
      if (objectURL) URL.revokeObjectURL(objectURL)
    }
  }, [handle, loadMedia])

  const label = part.name || (handle ? handle.slice(0, 22) : part.type || t('playground.attachment'))
  const meta = [mime || part.type, formatBytes(part.size)].filter(Boolean).join(' · ')

  if (handle && blobURL && isImage) {
    return (
      <div className="flex flex-col gap-1">
        <a href={blobURL} download={part.name || 'image'} className="block w-fit">
          <img src={blobURL} alt={label} className="max-h-[280px] max-w-full rounded border border-line object-contain bg-bg" />
        </a>
        <div className="flex items-center gap-2 text-xs text-muted">
          <span className="truncate font-mono">{label}</span>
          {meta && <span className="shrink-0 text-faint">{meta}</span>}
          <a href={blobURL} download={part.name || 'image'} className="ml-auto inline-flex items-center gap-1 text-accent hover:underline">
            <Download className="h-3 w-3" />
          </a>
        </div>
      </div>
    )
  }

  if (handle && blobURL && isAudio) {
    return (
      <div className="flex flex-col gap-1 rounded border border-line bg-bg p-2">
        <audio controls src={blobURL} className="w-full max-w-[320px]" />
        <div className="flex items-center gap-2 text-xs text-muted">
          <span className="truncate font-mono">{label}</span>
          {meta && <span className="shrink-0 text-faint">{meta}</span>}
        </div>
      </div>
    )
  }

  if (handle && blobURL) {
    return (
      <a
        href={blobURL}
        download={part.name || 'attachment'}
        className="flex items-center gap-2 rounded border border-line bg-bg px-2.5 py-1.5 text-xs hover:border-accent"
      >
        <Paperclip className="h-3.5 w-3.5 shrink-0 text-muted" />
        <span className="min-w-0 truncate font-mono text-text">{label}</span>
        {meta && <span className="shrink-0 text-faint">{meta}</span>}
        <Download className="ml-auto h-3.5 w-3.5 shrink-0 text-accent" />
      </a>
    )
  }

  if (!handle && isSafeHttpURL(part.url)) {
    return (
      <a
        href={part.url}
        target="_blank"
        rel="noreferrer noopener"
        className="flex items-center gap-2 rounded border border-line bg-bg px-2.5 py-1.5 text-xs hover:border-accent"
      >
        <Paperclip className="h-3.5 w-3.5 shrink-0 text-muted" />
        <span className="min-w-0 truncate font-mono text-accent underline">{label}</span>
        {meta && <span className="shrink-0 text-faint">{meta}</span>}
      </a>
    )
  }

  return (
    <div className="flex items-center gap-2 rounded border border-line bg-bg px-2.5 py-1.5 text-xs text-muted">
      <Paperclip className="h-3.5 w-3.5 shrink-0" />
      <span className="min-w-0 truncate font-mono">{label}</span>
      {meta && <span className="shrink-0 text-faint">{meta}</span>}
      {err && <span className="ml-auto text-danger">{err}</span>}
    </div>
  )
}

function Bubble({
  m,
  loadMedia,
  onDecideApproval,
}: {
  m: Message
  loadMedia?: MediaLoader
  onDecideApproval?: (id: string, approve: boolean) => void
}) {
  if (m.role === 'approval' && m.approval) {
    const { prompt, state, error } = m.approval
    const hasArgs = prompt.args_preview && Object.keys(prompt.args_preview).length > 0
    const canDecide = !!onDecideApproval && (state === 'pending' || state === 'error')
    return (
      <div className="flex justify-start px-2">
        <div className="min-w-[240px] max-w-[min(100%,560px)] rounded-card border border-warning/50 bg-surface px-3 py-2.5">
          <div className="flex items-center gap-2">
            <ShieldAlert className="h-3.5 w-3.5 shrink-0 text-warning" />
            <span className="text-xs font-medium text-warning">{t('playground.approvalRequired')}</span>
            <span className="ml-auto font-mono text-xs text-faint">{clockTime(m.at)}</span>
          </div>
          <div className="mt-1 flex flex-wrap items-baseline gap-2">
            <code className="font-mono text-sm font-medium text-text">{prompt.action}</code>
            {prompt.destination && (
              <span className="rounded border border-line bg-bg px-1.5 py-0.5 font-mono text-xs text-muted">
                {t('playground.approvalDestination')}: {prompt.destination}
              </span>
            )}
          </div>
          {prompt.description && <div className="mt-1 text-xs text-muted">{prompt.description}</div>}
          {hasArgs && (
            <pre className="mt-2 max-h-[160px] overflow-auto rounded border border-line bg-bg p-2 font-mono text-xs text-text">
              {JSON.stringify(prompt.args_preview, null, 2)}
            </pre>
          )}
          <div className="mt-2 flex flex-wrap items-center gap-2">
            {prompt.expires_at > 0 && (
              <span className="font-mono text-xs text-faint">
                {t('playground.approvalExpires', { time: clockTime(new Date(prompt.expires_at * 1000)) })}
              </span>
            )}
            <div className="ml-auto flex items-center gap-1.5">
              {state === 'approved' ? (
                <span className="inline-flex items-center gap-1 text-xs font-medium text-success">
                  <Check className="h-3.5 w-3.5" />
                  {t('playground.approvalApproved')}
                </span>
              ) : state === 'denied' ? (
                <span className="inline-flex items-center gap-1 text-xs font-medium text-danger">
                  <X className="h-3.5 w-3.5" />
                  {t('playground.approvalDenied')}
                </span>
              ) : (
                <>
                  <Button
                    size="sm"
                    variant="danger"
                    icon={X}
                    loading={state === 'submitting'}
                    disabled={!canDecide}
                    onClick={() => onDecideApproval?.(prompt.id, false)}
                  >
                    {t('playground.approvalDeny')}
                  </Button>
                  <Button
                    size="sm"
                    variant="primary"
                    icon={Check}
                    loading={state === 'submitting'}
                    disabled={!canDecide}
                    onClick={() => onDecideApproval?.(prompt.id, true)}
                  >
                    {t('playground.approvalApprove')}
                  </Button>
                </>
              )}
            </div>
          </div>
          {error && <div className="mt-1.5 text-xs text-danger">{error}</div>}
        </div>
      </div>
    )
  }

  const mine = m.role === 'you'
  const failed = m.role === 'error'
  const hasAttachments = (m.attachments?.length ?? 0) > 0
  // An attachment-only reply has no text but is not a missing reply.
  const waiting = m.role === 'agent' && m.text === '' && !m.note && !hasAttachments
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
        {(waiting || m.text !== '') && (
          <div className={cx('mt-0.5 select-text whitespace-pre-wrap break-words text-base leading-relaxed', failed && 'text-danger', waiting && 'text-muted')}>
            {waiting ? m.pending ? <span className="animate-pulse">…</span> : t('playground.noReply') : m.text}
          </div>
        )}
        {hasAttachments && (
          <div className="mt-1.5 flex flex-col gap-1.5">
            {m.attachments!.map((att, idx) => (
              <AttachmentView key={`${att.handle ?? att.url ?? idx}-${idx}`} part={att} loadMedia={loadMedia} />
            ))}
          </div>
        )}
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
    /** Optional pending inbound attachments for multimodal testing. */
    attachments?: MediaPart[]
    onAttachments?: (next: MediaPart[]) => void
    onSend: () => void
    onStop: () => void
    /** Optional operator approval decision callback for inline needs_confirm prompts. */
    onDecideApproval?: (id: string, approve: boolean) => void
    /** Optional loader for authenticated media handles (Studio preview/download). */
    loadMedia?: MediaLoader
    /** Extra header buttons, before Stop. */
    actions?: ReactNode
    /** The line beside Send; defaults to the send shortcut. */
    hint?: string
    /** What the empty transcript suggests. */
    emptyHint?: string
  }
>(function ChatPane({ agent, thread, status, messages, sending, stopping, draft, onDraft, attachments, onAttachments, onSend, onStop, onDecideApproval, loadMedia, actions, hint, emptyHint }, handle) {
  const { ref, onScroll, pin } = useStickToBottom<HTMLDivElement>(messages, agent + '\n' + thread)
  const input = useRef<HTMLTextAreaElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  useImperativeHandle(handle, () => ({ pinToEnd: pin, focusComposer: () => input.current?.focus() }), [pin])

  const pickFiles = async (files: FileList | null) => {
    if (!files || !onAttachments) return
    const next = [...(attachments ?? [])]
    const maxFileBytes = 10 * 1024 * 1024
    for (const file of Array.from(files)) {
      if (file.size > maxFileBytes) continue
      const dataURL = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => resolve(String(reader.result ?? ''))
        reader.onerror = () => reject(reader.error)
        reader.readAsDataURL(file)
      }).catch(() => '')
      const comma = dataURL.indexOf(',')
      if (comma < 0) continue
      const mime = file.type || 'application/octet-stream'
      const type = mime.startsWith('image/') ? 'image' : mime.startsWith('audio/') ? 'audio' : mime.startsWith('video/') ? 'video' : 'file'
      next.push({ type, mime, data: dataURL.slice(comma + 1), name: file.name, size: file.size })
    }
    onAttachments(next)
  }

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
              <Bubble key={m.id} m={m} loadMedia={loadMedia} onDecideApproval={onDecideApproval} />
            ))}
          </div>
        )}
      </div>

      <div className="shrink-0 border-t border-line p-2.5">
        <div className="rounded-ctl border border-line bg-bg transition-colors duration-100 focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/20 hover:border-faint focus-within:hover:border-accent">
          {attachments && attachments.length > 0 && onAttachments && (
            <div className="flex flex-wrap gap-1.5 border-b border-line px-2.5 py-1.5">
              {attachments.map((att, idx) => (
                <span key={`${att.name ?? idx}-${idx}`} className="inline-flex items-center gap-1 rounded border border-line bg-surface px-2 py-0.5 font-mono text-xs text-text">
                  <Paperclip className="h-3 w-3 text-muted" />
                  <span className="max-w-[160px] truncate">{att.name || att.type}</span>
                  <button
                    type="button"
                    className="text-muted hover:text-danger"
                    aria-label={t('playground.removeAttachment')}
                    onClick={() => onAttachments(attachments.filter((_, i) => i !== idx))}
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              ))}
            </div>
          )}
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
            {onAttachments && (
              <>
                <input
                  ref={fileInput}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={(e) => {
                    void pickFiles(e.target.files)
                    e.target.value = ''
                  }}
                />
                <IconButton icon={Paperclip} label={t('playground.attachMedia')} onClick={() => fileInput.current?.click()} disabled={sending} />
              </>
            )}
            <span className="min-w-0 flex-1 truncate text-xs text-faint">{hint ?? t('playground.sendHint', { mod: MOD })}</span>
            <Button
              variant="primary"
              icon={Send}
              className="w-[92px]"
              disabled={sending || (draft.trim() === '' && !(attachments && attachments.length > 0))}
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
