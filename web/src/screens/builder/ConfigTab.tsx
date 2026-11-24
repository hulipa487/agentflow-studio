// The Config tab: the definition as a form. Every change goes through
// `patch`, which is what marks the builder dirty.
import { Button, Checkbox, Field, Input, Section, Select, Textarea, type Option } from '@/components/ui'
import type { Catalog } from '@/api/types'
import { t } from '@/i18n'
import {
  AGENT_NAME_RE,
  DEFAULT_MODEL_LABEL_KEY,
  NAME_INVALID,
  effectiveCapabilities,
  extrasError,
  historyValid,
  matchingTemplateSettings,
  memoryOptions,
  parseExtrasObject,
  safetyOptions,
  settingDescription,
  settingLabel,
  toggleCheck,
  updateExtrasSetting,
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
          label={known.includes(label) ? label : <span title={t('builder.notInCatalogTitle')}>{t('builder.notInCatalog', { name: label })}</span>}
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
  const modelOptions: Option[] = [{ value: '', label: t(DEFAULT_MODEL_LABEL_KEY) }, ...models]
  // A model or profile the definition names but the catalog no longer lists
  // stays selectable, so opening an agent never silently rewrites it.
  if (form.model !== '' && !models.includes(form.model)) {
    modelOptions.push({ value: form.model, label: t('builder.notInCatalog', { name: form.model }) })
  }
  const profiles = safetyOptions(catalog)
  const safetyChoices: Option[] = [...profiles]
  if (form.safety !== '' && !profiles.includes(form.safety)) {
    safetyChoices.push({ value: form.safety, label: t('builder.notInCatalog', { name: form.safety }) })
  }
  const nameBad = isNew && form.name.trim() !== '' && !AGENT_NAME_RE.test(form.name.trim())
  const historyBad = !historyValid(form.history)
  const extrasBad = extrasError(form.extras)
  const extrasObj = parseExtrasObject(form.extras)
  const starterSettings = matchingTemplateSettings(catalog, extrasObj)
  const caps = effectiveCapabilities(form.capabilities, catalog)
  const memoryNeedsCap = form.memory !== '' && caps.length > 0 && !caps.includes('memory')

  return (
    <div className="flex flex-col gap-3 pb-6 pt-3">
      <Section title={t('builder.identity')} subtitle={t('builder.identitySubtitle')}>
        <div className="grid grid-cols-2 gap-3">
          <Field label={t('common.name')} error={nameBad ? t(NAME_INVALID) : undefined}>
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
          <Field label={t('builder.model')}>
            <Select value={form.model} onChange={(model) => patch({ model })} options={modelOptions} />
          </Field>
          <Field label={t('builder.safetyProfile')}>
            <Select
              value={form.safety}
              onChange={(safety) => patch({ safety })}
              options={safetyChoices}
              placeholder={form.safety === '' ? t('builder.selectOne') : undefined}
            />
          </Field>
          <Field label={t('builder.historyBudget')} error={historyBad ? t('builder.historyInvalid') : undefined}>
            <Input mono inputMode="numeric" value={form.history} invalid={historyBad} onChange={(e) => patch({ history: e.target.value })} />
          </Field>
          <Field
            label={t('builder.memoryProfile')}
            error={memoryNeedsCap ? t('builder.memoryNeedsCap') : undefined}
            hint={t('builder.memoryHint')}
          >
            <Select value={form.memory} onChange={(memory) => patch({ memory })} options={memoryOptions(catalog, form.memory)} />
          </Field>
        </div>
      </Section>

      {starterSettings.length > 0 && (
        <Section title={t('builder.starterSettings')} subtitle={t('builder.starterSettingsSubtitle')}>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {starterSettings.map((setting) => {
              const rawVal = extrasObj && setting.key in extrasObj ? extrasObj[setting.key] : setting.default
              const label = settingLabel(setting)
              const description = settingDescription(setting)
              if (setting.type === 'bool') {
                return (
                  <div key={setting.key} className="sm:col-span-2">
                    <Checkbox
                      checked={Boolean(rawVal)}
                      onChange={(v) => patch({ extras: updateExtrasSetting(form.extras, setting.key, v) })}
                      label={label}
                      hint={description}
                    />
                  </div>
                )
              }
              if (setting.type === 'number') {
                const numStr = rawVal === undefined || rawVal === null ? '' : String(rawVal)
                return (
                  <Field key={setting.key} label={label} hint={description}>
                    <Input
                      type="number"
                      mono
                      min={setting.min}
                      max={setting.max}
                      value={numStr}
                      onChange={(e) => {
                        const n = Number(e.target.value)
                        if (e.target.value !== '' && Number.isFinite(n)) {
                          patch({ extras: updateExtrasSetting(form.extras, setting.key, n) })
                        }
                      }}
                    />
                  </Field>
                )
              }
              if (setting.type === 'select') {
                return (
                  <Field key={setting.key} label={label} hint={description}>
                    <Select
                      value={rawVal === undefined || rawVal === null ? '' : String(rawVal)}
                      options={setting.options ?? []}
                      onChange={(v) => patch({ extras: updateExtrasSetting(form.extras, setting.key, v) })}
                    />
                  </Field>
                )
              }
              return (
                <Field key={setting.key} label={label} hint={description}>
                  <Input
                    mono
                    value={rawVal === undefined || rawVal === null ? '' : String(rawVal)}
                    placeholder={setting.placeholder}
                    onChange={(e) => patch({ extras: updateExtrasSetting(form.extras, setting.key, e.target.value) })}
                  />
                </Field>
              )
            })}
          </div>
        </Section>
      )}

      <Section
        title={t('builder.capabilities')}
        subtitle={t('builder.capabilitiesSubtitle')}
        action={
          <Button size="sm" variant="ghost" disabled={form.capabilities.length === 0} onClick={() => patch({ capabilities: [] })}>
            {t('builder.useDefaults')}
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

      <Section title={t('builder.tools')} subtitle={t('builder.toolsSubtitle')}>
        {skillLabels.length === 0 ? (
          <p className="text-xs text-muted">{t('builder.noTools')}</p>
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

      <Section title={t('builder.advanced')} subtitle={t('builder.advancedSubtitle')}>
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-2 gap-3">
            <Field label={t('builder.canContact')}>
              <Input mono value={form.canContact} placeholder="pm, researcher" onChange={(e) => patch({ canContact: e.target.value })} />
            </Field>
            <Field label={t('builder.credentials')}>
              <Input mono value={form.credentials} placeholder="openai, stripe" onChange={(e) => patch({ credentials: e.target.value })} />
            </Field>
            <Field label={t('builder.shellProfile')}>
              <Input mono value={form.shell} placeholder={t('builder.noneParen')} onChange={(e) => patch({ shell: e.target.value })} />
            </Field>
            <Field label={t('builder.sshProfile')}>
              <Input mono value={form.ssh} placeholder={t('builder.noneParen')} onChange={(e) => patch({ ssh: e.target.value })} />
            </Field>
            <div className="col-span-2 pt-1">
              <Checkbox checked={form.persistent} onChange={(persistent) => patch({ persistent })} label={t('builder.persistent')} />
            </div>
          </div>
          <Field label={t('builder.extras')} error={extrasBad || undefined}>
            <Textarea mono rows={3} value={form.extras} invalid={extrasBad !== ''} onChange={(e) => patch({ extras: e.target.value })} />
          </Field>
        </div>
      </Section>
    </div>
  )
}
