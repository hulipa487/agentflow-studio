// Memory: the backends memory lives in (memory.backends) and the named
// profiles agents pick (profiles.memory). Agents open their stores when
// agentflow starts, so every entry says whether it is live or waiting for a
// restart.
import { useRef, useState, type ReactNode } from 'react'
import { BookOpen, Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { docPath } from './docs/helpers'
import { ApiError, errorMessage } from '@/api/client'
import type { MemoryBackend, MemoryList, MemoryProfile } from '@/api/types'
import { AsyncView, Banner, Button, Chip, CopyButton, IconButton, Page, Section } from '@/components/ui'
import { t } from '@/i18n'
import { useAsync, useScreenActions } from '@/lib/hooks'
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
  const navigate = useNavigate()
  const mounted = useMounted()
  const { data: list, error, loading, refreshing, reload, setData } = useAsync<MemoryList>(
    async (signal) => {
      try {
        return await client.memory(signal)
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          throw new ApiError(404, t('memory.apiMissing'))
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
      title: t('memory.readOnlyTitle'),
      message: list?.note || t('memory.readOnlyMessage'),
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
      title: kind === 'profile' ? t('memory.deleteProfileTitle') : t('memory.deleteBackendTitle'),
      message: kind === 'profile' ? t('memory.deleteProfileMessage', { name }) : t('memory.deleteBackendMessage', { name }),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok || !mounted.current) return
    const mtime = latest.current?.mtime
    setDeleting(`${kind}:${name}`)
    try {
      const next = kind === 'profile' ? await client.deleteMemoryProfile(name, mtime) : await client.deleteMemoryBackend(name, mtime)
      if (!mounted.current) return
      applied(next, kind === 'profile' ? t('memory.profileDeleted') : t('memory.backendDeleted'))
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
      title={t('memory.title')}
      subtitle={t('memory.subtitle')}
      actions={
        <>
          <Button icon={BookOpen} onClick={() => navigate(docPath('memory'))}>
            {t('memory.guide')}
          </Button>
          <IconButton icon={RotateCw} label={t('common.refresh')} loading={refreshing} onClick={refresh} />
        </>
      }
      meta={
        list && (
          <>
            <LiveChip live={!restart} />
            <span>{t('memory.backendCount', { n: backends.length })}</span>
            <span>{t('memory.profileCount', { n: profiles.length })}</span>
            <span>{t('memory.source', { mode: list.mode || 'file' })}</span>
            {list.path && (
              <span className="flex min-w-0 items-center gap-1">
                <span className="truncate" title={list.path}>
                  {list.path}
                </span>
                <CopyButton text={list.path} what={t('memory.whatPath')} />
              </span>
            )}
            <span className={editable ? undefined : 'text-warning'}>{editable ? t('memory.editable') : t('memory.readOnly')}</span>
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
              <Banner tone="warning">{list.note || t('memory.readOnlyBanner')}</Banner>
            ) : (
              pendingNote && <Banner tone="warning">{restart ? t('safety.restartRequiredNote', { note: pendingNote }) : pendingNote}</Banner>
            )}
          </>
        ) : undefined
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!list} retry={refresh} loadingText={t('memory.loading')}>
        <div className="flex flex-col gap-3">
          <Section
            flush
            title={t('memory.profiles')}
            subtitle={builtin.length > 0 ? t('memory.profilesSubtitleBuiltin', { agents: builtin.join(', ') }) : t('memory.profilesSubtitle')}
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'profile', existing: null })}>
                {t('memory.newProfile')}
              </Button>
            }
          >
            {profiles.length === 0 && <p className="px-3 pb-2 text-sm text-muted">{t('memory.noProfiles')}</p>}
            {profiles.map((p) => {
              const inUse = profileInUse(p)
              return (
                <div key={p.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{p.name}</span>
                    <Chip tone="accent">{t('memory.storeCount', { n: (p.stores ?? []).length })}</Chip>
                    {p.recall && <Chip>{p.recall}</Chip>}
                    <LiveChip live={p.live} />
                    <div className="flex-1" />
                    <IconButton icon={Pencil} label={t('memory.editWhat', { name: p.name })} onClick={() => open({ kind: 'profile', existing: p })} />
                    <IconButton
                      icon={Trash2}
                      tone="danger"
                      label={inUse ? t('memory.cannotDelete', { name: p.name, usedBy: profileUsedBy(p) }) : t('memory.deleteWhat', { name: p.name })}
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
                      <Row label={t('memory.row.embedding')}>
                        <span className="font-mono">
                          {p.rerank_model ? t('memory.embeddingRerank', { embed: p.embed_model, rerank: p.rerank_model }) : p.embed_model}
                        </span>
                      </Row>
                    )}
                    <Row label={t('memory.row.usedBy')}>{profileUsedBy(p)}</Row>
                  </div>
                </div>
              )
            })}
          </Section>

          <Section
            flush
            title={t('memory.backends')}
            subtitle={t('memory.backendsSubtitle')}
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'backend', existing: null })}>
                {t('memory.newBackend')}
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
                    {b.implicit ? <Chip title={t('memory.builtinChipTitle')}>{t('memory.builtinChip')}</Chip> : <LiveChip live={b.live} />}
                    <div className="flex-1" />
                    <IconButton
                      icon={Pencil}
                      label={b.implicit ? t('memory.writeOutMainDb') : t('memory.editWhat', { name: b.name })}
                      onClick={() => open({ kind: 'backend', existing: b })}
                    />
                    {!b.implicit && (
                      <IconButton
                        icon={Trash2}
                        tone="danger"
                        label={
                          inUse
                            ? t('memory.cannotDelete', { name: b.name, usedBy: backendUsedBy(b, builtin) })
                            : t('memory.deleteWhat', { name: b.name })
                        }
                        disabled={inUse || deleting !== ''}
                        loading={deleting === 'backend:' + b.name}
                        onClick={() => void remove('backend', b.name)}
                      />
                    )}
                  </div>
                  <div className="mt-1.5 flex flex-col gap-1">
                    <Row label={t('memory.row.target')}>
                      <span className="font-mono">{backendTarget(b)}</span>
                    </Row>
                    {b.dim ? (
                      <Row label={t('memory.row.dimensions')}>
                        <span className="font-mono">{b.dim}</span>
                      </Row>
                    ) : null}
                    <Row label={t('memory.row.usedBy')}>
                      {b.implicit && builtin.length > 0 ? t('memory.builtinProfileUsedBy', { agents: builtin.join(', ') }) : backendUsedBy(b, builtin)}
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
            applied(next, next.saved_note || t('memory.profileSaved'))
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
            applied(next, next.saved_note || t('memory.backendSaved'))
          }}
          reload={reloadForEditor}
        />
      )}
    </Page>
  )
}
