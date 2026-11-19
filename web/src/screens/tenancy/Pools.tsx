// Memory pools: named shared stores owned by exactly one tenant or group, and
// the per-(agent, tenant) grants that open them.
import { useMemo, useState } from 'react'
import { Check, Database, Plus, X } from 'lucide-react'
import { errorMessage } from '@/api/client'
import type { Grant, Pool, PoolDraft } from '@/api/types'
import { Banner, Button, Chip, ComboInput, CopyButton, Field, IconButton, Input, KV, Loading, Panel, Section, Segmented, Select, Table } from '@/components/ui'
import { Split } from '@/components/ui/Split'
import { useAsync } from '@/lib/hooks'
import { t } from '@/i18n'
import { tr } from '@/i18n/rich'
import { relTime, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { byText, parseWhole, useAlive } from '../users/helpers'
import { PaneNote, RailRow } from '../users/rail'
import { FormModal, Ident, type Directory, type PaneProps } from './shared'

const MODES = [
  { value: 'rw', labelKey: 'tenancy.pools.modeOptRW' },
  { value: 'r', labelKey: 'tenancy.pools.modeOptR' },
]

/** The grant that covers every active member is stored as "*". */
const everyMember = (tenant: string | undefined) => tenant === '*' || !tenant

export function PoolsPane({ dir, selected, onSelect, rev, reloadAll, notify }: PaneProps) {
  const [creating, setCreating] = useState(false)
  const pool = dir.pools.find((p) => p.uuid === selected)

  const left = (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <Button icon={Plus} className="self-start" onClick={() => setCreating(true)}>
        {t('tenancy.pools.new')}
      </Button>
      <div className="-mx-1 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-1">
        {dir.pools.length === 0 && <PaneNote>{t('tenancy.pools.empty')}</PaneNote>}
        {dir.pools.map((p) => {
          const owner = p.owner?.group ? t('tenancy.pools.ownerGroup', { id: shortID(p.owner.group) }) : t('tenancy.pools.ownerTenant', { id: shortID(p.owner?.tenant) })
          return (
            <RailRow
              key={p.uuid}
              icon={Database}
              title={p.name}
              caption={owner + ' · ' + t('tenancy.pools.retentionShort', { days: p.retention_days })}
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
        <PaneNote lowered>{t('tenancy.pools.selectHint')}</PaneNote>
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
            toast(t('tenancy.pools.created', { name: p.name }), 'success')
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
      notify({ text: t('tenancy.pools.chooseAgent'), tone: 'warning' })
      return
    }
    // An empty tenant is the member wildcard.
    void run('grant', () => client.setGrant(p.uuid, ag, tenant.trim(), mode), t('tenancy.pools.grantSaved'), 'success').then((ok) => {
      if (ok && alive.current) {
        setAgent('')
        setTenant('')
        setMode('rw')
      }
    })
  }

  const revoke = async (g: Grant) => {
    const ok = await confirm({
      title: t('tenancy.pools.revokeTitle'),
      message: t('tenancy.pools.revokeMessage', { agent: g.agent, mode: g.mode, name: p.name }),
      confirmLabel: t('tenancy.pools.revoke'),
      danger: true,
    })
    if (!ok) return
    const tenantID = g.tenant === '*' ? '' : (g.tenant ?? '')
    await run(`revoke:${g.agent}:${g.tenant}`, () => client.removeGrant(p.uuid, g.agent, tenantID), t('tenancy.pools.grantRevoked'), 'warning')
  }

  return (
    <div className="flex flex-col gap-3 pb-2">
      <Panel className="flex flex-col gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-md font-semibold">{p.name}</span>
          <Chip tone="accent">{ownerGroup !== '' ? t('tenancy.pools.kindGroup') : t('tenancy.pools.kindTenant')}</Chip>
          <span className="flex-1" />
          <CopyButton text={'pool: ' + p.name} what={t('tenancy.pools.whatBinding')} size="md" />
        </div>
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <KV label={t('tenancy.pools.kvUuid')} value={p.uuid} copy={p.uuid} />
          <KV label={t('tenancy.pools.kvOwner')} value={owner} copy={ownerGroup !== '' ? ownerGroup : ownerTenant || undefined} />
          <KV label={t('tenancy.pools.kvRetention')} value={t('tenancy.pools.retentionValue', { days: p.retention_days })} />
          <KV label={t('tenancy.kvCreated')} value={t('tenancy.pools.createdBy', { when: relTime(p.created_at), who: shortID(p.created_by) })} />
        </div>
        <p className="break-words text-xs leading-snug text-faint">
          {tr(
            'tenancy.pools.bindStore',
            { binding: `stores: { shared: { backend: <name>, table: <table>, pool: ${p.name} } }` },
            { code: (c) => <code className="font-mono text-muted">{c}</code> },
          )}
        </p>
      </Panel>

      <Section
        title={t('tenancy.pools.grantsTitle')}
        subtitle={t('tenancy.pools.grantsSubtitle')}
        flush
      >
        {!data ? (
          error && !loading ? (
            <div className="px-3 pb-2">
              <Banner tone="danger">{errorMessage(error)}</Banner>
            </div>
          ) : (
            <Loading text={t('tenancy.pools.loadingGrants')} />
          )
        ) : (
          <Table<Grant>
            rows={grants}
            rowKey={(g) => g.agent + '\u0000' + g.tenant}
            empty={t('tenancy.pools.noGrants')}
            columns={[
              { key: 'agent', title: t('tenancy.colAgent'), width: 160, render: (g) => g.agent },
              {
                key: 'tenant',
                title: t('tenancy.colTenant'),
                render: (g) => (everyMember(g.tenant) ? <span className="text-muted">{t('tenancy.pools.everyMember')}</span> : <Ident label={who(g.tenant)} id={g.tenant} what={t('tenancy.whatTenantUuid')} />),
              },
              { key: 'mode', title: t('tenancy.pools.colMode'), width: 104, render: (g) => (g.mode === 'r' ? <Chip>{t('tenancy.pools.modeR')}</Chip> : <Chip tone="success">{t('tenancy.pools.modeRW')}</Chip>) },
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
                    label={t('tenancy.pools.revoke')}
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
          <ComboInput aria-label={t('tenancy.pools.agentAria')} options={dir.agents} placeholder={t('tenancy.pools.agentPlaceholder')} value={agent} onChange={(e) => setAgent(e.target.value)} />
          <ComboInput
            mono
            aria-label={t('tenancy.pools.tenantAria')}
            options={dir.userIDs}
            placeholder={t('tenancy.pools.tenantPlaceholder')}
            value={tenant}
            onChange={(e) => setTenant(e.target.value)}
          />
          <Select aria-label={t('tenancy.pools.modeAria')} value={mode} onChange={setMode} options={MODES.map((m) => ({ value: m.value, label: t(m.labelKey) }))} />
          <Button type="submit" icon={Check} loading={busy === 'grant'} disabled={busy !== ''}>
            {t('tenancy.pools.grant')}
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
      setError(t('tenancy.pools.retentionInvalid'))
      return
    }
    const draft: PoolDraft = { name: name.trim(), creator: creator.trim(), retention_days: days }
    if (kind === 'tenant') draft.owner_tenant = ownerTenant.trim()
    else draft.owner_group = resolveGroup(ownerGroup.trim())
    if (draft.name === '' || draft.creator === '' || (!draft.owner_tenant && !draft.owner_group)) {
      setError(t('tenancy.pools.required'))
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
      title={t('tenancy.pools.newTitle')}
      subtitle={t('tenancy.pools.newSubtitle')}
      okLabel={t('tenancy.pools.create')}
      busy={busy}
      error={error}
      onSubmit={submit}
      onClose={onClose}
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label={t('common.name')}>
          <Input autoFocus placeholder="team-notes" value={name} onChange={(e) => setName(e.target.value)} disabled={busy} />
        </Field>
        <Field label={t('tenancy.pools.retentionLabel')}>
          <Input mono inputMode="numeric" value={retention} onChange={(e) => setRetention(e.target.value)} disabled={busy} />
        </Field>
      </div>
      <Field label={t('tenancy.creator')}>
        <ComboInput mono options={dir.userIDs} placeholder={t('tenancy.pools.creatorPlaceholder')} value={creator} onChange={(e) => setCreator(e.target.value)} disabled={busy} />
      </Field>
      <Field label={t('tenancy.pools.owner')}>
        <Segmented
          className="self-start"
          value={kind}
          onChange={setKind}
          options={[
            { value: 'tenant', label: t('tenancy.pools.ownerKindTenant'), disabled: busy },
            { value: 'group', label: t('tenancy.pools.ownerKindGroup'), disabled: busy },
          ]}
        />
        {kind === 'group' ? (
          <ComboInput
            aria-label={t('tenancy.pools.ownerGroupAria')}
            options={groupLabels}
            placeholder={t('tenancy.pools.ownerGroupPlaceholder')}
            value={ownerGroup}
            onChange={(e) => setOwnerGroup(e.target.value)}
            disabled={busy}
          />
        ) : (
          <ComboInput
            mono
            aria-label={t('tenancy.pools.ownerTenantAria')}
            options={dir.userIDs}
            placeholder={t('tenancy.pools.ownerTenantPlaceholder')}
            value={ownerTenant}
            onChange={(e) => setOwnerTenant(e.target.value)}
            disabled={busy}
          />
        )}
      </Field>
      <p className="text-xs leading-snug text-faint">{t('tenancy.pools.namesNote')}</p>
    </FormModal>
  )
}
