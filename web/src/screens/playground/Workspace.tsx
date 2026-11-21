// The live Playground: one agent, one thread, one turn at a time. The session
// itself lives in useChatSession; this screen adds the agent picker, the
// thread list and the act-as field around it.
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import type { Client } from '@/api/client'
import type { MediaPart } from '@/api/types'
import { Button, Page } from '@/components/ui'
import { t } from '@/i18n'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { ChatPane, type ChatPaneHandle } from './ChatPane'
import { Panes } from './Panes'
import { ThreadsPane } from './ThreadsPane'
import { TracePane } from './TracePane'
import type { TraceFilter } from './model'
import { useChatSession } from './useChatSession'

export const TITLE_KEY = 'playground.title'
export const SUBTITLE_KEY = 'playground.subtitle'

export function Workspace({ client, agents, reloadAgents }: { client: Client; agents: string[]; reloadAgents: () => void }) {
  const [params, setParams] = useSearchParams()
  const wanted = params.get('agent') ?? ''

  const s = useChatSession(client, agents.includes(wanted) ? wanted : agents[0])
  const { agent, switchAgent } = s
  const [filter, setFilter] = useState<TraceFilter>('All events')
  const [actAs, setActAs] = useState('')
  const [draft, setDraft] = useState('')
  const [attachments, setAttachments] = useState<MediaPart[]>([])
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

  useScreenActions({
    send,
    refresh: () => {
      // An agent created since this screen opened joins the picker.
      reloadAgents()
      s.loadThreads()
      void s.refreshTrace()
    },
  })

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
        right={<TracePane events={s.events} scope={agent + '\n' + s.thread} filter={filter} onFilter={setFilter} onRefresh={() => void s.refreshTrace()} refreshing={s.tracing} />}
      />
    </Page>
  )
}
