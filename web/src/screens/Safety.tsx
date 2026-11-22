// Safety: the named filter chains (profiles.safety) and the model-backed
// classifiers they can call (safety.classifiers). Both are core-owned and
// bind when agentflow starts, so every entry says whether it is live or
// waiting for a restart.
import { useRef, useState, type ReactNode } from 'react'
import { Pencil, Plus, RotateCw, Trash2 } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { SafetyClassifier, SafetyList, SafetyLuaFilter, SafetyProfile } from '@/api/types'
import { AsyncView, Banner, Button, Chip, CopyButton, IconButton, Page, Section, type Tone } from '@/components/ui'
import { t } from '@/i18n'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { useClient, useSession } from '@/state/session'
import { confirm, info, toast } from '@/state/ui'
import { ClassifierEditor } from './safety/ClassifierEditor'
import { LuaFilterEditor } from './safety/LuaFilterEditor'
import { ProfileEditor } from './safety/ProfileEditor'
import {
  categorySummary,
  directionLabel,
  failActionLabel,
  formatThreshold,
  modeLabel,
  profileInUse,
  profileUsage,
  profileUsedBy,
  sourceLabel,
} from './safety/helpers'
import { ChainView, LiveChip, useMounted } from './safety/parts'

type Editor =
  | { kind: 'profile'; existing: SafetyProfile | null }
  | { kind: 'classifier'; existing: SafetyClassifier | null }
  | { kind: 'lua_filter'; existing: SafetyLuaFilter | null }

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
          throw new ApiError(404, t('safety.apiMissing'))
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
  const luaFilters = list?.lua_filters ?? []
  const editable = list?.editable ?? false

  const readOnly = () =>
    void info({
      title: t('safety.readOnlyTitle'),
      message: list?.note || t('safety.readOnlyMessage'),
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
            title: t('safety.deleteProfileTitle'),
            message: t('safety.deleteProfileMessage', { name }),
            confirmLabel: t('common.delete'),
            danger: true,
          }
        : kind === 'lua_filter'
          ? {
              title: t('safety.deleteLuaTitle'),
              message: t('safety.deleteLuaMessage', { name }),
              confirmLabel: t('common.delete'),
              danger: true,
            }
          : {
              title: t('safety.deleteClassifierTitle'),
              message: t('safety.deleteClassifierMessage', { name }),
              confirmLabel: t('common.delete'),
              danger: true,
            },
    )
    if (!ok || !mounted.current) return
    const mtime = latest.current?.mtime
    setDeleting(`${kind}:${name}`)
    try {
      const next =
        kind === 'profile'
          ? await client.deleteSafetyProfile(name, mtime)
          : kind === 'lua_filter'
            ? await client.deleteSafetyLuaFilter(name, mtime)
            : await client.deleteSafetyClassifier(name, mtime)
      if (!mounted.current) return
      applied(
        next,
        kind === 'profile'
          ? t('safety.profileDeleted')
          : kind === 'lua_filter'
            ? t('safety.luaDeleted')
            : t('safety.classifierDeleted'),
      )
    } catch (err) {
      // An entry that is still referenced, or a config that changed on disk
      // (409), is refused by the runtime; its words say which.
      if (mounted.current) setActionError(errorMessage(err))
    } finally {
      if (mounted.current) setDeleting('')
    }
  }

  const restart = list?.restart_required ?? false
  const pendingNote = list ? (restart ? list.saved_note || list.note || t('safety.pendingDefault') : list.saved_note) : ''

  return (
    <Page
      title={t('safety.title')}
      subtitle={t('safety.subtitle')}
      actions={<IconButton icon={RotateCw} label={t('common.refresh')} loading={refreshing} onClick={refresh} />}
      meta={
        list && (
          <>
            <LiveChip live={!restart} />
            <span>{t('safety.profileCount', { n: profiles.length })}</span>
            <span>{t('safety.classifierCount', { n: classifiers.length })}</span>
            <span>{t('safety.luaFilterCount', { n: luaFilters.length })}</span>
            <span>{t('safety.source', { mode: list.mode || 'file' })}</span>
            {list.path && (
              <span className="flex min-w-0 items-center gap-1">
                <span className="truncate" title={list.path}>
                  {list.path}
                </span>
                <CopyButton text={list.path} what={t('safety.whatPath')} />
              </span>
            )}
            <span className={editable ? undefined : 'text-warning'}>{editable ? t('safety.editable') : t('safety.readOnly')}</span>
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
              <Banner tone="warning">{list.note || t('safety.readOnlyBanner')}</Banner>
            ) : (
              pendingNote && <Banner tone="warning">{restart ? t('safety.restartRequiredNote', { note: pendingNote }) : pendingNote}</Banner>
            )}
          </>
        ) : undefined
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!list} retry={refresh} loadingText={t('safety.loading')}>
        <div className="flex flex-col gap-3">
          <Section
            flush
            title={t('safety.profiles')}
            subtitle={t('safety.profilesSubtitle')}
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'profile', existing: null })}>
                {t('safety.newProfile')}
              </Button>
            }
          >
            {profiles.length === 0 && <p className="px-3 pb-2 text-sm text-muted">{t('safety.noProfiles')}</p>}
            {profiles.map((p) => {
              const filters = p.filters ?? []
              const refs = p.classifiers ?? []
              const luaRefs = p.lua_filters ?? []
              const inUse = profileInUse(p)
              return (
                <div key={p.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{p.name}</span>
                    <Chip tone="accent">{t('safety.filterCount', { n: filters.length })}</Chip>
                    <LiveChip live={p.live} />
                    <div className="flex-1" />
                    <IconButton icon={Pencil} label={t('safety.editWhat', { name: p.name })} onClick={() => open({ kind: 'profile', existing: p })} />
                    <IconButton
                      icon={Trash2}
                      tone="danger"
                      label={
                        inUse
                          ? t('safety.cannotDeleteProfile', { name: p.name, refs: profileUsage(p) })
                          : t('safety.deleteWhat', { name: p.name })
                      }
                      disabled={inUse || deleting !== ''}
                      loading={deleting === 'profile:' + p.name}
                      onClick={() => void remove('profile', p.name)}
                    />
                  </div>
                  <div className="mt-1.5 flex flex-col gap-1.5">
                    <Row label={t('safety.row.chain')}>
                      <ChainView filters={filters} />
                    </Row>
                    <Row label={t('safety.row.usedBy')}>
                      {profileUsedBy(p)}
                      {inUse && <span className="text-faint"> · {t('safety.deleteOff')}</span>}
                    </Row>
                    {refs.length > 0 && (
                      <Row label={t('safety.row.classifiers')}>
                        <span className="font-mono text-text">{refs.join(', ')}</span>
                      </Row>
                    )}
                    {luaRefs.length > 0 && (
                      <Row label={t('safety.row.luaFilters')}>
                        <span className="font-mono text-text">{luaRefs.join(', ')}</span>
                      </Row>
                    )}
                  </div>
                </div>
              )
            })}
          </Section>

          <Section
            flush
            title={t('safety.classifiers')}
            subtitle={t('safety.classifiersSubtitle')}
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'classifier', existing: null })}>
                {t('safety.newClassifier')}
              </Button>
            }
          >
            {classifiers.length === 0 && <p className="px-3 pb-2 text-sm text-muted">{t('safety.noClassifiers')}</p>}
            {classifiers.map((c) => {
              const mode = c.mode || 'enforce'
              const usedBy = c.used_by ?? []
              const inUse = usedBy.length > 0
              return (
                <div key={c.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{c.name}</span>
                    <Chip tone="accent" title={t('safety.row.model')}>
                      {c.model}
                    </Chip>
                    <Chip tone={mode === 'shadow' ? 'warning' : 'success'}>{modeLabel(mode)}</Chip>
                    <LiveChip live={c.live} />
                    <div className="flex-1" />
                    <IconButton icon={Pencil} label={t('safety.editWhat', { name: c.name })} onClick={() => open({ kind: 'classifier', existing: c })} />
                    <IconButton
                      icon={Trash2}
                      tone="danger"
                      label={
                        inUse
                          ? t('safety.cannotDeleteClassifier', { name: c.name, n: usedBy.length, profiles: usedBy.join(', ') })
                          : t('safety.deleteWhat', { name: c.name })
                      }
                      disabled={inUse || deleting !== ''}
                      loading={deleting === 'classifier:' + c.name}
                      onClick={() => void remove('classifier', c.name)}
                    />
                  </div>
                  <div className="mt-1.5 grid grid-cols-1 gap-x-6 gap-y-1.5 md:grid-cols-2">
                    <Row label={t('safety.row.threshold')}>
                      <span className="font-mono">{typeof c.threshold === 'number' ? formatThreshold(c.threshold) : t('safety.thresholdDefault')}</span>
                    </Row>
                    <Row label={t('safety.row.directions')}>
                      {(c.directions ?? []).length > 0 ? (c.directions ?? []).map(directionLabel).join(', ') : t('safety.directionsDefault')}
                    </Row>
                    <Row label={t('safety.row.sources')}>
                      {(c.sources ?? []).length > 0 ? (c.sources ?? []).map(sourceLabel).join(', ') : t('safety.sourcesDefault')}
                    </Row>
                    <Row label={t('safety.row.usedBy')}>
                      {inUse ? (
                        <>
                          {usedBy.join(', ')}
                          <span className="text-faint"> · {t('safety.deleteOff')}</span>
                        </>
                      ) : (
                        t('safety.unused')
                      )}
                    </Row>
                    <Row label={t('safety.row.categories')} className="md:col-span-2">
                      <Categories categories={c.categories} />
                    </Row>
                  </div>
                </div>
              )
            })}
          </Section>

          <Section
            flush
            title={t('safety.luaFilters')}
            subtitle={t('safety.luaFiltersSubtitle')}
            action={
              <Button size="sm" icon={Plus} disabled={!editable} onClick={() => open({ kind: 'lua_filter', existing: null })}>
                {t('safety.newLuaFilter')}
              </Button>
            }
          >
            {luaFilters.length === 0 && <p className="px-3 pb-2 text-sm text-muted">{t('safety.noLuaFilters')}</p>}
            {luaFilters.map((lf) => {
              const onError = lf.on_error || 'pass'
              const usedBy = lf.used_by ?? []
              const inUse = usedBy.length > 0
              return (
                <div key={lf.name} className="border-t border-line/70 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-mono text-base font-semibold">{lf.name}</span>
                    <Chip tone={onError === 'block' ? 'warning' : 'neutral'}>{failActionLabel(onError)}</Chip>
                    {lf.file && <Chip>{lf.file}</Chip>}
                    <LiveChip live={lf.live} />
                    <div className="flex-1" />
                    <IconButton icon={Pencil} label={t('safety.editWhat', { name: lf.name })} onClick={() => open({ kind: 'lua_filter', existing: lf })} />
                    <IconButton
                      icon={Trash2}
                      tone="danger"
                      label={
                        inUse
                          ? t('safety.cannotDeleteLuaFilter', { name: lf.name, n: usedBy.length, profiles: usedBy.join(', ') })
                          : t('safety.deleteWhat', { name: lf.name })
                      }
                      disabled={inUse || deleting !== ''}
                      loading={deleting === 'lua_filter:' + lf.name}
                      onClick={() => void remove('lua_filter', lf.name)}
                    />
                  </div>
                  <div className="mt-1.5 grid grid-cols-1 gap-x-6 gap-y-1.5 md:grid-cols-2">
                    <Row label={t('safety.row.directions')}>
                      {(lf.directions ?? []).length > 0 ? (lf.directions ?? []).map(directionLabel).join(', ') : t('safety.directionsDefault')}
                    </Row>
                    <Row label={t('safety.row.sources')}>
                      {(lf.sources ?? []).length > 0 ? (lf.sources ?? []).map(sourceLabel).join(', ') : t('safety.sourcesAll')}
                    </Row>
                    <Row label={t('safety.row.timeout')}>
                      <span className="font-mono">{lf.timeout || '200ms'}</span>
                    </Row>
                    <Row label={t('safety.row.usedBy')}>
                      {inUse ? (
                        <>
                          {usedBy.join(', ')}
                          <span className="text-faint"> · {t('safety.deleteOff')}</span>
                        </>
                      ) : (
                        t('safety.unused')
                      )}
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
            applied(next, next.saved_note || t('safety.profileSaved'))
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
            applied(next, next.saved_note || t('safety.classifierSaved'))
          }}
          reload={reloadForEditor}
        />
      )}
      {list && editor?.kind === 'lua_filter' && (
        <LuaFilterEditor
          client={client}
          list={list}
          existing={editor.existing}
          onClose={() => setEditor(null)}
          onSaved={(next) => {
            setEditor(null)
            applied(next, next.saved_note || t('safety.luaSaved'))
          }}
          reload={reloadForEditor}
        />
      )}
    </Page>
  )
}
