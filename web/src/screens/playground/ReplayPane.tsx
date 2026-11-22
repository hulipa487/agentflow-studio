// Replay & Eval pane: runs an `agentflow loop test` YAML scenario against the
// runtime's deterministic scenario runner and shows assertion + trace results.
import { useState } from 'react'
import { CheckCircle2, FileCode2, Play, Sparkles, XCircle } from 'lucide-react'
import { errorMessage, type Client } from '@/api/client'
import type { LoopTestResult } from '@/api/types'
import { Banner, Button, Chip, cx } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { t } from '@/i18n'
import type { TraceFilter } from './model'
import { starterScenarioYAML, traceDurationMs, type PlaygroundRun } from './replay'
import { TracePane } from './TracePane'

export function ReplayPane({
  client,
  agent,
  yaml,
  onYaml,
  hasThreadEvents,
  onSeedFromThread,
  onRunRecorded,
}: {
  client: Client
  agent: string
  yaml: string
  onYaml: (next: string) => void
  hasThreadEvents: boolean
  onSeedFromThread: () => void
  onRunRecorded: (run: PlaygroundRun) => void
}) {
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<LoopTestResult | null>(null)
  const [traceFilter, setTraceFilter] = useState<TraceFilter>('All events')

  const onRun = async () => {
    if (running) return
    setRunning(true)
    setError('')
    try {
      const res = await client.runLoopTest({ yaml, agent })
      setResult(res)
      const now = new Date()
      onRunRecorded({
        id: `replay:${now.getTime()}`,
        label: `${res.agent || agent} (${now.toLocaleTimeString()})`,
        source: 'replay',
        agent: res.agent || agent,
        createdAt: now.toISOString(),
        ok: res.ok,
        durationMs: traceDurationMs(res.events, res.duration_ms),
        sends: res.sends ?? [],
        ops: res.ops ?? [],
        events: res.events ?? [],
        assertions: res.assertions ?? [],
        yaml,
      })
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-line px-2.5 py-2">
        <Button size="sm" icon={Sparkles} disabled={!hasThreadEvents || running} onClick={onSeedFromThread}>
          {t('playground.replayFromThread')}
        </Button>
        <Button size="sm" icon={FileCode2} disabled={running} onClick={() => onYaml(starterScenarioYAML(agent))}>
          {t('playground.replayTemplate')}
        </Button>
        <div className="flex-1" />
        <Button variant="primary" size="sm" icon={Play} loading={running} onClick={() => void onRun()}>
          {running ? t('playground.replayRunning') : t('playground.replayRun')}
        </Button>
      </div>

      <div className="shrink-0 border-b border-line p-2">
        <CodeEditor value={yaml} onChange={onYaml} language="yaml" lineWrapping className="h-[190px]" />
      </div>

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-2.5">
        {error && <Banner tone="danger">{error}</Banner>}

        {!result && !error && (
          <p className="py-4 text-center text-sm leading-relaxed text-muted">{t('playground.replayNoResult')}</p>
        )}

        {result && (
          <div className="flex flex-col gap-3">
            <Banner tone={result.ok ? 'success' : 'danger'}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-semibold">
                  {result.ok ? t('playground.replayPass', { n: result.assertions.length }) : t('playground.replayFail')}
                </span>
                <div className="flex items-center gap-1.5">
                  <Chip>{t('playground.replayDuration', { ms: result.duration_ms })}</Chip>
                  <Chip>{t('playground.timeTravelOps', { n: result.ops.length })}</Chip>
                </div>
              </div>
              {result.error && <div className="mt-1 font-mono text-xs">{result.error}</div>}
            </Banner>

            {result.assertions.length > 0 && (
              <div className="flex flex-col gap-1">
                <div className="text-xs font-semibold text-faint">{t('playground.replayAssertions')}</div>
                {result.assertions.map((a, idx) => (
                  <div
                    key={`${a.kind}-${a.index}-${idx}`}
                    className={cx(
                      'flex items-start gap-2 rounded-ctl border px-2 py-1.5 text-xs',
                      a.ok ? 'border-line bg-bg/60 text-text' : 'border-danger/40 bg-tone-danger text-danger',
                    )}
                  >
                    {a.ok ? <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-success" /> : <XCircle className="mt-0.5 size-3.5 shrink-0 text-danger" />}
                    <div className="min-w-0 flex-1 font-mono">
                      <div className="truncate font-semibold">{`${a.kind}: ${a.expected}`}</div>
                      {!a.ok && (
                        <div className="mt-0.5 whitespace-pre-wrap break-words opacity-90">
                          {t('playground.replayExpectedActual', { expected: a.expected, actual: a.actual || '∅' })}
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {result.sends.length > 0 && (
              <div className="flex flex-col gap-1">
                <div className="text-xs font-semibold text-faint">{t('playground.replaySends')}</div>
                {result.sends.map((s, idx) => (
                  <pre key={idx} className="whitespace-pre-wrap break-words rounded-ctl border border-line bg-bg/60 px-2.5 py-1.5 font-sans text-xs text-text">
                    {s}
                  </pre>
                ))}
              </div>
            )}

            {result.events.length > 0 && (
              <div className="flex min-h-[240px] flex-col rounded-ctl border border-line bg-surface">
                <div className="border-b border-line px-2.5 py-1.5 text-xs font-semibold text-faint">{t('playground.replayTrace')}</div>
                <TracePane
                  embedded
                  events={result.events}
                  scope={`replay:${result.duration_ms}:${result.events.length}`}
                  filter={traceFilter}
                  onFilter={setTraceFilter}
                  onRefresh={() => void onRun()}
                  refreshing={running}
                />
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
