// One profile: who they are, what they spent today, the handles linked to
// them, and what the journal says they did.
import { useState } from 'react'
import { Download, Gauge, SlidersHorizontal, Trash2, X } from 'lucide-react'
import { errorMessage } from '@/api/client'
import { billable, type Identity, type JournalEntry, type UsageEvent, type UserDetail as Detail } from '@/api/types'
import { Avatar, Button, Chip, Chips, CopyButton, IconButton, Meter, Panel, Section, StatTile, Table, type Tone } from '@/components/ui'
import { downloadText, humanCount, orDash, relTime, shortID } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, prompt, toast } from '@/state/ui'
import { SettingsDialog } from './dialogs'
import { parseWhole, quoted, useAlive, type Notice } from './helpers'

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
      title: 'Daily token limit',
      subtitle: 'Billable tokens (input + output) per UTC day. 0 or empty inherits usage.default_tokens_per_day.',
      label: 'Tokens per day',
      placeholder: 'e.g. 200000',
      initial: p.tokens_per_day > 0 ? String(p.tokens_per_day) : '',
      okLabel: 'Set limit',
      validate: (s) => {
        if (s === '') return null
        const n = parseWhole(s.replace(/_/g, ''))
        return n === null || n < 0 ? 'enter a whole number of tokens, 0 or more' : null
      },
    })
    if (value === null) return
    const n = parseWhole(value.replace(/_/g, '')) ?? 0
    await run(
      'limit',
      () => client.setUserLimit(id, n),
      () => {
        toast('limit updated', 'success')
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
        downloadText(name, text)
        toast('exported to ' + name, 'success')
      },
    )

  const unlink = async (idn: Identity) => {
    const ok = await confirm({
      title: 'Unlink handle',
      message: `Detach the ${idn.channel} handle ${quoted(orDash(idn.native_from))} from this profile?\n\nThat handle loses the personal scope until it is linked again.`,
      confirmLabel: 'Unlink',
      danger: true,
    })
    if (!ok) return
    await run(
      'unlink:' + idn.id,
      () => client.unlinkIdentity(id, idn.id),
      () => {
        toast('handle unlinked', 'warning')
        onChanged('all')
      },
    )
  }

  const remove = async () => {
    const ok = await confirm({
      title: 'Delete profile',
      message:
        'This purges the profile, its handles, usage, journal rows and files. Memory rows follow their store retention. It cannot be undone. Type the user id to confirm.',
      confirmLabel: 'Delete forever',
      danger: true,
      typeToConfirm: id,
    })
    if (!ok) return
    await run(
      'delete',
      () => client.deleteUser(id),
      () => {
        toast('profile deleted', 'warning')
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
              <CopyButton text={id} what="user id" />
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          {p.model && <Chip tone="accent">model {p.model}</Chip>}
          {p.instructions_append && <Chip tone="accent">+instructions</Chip>}
          <Chip>{p.tokens_per_day > 0 ? humanCount(p.tokens_per_day) + '/day' : 'default limit'}</Chip>
        </div>
        <div className="border-t border-line" />
        <div className="flex flex-wrap items-center gap-2">
          <Button icon={Gauge} loading={busy === 'limit'} disabled={busy !== ''} onClick={editLimit}>
            Daily limit…
          </Button>
          <Button icon={SlidersHorizontal} disabled={busy !== ''} onClick={() => setEditing(true)}>
            Model &amp; instructions…
          </Button>
          <Button icon={Download} loading={busy === 'export'} disabled={busy !== ''} onClick={exportProfile}>
            Export…
          </Button>
          <IconButton icon={Trash2} label="Delete profile" tone="danger" loading={busy === 'delete'} disabled={busy !== ''} onClick={remove} />
        </div>
      </Panel>

      <Section title="Quota" subtitle={`Day ${orDash(detail.day)} (UTC). The limit is the profile's own, or usage.default_tokens_per_day.`}>
        <div className="flex flex-col gap-3">
          <Meter used={used} limit={limit} />
          <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            <StatTile label="Billable today" value={humanCount(billable(today))} sub={`in ${humanCount(today.input)} · out ${humanCount(today.output)}`} />
            <StatTile label="Calls" value={humanCount(today.calls)} sub={`${today.failed ?? 0} failed`} />
            <StatTile label="Cached" value={humanCount(today.cached)} sub={`reasoning ${humanCount(today.reasoning)}`} />
            <StatTile label="In flight" value={humanCount(inFlight)} sub="held against the quota" />
          </div>
        </div>
      </Section>

      <Section title="Linked handles" subtitle="A person links a handle by sending /link <code> from it. The last handle cannot be unlinked." flush>
        <Table<Identity>
          rows={identities}
          rowKey={(idn) => idn.id}
          empty="No linked handles."
          columns={[
            { key: 'channel', title: 'channel', width: 110, render: (idn) => <Chip>{idn.channel}</Chip> },
            { key: 'handle', title: 'handle', render: (idn) => <span title={idn.native_from}>{handleName(idn)}</span> },
            { key: 'trust', title: 'trust', width: 96, render: (idn) => <Chip tone={idn.trust === 'verified' ? 'success' : 'warning'}>{orDash(idn.trust)}</Chip> },
            { key: 'seen', title: 'last seen', width: 100, render: (idn) => relTime(idn.last_seen) },
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
                  label={identities.length <= 1 ? 'The last handle cannot be unlinked' : 'Unlink'}
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
        <Section title="File projects" subtitle="The user-scoped file store (files.*).">
          <Chips items={projects} />
        </Section>
      )}

      {calls.length > 0 && (
        <Section title="Recent calls" subtitle="Per-call detail (usage.events), last 7 days." flush>
          <Table<UsageEvent>
            rows={calls}
            rowKey={(_, i) => String(i)}
            columns={[
              { key: 'when', title: 'when', width: 90, render: (e) => relTime(e.ts) },
              { key: 'agent', title: 'agent', width: 130, render: (e) => e.agent },
              { key: 'model', title: 'model', mono: true, render: (e) => <span title={e.model}>{e.model}</span> },
              { key: 'tokens', title: 'tokens', width: 80, align: 'right', mono: true, render: (e) => humanCount(billable(e)) },
              { key: 'ok', title: '', width: 76, render: (e) => (e.ok ? <Chip tone="success">ok</Chip> : <Chip tone="danger">failed</Chip>) },
            ]}
          />
        </Section>
      )}

      {audit.length > 0 && (
        <Section title="Audit trail" subtitle="The core-owned message journal for this person, newest first." flush>
          <Table<JournalEntry>
            rows={audit}
            rowKey={(m, i) => m.id + ':' + i}
            columns={[
              { key: 'when', title: 'when', width: 90, render: (m) => relTime(m.ts) },
              { key: 'dir', title: 'dir', width: 56, render: (m) => (m.direction === 'out' ? <Chip>out</Chip> : <Chip tone="accent">in</Chip>) },
              { key: 'status', title: 'status', width: 130, render: (m) => <Chip tone={statusTone(m.status)}>{m.status}</Chip> },
              { key: 'agent', title: 'agent', width: 110, render: (m) => orDash(m.agent) },
              {
                key: 'text',
                title: 'text',
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
            toast('settings saved · applies from the next turn', 'success')
            onChanged('detail')
          }}
        />
      )}
    </div>
  )
}
