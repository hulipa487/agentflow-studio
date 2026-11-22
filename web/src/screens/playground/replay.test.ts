import { describe, expect, it } from 'vitest'
import type { TraceEvent } from '@/api/types'
import type { Message } from './model'
import { compareRuns, sliceTraceAt, starterScenarioYAML, traceToScenarioYAML, type PlaygroundRun } from './replay'

const sampleEvents: TraceEvent[] = [
  { seq: 1, ts: '2026-11-22T10:00:00.000Z', agent: 'helper', session: 's1', turn: 't1', kind: 'turn.start', ok: true, text: 'hello tool' },
  { seq: 2, ts: '2026-11-22T10:00:00.050Z', agent: 'helper', session: 's1', turn: 't1', kind: 'op.end', op: 'llm.chat', model: 'gpt-test', ok: true, duration_ms: 50, tokens_in: 10, tokens_out: 4, text: '{"text":""}', tool_calls: 1 },
  { seq: 3, ts: '2026-11-22T10:00:00.120Z', agent: 'helper', session: 's1', turn: 't1', kind: 'op.end', op: 'tools.run', tool: 'weather', ok: true, duration_ms: 70 },
  { seq: 4, ts: '2026-11-22T10:00:00.200Z', agent: 'helper', session: 's1', turn: 't1', kind: 'op.end', op: 'llm.chat', model: 'gpt-test', ok: true, duration_ms: 80, tokens_in: 20, tokens_out: 12, tokens_reasoning: 3, text: '{"text":"Sunny 22C"}' },
  { seq: 5, ts: '2026-11-22T10:00:00.210Z', agent: 'helper', session: 's1', turn: 't1', kind: 'reply', ok: true, text: 'Sunny 22C' },
  { seq: 6, ts: '2026-11-22T10:00:00.220Z', agent: 'helper', session: 's1', turn: 't1', kind: 'turn.end', ok: true },
]

describe('playground replay helpers', () => {
  it('slices trace state accurately at any scrub step', () => {
    const mid = sliceTraceAt(sampleEvents, 2)
    expect(mid.step).toBe(2)
    expect(mid.total).toBe(6)
    expect(mid.elapsedMs).toBe(120)
    expect(mid.tokensIn).toBe(10)
    expect(mid.tokensOut).toBe(4)
    expect(mid.opsSoFar).toBe(2)
    expect(mid.repliesSoFar).toEqual([])
    expect(mid.visibleEvents).toHaveLength(3)

    const end = sliceTraceAt(sampleEvents, 99)
    expect(end.step).toBe(5)
    expect(end.tokensIn).toBe(30)
    expect(end.tokensOut).toBe(16)
    expect(end.tokensReasoning).toBe(3)
    expect(end.repliesSoFar).toEqual(['Sunny 22C'])
  })

  it('exports a valid scenario YAML from live messages and trace events', () => {
    const msgs: Message[] = [
      { id: 1, role: 'you', text: 'hello tool', at: new Date() },
      { id: 2, role: 'agent', text: 'Sunny 22C', at: new Date() },
    ]
    const yaml = traceToScenarioYAML('helper', msgs, sampleEvents)
    expect(yaml).toContain('agent: helper')
    expect(yaml).toContain('text: "hello tool"')
    expect(yaml).toContain('Sunny 22C')
    expect(yaml).toContain('weather')
    expect(yaml).toContain('tools.run')
    expect(starterScenarioYAML('demo')).toContain('agent: demo')
  })

  it('compares baseline and candidate runs with aligned op diff and deltas', () => {
    const base: PlaygroundRun = {
      id: 'b',
      label: 'base',
      source: 'live',
      agent: 'helper',
      createdAt: '2026-11-22T10:00:00Z',
      ok: true,
      durationMs: 220,
      sends: ['Sunny 22C'],
      ops: ['llm.chat', 'tools.run', 'llm.chat'],
      events: sampleEvents,
    }
    const cand: PlaygroundRun = {
      id: 'c',
      label: 'cand',
      source: 'replay',
      agent: 'helper',
      createdAt: '2026-11-22T10:01:00Z',
      ok: true,
      durationMs: 120,
      sends: ['Clear 22C'],
      ops: ['llm.chat'],
      events: [sampleEvents[0], { ...sampleEvents[1], duration_ms: 40, tokens_out: 6 }, sampleEvents[4], sampleEvents[5]],
    }
    const cmp = compareRuns(base, cand)
    expect(cmp.delta.durationMs).toBe(-100)
    expect(cmp.delta.opCount).toBe(-2)
    expect(cmp.replyDiff).toEqual([{ index: 0, base: 'Sunny 22C', cand: 'Clear 22C', same: false }])
    expect(cmp.opDiff.filter((r) => r.status === 'removed')).toHaveLength(2)
    expect(cmp.opDiff.some((r) => r.status === 'changed' || r.status === 'same')).toBe(true)
  })
})
