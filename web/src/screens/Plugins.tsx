// Shared & built-in Lua plugins workbench: shadow built-in support/route
// chunks in plugins.dir and manage shared packages (`packages:`) with live
// Lua 5.1 analysis, boot-compatible compile checks, and instant hot-reload.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { EditorView } from '@codemirror/view'
import { startCompletion } from '@codemirror/autocomplete'
import {
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  FileCode,
  FilePlus,
  FolderCode,
  List,
  Plus,
  RotateCcw,
  RotateCw,
  Save,
  Trash2,
  Undo2,
} from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { Mtime, PluginsList } from '@/api/types'
import {
  AsyncView,
  Banner,
  Button,
  Chip,
  CopyButton,
  Dot,
  IconButton,
  Page,
  cx,
  toneText,
  type Tone,
} from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { Split } from '@/components/ui/Split'
import { copyText } from '@/lib/format'
import { useAlive, useAsync, useDebounced, useScreenActions, useUnsavedGuard } from '@/lib/hooks'
import { t, useLang } from '@/i18n'
import { analyze, luaSupport, type LuaDiagnostic } from '@/lua'
import { SKIP_GUARD } from '@/lib/hooks'
import { useCatalog, useClient, useSession } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { DocsPane } from './builder/DocsPane'
import { EditorFrame } from './builder/EditorFrame'
import { loopStatus } from './builder/model'
import { NewFileDialog, NewPackageDialog } from './plugins/Dialogs'
import {
  defaultSelection,
  findSelectedBuiltin,
  findSelectedPackageFile,
  insertPreludeDirective,
  packageStarterSource,
  sameSelection,
  selectionExists,
  selectionKey,
  type Selection,
} from './plugins/helpers'

interface Notice {
  tone: Tone
  text: string
  conflict?: boolean
}

const SEVERITY_TONE = { error: 'danger', warning: 'warning', info: 'accent', hint: 'neutral' } as const
const SEVERITY_ORDER = { error: 0, warning: 1, info: 2, hint: 3 } as const

export default function Plugins() {
  const client = useClient()
  const navigate = useNavigate()
  const catalog = useCatalog()
  useLang()
  const alive = useAlive()

  const { data: list, error, loading, refreshing, reload, setData } = useAsync<PluginsList>(
    async (signal) => {
      try {
        return await client.plugins(signal)
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          throw new ApiError(404, t('plugins.apiMissing'))
        }
        throw err
      }
    },
    [client],
  )

  const [selected, setSelected] = useState<Selection | null>(null)
  const [loadedKey, setLoadedKey] = useState('')
  const [source, setSource] = useState('')
  const [baseline, setBaseline] = useState('')
  const [mtime, setMtime] = useState<Mtime | undefined>(undefined)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [busy, setBusy] = useState<'' | 'validate' | 'save' | 'delete'>('')
  const [issuesOpen, setIssuesOpen] = useState(true)
  const [docsOpen, setDocsOpen] = useState(false)
  const [newPkgOpen, setNewPkgOpen] = useState(false)
  const [newFilePkg, setNewFilePkg] = useState<string | null>(null)
  const viewRef = useRef<EditorView | null>(null)

  const selectedBuiltin = useMemo(() => findSelectedBuiltin(list, selected), [list, selected])
  const selectedPkgFile = useMemo(() => findSelectedPackageFile(list, selected), [list, selected])

  // Select the first available item on initial load or when the selected item disappears.
  useEffect(() => {
    if (!list) return
    if (!selectionExists(list, selected)) {
      const next = defaultSelection(list)
      setSelected(next)
    }
  }, [list, selected])

  const dirty = loadedKey !== '' && source !== baseline
  useUnsavedGuard(dirty)

  // Load editor contents when selection changes or after an explicit server refresh.
  useEffect(() => {
    if (!list || !selected) return
    const key = selectionKey(selected)
    if (key === loadedKey && dirty) return
    if (selected.kind === 'builtin') {
      const b = findSelectedBuiltin(list, selected)
      if (!b) return
      setLoadedKey(key)
      setSource(b.source)
      setBaseline(b.source)
      setMtime(b.mtime)
    } else {
      const pf = findSelectedPackageFile(list, selected)
      if (!pf) return
      setLoadedKey(key)
      setSource(pf.file.source)
      setBaseline(pf.file.source)
      setMtime(pf.file.mtime)
    }
  }, [list, selected, loadedKey, dirty])

  const editable = list?.editable ?? false
  const preludeVersion = list?.prelude_version || catalog?.prelude_version || 1
  const requireLoop = Boolean(selectedBuiltin && selectedBuiltin.kind === 'route')

  const debouncedSource = useDebounced(source, 220)
  const extensions = useMemo(
    () => [luaSupport({ preludeVersion, capabilities: null, requireLoop })],
    [preludeVersion, requireLoop],
  )
  const diagnostics = useMemo<LuaDiagnostic[]>(() => {
    if (!loadedKey) return []
    return analyze(debouncedSource, { preludeVersion, capabilities: null, requireLoop })
  }, [loadedKey, debouncedSource, preludeVersion, requireLoop])

  const checking = source !== debouncedSource
  const status = loopStatus(diagnostics, checking, '')
  const sortedIssues = useMemo(
    () => [...diagnostics].sort((a, b) => SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity] || a.from - b.from),
    [diagnostics],
  )

  const leave = async (go: () => void, message = t('plugins.discardMessage')) => {
    if (dirty && !(await confirm({ title: t('plugins.discardTitle'), message, confirmLabel: t('common.discard'), danger: true }))) {
      return
    }
    if (alive.current) go()
  }

  const pick = (next: Selection) => {
    if (sameSelection(selected, next)) return
    void leave(() => {
      setNotice(null)
      setLoadedKey('')
      setSelected(next)
    })
  }

  const applyListUpdate = (next: PluginsList, nextSel?: Selection | null) => {
    setData(next)
    const target = nextSel ?? selected
    if (target) {
      setSelected(target)
      setLoadedKey('')
    }
  }

  const validateRemote = async () => {
    if (!selected || busy) return
    setBusy('validate')
    setNotice(null)
    try {
      const errs =
        selected.kind === 'builtin'
          ? await client.validatePlugin({ kind: 'builtin', name: selected.name, source })
          : await client.validatePlugin({ kind: 'package', name: selected.pkg, file: selected.file, source })
      if (!alive.current) return
      if (errs.length > 0) {
        setNotice({ tone: 'danger', text: errs.join('\n') })
      } else {
        setNotice({ tone: 'success', text: t('plugins.valid') })
        toast(t('plugins.validationPassed'), 'success')
      }
    } catch (err) {
      if (alive.current) setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy('')
    }
  }

  const save = async () => {
    if (!selected || !editable || busy) return
    setBusy('save')
    setNotice(null)
    try {
      const next =
        selected.kind === 'builtin'
          ? await client.saveBuiltinPlugin(selected.name, source, mtime)
          : await client.savePackageFile(selected.pkg, selected.file, source, mtime)
      if (!alive.current) return
      setBaseline(source)
      applyListUpdate(next, selected)
      toast(
        selected.kind === 'builtin'
          ? t('plugins.savedBuiltin', { name: selected.name })
          : t('plugins.savedFile', { file: selected.file }),
        'success',
      )
    } catch (err) {
      if (!alive.current) return
      if (err instanceof ApiError && err.status === 409) {
        setNotice({ tone: 'warning', text: t('plugins.conflict'), conflict: true })
      } else {
        setNotice({ tone: 'danger', text: errorMessage(err) })
      }
    } finally {
      if (alive.current) setBusy('')
    }
  }

  const revert = async () => {
    if (!dirty) return
    const ok = await confirm({
      title: t('plugins.discardTitle'),
      message: t('plugins.revertMessage'),
      confirmLabel: t('common.revert'),
      danger: true,
    })
    if (!ok || !alive.current) return
    setSource(baseline)
    setNotice(null)
  }

  const reloadFromServer = async (ask = true) => {
    if (ask && dirty) {
      const ok = await confirm({
        title: t('plugins.discardTitle'),
        message: t('plugins.discardMessage'),
        confirmLabel: t('common.discard'),
        danger: true,
      })
      if (!ok || !alive.current) return
    }
    setNotice(null)
    setLoadedKey('')
    reload(list !== undefined)
  }

  const resetBuiltin = async () => {
    if (!selectedBuiltin || !selectedBuiltin.shadowed || !editable || busy) return
    const ok = await confirm({
      title: t('plugins.resetTitle'),
      message: t('plugins.resetMessage', { name: selectedBuiltin.name }),
      confirmLabel: t('plugins.resetBuiltin'),
      danger: true,
    })
    if (!ok || !alive.current) return
    setBusy('delete')
    setNotice(null)
    try {
      const next = await client.resetBuiltinPlugin(selectedBuiltin.name, mtime)
      if (!alive.current) return
      setLoadedKey('')
      applyListUpdate(next, { kind: 'builtin', name: selectedBuiltin.name })
      toast(t('plugins.resetDone', { name: selectedBuiltin.name }), 'success')
    } catch (err) {
      if (!alive.current) return
      if (err instanceof ApiError && err.status === 409) {
        setNotice({ tone: 'warning', text: t('plugins.conflict'), conflict: true })
      } else {
        setNotice({ tone: 'danger', text: errorMessage(err) })
      }
    } finally {
      if (alive.current) setBusy('')
    }
  }

  const handleCreatePackage = async (name: string, file: string) => {
    const next = await client.createLuaPackage(name, file, packageStarterSource(name, preludeVersion))
    if (!alive.current) return
    setNotice(null)
    applyListUpdate(next, { kind: 'package', pkg: name, file })
    void useSession.getState().reloadCatalog()
    toast(t('plugins.packageCreated', { name }), 'success')
  }

  const handleCreateFile = async (pkg: string, file: string) => {
    const starter = packageStarterSource(`${pkg}/${file}`, preludeVersion)
    const next = await client.savePackageFile(pkg, file, starter, '0' as Mtime)
    if (!alive.current) return
    setNotice(null)
    applyListUpdate(next, { kind: 'package', pkg, file })
    toast(t('plugins.savedFile', { file }), 'success')
  }

  const handleDeletePackage = async (pkgName: string, managed: boolean) => {
    if (!editable || busy) return
    const ok = await confirm({
      title: t('plugins.deletePackageTitle'),
      message: managed ? t('plugins.deletePackageManaged', { name: pkgName }) : t('plugins.deletePackageLinked', { name: pkgName }),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok || !alive.current) return
    setBusy('delete')
    setNotice(null)
    try {
      const next = await client.deleteLuaPackage(pkgName)
      if (!alive.current) return
      const nextSel = selected?.kind === 'package' && selected.pkg === pkgName ? defaultSelection(next) : selected
      setLoadedKey('')
      applyListUpdate(next, nextSel)
      void useSession.getState().reloadCatalog()
      toast(t('plugins.packageDeleted', { name: pkgName }), 'success')
    } catch (err) {
      if (alive.current) setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy('')
    }
  }

  const handleDeleteFile = async () => {
    if (!selectedPkgFile || !editable || busy) return
    const { pkg, file } = selectedPkgFile
    if ((pkg.files?.length ?? 0) <= 1) {
      setNotice({ tone: 'warning', text: t('plugins.onlyFileHint') })
      return
    }
    const ok = await confirm({
      title: t('plugins.deleteFileTitle'),
      message: t('plugins.deleteFileMessage', { pkg: pkg.name, file: file.name }),
      confirmLabel: t('common.delete'),
      danger: true,
    })
    if (!ok || !alive.current) return
    setBusy('delete')
    setNotice(null)
    try {
      const next = await client.deletePackageFile(pkg.name, file.name, mtime)
      if (!alive.current) return
      const updatedPkg = (next.packages ?? []).find((p) => p.name === pkg.name)
      const fallbackFile = updatedPkg?.files?.[0]?.name
      const nextSel: Selection | null = fallbackFile ? { kind: 'package', pkg: pkg.name, file: fallbackFile } : defaultSelection(next)
      setLoadedKey('')
      applyListUpdate(next, nextSel)
      toast(t('plugins.fileDeleted', { file: file.name }), 'success')
    } catch (err) {
      if (alive.current) setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy('')
    }
  }

  const jumpToIssue = (d: LuaDiagnostic) => {
    const v = viewRef.current
    if (!v) return
    const len = v.state.doc.length
    const from = Math.min(Math.max(d.from, 0), len)
    const to = Math.min(Math.max(d.to, from), len)
    v.dispatch({ selection: { anchor: from, head: to }, effects: EditorView.scrollIntoView(from, { y: 'center' }) })
    v.focus()
  }

  useScreenActions({
    save: editable ? () => void save() : undefined,
    refresh: () => void reloadFromServer(true),
    help: () => setDocsOpen((o) => !o),
  })

  const activeTitle = selectedBuiltin
    ? `plugin:${selectedBuiltin.name}`
    : selectedPkgFile
      ? `${selectedPkgFile.pkg.name} / ${selectedPkgFile.file.name}`
      : ''

  const activePath = selectedBuiltin
    ? (selectedBuiltin.path || `${list?.plugin_dir || './plugins'}/${selectedBuiltin.name}.lua`)
    : (selectedPkgFile?.file.path ?? '')

  const newFileTargetPkg = useMemo(() => {
    if (!newFilePkg || !list) return null
    return (list.packages ?? []).find((p) => p.name === newFilePkg) ?? null
  }, [list, newFilePkg])

  const editorCard = (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      {notice && (
        <Banner tone={notice.tone}>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="whitespace-pre-wrap">{notice.text}</span>
            {notice.conflict && (
              <Button size="sm" icon={RotateCw} onClick={() => void reloadFromServer(false)}>
                {t('plugins.reloadNow')}
              </Button>
            )}
          </div>
        </Banner>
      )}
      {!selected || (!selectedBuiltin && !selectedPkgFile) ? (
        <div className="flex flex-1 items-center justify-center rounded-card border border-line bg-surface p-6 text-sm text-muted">
          {t('plugins.selectPrompt')}
        </div>
      ) : (
        <EditorFrame
          title={activeTitle}
          hint={
            <span className="flex flex-wrap items-center gap-2">
              {selectedBuiltin ? (
                <>
                  <Chip tone={selectedBuiltin.shadowed ? 'warning' : 'accent'}>
                    {selectedBuiltin.shadowed ? t('plugins.overridden') : t('plugins.builtin')}
                  </Chip>
                  <span>{selectedBuiltin.summary || t('plugins.editorHint')}</span>
                </>
              ) : (
                <span>{t('plugins.editorHint')}</span>
              )}
              {activePath && <span className="truncate font-mono text-[11px] text-faint">{activePath}</span>}
            </span>
          }
          actions={
            <>
              <span className="mr-1 flex min-w-0 items-center gap-1.5" role="status">
                <Dot tone={status.tone} />
                <span className={cx('truncate text-xs', status.tone === 'neutral' ? 'text-muted' : toneText(status.tone))}>
                  {status.text}
                </span>
              </span>
              <CopyButton text={source} what={t('plugins.whatSource')} size="md" />
              <IconButton
                icon={List}
                label={t('builder.preludeApi')}
                onClick={() => {
                  const v = viewRef.current
                  if (!v) return
                  v.focus()
                  startCompletion(v)
                }}
              />
              {selectedBuiltin?.shadowed && editable && (
                <Button size="sm" icon={RotateCcw} loading={busy === 'delete'} disabled={busy !== ''} onClick={() => void resetBuiltin()}>
                  {t('plugins.resetBuiltin')}
                </Button>
              )}
              {selectedPkgFile && editable && (
                <IconButton
                  icon={Trash2}
                  tone="danger"
                  label={t('plugins.deleteFile')}
                  disabled={busy !== '' || (selectedPkgFile.pkg.files?.length ?? 0) <= 1}
                  onClick={() => void handleDeleteFile()}
                />
              )}
            </>
          }
          footer={
            sortedIssues.length > 0 ? (
              <div className="shrink-0 border-t border-line">
                <button
                  type="button"
                  onClick={() => setIssuesOpen((o) => !o)}
                  aria-expanded={issuesOpen}
                  className="flex h-7 w-full items-center gap-1.5 px-3 text-left text-xs font-semibold text-faint hover:text-text"
                >
                  {issuesOpen ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
                  {t('builder.issues')}
                  <span className="font-mono normal-case tracking-normal">{sortedIssues.length}</span>
                </button>
                {issuesOpen && (
                  <ul className="max-h-36 overflow-y-auto pb-1.5">
                    {sortedIssues.map((d, i) => (
                      <li key={`${d.from}:${d.to}:${d.code ?? ''}:${i}`}>
                        <div className="flex w-full items-center gap-2 px-3 py-1 hover:bg-hover">
                          <button
                            type="button"
                            onClick={() => jumpToIssue(d)}
                            title={t('builder.jumpToLine')}
                            className="flex min-w-0 flex-1 items-baseline gap-2 text-left text-xs focus-visible:outline-none"
                          >
                            <Dot tone={SEVERITY_TONE[d.severity]} className="translate-y-[-1px] self-center" />
                            <span className="w-12 shrink-0 font-mono text-xs text-faint">L{d.line}</span>
                            <span className="min-w-0 flex-1 break-words text-text">{d.message}</span>
                            {d.code && <Chip>{d.code}</Chip>}
                          </button>
                          {d.code === 'prelude.missing-directive' && (
                            <Button
                              size="sm"
                              icon={Plus}
                              onClick={() => setSource((prev) => insertPreludeDirective(prev, preludeVersion))}
                            >
                              {t('builder.insertDirective')}
                            </Button>
                          )}
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            ) : undefined
          }
        >
          <CodeEditor
            key={loadedKey}
            language="lua"
            value={source}
            readOnly={!editable}
            onChange={setSource}
            extensions={extensions}
            autoFocus
            onReady={(v) => {
              viewRef.current = v
            }}
            className="flex-1"
          />
        </EditorFrame>
      )}
    </div>
  )

  return (
    <Page
      title={
        <span className="flex items-center gap-2">
          <span>{t('plugins.title')}</span>
          {dirty && <Chip tone="warning">{t('plugins.statusUnsaved')}</Chip>}
        </span>
      }
      subtitle={t('plugins.subtitle')}
      scroll={false}
      banner={list && !editable ? <Banner tone="warning">{t('plugins.readOnlyBanner')}</Banner> : undefined}
      actions={
        <>
          {dirty && (
            <Button icon={Undo2} disabled={busy !== ''} onClick={() => void revert()}>
              {t('common.revert')}
            </Button>
          )}
          <Button icon={RotateCw} loading={refreshing && list !== undefined} disabled={busy !== ''} onClick={() => void reloadFromServer(true)}>
            {t('common.refresh')}
          </Button>
          <Button
            icon={Check}
            loading={busy === 'validate'}
            disabled={!selected || busy !== ''}
            onClick={() => void validateRemote()}
          >
            {busy === 'validate' ? t('plugins.checking') : t('plugins.validate')}
          </Button>
          <Button
            variant="primary"
            icon={Save}
            loading={busy === 'save'}
            disabled={!editable || !selected || busy !== ''}
            onClick={() => void save()}
          >
            {busy === 'save' ? t('plugins.saving') : t('common.save')}
          </Button>
          <Button icon={BookOpen} onClick={() => setDocsOpen((o) => !o)}>
            {t('builder.docsButton')}
          </Button>
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={list !== undefined} retry={() => reload()} loadingText={t('plugins.loading')}>
        {list && (
          <Split
            fixed="left"
            initial={290}
            min={230}
            max={440}
            storageKey="plugins-nav"
            className="min-h-0 flex-1"
            left={
              <aside className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto rounded-card border border-line bg-surface p-3">
                <div>
                  <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-faint">
                    {t('plugins.builtinsSection')}
                  </div>
                  <p className="mb-2 text-xs text-muted">
                    {list.plugin_dir_configured
                      ? t('plugins.builtinsHintConfigured', { dir: list.plugin_dir || './plugins' })
                      : t('plugins.builtinsHintAuto')}
                  </p>
                  <ul className="space-y-1">
                    {(list.builtins ?? []).map((b) => {
                      const active = selected?.kind === 'builtin' && selected.name === b.name
                      return (
                        <li key={b.name}>
                          <button
                            type="button"
                            onClick={() => pick({ kind: 'builtin', name: b.name })}
                            className={cx(
                              'flex w-full items-center gap-2 rounded-ctl px-2.5 py-1.5 text-left text-sm transition-colors',
                              active ? 'bg-accent/15 font-medium text-text' : 'text-muted hover:bg-hover hover:text-text',
                            )}
                          >
                            <FileCode className="size-3.5 shrink-0 text-faint" />
                            <span className="min-w-0 flex-1 truncate font-mono text-xs">{b.name}</span>
                            <Chip tone={b.shadowed ? 'warning' : 'neutral'}>
                              {b.shadowed ? t('plugins.overridden') : b.kind === 'route' ? t('plugins.kindRoute') : t('plugins.kindSupport')}
                            </Chip>
                          </button>
                        </li>
                      )
                    })}
                  </ul>
                </div>

                <div className="border-t border-line pt-3">
                  <div className="mb-2 flex items-center justify-between gap-2">
                    <span className="text-xs font-semibold uppercase tracking-wide text-faint">
                      {t('plugins.packagesSection')}
                    </span>
                    {editable && (
                      <Button size="sm" icon={Plus} onClick={() => setNewPkgOpen(true)}>
                        {t('plugins.newPackage')}
                      </Button>
                    )}
                  </div>
                  {(list.packages ?? []).length === 0 ? (
                    <p className="text-xs text-muted">{t('plugins.packagesEmpty')}</p>
                  ) : (
                    <div className="space-y-3">
                      {(list.packages ?? []).map((pkg) => {
                        const directive = `-- af-import: ${pkg.name}`
                        return (
                          <div key={pkg.name} className="rounded-ctl border border-line/70 bg-bg/40 p-2">
                            <div className="flex items-center gap-1.5">
                              <FolderCode className="size-3.5 shrink-0 text-accent" />
                              <span className="min-w-0 flex-1 truncate font-mono text-xs font-semibold text-text">
                                {pkg.name}
                              </span>
                              {editable && (
                                <>
                                  <IconButton
                                    size="sm"
                                    icon={FilePlus}
                                    label={t('plugins.newFile')}
                                    onClick={() => setNewFilePkg(pkg.name)}
                                  />
                                  <IconButton
                                    size="sm"
                                    tone="danger"
                                    icon={Trash2}
                                    label={t('plugins.deletePackage')}
                                    onClick={() => void handleDeletePackage(pkg.name, pkg.managed)}
                                  />
                                </>
                              )}
                            </div>
                            <button
                              type="button"
                              title={t('plugins.copyImport')}
                              onClick={async () => {
                                if (await copyText(directive)) toast(t('common.copiedWhat', { what: directive }), 'success')
                                else toast(t('common.copyFailed'), 'danger')
                              }}
                              className="mt-1 truncate font-mono text-[11px] text-faint hover:text-accent"
                            >
                              {directive}
                            </button>
                            <ul className="mt-1.5 space-y-0.5">
                              {(pkg.files ?? []).map((f) => {
                                const active =
                                  selected?.kind === 'package' && selected.pkg === pkg.name && selected.file === f.name
                                return (
                                  <li key={f.name}>
                                    <button
                                      type="button"
                                      onClick={() => pick({ kind: 'package', pkg: pkg.name, file: f.name })}
                                      className={cx(
                                        'flex w-full items-center gap-1.5 rounded-ctl px-2 py-1 text-left font-mono text-xs transition-colors',
                                        active
                                          ? 'bg-accent/15 font-medium text-text'
                                          : 'text-muted hover:bg-hover hover:text-text',
                                      )}
                                    >
                                      <FileCode className="size-3 shrink-0 text-faint" />
                                      <span className="min-w-0 flex-1 truncate">{f.name}</span>
                                    </button>
                                  </li>
                                )
                              })}
                            </ul>
                          </div>
                        )
                      })}
                    </div>
                  )}
                </div>
              </aside>
            }
            right={
              docsOpen ? (
                <Split
                  fixed="right"
                  initial={420}
                  min={280}
                  max={680}
                  storageKey="plugins-docs"
                  className="min-h-0 flex-1"
                  left={editorCard}
                  right={<DocsPane onOpenDocs={(p) => void leave(() => navigate(p, { state: SKIP_GUARD }))} />}
                />
              ) : (
                editorCard
              )
            }
          />
        )}
      </AsyncView>

      <NewPackageDialog
        open={newPkgOpen}
        existingNames={(list?.packages ?? []).map((p) => p.name)}
        onClose={() => setNewPkgOpen(false)}
        onCreate={handleCreatePackage}
      />
      <NewFileDialog
        open={Boolean(newFilePkg)}
        pkgName={newFilePkg ?? ''}
        existingFiles={(newFileTargetPkg?.files ?? []).map((f) => f.name)}
        onClose={() => setNewFilePkg(null)}
        onCreate={(file) => handleCreateFile(newFilePkg!, file)}
      />
    </Page>
  )
}
