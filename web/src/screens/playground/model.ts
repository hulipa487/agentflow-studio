// The Playground's data model and the pure rules ported from the desktop
// app's playground.go: trace filters, row tones, cost and detail lines.
import type { ApprovalPrompt, MediaPart, TraceEvent } from '@/api/types'
import type { Tone } from '@/components/ui'
import { t, type Params } from '@/i18n'

export interface Status {
  /** Text shown as is: already translated, or the runtime's own words. */
  text?: string
  /** A message key, read when drawn so a language change re-reads it. */
  key?: string
  params?: Params
  tone: Tone
}

/** What a status says, in the current language. */
export function statusText(s: Status): string {
  return s.key ? t(s.key, s.params) : (s.text ?? '')
}

export const READY: Status = { key: 'playground.statusReady', tone: 'success' }

// The filter values are identifiers (the builder's run panel names them too);
// FILTER_LABEL_KEY says what each one shows.
export const FILTERS = ['All events', 'Ops only', 'Errors only'] as const
export type TraceFilter = (typeof FILTERS)[number]

export const FILTER_LABEL_KEY: Record<TraceFilter, string> = {
  'All events': 'playground.filterAll',
  'Ops only': 'playground.filterOps',
  'Errors only': 'playground.filterErrors',
}

export type Role = 'you' | 'agent' | 'approval' | 'error'

export interface MessageApproval {
  prompt: ApprovalPrompt
  state: 'pending' | 'submitting' | 'approved' | 'denied' | 'error'
  error?: string
}

export interface Message {
  id: number
  role: Role
  text: string
  at: Date
  /** An agent bubble still waiting for its first reply. */
  pending?: boolean
  /** Rebuilt from trace payloads when an old thread is reopened, not typed in this tab. */
  fromTrace?: boolean
  /** A delivery status other than "delivered", or a note about parts not shown. */
  note?: string
  /** Media attachments returned with an agent reply. */
  attachments?: MediaPart[]
  /** Runtime-owned approval card streamed mid-turn. */
  approval?: MessageApproval
}

let lastMessageID = 0

export function messageID(): number {
  return ++lastMessageID
}

// The wire omits empty fields (Go's omitempty), so every optional field is
// read defensively even though the type declares it.

export function matchesFilter(ev: TraceEvent, filter: TraceFilter): boolean {
  if (filter === 'Ops only') return !!ev.op
  if (filter === 'Errors only') return !!ev.err || ev.kind === 'error'
  return true
}

export function traceTone(ev: TraceEvent): Tone {
  if (!ev.ok && ev.err) return 'danger'
  if (ev.kind === 'turn.start' || ev.kind === 'turn.end') return 'neutral'
  if (ev.kind === 'reply') return 'success'
  return 'accent'
}

/** "842ms · 412→96 tok", or "" when the event cost nothing. */
export function traceCost(ev: TraceEvent): string {
  const parts: string[] = []
  if ((ev.duration_ms ?? 0) > 0) parts.push(`${ev.duration_ms}ms`)
  const tin = ev.tokens_in || 0
  const tout = ev.tokens_out || 0
  if (tin + tout > 0) parts.push(t('playground.costTokens', { in: tin, out: tout }))
  return parts.join(' · ')
}

/** The op, tool, model and tool-call count, in that order. */
export function traceDetail(ev: TraceEvent): string {
  const parts = [ev.op, ev.tool, ev.model].filter(Boolean)
  if ((ev.tool_calls ?? 0) > 0) parts.push(t('playground.toolCalls', { n: ev.tool_calls ?? 0 }))
  return parts.join('  ')
}

export function traceTokens(events: TraceEvent[]): number {
  let n = 0
  for (const ev of events) n += (ev.tokens_in || 0) + (ev.tokens_out || 0)
  return n
}

export function eventKey(ev: TraceEvent): string {
  return `${ev.seq}@${ev.ts}`
}

/**
 * Appends a streamed event unless the list already holds it: a trace refresh
 * during a running turn can return an event the stream delivers a moment later.
 */
export function appendEvent(events: TraceEvent[] | null, ev: TraceEvent): TraceEvent[] {
  const list = events ?? []
  for (let i = list.length - 1; i >= 0 && i >= list.length - 64; i--) {
    if (list[i].seq === ev.seq && list[i].ts === ev.ts) return list
  }
  return [...list, ev]
}

/** Go's appendReply: the first reply fills the bubble, later ones follow a blank line. */
export function joinReply(current: string, text: string): string {
  return current === '' ? text : current + '\n\n' + text
}

function eventTime(ev: TraceEvent): Date {
  const at = new Date(ev.ts)
  return Number.isNaN(at.getTime()) ? new Date() : at
}

/**
 * What a reopened thread's trace says was said. Only deployments that capture
 * payloads (runtime.studio.trace_payloads) put text on turn.start and reply
 * events; without it this is empty and the thread shows its trace alone.
 */
export function transcriptFromTrace(events: TraceEvent[]): Message[] {
  const out: Message[] = []
  let replyTurn = ''
  for (const ev of events) {
    if (!ev.text) continue
    if (ev.kind === 'turn.start') {
      out.push({ id: messageID(), role: 'you', text: ev.text, at: eventTime(ev), fromTrace: true })
      replyTurn = ''
    } else if (ev.kind === 'reply' && ev.ok) {
      const last = out[out.length - 1]
      if (last && last.role === 'agent' && replyTurn !== '' && replyTurn === (ev.turn || '')) {
        last.text = joinReply(last.text, ev.text)
      } else {
        out.push({ id: messageID(), role: 'agent', text: ev.text, at: eventTime(ev), fromTrace: true })
        replyTurn = ev.turn || ''
      }
    }
  }
  return out
}
