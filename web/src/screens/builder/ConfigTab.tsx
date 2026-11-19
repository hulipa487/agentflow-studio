// The Config tab: the definition as a form. Every change goes through
// `patch`, which is what marks the builder dirty.
import { Button, Checkbox, Field, Input, Section, Select, Textarea, type Option } from '@/components/ui'
import type { Catalog } from '@/api/types'
import {
  AGENT_NAME_RE,
  DEFAULT_MODEL_OPTION,
  NAME_INVALID,
  effectiveCapabilities,
  extrasError,
  historyValid,
  memoryOptions,
  safetyOptions,
  toggleCheck,
  type BuilderForm,
} from './model'

function CheckGrid({
  labels,
  selected,
  known,
  onToggle,
}: {
  labels: string[]
  selected: string[]
  /** The catalog's entries; anything else is flagged as unknown to this runtime. */
  known: string[]
  onToggle: (label: string, checked: boolean) => void
}) {
  return (
    <div className="grid grid-cols-2 gap-x-3 gap-y-1">
      {labels.map((label) => (
        <Checkbox
          key={label}
          mono
          checked={selected.includes(label)}
          onChange={(checked) => onToggle(label, checked)}
          label={known.includes(label) ? label : <span title="Not in this runtime's catalog">{label} · not in catalog</span>}
        />
      ))}
    </div>
  )
}

export function ConfigTab({
  form,
  patch,
  catalog,
  isNew,
  capLabels,
  skillLabels,
}: {
  form: BuilderForm
  patch: (p: Partial<BuilderForm>) => void
  catalog: Catalog
  isNew: boolean
  capLabels: string[]
  skillLabels: string[]
}) {
  const models = catalog.models ?? []
  const modelOptions: Option[] = [{ value: '', label: DEFAULT_MODEL_OPTION }, ...models]
  // A model or profile the definition names but the catalog no longer lists
  // stays selectable, so opening an agent never silently rewrites it.
  if (form.model !== '' && !models.includes(form.model)) {
    modelOptions.push({ value: form.model, label: `${form.model} · not in catalog` })
  }
  const profiles = safetyOptions(catalog)
  const safetyChoices: Option[] = [...profiles]
  if (form.safety !== '' && !profiles.includes(form.safety)) {
    safetyChoices.push({ value: form.safety, label: `${form.safety} · not in catalog` })
  }
  const nameBad = isNew && form.name.trim() !== '' && !AGENT_NAME_RE.test(form.name.trim())
  const historyBad = !historyValid(form.history)
  const extrasBad = extrasError(form.extras)
  const caps = effectiveCapabilities(form.capabilities, catalog)
  const memoryNeedsCap = form.memory !== '' && caps.length > 0 && !caps.includes('memory')

  return (
    <div className="flex flex-col gap-3 pb-6 pt-3">
      <Section title="Identity" subtitle="Model, safety, memory and context budget.">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name" error={nameBad ? NAME_INVALID : undefined}>
            <Input
              mono
              value={form.name}
              placeholder="researcher"
              disabled={!isNew}
              invalid={nameBad}
              autoFocus={isNew}
              onChange={(e) => patch({ name: e.target.value })}
            />
          </Field>
          <Field label="Model">
            <Select value={form.model} onChange={(model) => patch({ model })} options={modelOptions} />
          </Field>
          <Field label="Safety profile">
            <Select
              value={form.safety}
              onChange={(safety) => patch({ safety })}
              options={safetyChoices}
              placeholder={form.safety === '' ? '(Select one)' : undefined}
            />
          </Field>
          <Field label="History budget" error={historyBad ? 'Not a positive whole number: the key is left out of the definition.' : undefined}>
            <Input mono inputMode="numeric" value={form.history} invalid={historyBad} onChange={(e) => patch({ history: e.target.value })} />
          </Field>
          <Field
            label="Memory profile"
            error={memoryNeedsCap ? 'Memory needs the memory capability, which this agent does not have.' : undefined}
            hint="The stores memory.write and memory.recall use. Profiles are set up on the Memory screen; a new one can be picked after a restart."
          >
            <Select value={form.memory} onChange={(memory) => patch({ memory })} options={memoryOptions(catalog, form.memory)} />
          </Field>
        </div>
      </Section>

      <Section
        title="Capabilities"
        subtitle="Empty uses the deployment defaults."
        action={
          <Button size="sm" variant="ghost" disabled={form.capabilities.length === 0} onClick={() => patch({ capabilities: [] })}>
            Use defaults
          </Button>
        }
      >
        <CheckGrid
          labels={capLabels}
          selected={form.capabilities}
          known={catalog.capabilities ?? []}
          onToggle={(label, checked) => patch({ capabilities: toggleCheck(capLabels, form.capabilities, label, checked) })}
        />
      </Section>

      <Section title="Tools (skills)" subtitle="Leave empty to expose every tool allowed by the deployment policy.">
        {skillLabels.length === 0 ? (
          <p className="text-xs text-muted">No tools are registered in this deployment.</p>
        ) : (
          <div className={skillLabels.length > 8 ? 'max-h-[180px] overflow-y-auto pr-1' : undefined}>
            <CheckGrid
              labels={skillLabels}
              selected={form.skills}
              known={catalog.tools ?? []}
              onToggle={(label, checked) => patch({ skills: toggleCheck(skillLabels, form.skills, label, checked) })}
            />
          </div>
        )}
      </Section>

      <Section title="Advanced" subtitle="Runtime, fleet contact and credentials.">
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Can contact">
              <Input mono value={form.canContact} placeholder="pm, researcher" onChange={(e) => patch({ canContact: e.target.value })} />
            </Field>
            <Field label="Credentials">
              <Input mono value={form.credentials} placeholder="openai, stripe" onChange={(e) => patch({ credentials: e.target.value })} />
            </Field>
            <Field label="Shell profile">
              <Input mono value={form.shell} placeholder="(none)" onChange={(e) => patch({ shell: e.target.value })} />
            </Field>
            <div className="flex items-end pb-1.5">
              <Checkbox checked={form.persistent} onChange={(persistent) => patch({ persistent })} label="Daemon (persistent)" />
            </div>
          </div>
          <Field label="Extras (JSON)" error={extrasBad || undefined}>
            <Textarea mono rows={3} value={form.extras} invalid={extrasBad !== ''} onChange={(e) => patch({ extras: e.target.value })} />
          </Field>
        </div>
      </Section>
    </div>
  )
}
