// One profile: who they are, what they spent today, the handles linked to
// them, and what the journal says they did.
import { useState } from 'react'
import { Download, Gauge, SlidersHorizontal, Trash2, X } from 'lucide-react'
import { errorMessage } from '@/api/client'
import { billable, type Identity, type JournalEntry, type UsageEvent, type UserDetail as Detail } from '@/api/types'
import { Avatar, Button, Chip, Chips, CopyButton, IconButton, Meter, Panel, Section, StatTile, Table, type Tone } from '@/components/ui'
import { t } from '@/i18n'
import { downloadText, humanCount, orDash, relTime, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, prompt, toast } from '@/state/ui'
import { SettingsDialog } from './dialogs'
import { parseWhole, useAlive, type Notice } from './helpers'

const NO_USAGE = { input: 0, output: 0, cached: 0, cache_write: 0, reasoning: 0, calls: 0, failed: 0 }

function handleName(idn: Identity): string {
  return idn.username || idn.name || idn.native_from || ''
}

function statusTone(status: string): Tone {
  switch (status) {
    case 'failed':
    case 'blocked_safety':
    case 'dropped_queue':
      return 'danger'
    case 'delivered':
    case 'routed':
      return 'success'
  }
  return 'neutral'
}

export function UserDetail({
  detail,
  onChanged,
  onDeleted,
  onNotice,
}: {
  detail: Detail
  /** A mutation landed: 'all' reloads the list too, 'detail' only this profile. */
  onChanged: (scope: 'all' | 'detail') => void
  onDeleted: () => void
  onNotice: (notice: Notice) => void
}) {
  const client = useClient()
  const alive = useAlive()
  const [busy, setBusy] = useState('')
  const [editing, setEditing] = useState(false)

  const p = detail.profile
  const id = p.user_id
  const identities = p.identities ?? []
  const projects = detail.projects ?? []
  const calls = detail.recent_calls ?? []
  const audit = detail.audit ?? []
  const today = detail.usage_today ?? NO_USAGE
  const used = detail.quota?.used ?? 0
  const limit = detail.quota?.limit ?? 0
  const inFlight = detail.quota?.in_flight ?? 0

  const title = p.display_name || orDash(p.email)
  const trimmed = title.trim()
  const letter = trimmed !== '' && trimmed !== '—' ? trimmed.slice(0, 1).toUpperCase() : '?'

  // Runs one mutation: the button that started it stays disabled meanwhile,
  // and a refusal lands in the page banner where it stays readable.
  const run = async <T,>(key: string, call: () => Promise<T>, done: (result: T) => void) => {
    setBusy(key)
    try {
      done(await call())
    } catch (err) {
      onNotice({ text: errorMessage(err), tone: 'danger' })
    } finally {
      if (alive.current) setBusy('')
    }
  }

  const editLimit = async () => {
    const value = await prompt({
      title: t('users.limit.title'),
      subtitle: t('users.limit.subtitle'),
      label: t('users.limit.label'),
      placeholder: t('users.limit.placeholder'),
      initial: p.tokens_per_day > 0 ? String(p.tokens_per_day) : '',
      okLabel: t('users.limit.ok'),
      validate: (s) => {
        if (s === '') return null
        const n = parseWhole(s.replace(/_/g, ''))
        return n === null || n < 0 ? t('users.limit.invalid') : null
      },
    })
    if (value === null) return
    const n = parseWhole(value.replace(/_/g, '')) ?? 0
    await run(
      'limit',
      () => client.setUserLimit(id, n),
      () => {
        toast(t('users.limit.updated'), 'success')
        onChanged('all')
      },
    )
  }

  // The export is fetched first and saved second, so a refusal is reported
  // instead of downloading an error body.
  const exportProfile = () =>
    run(
      'export',
      () => client.exportUser(id),
      (text) => {
        const name = `agentflow-user-${shortID(id)}.json`
        void downloadText(name, text).then((saved) => saved && toast(t('users.exported', { name: saved }), 'success'))
      },
    )

  const exportBundle = () =>
    run(
      'bundle',
      () => client.exportUserBundle(id),
      (text) => {
        const name = `agentflow-bundle-${shortID(id)}.json`
        void downloadText(name, text).then((saved) => saved && toast(t('users.exported', { name: saved }), 'success'))
      },
    )

  const unlink = async (idn: Identity) => {
    const ok = await confirm({
      title: t('users.unlink.title'),
      message: t('users.unlink.message', { channel: idn.channel, handle: orDash(idn.native_from) }),
      confirmLabel: t('users.handles.unlink'),
      danger: true,
    })
    if (!ok) return
    await run(
      'unlink:' + idn.id,
      () => client.unlinkIdentity(id, idn.id),
      () => {
        toast(t('users.unlink.done'), 'warning')
        onChanged('all')
      },
    )
  }

  const remove = async () => {
    const ok = await confirm({
      title: t('users.deleteProfile'),
      message: t('users.delete.message'),
      confirmLabel: t('users.delete.confirm'),
      danger: true,
      typeToConfirm: id,
    })
    if (!ok) return
    await run(
      'delete',
      () => client.deleteUser(id),
      () => {
        toast(t('users.delete.done'), 'warning')
        onDeleted()
      },
    )
  }

  return (
    <div className="flex flex-col gap-3 pb-2">
      <Panel className="flex flex-col gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <Avatar letter={letter} size={44} />
          <div className="min-w-0 flex-1">
            <div className="truncate text-md font-semibold leading-tight">{title}</div>
            <div className="mt-0.5 flex min-w-0 items-center gap-1">
              <span className="truncate font-mono text-xs text-muted">{id}</span>
              <CopyButton text={id} what={t('users.whatUserId')} />
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          {p.model && <Chip tone="accent">{t('users.chipModel', { model: p.model })}</Chip>}
          {p.instructions_append && <Chip tone="accent">{t('users.chipInstructions')}</Chip>}
          <Chip>{p.tokens_per_day > 0 ? t('users.chipLimit', { n: humanCount(p.tokens_per_day) }) : t('users.chipDefaultLimit')}</Chip>
        </div>
        <div className="border-t border-line" />
        <div className="flex flex-wrap items-center gap-2">
          <Button icon={Gauge} loading={busy === 'limit'} disabled={busy !== ''} onClick={editLimit}>
            {t('users.dailyLimit')}
          </Button>
          <Button icon={SlidersHorizontal} disabled={busy !== ''} onClick={() => setEditing(true)}>
            {t('users.modelAndInstructions')}
          </Button>
          <Button icon={Download} loading={busy === 'export'} disabled={busy !== ''} onClick={exportProfile}>
            {t('users.export')}
          </Button>
          <Button icon={Download} loading={busy === 'bundle'} disabled={busy !== ''} onClick={exportBundle}>
            Migration bundle
          </Button>
          <IconButton icon={Trash2} label={t('users.deleteProfile')} tone="danger" loading={busy === 'delete'} disabled={busy !== ''} onClick={remove} />
        </div>
      </Panel>

      <Section title={t('users.quota.title')} subtitle={t('users.quota.subtitle', { day: orDash(detail.day) })}>
        <div className="flex flex-col gap-3">
          <Meter used={used} limit={limit} />
          <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            <StatTile
              label={t('users.quota.billableToday')}
              value={humanCount(billable(today))}
              sub={t('users.quota.inOut', { input: humanCount(today.input), output: humanCount(today.output) })}
            />
            <StatTile label={t('users.quota.calls')} value={humanCount(today.calls)} sub={t('users.quota.failed', { n: today.failed ?? 0 })} />
            <StatTile label={t('users.quota.cached')} value={humanCount(today.cached)} sub={t('users.quota.reasoning', { n: humanCount(today.reasoning) })} />
            <StatTile label={t('users.quota.inFlight')} value={humanCount(inFlight)} sub={t('users.quota.held')} />
          </div>
        </div>
      </Section>

      <Section title={t('users.handles.title')} subtitle={t('users.handles.subtitle')} flush>
        <Table<Identity>
          rows={identities}
          rowKey={(idn) => idn.id}
          empty={t('users.handles.empty')}
          columns={[
            { key: 'channel', title: t('users.handles.colChannel'), width: 110, render: (idn) => <Chip>{idn.channel}</Chip> },
            { key: 'handle', title: t('users.handles.colHandle'), render: (idn) => <span title={idn.native_from}>{handleName(idn)}</span> },
            { key: 'trust', title: t('users.handles.colTrust'), width: 96, render: (idn) => <Chip tone={idn.trust === 'verified' ? 'success' : 'warning'}>{orDash(idn.trust)}</Chip> },
            { key: 'seen', title: t('users.handles.colLastSeen'), width: 100, render: (idn) => relTime(idn.last_seen) },
            {
              key: 'unlink',
              title: '',
              width: 48,
              align: 'right',
              render: (idn) => (
                <IconButton
                  icon={X}
                  size="sm"
                  tone="danger"
                  label={identities.length <= 1 ? t('users.handles.lastCannotUnlink') : t('users.handles.unlink')}
                  loading={busy === 'unlink:' + idn.id}
                  disabled={identities.length <= 1 || busy !== ''}
                  onClick={() => unlink(idn)}
                />
              ),
            },
          ]}
        />
      </Section>

      {projects.length > 0 && (
        <Section title={t('users.projects.title')} subtitle={t('users.projects.subtitle')}>
          <Chips items={projects} />
        </Section>
      )}

      {calls.length > 0 && (
        <Section title={t('users.calls.title')} subtitle={t('users.calls.subtitle')} flush>
          <Table<UsageEvent>
            rows={calls}
            rowKey={(_, i) => String(i)}
            columns={[
              { key: 'when', title: t('users.calls.colWhen'), width: 90, render: (e) => relTime(e.ts) },
              { key: 'agent', title: t('users.calls.colAgent'), width: 130, render: (e) => e.agent },
              { key: 'model', title: t('users.calls.colModel'), mono: true, render: (e) => <span title={e.model}>{e.model}</span> },
              { key: 'tokens', title: t('users.calls.colTokens'), width: 80, align: 'right', mono: true, render: (e) => humanCount(billable(e)) },
              { key: 'ok', title: '', width: 76, render: (e) => (e.ok ? <Chip tone="success">{t('users.calls.ok')}</Chip> : <Chip tone="danger">{t('users.calls.failed')}</Chip>) },
            ]}
          />
        </Section>
      )}

      {audit.length > 0 && (
        <Section title={t('users.audit.title')} subtitle={t('users.audit.subtitle')} flush>
          <Table<JournalEntry>
            rows={audit}
            rowKey={(m, i) => m.id + ':' + i}
            columns={[
              { key: 'when', title: t('users.calls.colWhen'), width: 90, render: (m) => relTime(m.ts) },
              { key: 'dir', title: t('users.audit.colDir'), width: 56, render: (m) => (m.direction === 'out' ? <Chip>{t('users.audit.out')}</Chip> : <Chip tone="accent">{t('users.audit.in')}</Chip>) },
              { key: 'status', title: t('users.audit.colStatus'), width: 130, render: (m) => <Chip tone={statusTone(m.status)}>{m.status}</Chip> },
              { key: 'agent', title: t('users.calls.colAgent'), width: 110, render: (m) => orDash(m.agent) },
              {
                key: 'text',
                title: t('users.audit.colText'),
                render: (m) => {
                  const text = m.err ? m.err : (m.text ?? '').replace(/\n/g, ' ')
                  return <span title={text}>{text}</span>
                },
              },
            ]}
          />
        </Section>
      )}

      {editing && (
        <SettingsDialog
          profile={p}
          onClose={() => setEditing(false)}
          onSaved={() => {
            if (alive.current) setEditing(false)
            toast(t('users.settings.saved'), 'success')
            onChanged('detail')
          }}
        />
      )}
    </div>
  )
}
