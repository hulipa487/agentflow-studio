// The backend editor: a provider and the keys that provider reads. The URL and
// API key are write only, like a channel token: the runtime never sends them
// back, an empty field keeps what is stored, and "clear" removes it.
import { useState } from 'react'
import type { Client } from '@/api/client'
import type { MemoryBackend, MemoryList } from '@/api/types'
import { Checkbox, Field, Input, Modal, Select } from '@/components/ui'
import { t } from '@/i18n'
import { tr } from '@/i18n/rich'
import { EditorFooter, EditorNotice, useEditorActions } from '../safety/parts'
import { QDRANT_DISTANCES, backendForm, buildBackendDraft, providerHas, providerHint, type BackendForm } from './helpers'

function storedSecret(has: boolean, ref: string | undefined, display: string | undefined): string {
  if (!has) return t('memory.backend.storedNothing')
  if (ref) return t('memory.backend.storedRef', { ref })
  if (display) return t('memory.backend.storedDisplay', { display })
  return t('memory.backend.storedValue')
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
    validText: () => t('memory.backend.validText'),
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
      title={
        existing === null
          ? t('memory.backend.newTitle')
          : existing.implicit
            ? t('memory.backend.writeOutTitle')
            : t('memory.editWhat', { name: existing.name })
      }
      subtitle={t('memory.backend.subtitle')}
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label={t('common.name')} hint={t('memory.backend.nameHint')}>
            <Input mono value={form.name} onChange={(e) => set('name', e.target.value)} placeholder="vectors" disabled={existing !== null} autoFocus={existing === null} />
          </Field>
          <Field label={t('memory.backend.provider')} hint={providerChanged ? t('memory.backend.providerChanged') : undefined}>
            <Select mono value={form.provider} onChange={(v) => set('provider', v)} options={list.providers ?? []} />
          </Field>
        </div>
        <p className="-mt-2 text-xs text-muted">{providerHint(form.provider)}</p>

        {has('path') && (
          <Field label={t('memory.backend.path')} hint={t('memory.backend.pathHint')}>
            <Input mono value={form.path} onChange={(e) => set('path', e.target.value)} placeholder="./data/memory.db" />
          </Field>
        )}
        {has('url') && (
          <Field
            label={t('memory.backend.url')}
            hint={t('memory.backend.urlHint', {
              stored: existing && !providerChanged ? storedSecret(existing.has_url, existing.url_ref, existing.url_display) : t('memory.backend.urlRequired'),
            })}
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
          <Field
            label={t('memory.backend.apiKey')}
            hint={existing && !providerChanged ? storedSecret(existing.has_api_key, existing.api_key_ref, undefined) : t('memory.backend.optional')}
          >
            <Input mono type="password" autoComplete="off" value={form.apiKey} onChange={(e) => set('apiKey', e.target.value)} placeholder="cred:qdrant" />
            {existing?.has_api_key && !providerChanged && (
              <Checkbox className="mt-1" checked={form.clearApiKey} onChange={(v) => set('clearApiKey', v)} label={t('memory.backend.clearApiKey')} />
            )}
          </Field>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          {has('collection') && (
            <Field label={t('memory.backend.collection')} hint={t('memory.backend.collectionHint')}>
              <Input mono value={form.collection} onChange={(e) => set('collection', e.target.value)} placeholder="agentflow" />
            </Field>
          )}
          {has('distance') && (
            <Field label={t('memory.backend.distance')}>
              <Select mono value={form.distance} onChange={(v) => set('distance', v)} placeholder={t('memory.backend.distanceDefault')} options={QDRANT_DISTANCES} />
            </Field>
          )}
          {has('timeout') && (
            <Field label={t('memory.backend.timeout')} hint={t('memory.backend.timeoutHint')}>
              <Input mono value={form.timeout} onChange={(e) => set('timeout', e.target.value)} placeholder="5s" />
            </Field>
          )}
          {has('dim') && (
            <Field label={t('memory.backend.dimensions')} hint={t('memory.backend.dimensionsHint')}>
              <Input mono inputMode="numeric" value={form.dim} onChange={(e) => set('dim', e.target.value)} placeholder="1536" />
            </Field>
          )}
          {has('vector_scan_limit') && (
            <Field label={t('memory.backend.vectorScanLimit')} hint={t('memory.backend.vectorScanLimitHint')}>
              <Input mono inputMode="numeric" value={form.vectorScanLimit} onChange={(e) => set('vectorScanLimit', e.target.value)} placeholder="50000" />
            </Field>
          )}
          {has('max_entries') && (
            <Field label={t('memory.backend.maxEntries')} hint={t('memory.backend.maxEntriesHint')}>
              <Input mono inputMode="numeric" value={form.maxEntries} onChange={(e) => set('maxEntries', e.target.value)} placeholder="10000" />
            </Field>
          )}
        </div>
        {(existing?.other_keys ?? []).length > 0 && (
          <p className="text-xs text-faint">
            {tr('memory.backend.otherKeys', { keys: (existing?.other_keys ?? []).join(', ') }, { code: (c) => <span className="font-mono">{c}</span> })}
          </p>
        )}
      </div>
    </Modal>
  )
}
