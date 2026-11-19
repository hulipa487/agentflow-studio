// The agent builder, a workbench: one toolbar (the agent, its state, the
// actions), the definition on four tabs at full width, and a dock that holds
// either a run panel (chat and trace with this agent) or the Luau reference.
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
import { SKIP_GUARD, useAsync, useDebounced, useScreenActions, useStored, useUnsavedGuard } from '@/lib/hooks'
import { analyze, luauSupport, type LuauDiagnostic, type LuauOptions } from '@/lua'
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
  countLabel,
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
}

export default function Builder() {
  const { name: param = '' } = useParams()
  const [search] = useSearchParams()
  const navigate = useNavigate()
  const creating = useLocation().pathname === '/new-agent'
  const template = creating ? (search.get('template') ?? '') : ''
  const route = creating ? 'new?' + template : 'edit:' + param

  // The first apply of a new agent moves the URL to /agents/<name>. That is
  // this builder's own navigation, so it must not reopen (and reload) the view.
  const adopted = useRef<string | null>(null)
  const [inst, setInst] = useState<Instance>({ id: 0, route, name: creating ? '' : param, template })
  if (inst.route !== route) {
    if (adopted.current === route) setInst({ ...inst, route })
    else setInst({ id: inst.id + 1, route, name: creating ? '' : param, template })
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
  const reopen = useCallback((name: string) => setInst((prev) => ({ ...prev, id: prev.id + 1, name, template: '' })), [])

  if (inst.name === '') return <NewAgent key={inst.id} template={inst.template} onAdopt={adopt} onReopen={reopen} />
  return <LoadedAgent key={inst.id} name={inst.name} onAdopt={adopt} onReopen={reopen} />
}

interface Handoff {
  onAdopt: (name: string) => void
  onReopen: (name: string) => void
}

function NewAgent({ template, onAdopt, onReopen }: Handoff & { template: string }) {
  const catalog = useCatalog()
  // Resolved once: a later catalog reload must not reseed a form in progress.
  const [seed] = useState<Template | null>(() => (catalog.templates ?? []).find((t) => t.name === template) ?? null)
  const missing = template !== '' && !seed ? template : ''
  return <BuilderView initialName="" initialDetail={null} template={seed} missingTemplate={missing} onAdopt={onAdopt} onReopen={onReopen} />
}

function LoadedAgent({ name, onAdopt, onReopen }: Handoff & { name: string }) {
  const client = useClient()
  const { data, error, loading, reload } = useAsync((signal) => client.agent(name, signal), [client, name])
  if (!data) return <LoadState name={name} loading={loading} error={error} retry={() => reload()} />
  return <BuilderView initialName={name} initialDetail={data} template={null} missingTemplate="" onAdopt={onAdopt} onReopen={onReopen} />
}

/** Loading, failed, or the studio API switched off: the builder before it has a definition. */
function LoadState({ name, loading, error, retry }: { name: string; loading: boolean; error: unknown; retry: () => void }) {
  const navigate = useNavigate()
  const back = () => navigate('/agents')
  useScreenActions({ back, refresh: retry })
  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex shrink-0 items-center gap-2 border-b border-line px-5 py-3">
        <IconButton icon={ArrowLeft} label="Back" onClick={back} />
        <h1 className="min-w-0 truncate text-xl font-semibold leading-tight tracking-[-0.01em]">{name}</h1>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
        <AsyncView loading={loading} error={error} hasData={false} retry={retry} loadingText="Loading agent…">
          {null}
        </AsyncView>
      </div>
    </div>
  )
}

// A visible source tab; the hidden ones stay mounted with `hidden` alone so
// two display utilities never compete.
const PANEL = 'flex min-h-0 flex-1 flex-col py-3'

const DISCARD = {
  title: 'Discard changes?',
  message: 'The agent has unsaved edits in this window. Leave without applying them?',
  confirmLabel: 'Discard',
  danger: true,
}

function BuilderView({
  initialName,
  initialDetail,
  template,
  missingTemplate,
  onAdopt,
  onReopen,
}: Handoff & {
  initialName: string
  initialDetail: AgentDetail | null
  template: Template | null
  missingTemplate: string
}) {
  const client = useClient()
  const catalog = useCatalog()
  const navigate = useNavigate()

  // The applied name: '' until a new agent is first applied.
  const [name, setName] = useState(initialName)
  const isNew = name === ''
  const [detail, setDetail] = useState(initialDetail)
  const [mtime, setMtime] = useState(initialDetail?.files?.mtime ?? 0)

  const [form, setForm] = useState<BuilderForm>(() => initialForm(initialName, initialDetail, template, catalog))
  const [loop, setLoop] = useState(() => lf(initialDetail ? initialDetail.loop : template?.loop))
  const [instructions, setInstructions] = useState(() => lf(initialDetail?.instructions))
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
  const [notice, setNotice] = useState<Notice | null>(() =>
    missingTemplate ? { text: `Template "${missingTemplate}" is not in this runtime's catalog. Starting from a blank agent.`, tone: 'warning' } : null,
  )
  // The verb shown on the Apply button while a request is in flight.
  const [busy, setBusy] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [tab, setTab] = useState<TabID>('config')
  const [visited, setVisited] = useState<Record<TabID, boolean>>({ config: true, loop: false, instructions: false, yaml: false })

  const alive = useRef(true)
  const inFlight = useRef(false)
  const request = useRef<AbortController | null>(null)
  // Counts edits, so an apply that finishes after further typing does not
  // declare the newer text applied.
  const edits = useRef(0)
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
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
  useUnsavedGuard(dirty, DISCARD)

  // Entries the loaded definition selects but the catalog does not list stay
  // in the grids for the life of this view, checked or not.
  const [unlisted] = useState(() => ({
    caps: form.capabilities.filter((c) => !(catalog.capabilities ?? []).includes(c)),
    skills: form.skills.filter((s) => !(catalog.tools ?? []).includes(s)),
  }))
  const capLabels = useMemo(() => checkLabels(catalog.capabilities, [...unlisted.caps, ...form.capabilities]), [catalog.capabilities, unlisted, form.capabilities])
  const skillLabels = useMemo(() => checkLabels(catalog.tools, [...unlisted.skills, ...form.skills]), [catalog.tools, unlisted, form.skills])

  // ---- language intelligence --------------------------------------------------

  const preludeVersion = catalog.prelude_version > 0 ? catalog.prelude_version : 1
  // Keyed by content so the extension is rebuilt exactly when the effective
  // list changes: that is what re-lints the loop the moment a box is toggled.
  const capsKey = effectiveCapabilities(form.capabilities, catalog).join('\n')
  const luaOptions = useMemo<LuauOptions>(
    // No selection and no deployment defaults: the effective list is unknown,
    // so capability checks are skipped rather than flagging every call.
    () => ({ preludeVersion, capabilities: capsKey === '' ? null : capsKey.split('\n') }),
    [preludeVersion, capsKey],
  )
  const support = useMemo(() => {
    try {
      return { extensions: [luauSupport(luaOptions)], failure: '' }
    } catch (err) {
      return { extensions: [], failure: errorMessage(err) }
    }
  }, [luaOptions])
  const linted = useDebounced(loop, 220)
  const analysis = useMemo<{ diagnostics: LuauDiagnostic[]; failure: string }>(() => {
    try {
      return { diagnostics: analyze(linted, luaOptions), failure: '' }
    } catch (err) {
      return { diagnostics: [], failure: errorMessage(err) }
    }
  }, [linted, luaOptions])
  const loopErrors = analysis.diagnostics.filter((d) => d.severity === 'error').length
  const loopWarnings = analysis.diagnostics.filter((d) => d.severity === 'warning').length

  // ---- actions ----------------------------------------------------------------

  const setError = (text: string) => setNotice({ text, tone: 'danger' })

  /** The name and the document to send, or null after reporting why not. */
  const prepare = (): { name: string; yaml: string } | null => {
    const bad = nameError(form.name)
    if (bad) {
      setError(bad)
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
    begin('Checking…')
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
      let text = 'Valid. The loop compiles and the definition passes boot validation.'
      if (warnings.length > 0) text += '\nWarnings: ' + warnings.join('; ')
      setNotice({ text, tone: warnings.length > 0 ? 'warning' : 'success' })
      toast('validation passed', 'success')
    } catch (err) {
      if (!alive.current) return
      end()
      setError(ctl.signal.aborted ? 'The runtime did not answer the validation within 30 seconds.' : errorMessage(err))
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
    begin('Applying…')
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
      setNotice({ text: res.note || 'Validated but not applied.', tone: 'warning' })
    } else if (res.persist_error) {
      setNotice({ text: 'Applied live, but persisting failed: ' + res.persist_error, tone: 'warning' })
    } else if (res.persisted) {
      setNotice({ text: 'Applied live and persisted. The definition loads at the next boot.', tone: 'success' })
    } else {
      setNotice({ text: 'Applied live on this instance.', tone: 'success' })
    }
    toast('applied ' + draft.name, 'success')
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
  const leave = async (go: () => void, message = DISCARD.message) => {
    if (dirty && !(await confirm({ ...DISCARD, message }))) return
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
    }, 'The agent has unsaved edits in this window. Reload the definition from the runtime and lose them?')
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

  // ---- render -----------------------------------------------------------------

  const locked = busy !== '' || deleting
  const status = headerStatus(dirty, detail)
  const applyLabel = busy || (persist ? 'Apply & save' : 'Apply')
  const dockToggle = (on: boolean) => cx(on && 'border-accent/50 bg-surface2 text-text')

  const tabs: TabDef<TabID>[] = [
    { id: 'config', label: 'Config', icon: Settings },
    {
      id: 'loop',
      label: 'Loop',
      icon: FileCode,
      // The verdict stays visible from the other tabs: toggling a capability
      // on Config can break (or fix) the loop.
      badge:
        loopErrors > 0 ? (
          <span className="font-mono text-xs text-danger" title={countLabel(loopErrors, 'error', 'errors') + ' in the loop'}>
            {loopErrors}
          </span>
        ) : loopWarnings > 0 ? (
          <span className="font-mono text-xs text-warning" title={countLabel(loopWarnings, 'warning', 'warnings') + ' in the loop'}>
            {loopWarnings}
          </span>
        ) : undefined,
    },
    { id: 'instructions', label: 'Instructions', icon: FileText },
    { id: 'yaml', label: 'YAML', icon: Braces },
  ]

  // The toolbar spans the editor and the dock, so its actions keep their room
  // however wide the dock is dragged.
  const toolbar = (
    <header className="shrink-0 border-b border-line pb-2.5 pl-3 pr-5 pt-2.5">
        <div className="flex items-center gap-2">
          <IconButton icon={ArrowLeft} label="Back" onClick={back} disabled={locked} />
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-semibold leading-tight tracking-[-0.01em]">
            {displayName(name)}
            {dirty && (
              <span className="text-accent" title="Unsaved changes">
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
          Validate
        </Button>
        <SplitButton
          label={applyLabel}
          icon={Upload}
          onClick={() => void apply()}
          disabled={locked}
          className="min-w-[136px]"
          menuLabel="Apply options"
          options={[
            {
              label: 'Apply',
              hint: 'Swap the live definition on this instance.',
              checked: !persist,
              onSelect: () => {
                setPersist(false)
                void apply(false)
              },
            },
            {
              label: 'Apply & save',
              hint: 'Also write it to the config, so it loads at the next boot.',
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
          title={`Chat with this agent beside the editor (${MOD}+Enter applies and runs)`}
        >
          Run
        </Button>
        <IconButton icon={BookOpen} label="Luau reference (F1)" onClick={() => toggleDock('docs')} aria-pressed={dock === 'docs'} className={dockToggle(dock === 'docs')} />
        <IconButton icon={Trash2} label="Delete" tone="danger" onClick={() => void remove()} disabled={locked || isNew} />
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

      <div className={cx('min-h-0 flex-1 overflow-y-auto', tab !== 'config' && 'hidden')}>
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
          />
        </div>
      )}

      {visited.instructions && (
        <div className={tab === 'instructions' ? PANEL : 'hidden'}>
          <EditorFrame
            title="Instructions"
            hint="Saved next to the loop as a prompt file; the definition points at it."
            actions={<CopyButton text={instructions} what="instructions" size="md" />}
          >
            <CodeEditor language="markdown" lineWrapping value={instructions} onChange={editInstructions} className="flex-1" />
          </EditorFrame>
        </div>
      )}

      {visited.yaml && (
        <div className={tab === 'yaml' ? PANEL : 'hidden'}>
          <EditorFrame
            title="Raw YAML"
            hint="Sent to the runtime. The loop and instructions sources above are sent separately; editing here overrides the form until you rebuild."
            actions={
              <>
                {rawDirty && (
                  <Chip tone="warning" title="Apply sends this text as written; the Config tab is ignored until you rebuild.">
                    overrides the form
                  </Chip>
                )}
                <Button size="sm" icon={RefreshCw} onClick={rebuild}>
                  Rebuild from form
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
