// The backend editor: a provider and the keys that provider reads. The URL and
// API key are write only, like a channel token: the runtime never sends them
// back, an empty field keeps what is stored, and "clear" removes it.
import { useState } from 'react'
import type { Client } from '@/api/client'
import type { MemoryBackend, MemoryList } from '@/api/types'
import { Checkbox, Field, Input, Modal, Select } from '@/components/ui'
import { EditorFooter, EditorNotice, useEditorActions } from '../safety/parts'
import { PROVIDER_HINTS, QDRANT_DISTANCES, backendForm, buildBackendDraft, providerHas, type BackendForm } from './helpers'

function storedSecret(has: boolean, ref: string | undefined, display: string | undefined): string {
  if (!has) return 'Nothing stored.'
  if (ref) return `Stored as the reference ${ref}.`
  if (display) return `Stored: ${display}. Leave empty to keep it.`
  return 'A value is stored. Leave empty to keep it.'
}

export function BackendEditor({
  client,
  list,
  existing,
  onClose,
  onSaved,
  reload,
}: {
  client: Client
  list: MemoryList
  /** Null creates a backend. */
  existing: MemoryBackend | null
  onClose: () => void
  onSaved: (list: MemoryList) => void
  reload: () => Promise<void>
}) {
  const creating = existing === null || existing.implicit
  const [form, setForm] = useState<BackendForm>(() => backendForm(existing))
  const set = <K extends keyof BackendForm>(key: K, value: BackendForm[K]) => setForm((f) => ({ ...f, [key]: value }))
  const has = (key: string) => providerHas(form.provider, key)
  const providerChanged = existing !== null && !existing.implicit && existing.provider !== form.provider

  const actions = useEditorActions<ReturnType<typeof buildBackendDraft>, MemoryList>({
    build: () => buildBackendDraft(form),
    validate: (d) => client.validateMemory({ kind: 'backend', creating, backend: d }),
    save: (d) => client.saveMemoryBackend(d.name, creating, d, list.mtime),
    validText: () => 'Valid. Save it to open the backend on the next runtime start.',
    onSaved,
    reload,
    what: 'memory',
  })
  const busy = actions.busy !== ''

  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!busy}
      width={600}
      title={existing === null ? 'New memory backend' : existing.implicit ? 'Write out main_db' : 'Edit ' + existing.name}
      subtitle="A backend is a database the memory stores live in. Backends open when agentflow starts."
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Name" hint="Stores refer to the backend by name, so it cannot change later.">
            <Input mono value={form.name} onChange={(e) => set('name', e.target.value)} placeholder="vectors" disabled={existing !== null} autoFocus={existing === null} />
          </Field>
          <Field label="Provider" hint={providerChanged ? 'Changing provider drops the settings the old one used.' : undefined}>
            <Select mono value={form.provider} onChange={(v) => set('provider', v)} options={list.providers ?? []} />
          </Field>
        </div>
        <p className="-mt-2 text-xs text-muted">{PROVIDER_HINTS[form.provider]}</p>

        {has('path') && (
          <Field label="Path" hint="A file path, relative to the config. Empty uses ./data/agentflow.db.">
            <Input mono value={form.path} onChange={(e) => set('path', e.target.value)} placeholder="./data/memory.db" />
          </Field>
        )}
        {has('url') && (
          <Field
            label="URL"
            hint={
              <>
                {existing && !providerChanged ? storedSecret(existing.has_url, existing.url_ref, existing.url_display) : 'Required.'} Use ${'{'}VAR{'}'} or cred:&lt;service&gt; to keep the
                secret out of the file.
              </>
            }
          >
            <Input
              mono
              type="password"
              autoComplete="off"
              value={form.url}
              onChange={(e) => set('url', e.target.value)}
              placeholder={form.provider === 'qdrant' ? 'http://localhost:6334' : form.provider.startsWith('redis') ? 'redis://localhost:6379/0' : '${PG_URL}'}
            />
          </Field>
        )}
        {has('api_key') && (
          <Field label="API key" hint={existing && !providerChanged ? storedSecret(existing.has_api_key, existing.api_key_ref, undefined) : 'Optional.'}>
            <Input mono type="password" autoComplete="off" value={form.apiKey} onChange={(e) => set('apiKey', e.target.value)} placeholder="cred:qdrant" />
            {existing?.has_api_key && !providerChanged && (
              <Checkbox className="mt-1" checked={form.clearApiKey} onChange={(v) => set('clearApiKey', v)} label="Remove the stored API key" />
            )}
          </Field>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          {has('collection') && (
            <Field label="Collection" hint="Empty uses the driver default.">
              <Input mono value={form.collection} onChange={(e) => set('collection', e.target.value)} placeholder="agentflow" />
            </Field>
          )}
          {has('distance') && (
            <Field label="Distance">
              <Select mono value={form.distance} onChange={(v) => set('distance', v)} placeholder="Cosine (default)" options={QDRANT_DISTANCES} />
            </Field>
          )}
          {has('timeout') && (
            <Field label="Timeout" hint="A duration, e.g. 5s.">
              <Input mono value={form.timeout} onChange={(e) => set('timeout', e.target.value)} placeholder="5s" />
            </Field>
          )}
          {has('dim') && (
            <Field label="Dimensions" hint="The embedding model's vector size.">
              <Input mono inputMode="numeric" value={form.dim} onChange={(e) => set('dim', e.target.value)} placeholder="1536" />
            </Field>
          )}
          {has('vector_scan_limit') && (
            <Field label="Vector scan limit" hint="Rows a vector search may scan. Default 50000.">
              <Input mono inputMode="numeric" value={form.vectorScanLimit} onChange={(e) => set('vectorScanLimit', e.target.value)} placeholder="50000" />
            </Field>
          )}
          {has('max_entries') && (
            <Field label="Max entries" hint="Bounds memory. Without it every store on this backend needs a retention.">
              <Input mono inputMode="numeric" value={form.maxEntries} onChange={(e) => set('maxEntries', e.target.value)} placeholder="10000" />
            </Field>
          )}
        </div>
        {(existing?.other_keys ?? []).length > 0 && (
          <p className="text-xs text-faint">
            Also in the file and kept as they are: <span className="font-mono">{(existing?.other_keys ?? []).join(', ')}</span>. Edit them in Config.
          </p>
        )}
      </div>
    </Modal>
  )
}
