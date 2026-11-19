// The memory profile editor: the logical stores an agent reads and writes,
// each on a backend, and how memory.recall finds records in them.
import { useState } from 'react'
import { Plus, X } from 'lucide-react'
import type { Client } from '@/api/client'
import type { MemoryList, MemoryProfile } from '@/api/types'
import { Button, Checkbox, ComboInput, Field, IconButton, Input, Modal, Select } from '@/components/ui'
import { EditorFooter, EditorNotice, useEditorActions } from '../safety/parts'
import { STORE_SCOPES, buildProfileDraft, profileForm, storeRow, type ProfileForm, type StoreRow } from './helpers'

export function ProfileEditor({
  client,
  list,
  existing,
  onClose,
  onSaved,
  reload,
}: {
  client: Client
  list: MemoryList
  /** Null creates a profile. */
  existing: MemoryProfile | null
  onClose: () => void
  onSaved: (list: MemoryList) => void
  reload: () => Promise<void>
}) {
  const creating = existing === null
  const backends = (list.backends ?? []).map((b) => b.name)
  const [form, setForm] = useState<ProfileForm>(() => profileForm(existing, backends))
  const set = <K extends keyof ProfileForm>(key: K, value: ProfileForm[K]) => setForm((f) => ({ ...f, [key]: value }))
  const setStore = (i: number, patch: Partial<StoreRow>) =>
    setForm((f) => ({ ...f, stores: f.stores.map((s, at) => (at === i ? { ...s, ...patch } : s)) }))

  const actions = useEditorActions<ReturnType<typeof buildProfileDraft>, MemoryList>({
    build: () => buildProfileDraft(form),
    validate: (d) => client.validateMemory({ kind: 'profile', creating, profile: d }),
    save: (d) => client.saveMemoryProfile(d.name, creating, d, list.mtime),
    validText: () => 'Valid. Save it, then pick it for an agent in the builder after the next restart.',
    onSaved,
    reload,
    what: 'memory',
  })
  const busy = actions.busy !== ''
  const semantic = form.recall === 'plugin:semantic'
  const requires = list.requires ?? ['kv', 'text_search', 'vector']

  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!busy}
      width={760}
      title={creating ? 'New memory profile' : 'Edit ' + existing.name}
      subtitle="Agents name a profile with memory: <name>. The built-in plugins look for stores called dialogue, facts and semantic."
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <Field label="Name" hint='Agents refer to the profile by name, so it cannot change later. "conversational" is the built-in one.'>
          <Input mono value={form.name} onChange={(e) => set('name', e.target.value)} placeholder="group_chat" disabled={!creating} autoFocus={creating} />
        </Field>

        <Field label="Stores" hint="Each logical store is a table on a backend. Retention is a duration such as 30d, or forever.">
          <div className="flex flex-col gap-2">
            {form.stores.length > 0 && (
              <div className="hidden grid-cols-[1fr_1fr_1fr_90px_80px_28px] gap-2 px-2.5 text-xs font-medium text-faint sm:grid" aria-hidden>
                <span>store</span>
                <span>backend</span>
                <span>table</span>
                <span>retention</span>
                <span>window</span>
                <span />
              </div>
            )}
            {form.stores.map((s, i) => (
              <div key={i} className="rounded-ctl border border-line bg-bg p-2.5">
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-[1fr_1fr_1fr_90px_80px_28px]">
                  <Input mono value={s.name} onChange={(e) => setStore(i, { name: e.target.value })} placeholder="store" aria-label="Store name" />
                  <Select mono value={s.backend} onChange={(v) => setStore(i, { backend: v })} placeholder="backend…" options={backends} aria-label="Backend" />
                  <Input mono value={s.table} onChange={(e) => setStore(i, { table: e.target.value })} placeholder="table" aria-label="Table" />
                  <Input mono value={s.retention} onChange={(e) => setStore(i, { retention: e.target.value })} placeholder="retention" aria-label="Retention" />
                  <Input mono inputMode="numeric" value={s.window} onChange={(e) => setStore(i, { window: e.target.value })} placeholder="window" aria-label="Window" title="How many recent records recall returns" />
                  <IconButton icon={X} tone="danger" label={`Remove store ${s.name || i + 1}`} onClick={() => set('stores', form.stores.filter((_, at) => at !== i))} />
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1.5">
                  {requires.map((r) => (
                    <Checkbox
                      key={r}
                      mono
                      checked={s.requires.includes(r)}
                      onChange={(on) => setStore(i, { requires: on ? [...s.requires, r] : s.requires.filter((x) => x !== r) })}
                      label={r}
                    />
                  ))}
                  <Select
                    className="w-[170px]"
                    value={s.scope}
                    onChange={(v) => setStore(i, { scope: v })}
                    options={STORE_SCOPES}
                    disabled={s.pool.trim() !== ''}
                    aria-label="Scope"
                    title="Rows are kept per user by default; whole agent shares one set across users"
                  />
                  <Input
                    mono
                    className="w-[150px]"
                    value={s.pool}
                    onChange={(e) => setStore(i, { pool: e.target.value })}
                    placeholder="shared pool"
                    aria-label="Pool"
                    title="A named shared pool from Tenancy; replaces the scope"
                  />
                  <Checkbox checked={s.localityOk} onChange={(v) => setStore(i, { localityOk: v })} label="instance-local is fine" />
                </div>
              </div>
            ))}
            <div>
              <Button size="sm" icon={Plus} onClick={() => set('stores', [...form.stores, storeRow(null, backends[0] ?? '')])}>
                Add store
              </Button>
            </div>
          </div>
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Recall" hint="plugin:recency reads dialogue and facts; plugin:semantic searches the semantic store by meaning.">
            <ComboInput mono value={form.recall} onChange={(e) => set('recall', e.target.value)} options={list.recall_plugins ?? []} placeholder="plugin:recency" />
          </Field>
          <Field label="Write" hint="Comma-separated write plugins, in order.">
            <ComboInput
              mono
              value={form.write.join(', ')}
              onChange={(e) => set('write', [e.target.value])}
              options={list.write_plugins ?? []}
              placeholder="plugin:routing_table"
            />
          </Field>
          <Field label="Embed model" hint={semantic ? 'Required for semantic recall: it embeds each record on write and each query.' : 'Embeds each record on write, for semantic search.'}>
            <Select mono value={form.embedModel} onChange={(v) => set('embedModel', v)} placeholder="none" options={list.models ?? []} />
          </Field>
          <Field label="Rerank model" hint="Optional: oversample vector hits and let a provider: rerank model order them.">
            <Select mono value={form.rerankModel} onChange={(v) => set('rerankModel', v)} placeholder="none" options={list.rerank_models ?? []} />
          </Field>
          {form.rerankModel !== '' && (
            <Field label="Oversample" hint="How many times k vector hits to rerank. Default 4.">
              <Input mono inputMode="numeric" value={form.oversample} onChange={(e) => set('oversample', e.target.value)} placeholder="4" />
            </Field>
          )}
        </div>
      </div>
    </Modal>
  )
}
