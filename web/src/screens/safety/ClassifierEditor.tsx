// The classifier editor: every knob of one model-backed policy. Blank optional
// fields are left out of the draft, so the runtime's defaults apply.
import { useState } from 'react'
import type { Client } from '@/api/client'
import type { SafetyClassifier, SafetyList } from '@/api/types'
import { Checkbox, Field, Input, Modal, Select, Textarea } from '@/components/ui'
import { t } from '@/i18n'
import {
  FAIL_ACTIONS,
  MODES,
  SOURCES,
  buildClassifierDraft,
  classifierForm,
  failActionLabel,
  modeLabel,
  sourceLabel,
  withCurrent,
  withCurrentOptions,
  type ClassifierForm,
} from './helpers'
import { EditorFooter, EditorNotice, useEditorActions } from './parts'

export function ClassifierEditor({
  client,
  list,
  existing,
  onClose,
  onSaved,
  reload,
}: {
  client: Client
  /** The current safety list: the models and prompts on offer and the mtime a save sends. */
  list: SafetyList
  /** Null creates a classifier. */
  existing: SafetyClassifier | null
  onClose: () => void
  onSaved: (list: SafetyList) => void
  reload: () => Promise<void>
}) {
  const creating = existing === null
  const [form, setForm] = useState<ClassifierForm>(() => classifierForm(existing, list.models ?? []))
  const set = <K extends keyof ClassifierForm>(key: K, value: ClassifierForm[K]) => setForm((f) => ({ ...f, [key]: value }))
  const text = (key: { [K in keyof ClassifierForm]: ClassifierForm[K] extends string ? K : never }[keyof ClassifierForm]) => ({
    value: form[key],
    onChange: (e: { target: { value: string } }) => set(key, e.target.value),
  })

  // A model or prompt the config no longer offers stays selectable, so
  // opening and saving an entry never silently changes it.
  const models = withCurrent(list.models ?? [], existing?.model ?? '')
  const prompts = withCurrent(list.prompts ?? [], existing?.prompt ?? '')

  const actions = useEditorActions({
    build: () => buildClassifierDraft(form),
    validate: (d) => client.validateSafety({ kind: 'classifier', creating, classifier: d }),
    save: (d) => client.saveSafetyClassifier(d.name, creating, d, list.mtime),
    validText: (d) => t('safety.classifier.validText', { name: d.name }),
    onSaved,
    reload,
  })
  const busy = actions.busy !== ''

  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!busy}
      width={760}
      title={creating ? t('safety.classifier.newTitle') : t('safety.editWhat', { name: existing.name })}
      subtitle={t('safety.classifier.subtitle')}
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label={t('common.name')}>
            <Input mono {...text('name')} placeholder="moderation" disabled={!creating} autoFocus={creating} />
          </Field>
          <Field
            label={t('safety.classifier.model')}
            hint={models.length === 0 ? t('safety.classifier.noModels') : undefined}
          >
            <Select
              mono
              value={form.model}
              onChange={(v) => set('model', v)}
              options={models}
              placeholder={form.model === '' ? t('safety.classifier.chooseModel') : undefined}
            />
          </Field>
          <Field label={t('safety.classifier.mode')}>
            <Select value={form.mode} onChange={(v) => set('mode', v)} options={withCurrentOptions(MODES, form.mode, modeLabel)} />
          </Field>
          <Field label={t('safety.classifier.prompt')}>
            <Select mono value={form.prompt} onChange={(v) => set('prompt', v)} options={prompts} placeholder={t('safety.classifier.embeddedDefault')} />
          </Field>
        </div>

        <Field label={t('safety.classifier.directions')}>
          <div className="flex flex-wrap gap-x-5 gap-y-1.5">
            <Checkbox checked={form.ingress} onChange={(v) => set('ingress', v)} label={t('safety.classifier.ingress')} />
            <Checkbox checked={form.egress} onChange={(v) => set('egress', v)} label={t('safety.classifier.egress')} />
          </div>
        </Field>
        <Field label={t('safety.classifier.sources')}>
          <div className="flex flex-wrap gap-x-5 gap-y-1.5">
            {SOURCES.map((s) => (
              <Checkbox key={s} checked={form.sources[s]} onChange={(v) => set('sources', { ...form.sources, [s]: v })} label={sourceLabel(s)} />
            ))}
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <Field label={t('safety.classifier.threshold')}>
            <Input mono {...text('threshold')} placeholder={t('safety.classifier.thresholdPlaceholder')} inputMode="decimal" />
          </Field>
          <Field label={t('safety.classifier.timeout')}>
            <Input mono {...text('timeout')} placeholder="2s" />
          </Field>
          <Field label={t('safety.classifier.onError')}>
            <Select value={form.onError} onChange={(v) => set('onError', v)} options={withCurrentOptions(FAIL_ACTIONS, form.onError, failActionLabel)} />
          </Field>
          <Field label={t('safety.classifier.onSaturated')}>
            <Select value={form.onSaturated} onChange={(v) => set('onSaturated', v)} options={withCurrentOptions(FAIL_ACTIONS, form.onSaturated, failActionLabel)} />
          </Field>
          <Field label={t('safety.classifier.onBudget')}>
            <Select value={form.onBudget} onChange={(v) => set('onBudget', v)} options={withCurrentOptions(FAIL_ACTIONS, form.onBudget, failActionLabel)} />
          </Field>
          <Field label={t('safety.classifier.maxTokens')}>
            <Input mono {...text('maxTokens')} placeholder="64" inputMode="numeric" />
          </Field>
          <Field label={t('safety.classifier.maxInputChars')}>
            <Input mono {...text('maxInputChars')} placeholder="8000" inputMode="numeric" />
          </Field>
          <Field label={t('safety.classifier.maxConcurrency')}>
            <Input mono {...text('maxConcurrency')} placeholder="8" inputMode="numeric" />
          </Field>
          <Field label={t('safety.classifier.cacheSize')}>
            <Input mono {...text('cacheSize')} placeholder="10000" inputMode="numeric" />
          </Field>
          <Field label={t('safety.classifier.cacheTTL')}>
            <Input mono {...text('cacheTTL')} placeholder="5m" />
          </Field>
          <Field label={t('safety.classifier.budget')}>
            <Input mono {...text('budget')} placeholder="500000" inputMode="numeric" />
          </Field>
        </div>

        <Field label={t('safety.classifier.categories')} hint={t('safety.classifier.categoriesHint')}>
          <Textarea mono rows={4} {...text('categories')} placeholder={'safe: allow\nself_harm: block\nharassment: review\nlegal: redirect:strong-model'} />
        </Field>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
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
