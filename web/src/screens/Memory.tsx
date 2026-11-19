// Memory: the backends memory lives in (memory.backends) and the named
// profiles agents pick (profiles.memory). Agents open their stores when
// agentflow starts, so every entry says whether it is live or waiting for a
// restart.
import { useRef, useState, type ReactNode } from 'react'
import { Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { MemoryBackend, MemoryList, MemoryProfile } from '@/api/types'
import { AsyncView, Banner, Button, Chip, CopyButton, IconButton, Page, Section } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { plural } from '@/lib/format'
import { useClient, useSession } from '@/state/session'
import { confirm, info, toast } from '@/state/ui'
import { LiveChip, useMounted } from './safety/parts'
import { BackendEditor } from './memory/BackendEditor'
import { ProfileEditor } from './memory/ProfileEditor'
import { backendInUse, backendTarget, backendUsedBy, profileInUse, profileUsedBy, storeSummary } from './memory/helpers'

type Editor = { kind: 'profile'; existing: MemoryProfile | null } | { kind: 'backend'; existing: MemoryBackend | null }

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 items-baseline gap-3">
      <span className="w-[76px] shrink-0 text-xs font-semibold text-faint">{label}</span>
      <div className="min-w-0 flex-1 text-xs text-muted">{children}</div>
    </div>
  )
}

export default function Memory() {
  const client = useClient()
  const mounted = useMounted()
  const { data: list, error, loading, refreshing, reload, setData } = useAsync<MemoryList>(
    async (signal) => {
      try {
        return await client.memory(signal)
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          throw new ApiError(404, 'This runtime does not expose the Studio memory API yet; rebuild and restart AgentFlow, then reopen Memory.')
        }
        throw err
      }
    },
    [client],
  )
  const [editor, setEditor] = useState<Editor | null>(null)
  const [actionError, setActionError] = useState('')
  const [deleting, setDeleting] = useState('')
  const latest = useRef(list)
  latest.current = list

  const refresh = () => {
    setActionError('')
    reload()
  }
  useScreenActions({ refresh })

  const backends = list?.backends ?? []
  const profiles = list?.profiles ?? []
  const builtin = list?.builtin_agents ?? []
  const editable = list?.editable ?? false

  const readOnly = () =>
    void info({
      title: 'Memory is read-only',
      message: list?.note || 'This deployment has no editable config source, so memory cannot be changed from Studio.',
    })

  const open = (next: Editor) => {
    if (!editable) return readOnly()
    setEditor(next)
  }

  // The catalog carries the profile names the agent builder offers.
  const applied = (next: MemoryList, status: string) => {
    setActionError('')
    setData(next)
    void useSession.getState().reloadCatalog()
    toast(status, 'warning')
  }

  const reloadForEditor = async () => {
    const next = await client.memory()
    if (mounted.current) setData(next)
  }

  const remove = async (kind: Editor['kind'], name: string) => {
    if (!editable) return readOnly()
    const ok = await confirm({
      title: kind === 'profile' ? 'Delete memory profile' : 'Delete memory backend',
      message:
        kind === 'profile'
          ? `Delete memory profile "${name}" from the deployment config?\n\nThe stored records are not deleted.`
          : `Delete memory backend "${name}" from the deployment config?\n\nThe database itself and its records are not touched.`,
      confirmLabel: 'Delete',
      danger: true,
    })
    if (!ok || !mounted.current) return
    const mtime = latest.current?.mtime
    setDeleting(`${kind}:${name}`)
    try {
      const next = kind === 'profile' ? await client.deleteMemoryProfile(name, mtime) : await client.deleteMemoryBackend(name, mtime)
      if (!mounted.current) return
      applied(next, `memory ${kind} deleted · restart required`)
    } catch (err) {
      if (mounted.current) setActionError(errorMessage(err))
    } finally {
      if (mounted.current) setDeleting('')
    }
  }

  const restart = list?.restart_required ?? false
  const pendingNote = list ? (restart ? list.saved_note || list.note : list.saved_note) : ''

  return (
    <Page
      title="Memory"
      subtitle="Where agents keep what they remember: backends are the databases, profiles are the stores an agent reads and writes. Saves go to the deployment config and open on the next runtime start."
      actions={<IconButton icon={RotateCw} label="Refresh" loading={refreshing} onClick={refresh} />}
      meta={
        list && (
          <>
            <LiveChip live={!restart} />
            <span>{plural(backends.length, 'backend')}</span>
            <span>{plural(profiles.length, 'profile')}</span>
            <span>source {list.mode || 'file'}</span>
            {list.path && (
              <span className="flex min-w-0 items-center gap-1">
                <span className="truncate" title={list.path}>
                  {list.path}
                </span>
                <CopyButton text={list.path} what="path" />
              </span>
            )}
            <span className={editable ? undefined : 'text-warning'}>{editable ? 'editable' : 'read-only'}</span>
          </>
        )
      }
      banner={
        list && (actionError || !editable || pendingNote) ? (
          <>
            {actionError && (
              <Banner tone="danger" onClose={() => setActionError('')}>
                {actionError}
              </Banner>
            )}
            {!editable ? (
              <Banner tone="warning">{list.note || 'This deployment has no editable config source for memory.'}</Banner>
            ) : (
              pendingNote && (
                <Banner tone="warning">
                  {restart && 'Restart required — '}
                  {pendingNote}
                </Banner>
              )
            )}
          </>
        ) : undefined
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!list} retry={refresh} loadingText="Loading memory…">
        <div className="flex flex-col gap-3">
          <Section
            flush
            title="Profiles"
            subtitle={
              builtin.length > 0
                ? `Named store sets agents pick in the builder. The built-in "conversational" profile is used by ${builtin.join(', ')}.`
                : 'Named store sets agents pick in the builder, besides the built-in "conversational" one.'
            }
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'profile', existing: null })}>
                New profile
              </Button>
            }
          >
            {profiles.length === 0 && <p className="px-3 pb-2 text-sm text-muted">No named memory profiles yet.</p>}
            {profiles.map((p) => {
              const inUse = profileInUse(p)
              return (
                <div key={p.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{p.name}</span>
                    <Chip tone="accent">{plural((p.stores ?? []).length, 'store')}</Chip>
                    {p.recall && <Chip>{p.recall}</Chip>}
                    <LiveChip live={p.live} />
                    <div className="flex-1" />
                    <IconButton icon={Pencil} label={`Edit ${p.name}`} onClick={() => open({ kind: 'profile', existing: p })} />
                    <IconButton
                      icon={Trash2}
                      tone="danger"
                      label={inUse ? `Cannot delete ${p.name}; it is used by ${profileUsedBy(p)}.` : `Delete ${p.name}`}
                      disabled={inUse || deleting !== ''}
                      loading={deleting === 'profile:' + p.name}
                      onClick={() => void remove('profile', p.name)}
                    />
                  </div>
                  <div className="mt-1.5 flex flex-col gap-1">
                    {(p.stores ?? []).map((s) => (
                      <Row key={s.name} label={s.name}>
                        <span className="font-mono">{storeSummary(s)}</span>
                      </Row>
                    ))}
                    {p.embed_model && (
                      <Row label="embedding">
                        <span className="font-mono">
                          {p.embed_model}
                          {p.rerank_model ? ` · rerank ${p.rerank_model}` : ''}
                        </span>
                      </Row>
                    )}
                    <Row label="used by">{profileUsedBy(p)}</Row>
                  </div>
                </div>
              )
            })}
          </Section>

          <Section
            flush
            title="Backends"
            subtitle="The databases stores live in. Secrets are never shown; use ${VAR} or cred:<service> to keep them out of the file."
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'backend', existing: null })}>
                New backend
              </Button>
            }
          >
            {backends.map((b) => {
              const inUse = backendInUse(b, builtin)
              return (
                <div key={b.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{b.name}</span>
                    <Chip tone="accent">{b.provider}</Chip>
                    {b.implicit ? <Chip title="Not in the file: the runtime store, used while no backend is declared">built-in</Chip> : <LiveChip live={b.live} />}
                    <div className="flex-1" />
                    <IconButton
                      icon={Pencil}
                      label={b.implicit ? 'Write main_db out to the config to change it' : `Edit ${b.name}`}
                      onClick={() => open({ kind: 'backend', existing: b })}
                    />
                    {!b.implicit && (
                      <IconButton
                        icon={Trash2}
                        tone="danger"
                        label={inUse ? `Cannot delete ${b.name}; it is used by ${backendUsedBy(b, builtin)}.` : `Delete ${b.name}`}
                        disabled={inUse || deleting !== ''}
                        loading={deleting === 'backend:' + b.name}
                        onClick={() => void remove('backend', b.name)}
                      />
                    )}
                  </div>
                  <div className="mt-1.5 flex flex-col gap-1">
                    <Row label="target">
                      <span className="font-mono">{backendTarget(b)}</span>
                    </Row>
                    {b.dim ? (
                      <Row label="dimensions">
                        <span className="font-mono">{b.dim}</span>
                      </Row>
                    ) : null}
                    <Row label="used by">{b.implicit && builtin.length > 0 ? `built-in profile: ${builtin.join(', ')}` : backendUsedBy(b, builtin)}</Row>
                  </div>
                </div>
              )
            })}
          </Section>
        </div>
      </AsyncView>

      {list && editor?.kind === 'profile' && (
        <ProfileEditor
          client={client}
          list={list}
          existing={editor.existing}
          onClose={() => setEditor(null)}
          onSaved={(next) => {
            setEditor(null)
            applied(next, next.saved_note || 'memory profile saved · restart required')
          }}
          reload={reloadForEditor}
        />
      )}
      {list && editor?.kind === 'backend' && (
        <BackendEditor
          client={client}
          list={list}
          existing={editor.existing}
          onClose={() => setEditor(null)}
          onSaved={(next) => {
            setEditor(null)
            applied(next, next.saved_note || 'memory backend saved · restart required')
          }}
          reload={reloadForEditor}
        />
      )}
    </Page>
  )
}
