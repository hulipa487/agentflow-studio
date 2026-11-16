// Memory pools: named shared stores owned by exactly one tenant or group, and
// the per-(agent, tenant) grants that open them.
import { useMemo, useState } from 'react'
import { Check, Database, Plus, X } from 'lucide-react'
import { errorMessage } from '@/api/client'
import type { Grant, Pool, PoolDraft } from '@/api/types'
import { Banner, Button, Chip, ComboInput, CopyButton, Field, IconButton, Input, KV, Loading, Panel, Section, Segmented, Select, Table } from '@/components/ui'
import { Split } from '@/components/ui/Split'
import { useAsync } from '@/lib/hooks'
import { relTime, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { byText, parseWhole, quoted, useAlive } from '../users/helpers'
import { PaneNote, RailRow } from '../users/rail'
import { FormModal, Ident, type Directory, type PaneProps } from './shared'

const MODES = [
  { value: 'rw', label: 'rw · read/write' },
  { value: 'r', label: 'r · read only' },
]

/** The grant that covers every active member is stored as "*". */
const everyMember = (tenant: string | undefined) => tenant === '*' || !tenant

export function PoolsPane({ dir, selected, onSelect, rev, reloadAll, notify }: PaneProps) {
  const [creating, setCreating] = useState(false)
  const pool = dir.pools.find((p) => p.uuid === selected)

  const left = (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <Button icon={Plus} className="self-start" onClick={() => setCreating(true)}>
        New pool
      </Button>
      <div className="-mx-1 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-1">
        {dir.pools.length === 0 && <PaneNote>No pools. Create one, grant agents access, then bind a store to it with pool: &lt;name&gt;.</PaneNote>}
        {dir.pools.map((p) => {
          const owner = p.owner?.group ? 'group ' + shortID(p.owner.group) : 'tenant ' + shortID(p.owner?.tenant)
          return (
            <RailRow
              key={p.uuid}
              icon={Database}
              title={p.name}
              caption={`${owner} · ${p.retention_days}d retention`}
              active={p.uuid === selected}
              onClick={() => onSelect(p.uuid)}
            />
          )
        })}
      </div>
    </div>
  )

  const right = (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {pool ? (
        <PoolDetail key={pool.uuid} pool={pool} dir={dir} rev={rev} notify={notify} />
      ) : (
        <PaneNote lowered>Select a pool to manage its grants.</PaneNote>
      )}
    </div>
  )

  return (
    <>
      <Split fixed="left" initial={300} min={220} max={520} storageKey="tenancy-pools" left={left} right={right} />
      {creating && (
        <NewPoolDialog
          dir={dir}
          onClose={() => setCreating(false)}
          onCreated={(p) => {
            setCreating(false)
            onSelect(p.uuid)
            toast(`pool ${p.name} created · grant agents before binding it`, 'success')
            reloadAll()
          }}
        />
      )}
    </>
  )
}

function PoolDetail({ pool: p, dir, rev, notify }: { pool: Pool; dir: Directory; rev: number; notify: PaneProps['notify'] }) {
  const client = useClient()
  const alive = useAlive()
  const { data, error, loading, reload } = useAsync((signal) => client.grants(p.uuid, signal), [client, p.uuid, rev])
  const [busy, setBusy] = useState('')
  const [agent, setAgent] = useState('')
  const [tenant, setTenant] = useState('')
  const [mode, setMode] = useState('rw')

  const grants = useMemo(() => [...(data ?? [])].sort((a, b) => byText(a.agent, b.agent) || byText(a.tenant ?? '', b.tenant ?? '')), [data])

  const { who } = dir
  const ownerGroup = p.owner?.group ?? ''
  const ownerTenant = p.owner?.tenant ?? ''
  let owner = who(ownerTenant)
  if (ownerGroup !== '') {
    const g = dir.groups.find((x) => x.uuid === ownerGroup)
    owner = g ? g.name + ' · ' + shortID(g.uuid) : ownerGroup
  }

  const run = async (key: string, call: () => Promise<unknown>, done: string, tone: 'success' | 'warning') => {
    setBusy(key)
    try {
      await call()
    } catch (err) {
      notify({ text: errorMessage(err), tone: 'danger' })
      if (alive.current) setBusy('')
      return false
    }
    toast(done, tone)
    notify(null)
    if (alive.current) {
      setBusy('')
      reload()
    }
    return true
  }

  const grant = () => {
    const ag = agent.trim()
    if (ag === '') {
      notify({ text: 'Choose the agent to grant.', tone: 'warning' })
      return
    }
    // An empty tenant is the member wildcard.
    void run('grant', () => client.setGrant(p.uuid, ag, tenant.trim(), mode), 'grant saved', 'success').then((ok) => {
      if (ok && alive.current) {
        setAgent('')
        setTenant('')
        setMode('rw')
      }
    })
  }

  const revoke = async (g: Grant) => {
    const ok = await confirm({
      title: 'Revoke grant',
      message: `Revoke ${g.agent}'s ${g.mode} access to pool ${quoted(p.name)}?`,
      confirmLabel: 'Revoke',
      danger: true,
    })
    if (!ok) return
    const t = g.tenant === '*' ? '' : (g.tenant ?? '')
    await run(`revoke:${g.agent}:${g.tenant}`, () => client.removeGrant(p.uuid, g.agent, t), 'grant revoked', 'warning')
  }

  return (
    <div className="flex flex-col gap-3 pb-2">
      <Panel className="flex flex-col gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[15px] font-semibold">{p.name}</span>
          <Chip tone="accent">{ownerGroup !== '' ? 'group' : 'tenant'} pool</Chip>
          <span className="flex-1" />
          <CopyButton text={'pool: ' + p.name} what="store binding" size="md" />
        </div>
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <KV label="pool uuid" value={p.uuid} copy={p.uuid} />
          <KV label="owner" value={owner} copy={ownerGroup !== '' ? ownerGroup : ownerTenant || undefined} />
          <KV label="retention" value={`${p.retention_days} days (immutable)`} />
          <KV label="created" value={`${relTime(p.created_at)} by ${shortID(p.created_by)}`} />
        </div>
        <p className="break-words text-[11px] leading-snug text-faint">
          Bind a memory store to it:{' '}
          <code className="font-mono text-muted">{`stores: { shared: { backend: <name>, table: <table>, pool: ${p.name} } }`}</code>
        </p>
      </Panel>

      <Section
        title="Grants"
        subtitle="Per (agent, tenant). The member wildcard covers everyone active in the owning group; a tenant pool's wildcard covers nobody but its owner."
        flush
      >
        {!data ? (
          error && !loading ? (
            <div className="px-3 pb-2">
              <Banner tone="danger">{errorMessage(error)}</Banner>
            </div>
          ) : (
            <Loading text="Loading grants…" />
          )
        ) : (
          <Table<Grant>
            rows={grants}
            rowKey={(g) => g.agent + '\u0000' + g.tenant}
            empty="No grants: no agent can read or write this pool yet."
            columns={[
              { key: 'agent', title: 'agent', width: 160, render: (g) => g.agent },
              {
                key: 'tenant',
                title: 'tenant',
                render: (g) => (everyMember(g.tenant) ? <span className="text-muted">every active member</span> : <Ident label={who(g.tenant)} id={g.tenant} what="tenant uuid" />),
              },
              { key: 'mode', title: 'mode', width: 104, render: (g) => (g.mode === 'r' ? <Chip>read only</Chip> : <Chip tone="success">read/write</Chip>) },
              {
                key: 'revoke',
                title: '',
                width: 48,
                align: 'right',
                render: (g) => (
                  <IconButton
                    icon={X}
                    size="sm"
                    tone="danger"
                    label="Revoke"
                    loading={busy === `revoke:${g.agent}:${g.tenant}`}
                    disabled={busy !== ''}
                    onClick={() => revoke(g)}
                  />
                ),
              },
            ]}
          />
        )}
        <form
          className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_150px_auto] items-center gap-2 border-t border-line/70 px-3 pb-2 pt-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (busy === '') grant()
          }}
        >
          <ComboInput aria-label="Agent" options={dir.agents} placeholder="agent" value={agent} onChange={(e) => setAgent(e.target.value)} />
          <ComboInput
            mono
            aria-label="Tenant uuid"
            options={dir.userIDs}
            placeholder="tenant uuid (empty = every member)"
            value={tenant}
            onChange={(e) => setTenant(e.target.value)}
          />
          <Select aria-label="Mode" value={mode} onChange={setMode} options={MODES} />
          <Button type="submit" icon={Check} loading={busy === 'grant'} disabled={busy !== ''}>
            Grant
          </Button>
        </form>
      </Section>
    </div>
  )
}

function NewPoolDialog({ dir, onClose, onCreated }: { dir: Directory; onClose: () => void; onCreated: (p: Pool) => void }) {
  const client = useClient()
  const alive = useAlive()
  const [name, setName] = useState('')
  const [retention, setRetention] = useState('90')
  const [creator, setCreator] = useState('')
  const [kind, setKind] = useState<'tenant' | 'group'>('group')
  const [ownerGroup, setOwnerGroup] = useState('')
  const [ownerTenant, setOwnerTenant] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  // Only a live group can own a new pool. The picker shows "name · short id";
  // what is sent is the uuid.
  const active = useMemo(() => dir.groups.filter((g) => (g.disbanded_at ?? 0) === 0), [dir.groups])
  const groupLabels = useMemo(() => active.map((g) => g.name + ' · ' + shortID(g.uuid)), [active])

  const resolveGroup = (typed: string): string => {
    const byLabel = active.find((g) => g.name + ' · ' + shortID(g.uuid) === typed)
    if (byLabel) return byLabel.uuid
    // The field invites a bare name too; it is only unambiguous when one live
    // group carries it.
    const byName = active.filter((g) => g.name === typed)
    return byName.length === 1 ? byName[0].uuid : typed
  }

  const submit = async () => {
    const days = parseWhole(retention.trim())
    if (days === null || days <= 0) {
      setError('Retention must be a positive number of days.')
      return
    }
    const draft: PoolDraft = { name: name.trim(), creator: creator.trim(), retention_days: days }
    if (kind === 'tenant') draft.owner_tenant = ownerTenant.trim()
    else draft.owner_group = resolveGroup(ownerGroup.trim())
    if (draft.name === '' || draft.creator === '' || (!draft.owner_tenant && !draft.owner_group)) {
      setError('A name, a creator and an owner are required.')
      return
    }
    setBusy(true)
    setError('')
    let p: Pool
    try {
      p = await client.createPool(draft)
    } catch (err) {
      if (alive.current) {
        setError(errorMessage(err))
        setBusy(false)
      }
      return
    }
    if (alive.current) onCreated(p)
  }

  return (
    <FormModal
      title="New memory pool"
      subtitle="A named shared store owned by exactly one tenant or group."
      okLabel="Create pool"
      busy={busy}
      error={error}
      onSubmit={submit}
      onClose={onClose}
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Name">
          <Input autoFocus placeholder="team-notes" value={name} onChange={(e) => setName(e.target.value)} disabled={busy} />
        </Field>
        <Field label="Retention (days)">
          <Input mono inputMode="numeric" value={retention} onChange={(e) => setRetention(e.target.value)} disabled={busy} />
        </Field>
      </div>
      <Field label="Creator">
        <ComboInput mono options={dir.userIDs} placeholder="creator tenant uuid" value={creator} onChange={(e) => setCreator(e.target.value)} disabled={busy} />
      </Field>
      <Field label="Owner">
        <Segmented
          className="self-start"
          value={kind}
          onChange={setKind}
          options={[
            { value: 'tenant', label: 'Tenant', disabled: busy },
            { value: 'group', label: 'Group', disabled: busy },
          ]}
        />
        {kind === 'group' ? (
          <ComboInput
            aria-label="Owner group"
            options={groupLabels}
            placeholder="group (name or uuid)"
            value={ownerGroup}
            onChange={(e) => setOwnerGroup(e.target.value)}
            disabled={busy}
          />
        ) : (
          <ComboInput
            mono
            aria-label="Owner tenant"
            options={dir.userIDs}
            placeholder="owner tenant uuid"
            value={ownerTenant}
            onChange={(e) => setOwnerTenant(e.target.value)}
            disabled={busy}
          />
        )}
      </Field>
      <p className="text-[11px] leading-snug text-faint">
        Names are global and are what config stores bind to. Retention is immutable once set: extending it means a new pool.
      </p>
    </FormModal>
  )
}
