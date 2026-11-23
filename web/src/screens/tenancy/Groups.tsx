// Groups: one master and any number of members sharing a context. The list is
// on the left; the selected group's members are managed on the right.
import { useMemo, useState } from 'react'
import { Plus, Trash2, Users, X } from 'lucide-react'
import { errorMessage } from '@/api/client'
import type { Group, Membership } from '@/api/types'
import { Banner, Button, Chip, ComboInput, EmptyState, Field, IconButton, Input, KV, Loading, Panel, Section, StatusChip, Table } from '@/components/ui'
import { Split } from '@/components/ui/Split'
import { useAsync } from '@/lib/hooks'
import { t } from '@/i18n'
import { relTime, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { useAlive } from '../users/helpers'
import { PaneNote, RailRow } from '../users/rail'
import { FormModal, Ident, type Directory, type PaneProps } from './shared'

const disbandedAt = (g: Group) => g.disbanded_at ?? 0

export function GroupsPane({ dir, selected, onSelect, rev, reloadAll, notify }: PaneProps) {
  const [creating, setCreating] = useState(false)
  const group = dir.groups.find((g) => g.uuid === selected)

  const left = (
    <div className="flex min-h-0 flex-1 flex-col gap-2 rounded-card border border-line bg-surface p-2.5">
      <Button icon={Plus} className="self-start" onClick={() => setCreating(true)}>
        {t('tenancy.groups.new')}
      </Button>
      <div className="-mx-1 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-1">
        {dir.groups.map((g) => (
          <RailRow
            key={g.uuid}
            icon={Users}
            title={g.name}
            caption={shortID(g.uuid) + (disbandedAt(g) > 0 ? ' · ' + t('tenancy.groups.disbandedWhen', { when: relTime(disbandedAt(g)) }) : '')}
            active={g.uuid === selected}
            onClick={() => onSelect(g.uuid)}
          />
        ))}
      </div>
    </div>
  )

  const right = (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {group ? (
        <GroupDetail key={group.uuid} id={group.uuid} dir={dir} rev={rev} reloadAll={reloadAll} notify={notify} />
      ) : (
        <PaneNote lowered>{t('tenancy.groups.selectHint')}</PaneNote>
      )}
    </div>
  )

  return (
    <>
      {dir.groups.length === 0 ? (
        <EmptyState
          icon={<Users className="mb-1 size-7 text-faint" strokeWidth={1.5} />}
          title={t('tenancy.tabGroups')}
          guidance={t('tenancy.groups.empty')}
          actionLabel={t('tenancy.groups.new')}
          onAction={() => setCreating(true)}
        />
      ) : (
        <Split fixed="left" initial={300} min={220} max={520} storageKey="tenancy-groups" left={left} right={right} />
      )}
      {creating && (
        <NewGroupDialog
          dir={dir}
          onClose={() => setCreating(false)}
          onCreated={(g) => {
            setCreating(false)
            onSelect(g.uuid)
            toast(t('tenancy.groups.created', { name: g.name }), 'success')
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
    return <Loading text={t('tenancy.groups.loading')} />
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
      notify({ text: t('tenancy.groups.enterUuid'), tone: 'warning' })
      return
    }
    void run('add', () => client.addMember(g.uuid, user), t('tenancy.groups.memberAdded'), 'success', 'detail').then((ok) => {
      if (ok && alive.current) setAdding('')
    })
  }

  const setMaster = async (user: string) => {
    const ok = await confirm({
      title: t('tenancy.groups.changeMaster'),
      message: t('tenancy.groups.changeMasterMessage', { name: g.name, from: who(g.master), to: who(user) }),
      confirmLabel: t('tenancy.groups.changeMaster'),
    })
    if (ok) await run('master:' + user, () => client.setMaster(g.uuid, g.master, user), t('tenancy.groups.masterChanged'), 'success', 'all')
  }

  const removeMember = async (user: string) => {
    const ok = await confirm({
      title: t('tenancy.groups.removeMember'),
      message: t('tenancy.groups.removeMemberMessage', { who: who(user), name: g.name }),
      confirmLabel: t('common.remove'),
      danger: true,
    })
    if (ok) await run('remove:' + user, () => client.removeMember(g.uuid, user), t('tenancy.groups.memberRemoved'), 'warning', 'detail')
  }

  const disband = async () => {
    const ok = await confirm({
      title: t('tenancy.groups.disbandTitle'),
      message: t('tenancy.groups.disbandMessage', { name: g.name, master: who(g.master) }),
      confirmLabel: t('tenancy.groups.disbandConfirm'),
      danger: true,
    })
    // The runtime only accepts a disband from the current master.
    if (ok) await run('disband', () => client.disbandGroup(g.uuid, g.master), t('tenancy.groups.disbandDone'), 'warning', 'all')
  }

  return (
    <div className="flex flex-col gap-3 pb-2">
      <Panel className="flex flex-col gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-md font-semibold">{g.name}</span>
          {disbanded ? <StatusChip tone="danger">{t('tenancy.groups.stateDisbanded')}</StatusChip> : <StatusChip tone="success">{t('tenancy.groups.stateActive')}</StatusChip>}
          <span className="flex-1" />
          <Button variant="danger" icon={Trash2} loading={busy === 'disband'} disabled={disbanded || busy !== ''} onClick={disband}>
            {t('tenancy.groups.disbandButton')}
          </Button>
        </div>
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-3">
          <KV label={t('tenancy.groups.kvUuid')} value={g.uuid} copy={g.uuid} />
          <KV label={t('tenancy.groups.master')} value={who(g.master)} copy={g.master} />
          <KV label={t('tenancy.kvCreated')} value={relTime(g.created_at)} />
          {disbanded && <KV label={t('tenancy.groups.kvDisbanded')} value={relTime(disbandedAt(g))} />}
        </div>
      </Panel>

      <Section
        title={t('tenancy.groups.membersTitle')}
        subtitle={t('tenancy.groups.membersSubtitle')}
        flush
      >
        <Table<Membership>
          rows={members}
          rowKey={(m) => m.user}
          empty={t('tenancy.groups.noMembers')}
          columns={[
            { key: 'role', title: t('tenancy.groups.colRole'), width: 86, render: (m) => (m.user === g.master ? <Chip tone="accent">{t('tenancy.groups.master')}</Chip> : <Chip>{t('tenancy.groups.member')}</Chip>) },
            { key: 'tenant', title: t('tenancy.colTenant'), render: (m) => <Ident label={who(m.user)} id={m.user} what={t('tenancy.whatTenantUuid')} /> },
            { key: 'joined', title: t('tenancy.groups.colJoined'), width: 100, render: (m) => relTime(m.joined_at) },
            {
              key: 'master',
              title: '',
              width: 124,
              align: 'right',
              render: (m) => (
                <Button size="sm" loading={busy === 'master:' + m.user} disabled={m.user === g.master || disbanded || busy !== ''} onClick={() => setMaster(m.user)}>
                  {t('tenancy.groups.makeMaster')}
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
                  label={t('common.remove')}
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
                aria-label={t('tenancy.groups.addAria')}
                options={dir.userIDs}
                placeholder={t('tenancy.groups.addPlaceholder')}
                value={adding}
                onChange={(e) => setAdding(e.target.value)}
              />
            </div>
            <Button type="submit" icon={Plus} loading={busy === 'add'} disabled={busy !== ''}>
              {t('tenancy.groups.addMember')}
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
      setError(t('tenancy.groups.required'))
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
      title={t('tenancy.groups.new')}
      subtitle={t('tenancy.groups.newSubtitle')}
      okLabel={t('common.create')}
      busy={busy}
      error={error}
      onSubmit={submit}
      onClose={onClose}
    >
      <Field label={t('common.name')}>
        <Input autoFocus placeholder="design-team" value={name} onChange={(e) => setName(e.target.value)} disabled={busy} />
      </Field>
      <Field label={t('tenancy.creator')} hint={t('tenancy.groups.creatorHint')}>
        <ComboInput
          mono
          options={dir.userIDs}
          placeholder={t('tenancy.groups.creatorPlaceholder')}
          value={creator}
          onChange={(e) => setCreator(e.target.value)}
          disabled={busy}
        />
      </Field>
    </FormModal>
  )
}
