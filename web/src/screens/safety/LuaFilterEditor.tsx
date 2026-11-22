// Custom Lua 5.1 safety filter editor: sandboxed ingress/egress hooks plus
// execution caps (timeout, instruction budget) and on_error fallback policy.
import { useState } from 'react'
import { FileCode2 } from 'lucide-react'
import type { Client } from '@/api/client'
import type { SafetyList, SafetyLuaFilter } from '@/api/types'
import { Button, Checkbox, Field, Input, Modal, Select } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { t } from '@/i18n'
import {
  LUA_FAIL_ACTIONS,
  LUA_FILTER_STARTER,
  SOURCES,
  buildLuaFilterDraft,
  failActionLabel,
  luaFilterForm,
  sourceLabel,
  withCurrentOptions,
  type LuaFilterForm,
} from './helpers'
import { EditorFooter, EditorNotice, useEditorActions } from './parts'

export function LuaFilterEditor({
  client,
  list,
  existing,
  onClose,
  onSaved,
  reload,
}: {
  client: Client
  list: SafetyList
  existing: SafetyLuaFilter | null
  onClose: () => void
  onSaved: (list: SafetyList) => void
  reload: () => Promise<void>
}) {
  const creating = existing === null
  const [form, setForm] = useState<LuaFilterForm>(() => luaFilterForm(existing))
  const set = <K extends keyof LuaFilterForm>(key: K, value: LuaFilterForm[K]) => setForm((f) => ({ ...f, [key]: value }))
  const text = (key: { [K in keyof LuaFilterForm]: LuaFilterForm[K] extends string ? K : never }[keyof LuaFilterForm]) => ({
    value: form[key],
    onChange: (e: { target: { value: string } }) => set(key, e.target.value),
  })

  const actions = useEditorActions({
    build: () => buildLuaFilterDraft(form),
    validate: (d) => client.validateSafety({ kind: 'lua_filter', creating, lua_filter: d }),
    save: (d) => client.saveSafetyLuaFilter(d.name, creating, d, list.mtime),
    validText: (d) => t('safety.lua.validText', { name: d.name }),
    onSaved,
    reload,
  })
  const busy = actions.busy !== ''

  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!busy}
      width={820}
      title={creating ? t('safety.lua.newTitle') : t('safety.editWhat', { name: existing.name })}
      subtitle={t('safety.lua.subtitle')}
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label={t('common.name')} hint={t('safety.lua.nameHint')}>
            <Input mono {...text('name')} placeholder="pii_mask" disabled={!creating} autoFocus={creating} />
          </Field>
          <Field label={t('safety.lua.onError')}>
            <Select
              value={form.onError}
              onChange={(v) => set('onError', v)}
              options={withCurrentOptions(LUA_FAIL_ACTIONS, form.onError, failActionLabel)}
            />
          </Field>
        </div>

        <Field
          label={form.useFile ? t('safety.lua.filePath') : t('safety.lua.source')}
          hint={form.useFile ? t('safety.lua.fileHint') : t('safety.lua.sourceHint')}
        >
          <div className="mb-1.5 flex flex-wrap items-center gap-2">
            <Checkbox
              checked={form.useFile}
              onChange={(v) => set('useFile', v)}
              label={t('safety.lua.useFileToggle')}
            />
            {!form.useFile && (
              <Button size="sm" icon={FileCode2} onClick={() => set('source', LUA_FILTER_STARTER)}>
                {t('safety.lua.starterButton')}
              </Button>
            )}
          </div>
          {form.useFile ? (
            <Input mono {...text('file')} placeholder="./filters/pii_mask.lua" />
          ) : (
            <CodeEditor
              value={form.source}
              onChange={(v) => set('source', v)}
              language="lua"
              className="h-[250px]"
            />
          )}
        </Field>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label={t('safety.classifier.directions')}>
            <div className="flex flex-wrap gap-x-5 gap-y-1.5">
              <Checkbox checked={form.ingress} onChange={(v) => set('ingress', v)} label={t('safety.classifier.ingress')} />
              <Checkbox checked={form.egress} onChange={(v) => set('egress', v)} label={t('safety.classifier.egress')} />
            </div>
          </Field>
          <Field label={t('safety.classifier.sources')}>
            <div className="flex flex-wrap gap-x-4 gap-y-1.5">
              {SOURCES.map((s) => (
                <Checkbox
                  key={s}
                  checked={form.sources[s]}
                  onChange={(v) => set('sources', { ...form.sources, [s]: v })}
                  label={sourceLabel(s)}
                />
              ))}
            </div>
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label={t('safety.classifier.timeout')} hint={t('safety.lua.timeoutHint')}>
            <Input mono {...text('timeout')} placeholder="200ms" />
          </Field>
          <Field label={t('safety.lua.instrBudget')} hint={t('safety.lua.instrBudgetHint')}>
            <Input mono {...text('instrBudget')} placeholder="100000" inputMode="numeric" />
          </Field>
          <Field label={t('safety.classifier.refusal')}>
            <Input {...text('refusal')} placeholder={t('safety.classifier.refusalPlaceholder')} />
          </Field>
          <Field label={t('safety.classifier.fallback')}>
            <Input {...text('fallback')} placeholder={t('safety.classifier.fallbackPlaceholder')} />
          </Field>
        </div>
      </div>
    </Modal>
  )
}
