// Event Center: inspect every inbound/outbound channel event — why a message
// was answered, ignored (session.skip), dropped (allowlist/safety), or failed —
// and drill directly into its session execution trace.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Pause, Play, Radio, RotateCw } from 'lucide-react'
import { errorMessage, featureOff } from '@/api/client'
import type { StudioEvent, StudioEventOutcome, TraceEvent } from '@/api/types'
import {
  Banner,
  Button,
  Chip,
  EmptyState,
  Input,
  Page,
  Panel,
  Select,
  StatusChip,
  cx,
  type Tone,
} from '@/components/ui'
import { t } from '@/i18n'
import { useScreenActions } from '@/lib/hooks'
import { useClient } from '@/state/session'

const OUTCOME_TONE: Record<StudioEventOutcome, Tone> = {
  answered: 'success',
  ignored: 'warning',
  dropped: 'danger',
  failed: 'danger',
  received: 'neutral',
}

const TRACE_KIND_TONE: Record<string, Tone> = {
  'turn.skip': 'warning',
  error: 'danger',
  safety: 'warning',
  reply: 'success',
  'op.end': 'neutral',
}

function formatTs(unixSec: number): string {
  if (!unixSec) return '—'
  try {
    return new Date(unixSec * 1000).toLocaleTimeString()
  } catch {
    return String(unixSec)
  }
}

function eventReasonText(ev: StudioEvent): string {
  if (ev.err && ev.text && ev.err !== ev.text) {
    return `${ev.err} — ${ev.text}`
  }
  if (ev.err) return ev.err
  if (ev.text) return ev.text
  if (ev.text_bytes && ev.text_bytes > 0) return t('events.redactedBytes', { n: ev.text_bytes })
  return '—'
}

export default function Events() {
  const client = useClient()
  const [channel, setChannel] = useState('')
  const [agent, setAgent] = useState('')
  const [chat, setChat] = useState('')
  const [status, setStatus] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(true)

  const [events, setEvents] = useState<StudioEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<unknown>(null)

  const [selected, setSelected] = useState<StudioEvent | null>(null)
  const [trace, setTrace] = useState<TraceEvent[]>([])
  const [traceLoading, setTraceLoading] = useState(false)
  const [traceError, setTraceError] = useState('')

  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const loadEvents = useCallback(
    async (silent = false) => {
      if (silent) setRefreshing(true)
      else setLoading(true)
      const ac = new AbortController()
      try {
        const list = await client.studioEvents(
          {
            channel: channel.trim() || undefined,
            agent: agent.trim() || undefined,
            chat: chat.trim() || undefined,
            status: status || undefined,
            limit: 100,
          },
          ac.signal,
        )
        if (!mounted.current) return
        setEvents(list)
        setError(null)
        setSelected((prev) => {
          if (!prev) return list[0] ?? null
          const match = list.find((e) => e.id === prev.id && e.direction === prev.direction && e.ts === prev.ts)
          return match ?? list[0] ?? null
        })
      } catch (err) {
        if (!mounted.current) return
        setError(err)
      } finally {
        if (mounted.current) {
          setLoading(false)
          setRefreshing(false)
        }
      }
    },
    [client, channel, agent, chat, status],
  )

  useEffect(() => {
    void loadEvents(false)
  }, [loadEvents])

  useEffect(() => {
    if (!autoRefresh) return
    const timer = window.setInterval(() => {
      void loadEvents(true)
    }, 4000)
    return () => window.clearInterval(timer)
  }, [autoRefresh, loadEvents])

  useScreenActions({ refresh: () => void loadEvents(true) })

  const selectedSession = selected?.session_id ?? ''
  const selectedHasTrace = selected?.has_trace ?? false

  useEffect(() => {
    if (!selectedSession || !selectedHasTrace) {
      setTrace([])
      setTraceError('')
      setTraceLoading(false)
      return
    }
    const ac = new AbortController()
    setTraceLoading(true)
    setTraceError('')
    client
      .studioSessionTrace(selectedSession, ac.signal)
      .then((res) => {
        if (!mounted.current) return
        setTrace(res.events ?? [])
      })
      .catch((err) => {
        if (!mounted.current || (err instanceof Error && err.name === 'AbortError')) return
        setTraceError(errorMessage(err))
      })
      .finally(() => {
        if (mounted.current) setTraceLoading(false)
      })
    return () => ac.abort()
  }, [client, selectedSession, selectedHasTrace])

  const off = featureOff(error)
  const outcomeOptions = [
    { value: '', label: t('events.outcome.all') },
    { value: 'answered', label: t('events.outcome.answered') },
    { value: 'ignored', label: t('events.outcome.ignored') },
    { value: 'dropped', label: t('events.outcome.dropped') },
    { value: 'failed', label: t('events.outcome.failed') },
    { value: 'received', label: t('events.outcome.received') },
  ]

  return (
    <Page
      title={t('events.title')}
      subtitle={t('events.subtitle')}
      scroll={false}
      actions={
        <>
          <Button
            icon={autoRefresh ? Pause : Play}
            onClick={() => setAutoRefresh((v) => !v)}
          >
            {autoRefresh ? t('events.pauseAuto') : t('events.autoRefresh')}
          </Button>
          <Button icon={RotateCw} loading={refreshing} onClick={() => void loadEvents(true)}>
            {t('common.refresh')}
          </Button>
        </>
      }
      banner={
        error && !off ? (
          <Banner tone="danger">{errorMessage(error)}</Banner>
        ) : null
      }
    >
      {off ? (
        <EmptyState title={t('common.notEnabled')} guidance={off} />
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          <div className="grid shrink-0 grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
            <Input
              aria-label={t('events.filter.channel')}
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              placeholder={t('events.filter.channelPlaceholder')}
              mono
            />
            <Input
              aria-label={t('events.filter.agent')}
              value={agent}
              onChange={(e) => setAgent(e.target.value)}
              placeholder={t('events.filter.agentPlaceholder')}
              mono
            />
            <Input
              aria-label={t('events.filter.chat')}
              value={chat}
              onChange={(e) => setChat(e.target.value)}
              placeholder={t('events.filter.chatPlaceholder')}
              mono
            />
            <Select
              aria-label={t('events.filter.outcome')}
              value={status}
              onChange={setStatus}
              options={outcomeOptions}
            />
          </div>

          <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-12">
            <Panel flush className="flex min-h-0 flex-col overflow-hidden lg:col-span-7">
              <div className="flex items-center justify-between border-b border-line px-3 py-2 text-xs font-semibold text-faint">
                <span>{t('events.listHeading')}</span>
                <span className="font-mono tabular-nums">{t('events.count', { n: events.length })}</span>
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto divide-y divide-line/60">
                {loading && events.length === 0 ? (
                  <div className="p-6 text-center text-sm text-muted">{t('events.loading')}</div>
                ) : events.length === 0 ? (
                  <div className="p-6">
                    <EmptyState title={t('events.emptyTitle')} guidance={t('events.emptyGuidance')} />
                  </div>
                ) : (
                  events.map((ev, idx) => {
                    const isSelected =
                      selected !== null &&
                      selected.id === ev.id &&
                      selected.direction === ev.direction &&
                      selected.ts === ev.ts
                    return (
                      <button
                        key={`${ev.id}-${ev.direction}-${ev.ts}-${idx}`}
                        type="button"
                        onClick={() => setSelected(ev)}
                        className={cx(
                          'w-full px-3 py-2.5 text-left transition-colors hover:bg-hover',
                          isSelected && 'bg-surface2',
                        )}
                      >
                        <div className="flex flex-wrap items-center gap-2">
                          <StatusChip tone={OUTCOME_TONE[ev.outcome] ?? 'neutral'}>
                            {t(`events.outcome.${ev.outcome}`)}
                          </StatusChip>
                          {ev.channel && <Chip tone="accent">{ev.channel}</Chip>}
                          {ev.agent && <span className="font-mono text-xs text-text">{ev.agent}</span>}
                          {ev.chat && <span className="font-mono text-xs text-muted">{ev.chat}</span>}
                          <span className="ml-auto font-mono text-xs text-faint tabular-nums">{formatTs(ev.ts)}</span>
                        </div>
                        <div className="mt-1 truncate text-sm text-text">{eventReasonText(ev)}</div>
                        <div className="mt-1 flex flex-wrap items-center gap-3 font-mono text-xs text-faint">
                          <span>{ev.direction}</span>
                          {ev.status && <span>{ev.status}</span>}
                          {ev.sender && <span>{ev.sender}</span>}
                          {ev.has_trace && (
                            <span className="inline-flex items-center gap-1 text-accent">
                              <Radio className="size-3" />
                              {t('events.hasTrace')}
                            </span>
                          )}
                        </div>
                      </button>
                    )
                  })
                )}
              </div>
            </Panel>

            <Panel flush className="flex min-h-0 flex-col overflow-hidden lg:col-span-5">
              <div className="flex items-center justify-between border-b border-line px-3 py-2 text-xs font-semibold text-faint">
                <span>{t('events.inspectorHeading')}</span>
                {selected?.session_id && (
                  <span className="truncate pl-2 font-mono text-xs text-muted" title={selected.session_id}>
                    {selected.session_id}
                  </span>
                )}
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto p-3">
                {!selected ? (
                  <div className="py-8 text-center text-sm text-muted">{t('events.selectPrompt')}</div>
                ) : (
                  <div className="flex flex-col gap-3">
                    <div className="rounded-ctl border border-line bg-surface2 p-2.5 text-xs">
                      <div className="flex flex-wrap items-center gap-2">
                        <StatusChip tone={OUTCOME_TONE[selected.outcome] ?? 'neutral'}>
                          {t(`events.outcome.${selected.outcome}`)}
                        </StatusChip>
                        {selected.status && <Chip>{selected.status}</Chip>}
                        {selected.channel && <Chip tone="accent">{selected.channel}</Chip>}
                        <span className="ml-auto font-mono text-faint">{formatTs(selected.ts)}</span>
                      </div>
                      <dl className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1">
                        <div>
                          <dt className="text-faint">{t('events.detail.agent')}</dt>
                          <dd className="font-mono text-text">{selected.agent || '—'}</dd>
                        </div>
                        <div>
                          <dt className="text-faint">{t('events.detail.chat')}</dt>
                          <dd className="font-mono text-text">{selected.chat || '—'}</dd>
                        </div>
                        <div>
                          <dt className="text-faint">{t('events.detail.sender')}</dt>
                          <dd className="font-mono text-text">{selected.sender || '—'}</dd>
                        </div>
                        <div>
                          <dt className="text-faint">{t('events.detail.direction')}</dt>
                          <dd className="font-mono text-text">{selected.direction || '—'}</dd>
                        </div>
                      </dl>
                      {(selected.text || selected.err) && (
                        <div className="mt-2 border-t border-line/60 pt-2">
                          <div className="text-faint">{t('events.detail.payload')}</div>
                          <pre className="mt-1 whitespace-pre-wrap break-words font-mono text-xs text-text">
                            {eventReasonText(selected)}
                          </pre>
                        </div>
                      )}
                    </div>

                    <div>
                      <div className="mb-1.5 text-xs font-semibold text-faint">{t('events.traceHeading')}</div>
                      {!selected.session_id || !selected.has_trace ? (
                        <div className="rounded-ctl border border-line/70 px-3 py-2 text-xs text-muted">
                          {t('events.noTraceForEvent')}
                        </div>
                      ) : traceLoading ? (
                        <div className="px-2 py-3 text-xs text-muted">{t('events.traceLoading')}</div>
                      ) : traceError ? (
                        <Banner tone="danger">{traceError}</Banner>
                      ) : trace.length === 0 ? (
                        <div className="rounded-ctl border border-line/70 px-3 py-2 text-xs text-muted">
                          {t('events.traceEmpty')}
                        </div>
                      ) : (
                        <div className="flex flex-col gap-1.5">
                          {trace.map((ev) => (
                            <div key={`${ev.seq}-${ev.kind}`} className="rounded-ctl border border-line/80 px-2.5 py-1.5 text-xs">
                              <div className="flex flex-wrap items-center gap-1.5">
                                <StatusChip tone={TRACE_KIND_TONE[ev.kind] ?? (ev.ok ? 'neutral' : 'danger')}>
                                  {ev.kind}
                                </StatusChip>
                                {ev.op && <Chip>{ev.op}</Chip>}
                                {ev.tool && <Chip tone="accent">{ev.tool}</Chip>}
                                {typeof ev.duration_ms === 'number' && ev.duration_ms > 0 && (
                                  <span className="font-mono text-faint">{ev.duration_ms}ms</span>
                                )}
                                {ev.turn && <span className="ml-auto font-mono text-faint">{ev.turn}</span>}
                              </div>
                              {(ev.text || ev.err) && (
                                <div className="mt-1 whitespace-pre-wrap break-words font-mono text-xs text-muted">
                                  {ev.err || ev.text}
                                </div>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </div>
            </Panel>
          </div>
        </div>
      )}
    </Page>
  )
}
