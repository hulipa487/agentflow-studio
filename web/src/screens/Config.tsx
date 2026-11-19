// Config: the raw config editor. Everything AgentFlow can be configured with
// but Studio has no structured screen for (memory backends and profiles,
// prompts, tools.policy and tools.http, MCP servers, triggers, search, files,
// media, net, usage, audit, runtime.*) is edited here, through the runtime's
// boot-equivalent validation and its atomic, backed-up save.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Check, Eye, Pencil, RotateCw, Save, Undo2 } from 'lucide-react'
import { EditorView } from '@codemirror/view'
import { ApiError, errorMessage } from '@/api/client'
import { AsyncView, Banner, Button, CopyButton, Page, Panel, Segmented, Tabs, type Tone } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { t, type Params } from '@/i18n'
import { useAsync, useScreenActions, useStored, useUnsavedGuard } from '@/lib/hooks'
import { copyText } from '@/lib/format'
import { MOD } from '@/nav'
import { useClient, useSession } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { EditorFrame, baseName, errorLine, renderView, stamp, type ViewFormat } from './config/parts'

/** The file as it was last read from (or written to) disk. */
interface Loaded {
  path: string
  mtime: number
  original: string
  view: unknown
}

interface Notice {
  tone: Tone
  /** A message key, read when drawn so a language change re-reads it. */
  key?: string
  params?: Params
  /** Text shown as is: the runtime's own words. */
  text?: string
  /** The save was refused because the file changed on disk. */
  conflict?: boolean
}

function noticeText(n: Notice): string {
  return n.key ? t(n.key, n.params) : (n.text ?? '')
}

const FRAGMENT_SCREENS = [
  { labelKey: 'config.screenAgents', path: '/agents' },
  { labelKey: 'config.screenChannels', path: '/channels' },
  { labelKey: 'config.screenModels', path: '/models' },
  { labelKey: 'config.screenSafety', path: '/safety' },
]

export default function Config() {
  const client = useClient()
  const navigate = useNavigate()
  const configDir = useSession((s) => s.runtime?.config_path ?? '')
  const { data, error, loading, refreshing, reload } = useAsync((signal) => client.config(signal), [client])

  const [doc, setDoc] = useState<Loaded | null>(null)
  const [text, setText] = useState('')
  const [notice, setNotice] = useState<Notice | null>(null)
  const [busy, setBusy] = useState<'' | 'validate' | 'save'>('')
  const [tab, setTab] = useState<'edit' | 'masked'>('edit')
  const [storedFormat, setFormat] = useStored<ViewFormat>('agentflow-studio:config-view-format', 'json')
  const format: ViewFormat = storedFormat === 'yaml' ? 'yaml' : 'json'
  const editor = useRef<EditorView | null>(null)
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
    }
  }, [])

  // Every answer from the runtime replaces the editor: the first load, and a
  // reload the operator asked for (and confirmed, if there were edits).
  useEffect(() => {
    if (!data) return
    setNotice(null)
    if (!data.ok) {
      setDoc(null)
      setText('')
      return
    }
    const raw = data.raw ?? ''
    setDoc({ path: data.path, mtime: data.mtime, original: raw, view: data.view })
    setText(raw)
  }, [data])

  const dirty = doc !== null && text !== doc.original
  useUnsavedGuard(dirty)

  const reloadFile = async (ask: boolean) => {
    if (ask && dirty) {
      const ok = await confirm({
        title: t('config.discardTitle'),
        message: t('config.reloadDiscard'),
        confirmLabel: t('common.discard'),
        danger: true,
      })
      if (!ok || !alive.current) return
    }
    // Quiet once something is on screen: a failed reload then keeps the
    // editor and its edits instead of replacing them with an error page.
    reload(data !== undefined)
  }

  const revert = async () => {
    if (!doc || !dirty) return
    const ok = await confirm({
      title: t('config.discardTitle'),
      message: t('config.revertDiscard'),
      confirmLabel: t('common.discard'),
      danger: true,
    })
    if (!ok || !alive.current) return
    setText(doc.original)
    setNotice(null)
  }

  const validate = async () => {
    if (!doc || busy) return
    setBusy('validate')
    setNotice({ tone: 'neutral', key: 'config.validating' })
    try {
      const r = await client.validateConfig(text)
      if (!alive.current) return
      if (!r.ok) {
        setNotice({ tone: 'danger', key: 'config.invalid', params: { error: r.error } })
        return
      }
      setNotice(r.note ? { tone: 'success', key: 'config.validWithNote', params: { note: r.note } } : { tone: 'success', key: 'config.valid' })
    } catch (err) {
      if (alive.current) setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy('')
    }
  }

  const save = async () => {
    if (!doc || busy) return
    if (!dirty) {
      toast(t('config.noChanges'))
      return
    }
    const raw = text
    const mtime = doc.mtime
    const ok = await confirm({
      title: t('config.saveTitle'),
      message: t('config.saveMessage', { path: doc.path }),
      confirmLabel: t('common.save'),
    })
    if (!ok || !alive.current) return
    setBusy('save')
    try {
      const r = await client.saveConfig(raw, mtime)
      if (!alive.current) return
      if (!r.ok) {
        setNotice({ tone: 'danger', text: r.error })
        return
      }
      setDoc((d) => (d ? { ...d, original: raw, mtime: r.mtime > 0 ? r.mtime : d.mtime } : d))
      setNotice(r.note ? { tone: 'success', text: r.note } : { tone: 'success', key: 'config.saved' })
      void useSession.getState().reloadCatalog()
      // The masked view shows the file on disk, which just changed. Only the
      // view is taken from this read, so typing meanwhile is never undone.
      client
        .config()
        .then((f) => {
          if (alive.current && f.ok && f.raw === raw) setDoc((d) => (d ? { ...d, view: f.view } : d))
        })
        .catch(() => {})
    } catch (err) {
      if (!alive.current) return
      if (err instanceof ApiError && err.status === 409) {
        setNotice({
          tone: 'warning',
          key: 'config.conflict',
          conflict: true,
        })
      } else {
        setNotice({ tone: 'danger', text: errorMessage(err) })
      }
    } finally {
      if (alive.current) setBusy('')
    }
  }

  useScreenActions({ save: () => void save(), refresh: () => void reloadFile(true) })

  const goToLine = (line: number) => {
    const view = editor.current
    if (!view) return
    setTab('edit')
    // The editor may be hidden behind the masked tab until the next render,
    // and a hidden editor cannot scroll to anything.
    requestAnimationFrame(() => {
      if (!alive.current) return
      const at = view.state.doc.line(Math.min(line, view.state.doc.lines))
      view.dispatch({ selection: { anchor: at.from, head: at.to }, effects: EditorView.scrollIntoView(at.from, { y: 'center' }) })
      view.focus()
    })
  }

  const lines = useMemo(() => text.split('\n').length, [text])
  const viewText = useMemo(() => (tab === 'masked' && doc ? renderView(doc.view, format) : ''), [tab, doc, format])
  const directory = data !== undefined && !data.ok
  const shownNotice = notice ? noticeText(notice) : ''
  const badLine = notice?.tone === 'danger' ? errorLine(shownNotice) : null

  const meta = refreshing ? (
    <span>{t('config.loadingMeta')}</span>
  ) : directory ? (
    <span>{t('config.directoryMeta')}</span>
  ) : doc ? (
    <>
      <span className="inline-flex min-w-0 items-center gap-1">
        <span className="truncate" title={doc.path}>
          {doc.path}
        </span>
        <CopyButton text={doc.path} what={t('config.whatPath')} />
      </span>
      {doc.mtime > 0 && <span>{t('config.modified', { time: stamp(doc.mtime) })}</span>}
      <span>{t('config.lines', { n: lines })}</span>
      {dirty && <span className="text-warning">{t('config.unsaved')}</span>}
    </>
  ) : null

  // A reload that failed with the file still on screen; a first load that
  // fails is AsyncView's to report.
  const reloadFailed = data !== undefined && !!error
  // Page makes room for the banner slot whenever it is handed anything.
  const banner = !reloadFailed && !directory && !notice ? null : (
    <>
      {reloadFailed && <Banner tone="danger">{errorMessage(error)}</Banner>}
      {directory && <Banner tone="warning">{`${data?.error ?? ''}\n\n${t('config.directoryNote')}`}</Banner>}
      {notice && (
        <Banner tone={notice.tone} onClose={() => setNotice(null)}>
          {shownNotice}
          {(notice.conflict || badLine !== null) && (
            <span className="mt-2 flex flex-wrap gap-2">
              {badLine !== null && (
                <Button size="sm" onClick={() => goToLine(badLine)}>
                  {t('config.goToLine', { n: badLine })}
                </Button>
              )}
              {notice.conflict && (
                <>
                  <Button size="sm" icon={RotateCw} onClick={() => void reloadFile(true)}>
                    {t('config.reloadFromDisk')}
                  </Button>
                  <Button
                    size="sm"
                    onClick={async () => {
                      // Reloading discards the edits; this is how they survive it.
                      if (await copyText(text)) toast(t('config.editsCopied'), 'success')
                      else toast(t('common.copyFailed'), 'danger')
                    }}
                  >
                    {t('config.copyEdits')}
                  </Button>
                </>
              )}
            </span>
          )}
        </Banner>
      )}
    </>
  )

  return (
    <Page
      title={
        <>
          {t('config.title')}
          {dirty && (
            <span className="text-warning" title={t('config.unsavedTitle')}>
              {' '}
              ●
            </span>
          )}
        </>
      }
      subtitle={t('config.subtitle')}
      scroll={false}
      meta={meta}
      banner={banner}
      actions={
        <>
          {dirty && (
            <Button icon={Undo2} onClick={() => void revert()} disabled={busy !== ''}>
              {t('common.revert')}
            </Button>
          )}
          <Button icon={RotateCw} onClick={() => void reloadFile(true)} loading={refreshing && data !== undefined} disabled={busy !== ''}>
            {t('config.reload')}
          </Button>
          <Button icon={Check} onClick={() => void validate()} loading={busy === 'validate'} disabled={!doc || busy !== '' || refreshing}>
            {t('config.validate')}
          </Button>
          <Button variant="primary" icon={Save} onClick={() => void save()} loading={busy === 'save'} disabled={!doc || busy !== '' || refreshing}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={data !== undefined} retry={() => reload()} loadingText={t('config.loadingFile')}>
        {directory && (
          <Panel className="flex flex-col gap-3">
            <div className="min-w-0">
              <div className="text-xs text-muted">{t('config.directoryLabel')}</div>
              <div className="flex min-w-0 items-center gap-1">
                <span className="truncate font-mono text-xs text-text">{configDir || '—'}</span>
                {configDir && <CopyButton text={configDir} what={t('config.whatPath')} />}
              </div>
              <p className="mt-1 text-xs leading-snug text-muted">{t('config.directoryHelp')}</p>
            </div>
            <div className="flex flex-wrap gap-2">
              {FRAGMENT_SCREENS.map((s) => (
                <Button key={s.path} size="sm" onClick={() => navigate(s.path)}>
                  {t(s.labelKey)}
                </Button>
              ))}
            </div>
          </Panel>
        )}
        {doc && (
          <div className="flex min-h-0 flex-1 flex-col gap-2">
            <Tabs
              value={tab}
              onChange={setTab}
              tabs={[
                { id: 'edit', label: t('common.edit'), icon: Pencil },
                { id: 'masked', label: t('config.tabMasked'), icon: Eye },
              ]}
            />
            {/* The editor stays mounted behind the masked tab so its undo history and scroll position survive the visit. */}
            <EditorFrame
              hidden={tab !== 'edit'}
              title={baseName(doc.path)}
              hint={t('config.editHint', { placeholder: '${VAR}', mod: MOD })}
            >
              <CodeEditor
                value={text}
                onChange={setText}
                language="yaml"
                onReady={(v) => (editor.current = v)}
                autoFocus
                className="min-h-0 flex-1"
              />
            </EditorFrame>
            {tab === 'masked' && (
              <EditorFrame
                title={t('config.parsedTitle')}
                hint={t(dirty ? 'config.maskedHintDirty' : 'config.maskedHint')}
                trailing={
                  <>
                    <Segmented
                      value={format}
                      onChange={setFormat}
                      options={[
                        { value: 'json', label: 'JSON' },
                        { value: 'yaml', label: 'YAML' },
                      ]}
                    />
                    {viewText !== '' && <CopyButton text={viewText} what={t('config.whatMasked')} size="md" />}
                  </>
                }
              >
                {viewText === '' ? (
                  <div className="flex min-h-0 flex-1 items-center justify-center rounded-ctl border border-line bg-bg px-6 text-center text-sm text-muted">
                    {t('config.noParse')}
                  </div>
                ) : (
                  <CodeEditor value={viewText} language={format} readOnly className="min-h-0 flex-1" />
                )}
              </EditorFrame>
            )}
          </div>
        )}
      </AsyncView>
    </Page>
  )
}
