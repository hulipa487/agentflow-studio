// The memory profile editor: the logical stores an agent reads and writes,
// each on a backend, and how memory.recall finds records in them.
import { useState } from 'react'
import { Plus, X } from 'lucide-react'
import type { Client } from '@/api/client'
import type { MemoryList, MemoryProfile } from '@/api/types'
import { Button, Checkbox, ComboInput, Field, IconButton, Input, Modal, Select } from '@/components/ui'
import { t } from '@/i18n'
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
    validText: () => t('memory.profile.validText'),
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
      title={creating ? t('memory.profile.newTitle') : t('memory.editWhat', { name: existing.name })}
      subtitle={t('memory.profile.subtitle')}
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <Field label={t('common.name')} hint={t('memory.profile.nameHint')}>
          <Input mono value={form.name} onChange={(e) => set('name', e.target.value)} placeholder="group_chat" disabled={!creating} autoFocus={creating} />
        </Field>

        <Field label={t('memory.profile.stores')} hint={t('memory.profile.storesHint')}>
          <div className="flex flex-col gap-2">
            {form.stores.length > 0 && (
              <div className="hidden grid-cols-[1fr_1fr_1fr_90px_80px_28px] gap-2 px-2.5 text-xs font-medium text-faint sm:grid" aria-hidden>
                <span>{t('memory.col.store')}</span>
                <span>{t('memory.col.backend')}</span>
                <span>{t('memory.col.table')}</span>
                <span>{t('memory.col.retention')}</span>
                <span>{t('memory.col.window')}</span>
                <span />
              </div>
            )}
            {form.stores.map((s, i) => (
              <div key={i} className="rounded-ctl border border-line bg-bg p-2.5">
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-[1fr_1fr_1fr_90px_80px_28px]">
                  <Input mono value={s.name} onChange={(e) => setStore(i, { name: e.target.value })} placeholder={t('memory.col.store')} aria-label={t('memory.store.nameAria')} />
                  <Select mono value={s.backend} onChange={(v) => setStore(i, { backend: v })} placeholder={t('memory.store.backendPick')} options={backends} aria-label={t('memory.store.backendAria')} />
                  <Input mono value={s.table} onChange={(e) => setStore(i, { table: e.target.value })} placeholder={t('memory.col.table')} aria-label={t('memory.store.tableAria')} />
                  <Input mono value={s.retention} onChange={(e) => setStore(i, { retention: e.target.value })} placeholder={t('memory.col.retention')} aria-label={t('memory.store.retentionAria')} />
                  <Input
                    mono
                    inputMode="numeric"
                    value={s.window}
                    onChange={(e) => setStore(i, { window: e.target.value })}
                    placeholder={t('memory.col.window')}
                    aria-label={t('memory.store.windowAria')}
                    title={t('memory.store.windowTitle')}
                  />
                  <IconButton
                    icon={X}
                    tone="danger"
                    label={t('memory.store.removeNamed', { name: s.name || i + 1 })}
                    onClick={() => set('stores', form.stores.filter((_, at) => at !== i))}
                  />
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
                    options={STORE_SCOPES.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                    disabled={s.pool.trim() !== ''}
                    aria-label={t('memory.store.scopeAria')}
                    title={t('memory.store.scopeTitle')}
                  />
                  <Input
                    mono
                    className="w-[150px]"
                    value={s.pool}
                    onChange={(e) => setStore(i, { pool: e.target.value })}
                    placeholder={t('memory.store.poolPlaceholder')}
                    aria-label={t('memory.store.poolAria')}
                    title={t('memory.store.poolTitle')}
                  />
                  <Checkbox checked={s.localityOk} onChange={(v) => setStore(i, { localityOk: v })} label={t('memory.store.localityOk')} />
                </div>
              </div>
            ))}
            <div>
              <Button size="sm" icon={Plus} onClick={() => set('stores', [...form.stores, storeRow(null, backends[0] ?? '')])}>
                {t('memory.store.add')}
              </Button>
            </div>
          </div>
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label={t('memory.profile.recall')} hint={t('memory.profile.recallHint')}>
            <ComboInput mono value={form.recall} onChange={(e) => set('recall', e.target.value)} options={list.recall_plugins ?? []} placeholder="plugin:recency" />
          </Field>
          <Field label={t('memory.profile.write')} hint={t('memory.profile.writeHint')}>
            <ComboInput
              mono
              value={form.write.join(', ')}
              onChange={(e) => set('write', [e.target.value])}
              options={list.write_plugins ?? []}
              placeholder="plugin:routing_table"
            />
          </Field>
          <Field label={t('memory.profile.embedModel')} hint={semantic ? t('memory.profile.embedHintSemantic') : t('memory.profile.embedHint')}>
            <Select mono value={form.embedModel} onChange={(v) => set('embedModel', v)} placeholder={t('common.none')} options={list.models ?? []} />
          </Field>
          <Field label={t('memory.profile.rerankModel')} hint={t('memory.profile.rerankHint')}>
            <Select mono value={form.rerankModel} onChange={(v) => set('rerankModel', v)} placeholder={t('common.none')} options={list.rerank_models ?? []} />
          </Field>
          {form.rerankModel !== '' && (
            <Field label={t('memory.profile.oversample')} hint={t('memory.profile.oversampleHint')}>
              <Input mono inputMode="numeric" value={form.oversample} onChange={(e) => set('oversample', e.target.value)} placeholder="4" />
            </Field>
          )}
        </div>
      </div>
    </Modal>
  )
}
