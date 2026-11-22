// Pure helpers for time-travel scrubbing, `agentflow loop test` scenario
// export, and side-by-side run comparison in the Playground.
import type { LoopTestAssertion, TraceEvent } from '@/api/types'
import type { Message } from './model'

export interface PlaygroundRun {
  id: string
  label: string
  source: 'live' | 'replay'
  agent: string
  createdAt: string
  ok: boolean
  durationMs: number
  sends: string[]
  ops: string[]
  events: TraceEvent[]
  assertions?: LoopTestAssertion[]
  yaml?: string
}

export interface TimeTravelSlice {
  step: number
  total: number
  current: TraceEvent | null
  elapsedMs: number
  tokensIn: number
  tokensOut: number
  tokensReasoning: number
  errorCount: number
  safetyCount: number
  opsSoFar: number
  repliesSoFar: string[]
  visibleEvents: TraceEvent[]
}

export interface RunMetricsSummary {
  ok: boolean
  durationMs: number
  tokensIn: number
  tokensOut: number
  tokensReasoning: number
  opCount: number
  errorCount: number
  sendCount: number
}

export interface RunReplyDiffRow {
  index: number
  base: string
  cand: string
  same: boolean
}

export interface RunOpDiffRow {
  index: number
  status: 'same' | 'changed' | 'added' | 'removed'
  base?: TraceEvent
  cand?: TraceEvent
}

export interface RunComparison {
  base: RunMetricsSummary
  cand: RunMetricsSummary
  delta: {
    durationMs: number
    tokensIn: number
    tokensOut: number
    tokensReasoning: number
    opCount: number
    errorCount: number
    sendCount: number
  }
  replyDiff: RunReplyDiffRow[]
  opDiff: RunOpDiffRow[]
}

export function traceDurationMs(events: TraceEvent[], explicitMs?: number): number {
  if (explicitMs !== undefined && explicitMs > 0) return explicitMs
  if (events.length >= 2) {
    const start = Date.parse(events[0].ts)
    const end = Date.parse(events[events.length - 1].ts)
    if (!Number.isNaN(start) && !Number.isNaN(end) && end >= start) {
      const span = end - start
      if (span > 0) return span
    }
  }
  let sum = 0
  for (const ev of events) {
    if (ev.kind === 'op.end' && (ev.duration_ms ?? 0) > 0) sum += ev.duration_ms ?? 0
  }
  return sum
}

export function sliceTraceAt(events: TraceEvent[], requestedStep: number): TimeTravelSlice {
  const total = events.length
  if (total === 0) {
    return {
      step: 0,
      total: 0,
      current: null,
      elapsedMs: 0,
      tokensIn: 0,
      tokensOut: 0,
      tokensReasoning: 0,
      errorCount: 0,
      safetyCount: 0,
      opsSoFar: 0,
      repliesSoFar: [],
      visibleEvents: [],
    }
  }
  const step = Math.max(0, Math.min(requestedStep, total - 1))
  const visible = events.slice(0, step + 1)
  const firstTs = Date.parse(visible[0].ts)
  const curTs = Date.parse(visible[visible.length - 1].ts)
  let elapsedMs = !Number.isNaN(firstTs) && !Number.isNaN(curTs) && curTs >= firstTs ? curTs - firstTs : 0
  let opMsSum = 0
  let tokensIn = 0
  let tokensOut = 0
  let tokensReasoning = 0
  let errorCount = 0
  let safetyCount = 0
  let opsSoFar = 0
  const repliesSoFar: string[] = []

  for (const ev of visible) {
    tokensIn += ev.tokens_in || 0
    tokensOut += ev.tokens_out || 0
    tokensReasoning += ev.tokens_reasoning || 0
    if (ev.kind === 'error' || !ev.ok) {
      if (ev.err || ev.kind === 'error') errorCount++
    }
    if (ev.kind === 'safety') safetyCount++
    if (ev.kind === 'op.end') {
      opsSoFar++
      opMsSum += ev.duration_ms || 0
    }
    if (ev.kind === 'reply' && ev.ok && ev.text) {
      repliesSoFar.push(ev.text)
    }
  }
  if (elapsedMs === 0 && opMsSum > 0) elapsedMs = opMsSum

  return {
    step,
    total,
    current: visible[visible.length - 1],
    elapsedMs,
    tokensIn,
    tokensOut,
    tokensReasoning,
    errorCount,
    safetyCount,
    opsSoFar,
    repliesSoFar,
    visibleEvents: visible,
  }
}

export function starterScenarioYAML(agent: string): string {
  const target = agent.trim() || 'assistant'
  return [
    `agent: ${target}`,
    `messages:`,
    `  - from: tester`,
    `    text: "Hello!"`,
    `llm:`,
    `  - match: "Hello"`,
    `    text: "Hi! How can I help today?"`,
    `    usage:`,
    `      input: 16`,
    `      output: 9`,
    `expect_contains:`,
    `  - "How can I help"`,
    `expect_ops:`,
    `  - llm.chat`,
    '',
  ].join('\n')
}

function yamlStr(raw: string): string {
  return JSON.stringify(raw)
}

function extractLLMReplyText(ev: TraceEvent): string {
  if (!ev.text) return ''
  try {
    const parsed = JSON.parse(ev.text) as { text?: unknown }
    if (typeof parsed?.text === 'string') return parsed.text
  } catch {
    /* non-JSON text passes through */
  }
  return ev.text
}

function matchSnippet(text: string): string {
  const line = text.trim().split(/\r?\n/)[0]?.trim() ?? ''
  return line.length > 48 ? line.slice(0, 48).trim() : line
}

export function traceToScenarioYAML(agent: string, messages: Message[], events: TraceEvent[]): string {
  const target = agent.trim() || events.find((e) => e.agent)?.agent || 'assistant'

  const userTexts: string[] = []
  for (const m of messages) {
    if (m.role === 'you' && m.text.trim() !== '') userTexts.push(m.text)
  }
  if (userTexts.length === 0) {
    for (const ev of events) {
      if (ev.kind === 'turn.start' && ev.text?.trim()) userTexts.push(ev.text)
    }
  }
  if (userTexts.length === 0) userTexts.push('hello')

  const replyTexts: string[] = []
  for (const m of messages) {
    if (m.role === 'agent' && m.text.trim() !== '' && !m.pending) replyTexts.push(m.text)
  }
  if (replyTexts.length === 0) {
    for (const ev of events) {
      if (ev.kind === 'reply' && ev.ok && ev.text?.trim()) replyTexts.push(ev.text)
    }
  }

  const llmEvents = events.filter((ev) => ev.kind === 'op.end' && ev.op === 'llm.chat' && ev.ok)
  const llmMocks: Array<{ match: string; text: string; inTok?: number; outTok?: number; reasTok?: number }> = []
  const count = Math.max(llmEvents.length, replyTexts.length)
  for (let i = 0; i < count; i++) {
    const ev = llmEvents[i]
    const evText = ev ? extractLLMReplyText(ev) : ''
    const txt = evText || replyTexts[i] || 'scenario reply'
    const match = matchSnippet(userTexts[Math.min(i, userTexts.length - 1)] ?? '')
    llmMocks.push({
      match,
      text: txt,
      inTok: ev?.tokens_in,
      outTok: ev?.tokens_out,
      reasTok: ev?.tokens_reasoning,
    })
  }

  const toolsUsed = Array.from(
    new Set(events.filter((ev) => ev.kind === 'op.end' && ev.op === 'tools.run' && ev.tool).map((ev) => ev.tool!)),
  )
  const opsUsed = events.filter((ev) => ev.kind === 'op.end' && ev.op).map((ev) => ev.op!)

  const lines: string[] = [`agent: ${target}`, `messages:`]
  for (const u of userTexts) {
    lines.push(`  - from: tester`)
    lines.push(`    text: ${yamlStr(u)}`)
  }
  if (llmMocks.length > 0) {
    lines.push(`llm:`)
    for (const m of llmMocks) {
      lines.push(`  - match: ${yamlStr(m.match)}`)
      lines.push(`    text: ${yamlStr(m.text)}`)
      if ((m.inTok || 0) + (m.outTok || 0) + (m.reasTok || 0) > 0) {
        lines.push(`    usage:`)
        if ((m.inTok || 0) > 0) lines.push(`      input: ${m.inTok}`)
        if ((m.outTok || 0) > 0) lines.push(`      output: ${m.outTok}`)
        if ((m.reasTok || 0) > 0) lines.push(`      reasoning: ${m.reasTok}`)
      }
    }
  }
  if (toolsUsed.length > 0) {
    lines.push(`tools:`)
    for (const name of toolsUsed) {
      lines.push(`  ${JSON.stringify(name)}:`)
      lines.push(`    ok: true`)
    }
  }
  if (replyTexts.length > 0) {
    lines.push(`expect_contains:`)
    for (const r of replyTexts) {
      lines.push(`  - ${yamlStr(matchSnippet(r) || r)}`)
    }
  }
  if (opsUsed.length > 0) {
    lines.push(`expect_ops:`)
    for (const op of opsUsed) {
      lines.push(`  - ${op}`)
    }
  }
  lines.push('')
  return lines.join('\n')
}

export function buildRunFromLiveThread(
  agent: string,
  thread: string,
  messages: Message[],
  events: TraceEvent[],
  label?: string,
): PlaygroundRun {
  const sends: string[] = []
  for (const m of messages) {
    if (m.role === 'agent' && !m.pending && m.text !== '') sends.push(m.text)
  }
  if (sends.length === 0) {
    for (const ev of events) {
      if (ev.kind === 'reply' && ev.ok && ev.text) sends.push(ev.text)
    }
  }
  const ops = events.filter((ev) => ev.kind === 'op.end' && ev.op).map((ev) => ev.op!)
  const hasErr = events.some((ev) => ev.kind === 'error' || (!ev.ok && !!ev.err))
  return {
    id: `live:${agent}:${thread}:${events.length}:${Date.now()}`,
    label: label ?? `${agent} · ${thread.slice(0, 8)}`,
    source: 'live',
    agent,
    createdAt: new Date().toISOString(),
    ok: !hasErr,
    durationMs: traceDurationMs(events),
    sends,
    ops,
    events,
    yaml: traceToScenarioYAML(agent, messages, events),
  }
}

export function summarizeRun(run: PlaygroundRun): RunMetricsSummary {
  const s = sliceTraceAt(run.events, Math.max(0, run.events.length - 1))
  return {
    ok: run.ok,
    durationMs: run.durationMs > 0 ? run.durationMs : s.elapsedMs,
    tokensIn: s.tokensIn,
    tokensOut: s.tokensOut,
    tokensReasoning: s.tokensReasoning,
    opCount: run.ops.length > 0 ? run.ops.length : s.opsSoFar,
    errorCount: s.errorCount,
    sendCount: run.sends.length > 0 ? run.sends.length : s.repliesSoFar.length,
  }
}

function timelineEvents(events: TraceEvent[]): TraceEvent[] {
  return events.filter((ev) => ev.kind === 'op.end' || ev.kind === 'safety' || ev.kind === 'error' || ev.kind === 'reply')
}

function sigOf(ev: TraceEvent): string {
  return `${ev.kind}|${ev.op ?? ''}|${ev.tool ?? ''}`
}

function sameStep(a: TraceEvent, b: TraceEvent): boolean {
  return (
    a.kind === b.kind &&
    (a.op ?? '') === (b.op ?? '') &&
    (a.tool ?? '') === (b.tool ?? '') &&
    (a.model ?? '') === (b.model ?? '') &&
    a.ok === b.ok &&
    (a.err ?? '') === (b.err ?? '')
  )
}

export function compareRuns(baseRun: PlaygroundRun, candRun: PlaygroundRun): RunComparison {
  const base = summarizeRun(baseRun)
  const cand = summarizeRun(candRun)
  const maxReplies = Math.max(baseRun.sends.length, candRun.sends.length)
  const replyDiff: RunReplyDiffRow[] = []
  for (let i = 0; i < maxReplies; i++) {
    const a = baseRun.sends[i] ?? ''
    const b = candRun.sends[i] ?? ''
    replyDiff.push({ index: i, base: a, cand: b, same: a.trim() === b.trim() })
  }

  const aEv = timelineEvents(baseRun.events)
  const bEv = timelineEvents(candRun.events)
  const n = aEv.length
  const m = bEv.length
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      if (sigOf(aEv[i]) === sigOf(bEv[j])) {
        dp[i][j] = 1 + dp[i + 1][j + 1]
      } else {
        dp[i][j] = Math.max(dp[i + 1][j], dp[i][j + 1])
      }
    }
  }

  const opDiff: RunOpDiffRow[] = []
  let i = 0
  let j = 0
  let idx = 0
  while (i < n && j < m) {
    if (sigOf(aEv[i]) === sigOf(bEv[j])) {
      opDiff.push({
        index: idx++,
        status: sameStep(aEv[i], bEv[j]) ? 'same' : 'changed',
        base: aEv[i],
        cand: bEv[j],
      })
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      opDiff.push({ index: idx++, status: 'removed', base: aEv[i++] })
    } else {
      opDiff.push({ index: idx++, status: 'added', cand: bEv[j++] })
    }
  }
  while (i < n) opDiff.push({ index: idx++, status: 'removed', base: aEv[i++] })
  while (j < m) opDiff.push({ index: idx++, status: 'added', cand: bEv[j++] })

  return {
    base,
    cand,
    delta: {
      durationMs: cand.durationMs - base.durationMs,
      tokensIn: cand.tokensIn - base.tokensIn,
      tokensOut: cand.tokensOut - base.tokensOut,
      tokensReasoning: cand.tokensReasoning - base.tokensReasoning,
      opCount: cand.opCount - base.opCount,
      errorCount: cand.errorCount - base.errorCount,
      sendCount: cand.sendCount - base.sendCount,
    },
    replyDiff,
    opDiff,
  }
}
