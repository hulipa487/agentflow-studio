// Playground: a threaded chat with one agent beside the live trace of each
// turn. This file is the shell for the loading, failed and empty states; the
// three-pane workspace lives in ./playground.
import { useMemo } from 'react'
import { AsyncView, EmptyState, Page } from '@/components/ui'
import { useAsync } from '@/lib/hooks'
import { t } from '@/i18n'
import { useClient } from '@/state/session'
import { useUI } from '@/state/ui'
import { SUBTITLE_KEY, TITLE_KEY, Workspace } from './playground/Workspace'

export default function Playground() {
  const client = useClient()
  const { data: rows, error, loading, reload } = useAsync((signal) => client.agents(signal), [client])
  // Spawn profiles are templates; the runtime refuses to chat with one.
  const agents = useMemo(() => (rows ?? []).filter((r) => !r.spawn).map((r) => r.name), [rows])

  // Quiet: a failed re-read keeps the list, and the workspace with it.
  if (rows && agents.length > 0) return <Workspace client={client} agents={agents} reloadAgents={() => reload(true)} />

  return (
    <Page title={t(TITLE_KEY)} subtitle={t(SUBTITLE_KEY)}>
      <AsyncView loading={loading} error={error} hasData={!!rows} retry={() => reload()} loadingText={t('playground.loadingAgents')}>
        <EmptyState
          title={t('playground.noAgentsTitle')}
          guidance={t('playground.noAgentsGuidance')}
          actionLabel={t('playground.createAgent')}
          onAction={() => useUI.getState().setNewAgent(true)}
        />
      </AsyncView>
    </Page>
  )
}
