// Models: the deployment's LLM endpoints. Edits apply to the running instance
// at once; "Save to config" writes the live set to the config file and
// "Revert to config" throws the live edits away.
import { useState, type ReactNode } from 'react'
import { Pencil, Play, Plus, RotateCw, Save, Trash2, Undo2 } from 'lucide-react'
import type { Model, ModelTest } from '@/api/types'
import { AsyncView, Banner, Button, Chip, Chips, EmptyState, IconButton, Page, Panel, StatusChip } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { plural } from '@/lib/format'
import { useClient, useSession } from '@/state/session'
import { confirm, info, toast, toastError } from '@/state/ui'
import { useAlive } from './channels/useAlive'
import { ModelEditor } from './models/ModelEditor'
import { driftSummary, fileOnly, modelState, testFailure } from './models/model'

interface ModelsData {
  models: Model[]
  fileError: string
  /** The deployment tier; a regional fleet applies a hot edit to one instance only. */
  topology: string
}

// Fixed and fractional tracks only: every row is its own grid, so a
// content-sized track would stop the columns lining up.
const GRID = 'grid grid-cols-[minmax(170px,1fr)_minmax(200px,1.5fr)_minmax(160px,1.2fr)_150px_124px_96px] items-center gap-3'

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1">
      <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-faint">{label}</span>
      <span className="truncate font-mono text-[11.5px] text-muted">{children}</span>
    </span>
  )
}

function ModelSettings({ m }: { m: Model }) {
  const tools = m.server_tools ?? []
  const plain = !m.timeout && !(m.retry > 0) && !(m.max_tokens > 0) && !m.thinking && tools.length === 0
  if (plain) return <span className="font-mono text-[10.5px] text-faint">provider defaults</span>
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
      {m.timeout && <Fact label="timeout">{m.timeout}</Fact>}
      {m.retry > 0 && <Fact label="retry">{m.retry}</Fact>}
      {m.max_tokens > 0 && <Fact label="max tokens">{m.max_tokens}</Fact>}
      {m.thinking && <Fact label="thinking">{m.thinking}</Fact>}
      {tools.length > 0 && <Chips items={tools} max={2} />}
    </div>
  )
}

export default function Models() {
  const client = useClient()
  const alive = useAlive()
  const heartbeatTopology = useSession((s) => s.runtime?.topology ?? '')
  const [editing, setEditing] = useState<{ existing: Model | null } | null>(null)
  const [testing, setTesting] = useState<ReadonlySet<string>>(new Set())
  const [results, setResults] = useState<Record<string, ModelTest>>({})
  const [busy, setBusy] = useState<'persist' | 'revert' | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)
  // One write to the model set at a time, so each one's reload shows its own result.
  const writing = busy !== null || deleting !== null

  const { data, error, loading, refreshing, reload } = useAsync<ModelsData>(
    async (signal) => {
      // The topology only decides a warning, so a failed state call must not
      // fail the screen; the heartbeat's last snapshot stands in below.
      const [list, topology] = await Promise.all([
        client.models(signal),
        client.state(signal).then(
          (st) => st.topology ?? '',
          () => '',
        ),
      ])
      return { models: list.models ?? [], fileError: list.file_error ?? '', topology }
    },
    [client],
  )
  const refresh = () => reload()
  useScreenActions({ refresh })

  const models = data?.models ?? []
  const drift = driftSummary(models)
  const regional = (data?.topology || heartbeatTopology) === 'regional'

  /** Re-reads the list and the catalog the builder's model pickers use. */
  const changed = () => {
    reload()
    void useSession.getState().reloadCatalog()
  }
  /** Drops a test result that no longer describes the model. */
  const forget = (name: string) => setResults((r) => Object.fromEntries(Object.entries(r).filter(([k]) => k !== name)))

  const test = async (name: string) => {
    if (testing.has(name)) return
    setTesting((s) => new Set(s).add(name))
    try {
      const res = await client.testModel(name)
      if (!alive.current) return
      setResults((r) => ({ ...r, [name]: res }))
      if (res.ok) toast(`${name} ok · ${res.latency_ms}ms`, 'success')
      else void info({ title: 'Model test failed', message: testFailure(res) })
    } catch (err) {
      if (alive.current) toastError(err)
    } finally {
      if (alive.current) {
        setTesting((s) => {
          const next = new Set(s)
          next.delete(name)
          return next
        })
      }
    }
  }

  const remove = async (m: Model) => {
    if (fileOnly(m)) {
      void info({
        title: 'Model is only in the config',
        message: 'This model is not live on the running instance. Save the live model set to config to remove it, or revert to config to load it.',
      })
      return
    }
    let message = `Remove model "${m.name}" from the running instance?`
    if (m.in_file) message += '\n\nIt stays in the config file until you save changes to the config.'
    if (!(await confirm({ title: 'Delete model', message, confirmLabel: 'Delete', danger: true })) || !alive.current) return
    setDeleting(m.name)
    try {
      await client.deleteModel(m.name)
      if (!alive.current) return
      forget(m.name)
      toast('model removed · live', 'warning')
      changed()
    } catch (err) {
      if (alive.current) toastError(err)
    } finally {
      if (alive.current) setDeleting(null)
    }
  }

  const persist = async () => {
    if (writing) return
    setBusy('persist')
    try {
      await client.persistModels()
      if (!alive.current) return
      toast('models saved to config', 'success')
      changed()
    } catch (err) {
      if (alive.current) toastError(err)
    } finally {
      if (alive.current) setBusy(null)
    }
  }

  const revert = async () => {
    if (writing) return
    const ok = await confirm({
      title: 'Revert model changes?',
      message: "Discard live model changes and reload the deployment config's models?",
      confirmLabel: 'Revert',
      danger: true,
    })
    if (!ok || !alive.current) return
    setBusy('revert')
    try {
      await client.revertModels()
      if (!alive.current) return
      setResults({})
      toast('models reverted to config', 'success')
      changed()
    } catch (err) {
      if (alive.current) toastError(err)
    } finally {
      if (alive.current) setBusy(null)
    }
  }

  const banners: ReactNode[] = []
  if (data?.fileError) {
    banners.push(
      <Banner key="file" tone="warning">
        config read failed: {data.fileError}
      </Banner>,
    )
  }

  return (
    <Page
      title="Models"
      subtitle="LLM endpoints available to agents. Changes apply live; save them to the deployment config to keep them across restarts."
      meta={
        data ? (
          <>
            <StatusChip tone={drift.count > 0 ? 'warning' : 'success'}>{drift.text}</StatusChip>
            {data.fileError && <span className="text-warning">config read failed</span>}
            {regional && <span className="text-warning">regional fleet: edits apply to this instance only; save and roll the fleet</span>}
          </>
        ) : null
      }
      banner={banners.length > 0 ? banners : null}
      actions={
        <>
          {data && <span className="font-mono text-[11px] text-faint">{plural(models.length, 'model')}</span>}
          {/* Only drift gives these two something to do. */}
          {drift.count > 0 && (
            <>
              <Button icon={Save} onClick={persist} loading={busy === 'persist'} disabled={writing}>
                Save to config
              </Button>
              <Button icon={Undo2} onClick={revert} loading={busy === 'revert'} disabled={writing}>
                Revert to config
              </Button>
            </>
          )}
          <IconButton icon={RotateCw} label="Refresh" onClick={refresh} loading={refreshing} />
          {/* While the list is empty the empty state carries this action. */}
          {models.length > 0 && (
            <Button variant="primary" icon={Plus} onClick={() => setEditing({ existing: null })}>
              New model
            </Button>
          )}
        </>
      }
    >
      <AsyncView loading={loading} error={error} hasData={!!data} retry={refresh} loadingText="Loading models…">
        {models.length === 0 ? (
          <EmptyState
            title="No models yet"
            guidance="Add an OpenAI, Anthropic or Gemini endpoint to give agents a model."
            actionLabel="Add a model"
            onAction={() => setEditing({ existing: null })}
          />
        ) : (
          <Panel flush className="overflow-x-auto">
            <div className="min-w-[990px]">
              <div className={`${GRID} border-b border-line px-3 py-1.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-faint`}>
                <span>Model</span>
                <span>Endpoint</span>
                <span>Settings</span>
                <span>API key</span>
                <span>State</span>
                <span />
              </div>
              {models.map((m) => {
                const state = modelState(m)
                const result = results[m.name]
                return (
                  <div key={m.name} className={`${GRID} border-b border-line/70 px-3 py-2 last:border-b-0`}>
                    <div className="flex min-w-0 items-center gap-2">
                      <span className="truncate text-[13px] font-semibold" title={m.name}>
                        {m.name}
                      </span>
                      <Chip tone="accent">{m.provider}</Chip>
                    </div>
                    <div className="min-w-0">
                      <div className="truncate font-mono text-xs" title={m.model}>
                        {m.model}
                      </div>
                      <div className="truncate font-mono text-[11px] text-muted" title={m.base_url}>
                        {m.base_url || <span className="text-faint">provider default</span>}
                      </div>
                    </div>
                    <ModelSettings m={m} />
                    <div className="min-w-0">
                      {m.has_key ? (
                        <Chip tone="success">configured{m.key_fingerprint ? ' · ' + m.key_fingerprint : ''}</Chip>
                      ) : (
                        <Chip tone="warning">not set</Chip>
                      )}
                    </div>
                    <div className="flex min-w-0 flex-col items-start gap-1">
                      <span title={state.title}>
                        <StatusChip tone={state.tone}>{state.label}</StatusChip>
                      </span>
                      {result && (
                        <Chip tone={result.ok ? 'success' : 'danger'} title={result.ok ? 'Last test on this screen' : testFailure(result)}>
                          {result.ok ? `ok · ${result.latency_ms}ms` : `test failed · ${result.class || 'error'}`}
                        </Chip>
                      )}
                    </div>
                    <div className="flex items-center justify-end">
                      <IconButton icon={Play} label={`Test ${m.name}`} loading={testing.has(m.name)} onClick={() => test(m.name)} />
                      <IconButton icon={Pencil} label={`Edit ${m.name}`} onClick={() => setEditing({ existing: m })} />
                      <IconButton icon={Trash2} label={`Delete ${m.name}`} tone="danger" loading={deleting === m.name} disabled={writing} onClick={() => remove(m)} />
                    </div>
                  </div>
                )
              })}
            </div>
          </Panel>
        )}
      </AsyncView>
      {editing && (
        <ModelEditor
          // A fresh form per model: reopening never shows the last one's text.
          key={editing.existing?.name ?? ''}
          existing={editing.existing}
          onClose={() => setEditing(null)}
          onSaved={(name) => {
            setEditing(null)
            forget(name)
            toast(
              regional ? `model ${name} applied to this instance only · save to config and roll the fleet` : `model ${name} applied · save to config to keep`,
              'warning',
            )
            changed()
          }}
        />
      )}
    </Page>
  )
}
