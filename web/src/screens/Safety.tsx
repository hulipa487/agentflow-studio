// Safety: the named filter chains (profiles.safety) and the model-backed
// classifiers they can call (safety.classifiers). Both are core-owned and
// bind when agentflow starts, so every entry says whether it is live or
// waiting for a restart.
import { useRef, useState, type ReactNode } from 'react'
import { Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { SafetyClassifier, SafetyList, SafetyProfile } from '@/api/types'
import { AsyncView, Banner, Button, Chip, CopyButton, IconButton, Page, Section, type Tone } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { plural } from '@/lib/format'
import { useClient, useSession } from '@/state/session'
import { confirm, info, toast } from '@/state/ui'
import { ClassifierEditor } from './safety/ClassifierEditor'
import { ProfileEditor } from './safety/ProfileEditor'
import { categorySummary, formatThreshold, profileInUse, profileUsage, profileUsedBy } from './safety/helpers'
import { ChainView, LiveChip, useMounted } from './safety/parts'

type Editor = { kind: 'profile'; existing: SafetyProfile | null } | { kind: 'classifier'; existing: SafetyClassifier | null }

const ACTION_TONE: Record<string, Tone> = { allow: 'success', review: 'warning', block: 'danger' }

function Row({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <div className={`flex min-w-0 items-baseline gap-3 ${className ?? ''}`}>
      <span className="w-[76px] shrink-0 text-xs font-semibold text-faint">{label}</span>
      <div className="min-w-0 flex-1 text-xs text-muted">{children}</div>
    </div>
  )
}

function Categories({ categories }: { categories: Record<string, string> | null | undefined }) {
  const labels = Object.keys(categories ?? {}).sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
  if (!categories || labels.length === 0) return <>—</>
  return (
    <span className="flex flex-wrap gap-1" title={categorySummary(categories)}>
      {labels.map((label) => (
        <Chip key={label} tone={ACTION_TONE[categories[label]] ?? 'neutral'}>
          {label}={categories[label]}
        </Chip>
      ))}
    </span>
  )
}

export default function Safety() {
  const client = useClient()
  const mounted = useMounted()
  const {
    data: list,
    error,
    loading,
    refreshing,
    reload,
    setData,
  } = useAsync<SafetyList>(
    async (signal) => {
      try {
        return await client.safety(signal)
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          throw new ApiError(404, 'This runtime does not expose the Studio safety API yet; rebuild and restart AgentFlow, then reopen Safety.')
        }
        throw err
      }
    },
    [client],
  )
  const [editor, setEditor] = useState<Editor | null>(null)
  // The runtime's refusal of a delete; saves report inside their dialog.
  const [actionError, setActionError] = useState('')
  const [deleting, setDeleting] = useState('')
  // A delete waits on a confirmation; it must send the mtime of the list as
  // it is when the request leaves, not as it was when the dialog opened.
  const latest = useRef(list)
  latest.current = list

  const refresh = () => {
    setActionError('')
    reload()
  }
  useScreenActions({ refresh })

  const profiles = list?.profiles ?? []
  const classifiers = list?.classifiers ?? []
  const editable = list?.editable ?? false

  const readOnly = () =>
    void info({
      title: 'Safety is read-only',
      message: list?.note || 'This deployment has no editable config source, so safety profiles cannot be changed from Studio.',
    })

  const open = (next: Editor) => {
    if (!editable) return readOnly()
    setEditor(next)
  }

  // Applies the list a write returned. The catalog carries the profile names
  // the agent builder offers, so it is refreshed too.
  const applied = (next: SafetyList, status: string) => {
    setActionError('')
    setData(next)
    void useSession.getState().reloadCatalog()
    toast(status, 'warning')
  }

  const reloadForEditor = async () => {
    const next = await client.safety()
    if (mounted.current) setData(next)
  }

  const remove = async (kind: Editor['kind'], name: string) => {
    if (!editable) return readOnly()
    const ok = await confirm(
      kind === 'profile'
        ? {
            title: 'Delete safety profile',
            message: `Delete safety profile "${name}" from the deployment config?\n\nThe running instance keeps its current chain until the next restart.`,
            confirmLabel: 'Delete',
            danger: true,
          }
        : {
            title: 'Delete classifier',
            message: `Delete classifier "${name}" from the deployment config?\n\nThe running instance keeps using it until the next restart.`,
            confirmLabel: 'Delete',
            danger: true,
          },
    )
    if (!ok || !mounted.current) return
    const mtime = latest.current?.mtime
    setDeleting(`${kind}:${name}`)
    try {
      const next = kind === 'profile' ? await client.deleteSafetyProfile(name, mtime) : await client.deleteSafetyClassifier(name, mtime)
      if (!mounted.current) return
      applied(next, kind === 'profile' ? 'safety profile deleted · restart required' : 'safety classifier deleted · restart required')
    } catch (err) {
      // An entry that is still referenced, or a config that changed on disk
      // (409), is refused by the runtime; its words say which.
      if (mounted.current) setActionError(errorMessage(err))
    } finally {
      if (mounted.current) setDeleting('')
    }
  }

  const restart = list?.restart_required ?? false
  const pendingNote = list ? (restart ? list.saved_note || list.note || 'Saved changes take effect after a restart.' : list.saved_note) : ''

  return (
    <Page
      title="Safety"
      subtitle="Core-owned ingress and egress filter chains, plus the model-backed classifiers they can call. Saves go to the deployment config and bind on the next runtime start."
      actions={<IconButton icon={RotateCw} label="Refresh" loading={refreshing} onClick={refresh} />}
      meta={
        list && (
          <>
            <LiveChip live={!restart} />
            <span>{plural(profiles.length, 'profile')}</span>
            <span>{plural(classifiers.length, 'classifier')}</span>
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
              <Banner tone="warning">{list.note || 'This deployment has no editable config source for safety profiles.'}</Banner>
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
      <AsyncView loading={loading} error={error} hasData={!!list} retry={refresh} loadingText="Loading safety…">
        <div className="flex flex-col gap-3">
          <Section
            flush
            title="Filter profiles"
            subtitle="The chain runs in order. Builtins are compiled in; classifier:<name> entries call a model-backed policy."
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'profile', existing: null })}>
                New profile
              </Button>
            }
          >
            {profiles.length === 0 && (
              <p className="px-3 pb-2 text-sm text-muted">No named safety profiles. Agents can still use the built-in "default" chain or "none".</p>
            )}
            {profiles.map((p) => {
              const filters = p.filters ?? []
              const refs = p.classifiers ?? []
              const inUse = profileInUse(p)
              return (
                <div key={p.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{p.name}</span>
                    <Chip tone="accent">{plural(filters.length, 'filter')}</Chip>
                    <LiveChip live={p.live} />
                    <div className="flex-1" />
                    <IconButton icon={Pencil} label={`Edit ${p.name}`} onClick={() => open({ kind: 'profile', existing: p })} />
                    <IconButton
                      icon={Trash2}
                      tone="danger"
                      label={
                        inUse
                          ? `Cannot delete ${p.name}: it is still referenced by ${profileUsage(p)}; remove those references first.`
                          : `Delete ${p.name}`
                      }
                      disabled={inUse || deleting !== ''}
                      loading={deleting === 'profile:' + p.name}
                      onClick={() => void remove('profile', p.name)}
                    />
                  </div>
                  <div className="mt-1.5 flex flex-col gap-1.5">
                    <Row label="chain">
                      <ChainView filters={filters} />
                    </Row>
                    <Row label="used by">
                      {profileUsedBy(p)}
                      {inUse && <span className="text-faint"> · delete is off until these references are removed</span>}
                    </Row>
                    {refs.length > 0 && (
                      <Row label="classifiers">
                        <span className="font-mono text-text">{refs.join(', ')}</span>
                      </Row>
                    )}
                  </div>
                </div>
              )
            })}
          </Section>

          <Section
            flush
            title="Model-backed classifiers"
            subtitle="A dedicated engine-keyed model returns a label and score; the policy here maps labels to allow, review, or block."
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'classifier', existing: null })}>
                New classifier
              </Button>
            }
          >
            {classifiers.length === 0 && (
              <p className="px-3 pb-2 text-sm text-muted">
                No model-backed classifiers yet. Add one to enable classifier:&lt;name&gt; filters in a profile.
              </p>
            )}
            {classifiers.map((c) => {
              const mode = c.mode || 'enforce'
              const usedBy = c.used_by ?? []
              const inUse = usedBy.length > 0
              return (
                <div key={c.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{c.name}</span>
                    <Chip tone="accent" title="model">
                      {c.model}
                    </Chip>
                    <Chip tone={mode === 'shadow' ? 'warning' : 'success'}>{mode}</Chip>
                    <LiveChip live={c.live} />
                    <div className="flex-1" />
                    <IconButton icon={Pencil} label={`Edit ${c.name}`} onClick={() => open({ kind: 'classifier', existing: c })} />
                    <IconButton
                      icon={Trash2}
                      tone="danger"
                      label={
                        inUse
                          ? `Cannot delete ${c.name}: it is referenced by profile(s): ${usedBy.join(', ')}. Remove those references first.`
                          : `Delete ${c.name}`
                      }
                      disabled={inUse || deleting !== ''}
                      loading={deleting === 'classifier:' + c.name}
                      onClick={() => void remove('classifier', c.name)}
                    />
                  </div>
                  <div className="mt-1.5 grid grid-cols-1 gap-x-6 gap-y-1.5 md:grid-cols-2">
                    <Row label="threshold">
                      <span className="font-mono">{typeof c.threshold === 'number' ? formatThreshold(c.threshold) : '0.7 (default)'}</span>
                    </Row>
                    <Row label="directions">{(c.directions ?? []).length > 0 ? (c.directions ?? []).join(', ') : 'ingress + egress'}</Row>
                    <Row label="sources">{(c.sources ?? []).length > 0 ? (c.sources ?? []).join(', ') : 'channel + agent'}</Row>
                    <Row label="used by">
                      {inUse ? (
                        <>
                          {usedBy.join(', ')}
                          <span className="text-faint"> · delete is off until these references are removed</span>
                        </>
                      ) : (
                        'unused'
                      )}
                    </Row>
                    <Row label="categories" className="md:col-span-2">
                      <Categories categories={c.categories} />
                    </Row>
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
            applied(next, next.saved_note || 'safety profile saved · restart required')
          }}
          reload={reloadForEditor}
        />
      )}
      {list && editor?.kind === 'classifier' && (
        <ClassifierEditor
          client={client}
          list={list}
          existing={editor.existing}
          onClose={() => setEditor(null)}
          onSaved={(next) => {
            setEditor(null)
            applied(next, next.saved_note || 'safety classifier saved · restart required')
          }}
          reload={reloadForEditor}
        />
      )}
    </Page>
  )
}
