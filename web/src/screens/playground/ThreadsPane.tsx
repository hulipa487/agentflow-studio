// Left pane: which agent, its recent threads, and who the turn acts as.
import type { Thread } from '@/api/types'
import { ComboInput, Field, Panel, Select, cx } from '@/components/ui'
import { t } from '@/i18n'
import { relTime, shortID } from '@/lib/format'

function lastActive(th: Thread): string {
  const ms = Date.parse(th.last)
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
      <Field label={t('playground.agent')}>
        <Select aria-label={t('playground.agent')} value={agent} onChange={onAgent} options={agents} />
      </Field>

      <div className="mt-1 text-xs font-medium text-muted">{t('playground.recentThreads')}</div>
      <div className="-mx-1 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-1">
        {threads === null ? null : threads.length === 0 ? (
          <div className="py-1 text-sm text-muted">{t('playground.noThreads')}</div>
        ) : (
          threads.map((th) => {
            const active = th.thread === thread
            return (
              <button
                key={th.thread}
                type="button"
                title={th.thread}
                aria-current={active ? 'true' : undefined}
                onClick={() => onOpen(th.thread)}
                className={cx(
                  'flex h-7 w-full shrink-0 items-center gap-1.5 rounded-ctl px-2 text-left font-mono text-sm transition-colors duration-100',
                  active ? 'bg-accent-dim text-accent' : 'text-muted hover:bg-hover hover:text-text active:bg-pressed',
                )}
              >
                <span className={cx('shrink-0 font-semibold', !active && 'text-text')}>{shortID(th.thread)}</span>
                <span className="min-w-0 truncate">· {t('playground.threadEvents', { n: th.events })}</span>
                <span className={cx('ml-auto shrink-0 pl-1 text-xs', active ? 'text-accent' : 'text-faint')}>{lastActive(th)}</span>
              </button>
            )
          })
        )}
      </div>

      <div className="border-t border-line pt-2">
        <Field label={t('playground.actAs')} hint={t('playground.actAsHint')}>
          <ComboInput aria-label={t('playground.actAs')} mono options={userIDs} value={actAs} onChange={(e) => onActAs(e.target.value)} placeholder={t('playground.actAsPlaceholder')} />
        </Field>
      </div>
    </Panel>
  )
}
