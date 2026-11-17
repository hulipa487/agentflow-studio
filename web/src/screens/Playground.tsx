// Playground: a threaded chat with one agent beside the live trace of each
// turn. This file is the shell for the loading, failed and empty states; the
// three-pane workspace lives in ./playground.
import { useMemo } from 'react'
import { AsyncView, EmptyState, Page } from '@/components/ui'
import { useAsync } from '@/lib/hooks'
import { useClient } from '@/state/session'
import { useUI } from '@/state/ui'
import { SUBTITLE, TITLE, Workspace } from './playground/Workspace'

export default function Playground() {
  const client = useClient()
  const { data: rows, error, loading, reload } = useAsync((signal) => client.agents(signal), [client])
  // Spawn profiles are templates; the runtime refuses to chat with one.
  const agents = useMemo(() => (rows ?? []).filter((r) => !r.spawn).map((r) => r.name), [rows])

  // Quiet: a failed re-read keeps the list, and the workspace with it.
  if (rows && agents.length > 0) return <Workspace client={client} agents={agents} reloadAgents={() => reload(true)} />

  return (
    <Page title={TITLE} subtitle={SUBTITLE}>
      <AsyncView loading={loading} error={error} hasData={!!rows} retry={() => reload()} loadingText="Loading agents…">
        <EmptyState
          title="No agents yet"
          guidance="Create an agent to start a chat and trace its turns."
          actionLabel="Create an agent"
          onAction={() => useUI.getState().setNewAgent(true)}
        />
      </AsyncView>
    </Page>
  )
}
