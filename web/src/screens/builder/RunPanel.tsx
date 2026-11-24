// The builder's docked run panel: a chat with the agent being edited above
// the trace of its turns, so an edit can be applied and tried without leaving
// the builder. The session logic is the Playground's own (useChatSession).
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
import { ExternalLink, Plus } from 'lucide-react'
import type { Client } from '@/api/client'
import { IconButton } from '@/components/ui'
import { t } from '@/i18n'
import { MOD } from '@/nav'
import { ChatPane, type ChatPaneHandle } from '@/screens/playground/ChatPane'
import type { TraceFilter } from '@/screens/playground/model'
import { TracePane } from '@/screens/playground/TracePane'
import { useChatSession } from '@/screens/playground/useChatSession'
import { toast } from '@/state/ui'

export interface RunPanelHandle {
  /**
   * Sends the composer's text to `agent`, or repeats the last message when the
   * composer is empty. False when nothing was sent.
   */
  run: (agent: string) => boolean
  focus: () => void
}

export const RunPanel = forwardRef<
  RunPanelHandle,
  {
    client: Client
    /** The applied agent; '' before a new agent's first apply. */
    agent: string
    /** Ctrl+Enter and Send: apply pending edits, then run. */
    onRun: () => void
    onOpenPlayground: () => void
  }
>(function RunPanel({ client, agent, onRun, onOpenPlayground }, handle) {
  const s = useChatSession(client, agent, { threads: false })
  const { switchAgent } = s
  const [draft, setDraft] = useState('')
  const [filter, setFilter] = useState<TraceFilter>('All events')
  const chat = useRef<ChatPaneHandle>(null)
  // Read by run(), which the builder calls before this panel re-renders.
  const draftRef = useRef(draft)
  draftRef.current = draft
  const last = useRef('')

  // A new agent's first apply gives the panel something to talk to.
  useEffect(() => {
    if (agent !== '') switchAgent(agent)
  }, [agent, switchAgent])

  useImperativeHandle(
    handle,
    () => ({
      run: (target) => {
        // Synchronous: right after a first apply, `agent` has not reached this panel yet.
        switchAgent(target)
        const typed = draftRef.current
        const text = typed.trim() !== '' ? typed : last.current
        if (text === '') {
          chat.current?.focusComposer()
          return false
        }
        if (s.busy()) {
          toast(t('builder.turnRunning'), 'warning')
          return false
        }
        if (!s.send(text)) return false
        last.current = text
        if (text === typed) setDraft('')
        chat.current?.pinToEnd()
        return true
      },
      focus: () => chat.current?.focusComposer(),
    }),
    [s, switchAgent],
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="flex min-h-0 flex-[3] flex-col">
        <ChatPane
          ref={chat}
          agent={agent === '' ? t('builder.notAppliedYet') : agent}
          thread={s.thread}
          status={agent === '' ? { key: 'builder.applyToStart', tone: 'neutral' } : s.status}
          messages={s.messages}
          sending={s.sending}
          stopping={s.stopping}
          draft={draft}
          onDraft={setDraft}
          onSend={onRun}
          onStop={() => void s.stop()}
          onDecideApproval={(id, approve) => void s.decideApproval(id, approve)}
          loadMedia={(h, sig) => client.mediaBlob(h, sig)}
          hint={t('builder.runHint', { mod: MOD })}
          emptyHint={t('builder.runEmptyHint')}
          actions={
            <>
              <IconButton icon={Plus} label={t('builder.newThread')} onClick={s.newThread} disabled={agent === ''} />
              <IconButton icon={ExternalLink} label={t('builder.openPlayground')} onClick={onOpenPlayground} disabled={agent === ''} />
            </>
          }
        />
      </div>
      <div className="flex min-h-0 flex-[2] flex-col">
        <TracePane events={s.events} scope={s.agent + '\n' + s.thread} filter={filter} onFilter={setFilter} onRefresh={() => void s.refreshTrace()} refreshing={s.tracing} />
      </div>
    </div>
  )
})
