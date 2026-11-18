// Groups: one master and any number of members sharing a context. The list is
// on the left; the selected group's members are managed on the right.
import { useMemo, useState } from 'react'
import { Plus, Trash2, Users, X } from 'lucide-react'
import { errorMessage } from '@/api/client'
import type { Group, Membership } from '@/api/types'
import { Banner, Button, Chip, ComboInput, Field, IconButton, Input, KV, Loading, Panel, Section, StatusChip, Table } from '@/components/ui'
import { Split } from '@/components/ui/Split'
import { useAsync } from '@/lib/hooks'
import { relTime, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { quoted, useAlive } from '../users/helpers'
import { PaneNote, RailRow } from '../users/rail'
import { FormModal, Ident, type Directory, type PaneProps } from './shared'

const disbandedAt = (g: Group) => g.disbanded_at ?? 0

export function GroupsPane({ dir, selected, onSelect, rev, reloadAll, notify }: PaneProps) {
  const [creating, setCreating] = useState(false)

  const left = (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <Button icon={Plus} className="self-start" onClick={() => setCreating(true)}>
        New group
      </Button>
      <div className="-mx-1 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-1">
        {dir.groups.length === 0 && (
          <PaneNote>No groups. A group has one master and any number of members; a chat enters it with /group &lt;uuid&gt;.</PaneNote>
        )}
        {dir.groups.map((g) => (
          <RailRow
            key={g.uuid}
            icon={Users}
            title={g.name}
            caption={shortID(g.uuid) + (disbandedAt(g) > 0 ? ' · disbanded ' + relTime(disbandedAt(g)) : '')}
            active={g.uuid === selected}
            onClick={() => onSelect(g.uuid)}
          />
        ))}
      </div>
    </div>
  )

  const right = (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {selected === '' ? (
        <PaneNote lowered>Select a group to manage its members.</PaneNote>
      ) : (
        <GroupDetail key={selected} id={selected} dir={dir} rev={rev} reloadAll={reloadAll} notify={notify} />
      )}
    </div>
  )

  return (
    <>
      <Split fixed="left" initial={300} min={220} max={520} storageKey="tenancy-groups" left={left} right={right} />
      {creating && (
        <NewGroupDialog
          dir={dir}
          onClose={() => setCreating(false)}
          onCreated={(g) => {
            setCreating(false)
            onSelect(g.uuid)
            toast(`group ${g.name} created`, 'success')
            reloadAll()
          }}
        />
      )}
    </>
  )
}

function GroupDetail({ id, dir, rev, reloadAll, notify }: { id: string; dir: Directory; rev: number; reloadAll: () => void; notify: PaneProps['notify'] }) {
  const client = useClient()
  const alive = useAlive()
  const { data, error, loading, reload } = useAsync((signal) => client.group(id, signal), [client, id, rev])
  const [busy, setBusy] = useState('')
  const [adding, setAdding] = useState('')

  const members = useMemo(() => [...(data?.members ?? [])].sort((a, b) => a.joined_at - b.joined_at), [data])

  if (!data) {
    if (error && !loading) return <Banner tone="danger">{errorMessage(error)}</Banner>
    return <Loading text="Loading group…" />
  }

  const g = data.group
  const disbanded = disbandedAt(g) > 0
  const { who } = dir

  // Runs one registry change. 'all' reloads the lists as well (the change
  // shows in them); 'detail' only refetches this group.
  const run = async (key: string, call: () => Promise<unknown>, done: string, tone: 'success' | 'warning', scope: 'all' | 'detail') => {
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
    if (alive.current) setBusy('')
    if (scope === 'all') reloadAll()
    else if (alive.current) reload()
    return true
  }

  const addMember = () => {
    const user = adding.trim()
    if (user === '') {
      notify({ text: 'Enter the tenant uuid to add.', tone: 'warning' })
      return
    }
    void run('add', () => client.addMember(g.uuid, user), 'member added', 'success', 'detail').then((ok) => {
      if (ok && alive.current) setAdding('')
    })
  }

  const setMaster = async (user: string) => {
    const ok = await confirm({
      title: 'Change master',
      message: `Hand ${quoted(g.name)} from ${who(g.master)} to ${who(user)}?`,
      confirmLabel: 'Change master',
    })
    if (ok) await run('master:' + user, () => client.setMaster(g.uuid, g.master, user), 'master changed', 'success', 'all')
  }

  const removeMember = async (user: string) => {
    const ok = await confirm({
      title: 'Remove member',
      message: `Remove ${who(user)} from ${quoted(g.name)}? Their turns stop acting in this group immediately.`,
      confirmLabel: 'Remove',
      danger: true,
    })
    if (ok) await run('remove:' + user, () => client.removeMember(g.uuid, user), 'member removed', 'warning', 'detail')
  }

  const disband = async () => {
    const ok = await confirm({
      title: 'Disband group',
      message: `Disband ${quoted(g.name)}? Access ends now for every member; its rows are deleted when the disband retention elapses. This is done as the master (${who(g.master)}).`,
      confirmLabel: 'Disband',
      danger: true,
    })
    // The runtime only accepts a disband from the current master.
    if (ok) await run('disband', () => client.disbandGroup(g.uuid, g.master), 'group disbanded', 'warning', 'all')
  }

  return (
    <div className="flex flex-col gap-3 pb-2">
      <Panel className="flex flex-col gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-md font-semibold">{g.name}</span>
          {disbanded ? <StatusChip tone="danger">disbanded</StatusChip> : <StatusChip tone="success">active</StatusChip>}
          <span className="flex-1" />
          <Button variant="danger" icon={Trash2} loading={busy === 'disband'} disabled={disbanded || busy !== ''} onClick={disband}>
            Disband…
          </Button>
        </div>
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-3">
          <KV label="group uuid" value={g.uuid} copy={g.uuid} />
          <KV label="master" value={who(g.master)} copy={g.master} />
          <KV label="created" value={relTime(g.created_at)} />
          {disbanded && <KV label="disbanded" value={relTime(disbandedAt(g))} />}
        </div>
      </Panel>

      <Section
        title="Members"
        subtitle="A turn acts in the group only when the sender is an active member; the engine validates it, the route only proposes."
        flush
      >
        <Table<Membership>
          rows={members}
          rowKey={(m) => m.user}
          empty="No members."
          columns={[
            { key: 'role', title: 'role', width: 86, render: (m) => (m.user === g.master ? <Chip tone="accent">master</Chip> : <Chip>member</Chip>) },
            { key: 'tenant', title: 'tenant', render: (m) => <Ident label={who(m.user)} id={m.user} what="tenant uuid" /> },
            { key: 'joined', title: 'joined', width: 100, render: (m) => relTime(m.joined_at) },
            {
              key: 'master',
              title: '',
              width: 124,
              align: 'right',
              render: (m) => (
                <Button size="sm" loading={busy === 'master:' + m.user} disabled={m.user === g.master || disbanded || busy !== ''} onClick={() => setMaster(m.user)}>
                  Make master
                </Button>
              ),
            },
            {
              key: 'remove',
              title: '',
              width: 48,
              align: 'right',
              render: (m) => (
                <IconButton
                  icon={X}
                  size="sm"
                  tone="danger"
                  label="Remove"
                  loading={busy === 'remove:' + m.user}
                  disabled={m.user === g.master || disbanded || busy !== ''}
                  onClick={() => removeMember(m.user)}
                />
              ),
            },
          ]}
        />
        {!disbanded && (
          <form
            className="flex items-center gap-2 border-t border-line/70 px-3 pb-2 pt-3"
            onSubmit={(e) => {
              e.preventDefault()
              if (busy === '') addMember()
            }}
          >
            <div className="min-w-0 flex-1">
              <ComboInput
                mono
                aria-label="Tenant uuid to add"
                options={dir.userIDs}
                placeholder="tenant uuid to add"
                value={adding}
                onChange={(e) => setAdding(e.target.value)}
              />
            </div>
            <Button type="submit" icon={Plus} loading={busy === 'add'} disabled={busy !== ''}>
              Add member
            </Button>
          </form>
        )}
      </Section>
    </div>
  )
}

function NewGroupDialog({ dir, onClose, onCreated }: { dir: Directory; onClose: () => void; onCreated: (g: Group) => void }) {
  const client = useClient()
  const alive = useAlive()
  const [name, setName] = useState('')
  const [creator, setCreator] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    const n = name.trim()
    const c = creator.trim()
    if (n === '' || c === '') {
      setError('A name and a creator are required.')
      return
    }
    setBusy(true)
    setError('')
    let g: Group
    try {
      g = await client.createGroup(n, c)
    } catch (err) {
      if (alive.current) {
        setError(errorMessage(err))
        setBusy(false)
      }
      return
    }
    if (alive.current) onCreated(g)
  }

  return (
    <FormModal
      title="New group"
      subtitle="Groups are recorded in the runtime store and are live immediately."
      okLabel="Create"
      busy={busy}
      error={error}
      onSubmit={submit}
      onClose={onClose}
    >
      <Field label="Name">
        <Input autoFocus placeholder="design-team" value={name} onChange={(e) => setName(e.target.value)} disabled={busy} />
      </Field>
      <Field label="Creator" hint="The creator's personal uuid. They become the master and the first member.">
        <ComboInput
          mono
          options={dir.userIDs}
          placeholder="creator's tenant uuid (becomes master)"
          value={creator}
          onChange={(e) => setCreator(e.target.value)}
          disabled={busy}
        />
      </Field>
    </FormModal>
  )
}
