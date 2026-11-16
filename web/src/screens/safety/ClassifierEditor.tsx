// The classifier editor: every knob of one model-backed policy. Blank optional
// fields are left out of the draft, so the runtime's defaults apply.
import { useState } from 'react'
import type { Client } from '@/api/client'
import type { SafetyClassifier, SafetyList } from '@/api/types'
import { Checkbox, Field, Input, Modal, Select, Textarea } from '@/components/ui'
import { FAIL_ACTIONS, MODES, SOURCES, buildClassifierDraft, classifierForm, withCurrent, type ClassifierForm } from './helpers'
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
    validText: (d) => `Valid. Save it, then reference classifier:${d.name} from a profile.`,
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
      title={creating ? 'New classifier' : 'Edit ' + existing.name}
      subtitle="Classifier spend is metered under __moderation_<name>; refusals and fallbacks are rate-limited per session."
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label="Name">
            <Input mono {...text('name')} placeholder="moderation" disabled={!creating} autoFocus={creating} />
          </Field>
          <Field
            label="Model"
            hint={
              models.length === 0
                ? 'No engine-keyed classifier models are configured. Add one on the Models screen and save it to config first.'
                : undefined
            }
          >
            <Select mono value={form.model} onChange={(v) => set('model', v)} options={models} placeholder={form.model === '' ? 'choose a model' : undefined} />
          </Field>
          <Field label="Mode">
            <Select value={form.mode} onChange={(v) => set('mode', v)} options={withCurrent(MODES, form.mode)} />
          </Field>
          <Field label="Prompt">
            <Select mono value={form.prompt} onChange={(v) => set('prompt', v)} options={prompts} placeholder="embedded default" />
          </Field>
        </div>

        <Field label="Directions">
          <div className="flex flex-wrap gap-x-5 gap-y-1.5">
            <Checkbox checked={form.ingress} onChange={(v) => set('ingress', v)} label="Ingress (inbound messages)" />
            <Checkbox checked={form.egress} onChange={(v) => set('egress', v)} label="Egress (outbound replies)" />
          </div>
        </Field>
        <Field label="Sources">
          <div className="flex flex-wrap gap-x-5 gap-y-1.5">
            {SOURCES.map((s) => (
              <Checkbox key={s} checked={form.sources[s]} onChange={(v) => set('sources', { ...form.sources, [s]: v })} label={s} />
            ))}
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <Field label="Threshold">
            <Input mono {...text('threshold')} placeholder="0.7 (blank = runtime default)" inputMode="decimal" />
          </Field>
          <Field label="Timeout">
            <Input mono {...text('timeout')} placeholder="2s" />
          </Field>
          <Field label="On error">
            <Select value={form.onError} onChange={(v) => set('onError', v)} options={withCurrent(FAIL_ACTIONS, form.onError)} />
          </Field>
          <Field label="On saturated">
            <Select value={form.onSaturated} onChange={(v) => set('onSaturated', v)} options={withCurrent(FAIL_ACTIONS, form.onSaturated)} />
          </Field>
          <Field label="On budget exhausted">
            <Select value={form.onBudget} onChange={(v) => set('onBudget', v)} options={withCurrent(FAIL_ACTIONS, form.onBudget)} />
          </Field>
          <Field label="Max tokens">
            <Input mono {...text('maxTokens')} placeholder="64" inputMode="numeric" />
          </Field>
          <Field label="Max input chars">
            <Input mono {...text('maxInputChars')} placeholder="8000" inputMode="numeric" />
          </Field>
          <Field label="Max concurrency">
            <Input mono {...text('maxConcurrency')} placeholder="8" inputMode="numeric" />
          </Field>
          <Field label="Cache size">
            <Input mono {...text('cacheSize')} placeholder="10000" inputMode="numeric" />
          </Field>
          <Field label="Cache TTL">
            <Input mono {...text('cacheTTL')} placeholder="5m" />
          </Field>
          <Field label="Daily token budget">
            <Input mono {...text('budget')} placeholder="500000" inputMode="numeric" />
          </Field>
        </div>

        <Field label="Categories" hint="One per line, label: allow|review|block. At least one allow label is required.">
          <Textarea mono rows={4} {...text('categories')} placeholder={'safe: allow\nself_harm: block\nharassment: review'} />
        </Field>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label="Refusal (ingress block)">
            <Input {...text('refusal')} placeholder="optional ingress refusal text" />
          </Field>
          <Field label="Fallback (egress block)">
            <Input {...text('fallback')} placeholder="optional egress fallback text" />
          </Field>
        </div>
      </div>
    </Modal>
  )
}
