// The live Playground: one agent, one thread, one turn at a time — plus
// step-by-step time-travel, deterministic Replay & Eval scenarios, and
// side-by-side run comparison in the right pane.
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import type { Client } from '@/api/client'
import type { MediaPart } from '@/api/types'
import { Button, Page, Panel, cx } from '@/components/ui'
import { t } from '@/i18n'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { toast } from '@/state/ui'
import { ChatPane, type ChatPaneHandle } from './ChatPane'
import { ComparePane } from './ComparePane'
import { Panes } from './Panes'
import { ReplayPane } from './ReplayPane'
import { ThreadsPane } from './ThreadsPane'
import { TracePane } from './TracePane'
import type { TraceFilter } from './model'
import { buildRunFromLiveThread, starterScenarioYAML, traceToScenarioYAML, type PlaygroundRun } from './replay'
import { useChatSession } from './useChatSession'

export const TITLE_KEY = 'playground.title'
export const SUBTITLE_KEY = 'playground.subtitle'

type RightTab = 'trace' | 'replay' | 'compare'

export function Workspace({ client, agents, reloadAgents }: { client: Client; agents: string[]; reloadAgents: () => void }) {
  const [params, setParams] = useSearchParams()
  const wanted = params.get('agent') ?? ''

  const s = useChatSession(client, agents.includes(wanted) ? wanted : agents[0])
  const { agent, switchAgent } = s
  const [filter, setFilter] = useState<TraceFilter>('All events')
  const [actAs, setActAs] = useState('')
  const [draft, setDraft] = useState('')
  const [attachments, setAttachments] = useState<MediaPart[]>([])
  const [rightTab, setRightTab] = useState<RightTab>('trace')
  const [scenarioYAML, setScenarioYAML] = useState('')
  const [yamlTouched, setYamlTouched] = useState(false)
  const [runs, setRuns] = useState<PlaygroundRun[]>([])
  const [compareA, setCompareA] = useState('')
  const [compareB, setCompareB] = useState('')
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

  // The address bar can change under the open screen (a "chat with" link, the
  // back button); follow it like a pick from the select.
  useEffect(() => {
    if (agents.includes(wanted)) switchAgent(wanted)
  }, [wanted, agents, switchAgent])

  // A refresh can find the open agent deleted: move to one that still exists.
  useEffect(() => {
    if (agents.includes(agent)) return
    switchAgent(agents[0])
    setParams({ agent: agents[0] }, { replace: true })
  }, [agents, agent, switchAgent, setParams])

  const pickAgent = (name: string) => {
    switchAgent(name)
    setParams({ agent: name }, { replace: true })
  }

  const newThread = () => {
    s.newThread()
    chat.current?.focusComposer()
  }

  const send = () => {
    if (!s.send(draft, actAs, attachments)) return
    setDraft('')
    setAttachments([])
    chat.current?.pinToEnd()
  }

  const recordRun = (run: PlaygroundRun) => {
    setRuns((prev) => {
      const next = [run, ...prev].slice(0, 20)
      if (next.length >= 2) {
        setCompareB(next[0].id)
        setCompareA(next[1].id)
      } else if (next.length === 1) {
        setCompareB(next[0].id)
      }
      return next
    })
  }

  const pinLiveRun = () => {
    if (!s.events || s.events.length === 0) return
    recordRun(buildRunFromLiveThread(agent, s.thread, s.messages, s.events))
    toast(t('playground.pinnedToast'), 'success')
  }

  const seedScenarioFromThread = () => {
    const next = s.events && s.events.length > 0 ? traceToScenarioYAML(agent, s.messages, s.events) : starterScenarioYAML(agent)
    setScenarioYAML(next)
    setYamlTouched(true)
  }

  const openReplayTab = () => {
    if (!yamlTouched || scenarioYAML.trim() === '') {
      const next = s.events && s.events.length > 0 ? traceToScenarioYAML(agent, s.messages, s.events) : starterScenarioYAML(agent)
      setScenarioYAML(next)
    }
    setRightTab('replay')
  }

  const exportThreadToReplay = () => {
    seedScenarioFromThread()
    setRightTab('replay')
  }

  useScreenActions({
    send,
    refresh: () => {
      // An agent created since this screen opened joins the picker.
      reloadAgents()
      s.loadThreads()
      void s.refreshTrace()
    },
  })

  const tabClass = (active: boolean) =>
    cx(
      'rounded-ctl px-2.5 py-1 text-xs font-medium transition-colors',
      active ? 'bg-surface2 text-text' : 'text-muted hover:bg-hover hover:text-text',
    )

  return (
    <Page
      title={t(TITLE_KEY)}
      subtitle={t(SUBTITLE_KEY)}
      scroll={false}
      actions={
        <Button variant="primary" icon={Plus} onClick={newThread}>
          {t('playground.newThread')}
        </Button>
      }
    >
      <Panes
        left={
          <ThreadsPane
            agents={agents}
            agent={agent}
            onAgent={pickAgent}
            threads={s.threads}
            thread={s.thread}
            onOpen={(id) => void s.openThread(id)}
            actAs={actAs}
            onActAs={setActAs}
            userIDs={users.data ?? []}
          />
        }
        center={
          <ChatPane
            ref={chat}
            agent={agent}
            thread={s.thread}
            status={s.status}
            messages={s.messages}
            sending={s.sending}
            stopping={s.stopping}
            draft={draft}
            onDraft={setDraft}
            attachments={attachments}
            onAttachments={setAttachments}
            onSend={send}
            onStop={() => void s.stop()}
            loadMedia={(h, sig) => client.mediaBlob(h, sig)}
          />
        }
        right={
          <Panel flush className="flex h-full min-h-0 flex-col">
            <div role="tablist" className="flex shrink-0 items-center gap-1 border-b border-line px-2 py-1.5">
              <button type="button" role="tab" aria-selected={rightTab === 'trace'} onClick={() => setRightTab('trace')} className={tabClass(rightTab === 'trace')}>
                {t('playground.tabTrace')}
              </button>
              <button type="button" role="tab" aria-selected={rightTab === 'replay'} onClick={openReplayTab} className={tabClass(rightTab === 'replay')}>
                {t('playground.tabReplay')}
              </button>
              <button type="button" role="tab" aria-selected={rightTab === 'compare'} onClick={() => setRightTab('compare')} className={tabClass(rightTab === 'compare')}>
                {t('playground.tabCompare', { n: runs.length })}
              </button>
            </div>
            <div className="min-h-0 flex-1">
              {rightTab === 'trace' && (
                <TracePane
                  embedded
                  events={s.events}
                  scope={agent + '\n' + s.thread}
                  filter={filter}
                  onFilter={setFilter}
                  onRefresh={() => void s.refreshTrace()}
                  refreshing={s.tracing}
                  onPinRun={pinLiveRun}
                  onExportScenario={exportThreadToReplay}
                />
              )}
              {rightTab === 'replay' && (
                <ReplayPane
                  client={client}
                  agent={agent}
                  yaml={scenarioYAML || starterScenarioYAML(agent)}
                  onYaml={(v) => {
                    setYamlTouched(true)
                    setScenarioYAML(v)
                  }}
                  hasThreadEvents={(s.events?.length ?? 0) > 0}
                  onSeedFromThread={seedScenarioFromThread}
                  onRunRecorded={recordRun}
                />
              )}
              {rightTab === 'compare' && (
                <ComparePane runs={runs} baseId={compareA} candId={compareB} onSelectBase={setCompareA} onSelectCand={setCompareB} />
              )}
            </div>
          </Panel>
        }
      />
    </Page>
  )
}
