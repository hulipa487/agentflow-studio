// The agent builder, a workbench: one toolbar (the agent, its state, the
// actions), the definition on four tabs at full width, and a dock that holds
// either a run panel (chat and trace with this agent) or the Lua reference.
// A port of the desktop Studio's builder.go: the form renders the agent
// document, the loop and instructions travel as sources, and Apply swaps the
// live definition. Ctrl+Enter applies pending edits and runs a turn.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ArrowLeft, BookOpen, Braces, Check, FileCode, FileText, Play, RefreshCw, Settings, Trash2, Upload } from 'lucide-react'
import { errorMessage } from '@/api/client'
import type { AgentDetail, ApplyResult, Template } from '@/api/types'
import { AsyncView, Banner, Button, Chip, CopyButton, Dot, IconButton, Spinner, SplitButton, Tabs, cx, toneText, type TabDef } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { Split } from '@/components/ui/Split'
import { findExample, type ExampleLoop } from '@/docs/examples'
import { t, useLang } from '@/i18n'
import { SKIP_GUARD, useAlive, useAsync, useDebounced, useScreenActions, useStored, useUnsavedGuard } from '@/lib/hooks'
import { analyze, scriptSupport, type ScriptDiagnostic, type ScriptOptions } from '@/script'
import { MOD } from '@/nav'
import { useCatalog, useClient, useSession } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { deleteAgentFlow } from './agentActions'
import { ConfigTab } from './builder/ConfigTab'
import { DocsPane } from './builder/DocsPane'
import { EditorFrame } from './builder/EditorFrame'
import { LoopTab } from './builder/LoopTab'
import { RunPanel, type RunPanelHandle } from './builder/RunPanel'
import {
  buildYAML,
  checkLabels,
  displayName,
  effectiveCapabilities,
  headerStatus,
  initialForm,
  lf,
  nameError,
  type BuilderForm,
  type Notice,
} from './builder/model'

type TabID = 'config' | 'loop' | 'instructions' | 'yaml'
/** What the dock beside the editor shows; '' is closed. */
type Dock = '' | 'run' | 'docs'

/** One open builder. `name` is '' for an agent that does not exist yet. */
interface Instance {
  id: number
  route: string
  name: string
  template: string
  example: string
}

export default function Builder() {
  const { name: param = '' } = useParams()
  const [search] = useSearchParams()
  const navigate = useNavigate()
  const creating = useLocation().pathname === '/new-agent'
  const template = creating ? (search.get('template') ?? '') : ''
  const example = creating && template === '' ? (search.get('example') ?? '') : ''
  const route = creating ? `new?t=${template}&e=${example}` : 'edit:' + param

  // The first apply of a new agent moves the URL to /agents/<name>. That is
  // this builder's own navigation, so it must not reopen (and reload) the view.
  const adopted = useRef<string | null>(null)
  const [inst, setInst] = useState<Instance>({ id: 0, route, name: creating ? '' : param, template, example })
  if (inst.route !== route) {
    if (adopted.current === route) setInst({ ...inst, route })
    else setInst({ id: inst.id + 1, route, name: creating ? '' : param, template, example })
  }
  useEffect(() => {
    if (adopted.current === route) adopted.current = null
  }, [route])

  const adopt = useCallback(
    (name: string) => {
      adopted.current = 'edit:' + name
      // The builder's own move after a first apply: not a navigation to guard.
      navigate('/agents/' + encodeURIComponent(name), { replace: true, state: SKIP_GUARD })
    },
    [navigate],
  )
  // Reopens the view from the runtime's copy (refresh), discarding local state.
  const reopen = useCallback((name: string) => setInst((prev) => ({ ...prev, id: prev.id + 1, name, template: '', example: '' })), [])

  if (inst.name === '') return <NewAgent key={inst.id} template={inst.template} example={inst.example} onAdopt={adopt} onReopen={reopen} />
  return <LoadedAgent key={inst.id} name={inst.name} onAdopt={adopt} onReopen={reopen} />
}

interface Handoff {
  onAdopt: (name: string) => void
  onReopen: (name: string) => void
}

function NewAgent({ template, example, onAdopt, onReopen }: Handoff & { template: string; example: string }) {
  const catalog = useCatalog()
  // Resolved once: a later catalog reload must not reseed a form in progress.
  const [seed] = useState<Template | null>(() => (catalog.templates ?? []).find((tpl) => tpl.name === template) ?? null)
  const [exampleSeed] = useState<ExampleLoop | null>(() => (!seed && example !== '' ? (findExample(example) ?? null) : null))
  const missing = template !== '' && !seed ? template : ''
  const missingExample = template === '' && example !== '' && !exampleSeed ? example : ''
  return (
    <BuilderView
      initialName=""
      initialDetail={null}
      template={seed}
      example={exampleSeed}
      missingTemplate={missing}
      missingExample={missingExample}
      onAdopt={onAdopt}
      onReopen={onReopen}
    />
  )
}

function LoadedAgent({ name, onAdopt, onReopen }: Handoff & { name: string }) {
  const client = useClient()
  const { data, error, loading, reload } = useAsync((signal) => client.agent(name, signal), [client, name])
  if (!data) return <LoadState name={name} loading={loading} error={error} retry={() => reload()} />
  return <BuilderView initialName={name} initialDetail={data} template={null} example={null} missingTemplate="" missingExample="" onAdopt={onAdopt} onReopen={onReopen} />
}

/** Loading, failed, or the studio API switched off: the builder before it has a definition. */
function LoadState({ name, loading, error, retry }: { name: string; loading: boolean; error: unknown; retry: () => void }) {
  const navigate = useNavigate()
  const back = () => navigate('/agents')
  useScreenActions({ back, refresh: retry })
  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex shrink-0 items-center gap-2 border-b border-line px-5 py-3">
        <IconButton icon={ArrowLeft} label={t('common.back')} onClick={back} />
        <h1 className="min-w-0 truncate text-xl font-semibold leading-tight tracking-[-0.01em]">{name}</h1>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
        <AsyncView loading={loading} error={error} hasData={false} retry={retry} loadingText={t('builder.loadingAgent')}>
          {null}
        </AsyncView>
      </div>
    </div>
  )
}

// A visible source tab; the hidden ones stay mounted with `hidden` alone so
// two display utilities never compete.
const PANEL = 'flex min-h-0 flex-1 flex-col py-3'

/** The discard question, built when asked so it reads the current language. */
function discardPrompt() {
  return {
    title: t('common.discardTitle'),
    message: t('builder.discardMessage'),
    confirmLabel: t('common.discard'),
    danger: true,
  }
}

function BuilderView({
  initialName,
  initialDetail,
  template,
  example,
  missingTemplate,
  missingExample,
  onAdopt,
  onReopen,
}: Handoff & {
  initialName: string
  initialDetail: AgentDetail | null
  template: Template | null
  example: ExampleLoop | null
  missingTemplate: string
  missingExample: string
}) {
  const client = useClient()
  const catalog = useCatalog()
  const navigate = useNavigate()

  // The applied name: '' until a new agent is first applied.
  const [name, setName] = useState(initialName)
  const isNew = name === ''
  const [detail, setDetail] = useState(initialDetail)
  const [mtime, setMtime] = useState(initialDetail?.files?.mtime ?? 0)

  const [form, setForm] = useState<BuilderForm>(() => {
    const f = initialForm(initialName, initialDetail, template, catalog)
    if (example && f.name === '') f.name = example.slug
    return f
  })
  const [loop, setLoop] = useState(() => lf(initialDetail ? initialDetail.loop : (template?.loop ?? example?.source)))
  const [instructions, setInstructions] = useState(() => lf(initialDetail ? initialDetail.instructions : template?.instructions))
  // The YAML pane starts from the runtime's own rendering of the definition
  // and is only replaced by "Rebuild from form" or by typing in it.
  const [yaml, setYaml] = useState(() => {
    if (initialDetail?.agent_yaml) return lf(initialDetail.agent_yaml)
    try {
      return buildYAML(form, initialDetail)
    } catch {
      return ''
    }
  })
  // True once the raw YAML was edited by hand: it is then sent verbatim and
  // the form is ignored until the YAML is rebuilt from it.
  const [rawDirty, setRawDirty] = useState(false)
  const [dirty, setDirty] = useState(false)
  // The Apply button's choice, remembered across agents like a merge button's.
  const [persistChoice, setPersist] = useStored<boolean>('agentflow-studio:builder.persist', false)
  const persist = persistChoice === true
  const [dockChoice, setDock] = useStored<Dock>('agentflow-studio:builder.dock', '')
  const dock: Dock = dockChoice === 'run' || dockChoice === 'docs' ? dockChoice : ''
  const runPanel = useRef<RunPanelHandle>(null)
  const [notice, setNotice] = useState<Notice | null>(() => {
    if (missingTemplate) return { text: t('builder.missingTemplate', { name: missingTemplate }), tone: 'warning' }
    if (missingExample) return { text: t('builder.missingExample', { name: missingExample }), tone: 'warning' }
    return null
  })
  // The verb shown on the Apply button while a request is in flight.
  const [busy, setBusy] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [tab, setTab] = useState<TabID>(() => (example ? 'loop' : 'config'))
  const [visited, setVisited] = useState<Record<TabID, boolean>>(() => ({ config: true, loop: !!example, instructions: false, yaml: false }))

  const alive = useAlive()
  const inFlight = useRef(false)
  const request = useRef<AbortController | null>(null)
  // Counts edits, so an apply that finishes after further typing does not
  // declare the newer text applied.
  const edits = useRef(0)
  useEffect(() => {
    return () => {
      request.current?.abort()
    }
  }, [])

  const markDirty = useCallback(() => {
    edits.current++
    setDirty(true)
  }, [])
  const patch = useCallback(
    (p: Partial<BuilderForm>) => {
      setForm((f) => ({ ...f, ...p }))
      markDirty()
    },
    [markDirty],
  )
  // The editors echo a programmatic replacement (load, rebuild) back through
  // onChange; only a text that differs from the state is an edit.
  const editLoop = (text: string) => {
    if (text === loop) return
    setLoop(text)
    markDirty()
  }
  const editInstructions = (text: string) => {
    if (text === instructions) return
    setInstructions(text)
    markDirty()
  }
  const editYAML = (text: string) => {
    if (text === yaml) return
    setYaml(text)
    setRawDirty(true)
    markDirty()
  }

  // Every way out of a dirty builder (back, Test, the rail, the palette, a
  // shortcut, the browser's Back button) asks this one question.
  useUnsavedGuard(dirty, discardPrompt())

  // Entries the loaded definition selects but the catalog does not list stay
  // in the grids for the life of this view, checked or not.
  const [unlisted] = useState(() => ({
    caps: form.capabilities.filter((c) => !(catalog.capabilities ?? []).includes(c)),
    skills: form.skills.filter((s) => !(catalog.tools ?? []).includes(s)),
  }))
  const capLabels = useMemo(() => checkLabels(catalog.capabilities, [...unlisted.caps, ...form.capabilities]), [catalog.capabilities, unlisted, form.capabilities])
  const skillLabels = useMemo(() => checkLabels(catalog.tools, [...unlisted.skills, ...form.skills]), [catalog.tools, unlisted, form.skills])

  // ---- language intelligence --------------------------------------------------

  const preludeVersion = catalog.prelude_version > 0 ? catalog.prelude_version : 2
  // Keyed by content so the extension is rebuilt exactly when the effective
  // list changes: that is what re-lints the loop the moment a box is toggled.
  const capsKey = effectiveCapabilities(form.capabilities, catalog).join('\n')
  const scriptOptions = useMemo<ScriptOptions>(
    // No selection and no deployment defaults: the effective list is unknown,
    // so capability checks are skipped rather than flagging every call.
    () => ({ preludeVersion, capabilities: capsKey === '' ? null : capsKey.split('\n') }),
    [preludeVersion, capsKey],
  )
  const support = useMemo(() => {
    try {
      return { extensions: [scriptSupport(scriptOptions)], failure: '' }
    } catch (err) {
      return { extensions: [], failure: errorMessage(err) }
    }
  }, [scriptOptions])
  const linted = useDebounced(loop, 220)
  // Diagnostics are written in the language current when they are computed.
  const lang = useLang()
  const analysis = useMemo<{ diagnostics: ScriptDiagnostic[]; failure: string }>(() => {
    try {
      return { diagnostics: analyze(linted, scriptOptions), failure: '' }
    } catch (err) {
      return { diagnostics: [], failure: errorMessage(err) }
    }
  }, [linted, scriptOptions, lang])
  const loopErrors = analysis.diagnostics.filter((d) => d.severity === 'error').length
  const loopWarnings = analysis.diagnostics.filter((d) => d.severity === 'warning').length

  // ---- actions ----------------------------------------------------------------

  const setError = (text: string) => setNotice({ text, tone: 'danger' })

  /** The name and the document to send, or null after reporting why not. */
  const prepare = (): { name: string; yaml: string } | null => {
    const bad = nameError(form.name)
    if (bad) {
      setError(t(bad))
      return null
    }
    try {
      return { name: form.name.trim(), yaml: rawDirty ? yaml : buildYAML(form, detail) }
    } catch (err) {
      setError(errorMessage(err))
      return null
    }
  }

  const begin = (verb: string) => {
    inFlight.current = true
    setBusy(verb)
  }
  const end = () => {
    inFlight.current = false
    setBusy('')
  }

  const validate = async () => {
    if (inFlight.current) return
    const draft = prepare()
    if (!draft) return
    setNotice(null)
    begin(t('builder.checking'))
    const ctl = new AbortController()
    request.current = ctl
    const timer = setTimeout(() => ctl.abort(), 30_000)
    try {
      const res = await client.validateAgent({ name: draft.name, agent_yaml: draft.yaml, loop, instructions }, ctl.signal)
      if (!alive.current) return
      end()
      if (!res.ok) {
        setError((res.errors ?? []).join('\n'))
        return
      }
      const warnings = res.warnings ?? []
      const text = warnings.length > 0 ? t('builder.validWithWarnings', { warnings: warnings.join('; ') }) : t('builder.valid')
      setNotice({ text, tone: warnings.length > 0 ? 'warning' : 'success' })
      toast(t('builder.validationPassed'), 'success')
    } catch (err) {
      if (!alive.current) return
      end()
      setError(ctl.signal.aborted ? t('builder.validateTimeout') : errorMessage(err))
    } finally {
      clearTimeout(timer)
    }
  }

  /** Applies the definition (and saves it with `save`); true once it is live. */
  const apply = async (save = persist): Promise<boolean> => {
    if (inFlight.current) return false
    const draft = prepare()
    if (!draft) return false
    const first = isNew
    const editsAtSend = edits.current
    setNotice(null)
    begin(t('builder.applying'))
    let res: ApplyResult
    try {
      res = await client.putAgent(draft.name, {
        agent_yaml: draft.yaml,
        loop,
        instructions,
        apply: true,
        persist: save,
        force: false,
        expect_mtime: mtime,
      })
    } catch (err) {
      if (!alive.current) return false
      end()
      setError(errorMessage(err))
      return false
    }
    // The models, tools and templates other screens offer may depend on it.
    void useSession.getState().reloadCatalog()
    let fresh: AgentDetail | null = null
    try {
      fresh = await client.agent(draft.name)
    } catch {
      /* the header keeps the facts it had */
    }
    if (!alive.current) return false
    end()
    if (edits.current === editsAtSend) setDirty(false)
    if (first) {
      setName(draft.name)
      setForm((f) => ({ ...f, name: draft.name }))
      onAdopt(draft.name)
    }
    if (fresh) {
      setDetail(fresh)
      setMtime(fresh.files?.mtime ?? 0)
    } else if (res.persisted && res.files?.mtime) {
      // The reread failed, but the save itself reported the file's new stamp.
      setMtime(res.files.mtime)
    }
    if (!res.applied) {
      // Not a path this screen asks for, but never claim a swap that did not happen.
      setNotice({ text: res.note || t('builder.notApplied'), tone: 'warning' })
    } else if (res.persist_error) {
      setNotice({ text: t('builder.persistFailed', { error: res.persist_error }), tone: 'warning' })
    } else if (res.persisted) {
      setNotice({ text: t('builder.appliedPersisted'), tone: 'success' })
    } else {
      setNotice({ text: t('builder.appliedLive'), tone: 'success' })
    }
    toast(t('builder.applied', { name: draft.name }), 'success')
    return res.applied
  }

  /**
   * Ctrl+Enter: applies pending edits (live only, whatever the Apply button's
   * choice), then sends the run panel's message so the turn runs what is on
   * screen.
   */
  const run = async () => {
    if (inFlight.current || deleting) return
    setDock('run')
    let target = name
    if (dirty || isNew) {
      if (!(await apply(false))) return
      target = form.name.trim()
    }
    if (!alive.current) return
    runPanel.current?.run(target)
  }

  const toggleDock = (next: Exclude<Dock, ''>) => {
    setDock(dock === next ? '' : next)
    if (next === 'run' && dock !== 'run') setTimeout(() => runPanel.current?.focus(), 0)
  }

  /**
   * Runs `go` at once, or after the operator agrees to lose unapplied edits.
   * For actions that stay on this route; leaving it is the guard's job.
   */
  const leave = async (go: () => void, message = t('builder.discardMessage')) => {
    if (dirty && !(await confirm({ ...discardPrompt(), message }))) return
    if (alive.current) go()
  }

  const back = () => {
    if (inFlight.current || deleting) return
    navigate('/agents')
  }

  const openPlayground = () => {
    if (isNew || inFlight.current || deleting) return
    navigate('/playground?agent=' + encodeURIComponent(name))
  }

  const refresh = () => {
    if (inFlight.current || deleting) return
    if (isNew) {
      // Nothing on the runtime to reread yet; the pickers still can be.
      void useSession.getState().reloadCatalog()
      return
    }
    void leave(() => {
      void useSession.getState().reloadCatalog()
      onReopen(name)
    }, t('builder.reloadMessage'))
  }

  const remove = async () => {
    if (isNew || inFlight.current || deleting) return
    setDeleting(true)
    const gone = await deleteAgentFlow(client, name)
    if (!alive.current) return
    setDeleting(false)
    if (gone) navigate('/agents', { state: SKIP_GUARD })
  }

  const rebuild = () => {
    try {
      // Replacing the text is not an edit: the form is the source again.
      setYaml(buildYAML(form, detail))
      setRawDirty(false)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  useScreenActions({ save: () => void apply(), send: () => void run(), back, refresh, help: () => toggleDock('docs') })

  const selectTab = (next: TabID) => {
    setTab(next)
    setVisited((v) => (v[next] ? v : { ...v, [next]: true }))
  }

  const insertDirective = () => {
    const directive = `// af-prelude-version: ${preludeVersion}`
    if (loop.split('\n').some((l) => l.trim().startsWith('// af-prelude-version:'))) return
    editLoop(loop === '' ? `${directive}\n` : `${directive}\n${loop}`)
  }

  const enableCapability = (cap: string) => {
    const base = effectiveCapabilities(form.capabilities, catalog)
    if (base.includes(cap)) return
    const nextSet = new Set([...base, cap])
    const next = capLabels.filter((l) => nextSet.has(l))
    if (!next.includes(cap)) next.push(cap)
    patch({ capabilities: next })
  }

  // ---- render -----------------------------------------------------------------

  const locked = busy !== '' || deleting
  const status = headerStatus(dirty, detail)
  const applyLabel = busy || (persist ? t('builder.applySave') : t('common.apply'))
  const dockToggle = (on: boolean) => cx(on && 'border-accent/50 bg-surface2 text-text')

  const tabs: TabDef<TabID>[] = [
    { id: 'config', label: t('builder.tabConfig'), icon: Settings },
    {
      id: 'loop',
      label: t('builder.tabLoop'),
      icon: FileCode,
      // The verdict stays visible from the other tabs: toggling a capability
      // on Config can break (or fix) the loop.
      badge:
        loopErrors > 0 ? (
          <span className="font-mono text-xs text-danger" title={t('builder.loopErrors', { n: loopErrors })}>
            {loopErrors}
          </span>
        ) : loopWarnings > 0 ? (
          <span className="font-mono text-xs text-warning" title={t('builder.loopWarnings', { n: loopWarnings })}>
            {loopWarnings}
          </span>
        ) : undefined,
    },
    { id: 'instructions', label: t('builder.tabInstructions'), icon: FileText },
    { id: 'yaml', label: 'YAML', icon: Braces },
  ]

  // The toolbar spans the editor and the dock, so its actions keep their room
  // however wide the dock is dragged.
  const toolbar = (
    <header className="shrink-0 border-b border-line pb-2.5 pl-3 pr-5 pt-2.5">
        <div className="flex items-center gap-2">
          <IconButton icon={ArrowLeft} label={t('common.back')} onClick={back} disabled={locked} />
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-semibold leading-tight tracking-[-0.01em]">
            {displayName(name)}
            {dirty && (
              <span className="text-accent" title={t('builder.statusUnsaved')}>
                {' '}
                •
              </span>
            )}
          </h1>
          <div className="mt-0.5 flex items-center gap-1.5">
            <Dot tone={status.tone} />
            <span className={cx('text-xs', toneText(status.tone))}>{status.text}</span>
          </div>
        </div>
        {busy !== '' && <Spinner />}
        <Button icon={Check} onClick={() => void validate()} disabled={locked}>
          {t('builder.validate')}
        </Button>
        <SplitButton
          label={applyLabel}
          icon={Upload}
          onClick={() => void apply()}
          disabled={locked}
          className="min-w-[136px]"
          menuLabel={t('builder.applyOptions')}
          options={[
            {
              label: t('common.apply'),
              hint: t('builder.applyHint'),
              checked: !persist,
              onSelect: () => {
                setPersist(false)
                void apply(false)
              },
            },
            {
              label: t('builder.applySave'),
              hint: t('builder.applySaveHint'),
              checked: persist,
              onSelect: () => {
                setPersist(true)
                void apply(true)
              },
            },
          ]}
        />
        <Button
          icon={Play}
          onClick={() => toggleDock('run')}
          aria-pressed={dock === 'run'}
          className={dockToggle(dock === 'run')}
          title={t('builder.runTitle', { mod: MOD })}
        >
          {t('builder.run')}
        </Button>
        <IconButton icon={BookOpen} label={t('builder.docsButton')} onClick={() => toggleDock('docs')} aria-pressed={dock === 'docs'} className={dockToggle(dock === 'docs')} />
        <IconButton icon={Trash2} label={t('common.delete')} tone="danger" onClick={() => void remove()} disabled={locked || isNew} />
      </div>
      {notice && (
        <Banner tone={notice.tone} onClose={() => setNotice(null)} className="mt-2">
          {notice.text}
        </Banner>
      )}
    </header>
  )

  const main = (
    <div className={cx('flex min-h-0 flex-1 flex-col pl-5 pt-1', dock === '' ? 'pr-5' : 'pr-2')}>
      <Tabs tabs={tabs} value={tab} onChange={selectTab} />

      {/* The scrollbar sits in the gutter to the right of the cards (-mr/pr), not over their edge. */}
      <div className={cx('-mr-2 min-h-0 flex-1 overflow-y-auto pr-2', tab !== 'config' && 'hidden')}>
        <ConfigTab form={form} patch={patch} catalog={catalog} isNew={isNew} capLabels={capLabels} skillLabels={skillLabels} />
      </div>

      {/* The source tabs mount on first visit and then stay, so each editor keeps its undo history and scroll position. */}
      {visited.loop && (
        <div className={tab === 'loop' ? PANEL : 'hidden'}>
          <LoopTab
            source={loop}
            onChange={editLoop}
            extensions={support.extensions}
            diagnostics={analysis.diagnostics}
            checking={linted !== loop}
            failure={support.failure || analysis.failure}
            active={tab === 'loop'}
            onInsertDirective={insertDirective}
            onEnableCapability={enableCapability}
          />
        </div>
      )}

      {visited.instructions && (
        <div className={tab === 'instructions' ? PANEL : 'hidden'}>
          <EditorFrame
            title={t('builder.tabInstructions')}
            hint={t('builder.instructionsHint')}
            actions={<CopyButton text={instructions} what={t('builder.whatInstructions')} size="md" />}
          >
            <CodeEditor language="markdown" lineWrapping value={instructions} onChange={editInstructions} className="flex-1" />
          </EditorFrame>
        </div>
      )}

      {visited.yaml && (
        <div className={tab === 'yaml' ? PANEL : 'hidden'}>
          <EditorFrame
            title={t('builder.rawYaml')}
            hint={t('builder.rawYamlHint')}
            actions={
              <>
                {rawDirty && (
                  <Chip tone="warning" title={t('builder.overridesFormTitle')}>
                    {t('builder.overridesForm')}
                  </Chip>
                )}
                <Button size="sm" icon={RefreshCw} onClick={rebuild}>
                  {t('builder.rebuild')}
                </Button>
                <CopyButton text={yaml} what="YAML" size="md" />
              </>
            }
          >
            <CodeEditor language="yaml" value={yaml} onChange={editYAML} className="flex-1" />
          </EditorFrame>
        </div>
      )}
    </div>
  )

  // Both dock bodies stay mounted while the builder is open, so the chat keeps
  // its thread and the reference its scroll position when the dock flips.
  const docked = (
    <div className="flex min-h-0 flex-1 flex-col py-3 pr-5">
      <div className={dock === 'run' ? 'flex min-h-0 flex-1 flex-col' : 'hidden'}>
        <RunPanel ref={runPanel} client={client} agent={name} onRun={() => void run()} onOpenPlayground={openPlayground} />
      </div>
      <div className={dock === 'docs' ? 'flex min-h-0 flex-1 flex-col' : 'hidden'}>
        <DocsPane onOpenDocs={(path) => navigate(path)} />
      </div>
    </div>
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      {toolbar}
      <Split storageKey="builder.dock" fixed="right" initial={420} min={320} max={760} minOther={480} collapsed={dock === ''} left={main} right={docked} />
    </div>
  )
}
