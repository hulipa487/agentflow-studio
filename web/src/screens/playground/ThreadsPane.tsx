// Left pane: which agent, its recent threads, and who the turn acts as.
import type { Thread } from '@/api/types'
import { ComboInput, Field, Panel, Select, cx } from '@/components/ui'
import { relTime, shortID } from '@/lib/format'

function lastActive(t: Thread): string {
  const ms = Date.parse(t.last)
  return Number.isNaN(ms) ? '' : relTime(ms / 1000)
}

export function ThreadsPane({
  agents,
  agent,
  onAgent,
  threads,
  thread,
  onOpen,
  actAs,
  onActAs,
  userIDs,
}: {
  agents: string[]
  agent: string
  onAgent: (name: string) => void
  /** null until the first list for this agent arrives. */
  threads: Thread[] | null
  thread: string
  onOpen: (thread: string) => void
  actAs: string
  onActAs: (v: string) => void
  userIDs: string[]
}) {
  return (
    <Panel className="flex h-full min-h-0 flex-col gap-2">
      <Field label="Agent">
        <Select aria-label="Agent" value={agent} onChange={onAgent} options={agents} />
      </Field>

      <div className="mt-1 text-xs font-medium text-muted">Recent threads</div>
      <div className="-mx-1 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-1">
        {threads === null ? null : threads.length === 0 ? (
          <div className="py-1 text-sm text-muted">No threads yet.</div>
        ) : (
          threads.map((t) => {
            const active = t.thread === thread
            return (
              <button
                key={t.thread}
                type="button"
                title={t.thread}
                aria-current={active ? 'true' : undefined}
                onClick={() => onOpen(t.thread)}
                className={cx(
                  'flex h-7 w-full shrink-0 items-center gap-1.5 rounded-ctl px-2 text-left font-mono text-sm transition-colors duration-100',
                  active ? 'bg-accent-dim text-accent' : 'text-muted hover:bg-hover hover:text-text active:bg-pressed',
                )}
              >
                <span className={cx('shrink-0 font-semibold', !active && 'text-text')}>{shortID(t.thread)}</span>
                <span className="min-w-0 truncate">· {t.events} events</span>
                <span className={cx('ml-auto shrink-0 pl-1 text-xs', active ? 'text-accent' : 'text-faint')}>{lastActive(t)}</span>
              </button>
            )
          })
        )}
      </div>

      <div className="border-t border-line pt-2">
        <Field label="Act as user" hint="Optional. Tenant-less turns skip personal scopes and grants.">
          <ComboInput aria-label="Act as user" mono options={userIDs} value={actAs} onChange={(e) => onActAs(e.target.value)} placeholder="profile uuid" />
        </Field>
      </div>
    </Panel>
  )
}
