// Tenancy: the runtime's live registry of groups (a master and its members)
// and named memory pools with per-agent grants. All of it lives in the runtime
// store, not in the config file, so every change applies at once.
import { useCallback, useMemo, useState } from 'react'
import { Database, RotateCw, Users } from 'lucide-react'
import { AsyncView, Banner, IconButton, Page, Tabs } from '@/components/ui'
import { useAsync, useScreenActions, useStored } from '@/lib/hooks'
import { plural, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { byText, useAlive, userTitle, type Notice } from './users/helpers'
import { GroupsPane } from './tenancy/Groups'
import { PoolsPane } from './tenancy/Pools'
import type { Directory } from './tenancy/shared'

type Tab = 'groups' | 'pools'

export default function Tenancy() {
  const client = useClient()
  const alive = useAlive()
  const [stored, setTab] = useStored<Tab>('agentflow-studio:tenancy-tab', 'groups')
  const tab: Tab = stored === 'pools' ? 'pools' : 'groups'
  const [selGroup, setSelGroup] = useState('')
  const [selPool, setSelPool] = useState('')
  const [notice, setNotice] = useState<Notice | null>(null)
  const [rev, setRev] = useState(0)

  const { data, error, loading, reload } = useAsync(
    async (signal) => {
      const [groups, pools, users, agents] = await Promise.all([
        client.groups(signal),
        client.pools(signal),
        // Pickers only: a runtime without identity still has tenancy.
        client.users(signal).then(
          (l) => l.users ?? [],
          () => [],
        ),
        client.agents(signal).catch(() => []),
      ])
      if (!signal.aborted) setNotice(null)
      return { groups, pools, users, agents }
    },
    [client],
  )

  const reloadAll = useCallback(() => {
    if (!alive.current) return
    reload()
    setRev((r) => r + 1)
  }, [alive, reload])
  const notify = useCallback(
    (n: Notice | null) => {
      if (alive.current) setNotice(n)
    },
    [alive],
  )
  useScreenActions({ refresh: reloadAll })

  const dir = useMemo<Directory>(() => {
    const names = new Map<string, string>()
    for (const u of data?.users ?? []) names.set(u.user_id, userTitle(u))
    return {
      groups: [...(data?.groups ?? [])].sort((a, b) => byText(a.name, b.name)),
      pools: [...(data?.pools ?? [])].sort((a, b) => byText(a.name, b.name)),
      userIDs: [...names.keys()].sort(byText),
      agents: (data?.agents ?? [])
        .filter((a) => !a.spawn)
        .map((a) => a.name)
        .sort(byText),
      who: (id) => {
        if (!id) return '—'
        const n = names.get(id)
        return n !== undefined && n !== id ? n + ' · ' + shortID(id) : id
      },
    }
  }, [data])

  const active = dir.groups.filter((g) => (g.disbanded_at ?? 0) === 0).length

  return (
    <Page
      title="Tenancy"
      subtitle="Groups give several people one shared context; pools are named memory stores that agents bind with pool: <name>, opened to tenants by grants. Changes are live."
      scroll={false}
      meta={
        data ? (
          <span>
            {plural(dir.groups.length, 'group')} ({active} active) · {plural(dir.pools.length, 'pool')}
          </span>
        ) : null
      }
      banner={
        notice ? (
          <Banner tone={notice.tone} onClose={() => setNotice(null)}>
            {notice.text}
          </Banner>
        ) : null
      }
      actions={<IconButton icon={RotateCw} label="Refresh" onClick={reloadAll} />}
    >
      <AsyncView loading={loading} error={error} hasData={!!data} retry={reloadAll} loadingText="Loading…">
        <Tabs<Tab>
          className="mb-3"
          value={tab}
          onChange={setTab}
          tabs={[
            { id: 'groups', label: 'Groups', icon: Users },
            { id: 'pools', label: 'Memory pools', icon: Database },
          ]}
        />
        {tab === 'groups' ? (
          <GroupsPane dir={dir} selected={selGroup} onSelect={setSelGroup} rev={rev} reloadAll={reloadAll} notify={notify} />
        ) : (
          <PoolsPane dir={dir} selected={selPool} onSelect={setSelPool} rev={rev} reloadAll={reloadAll} notify={notify} />
        )}
      </AsyncView>
    </Page>
  )
}
