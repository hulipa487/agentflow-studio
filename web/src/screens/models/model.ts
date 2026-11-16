// Model helpers shared by the list and the editor: the state chip's rules,
// the form model and the draft the API takes. The rules mirror the desktop
// app's models.go so both clients agree.
import type { Model, ModelDraft, ModelTest } from '@/api/types'
import { splitList } from '@/lib/format'
import type { Tone } from '@/state/ui'

/** The runtime's own rule for a model name: path- and YAML-safe. */
const MODEL_NAME = /^[A-Za-z0-9_.:-]{1,64}$/

export const MODEL_PROVIDERS = ['openai', 'openai-responses', 'anthropic', 'gemini', 'rerank']

/** The empty level means "provider default" and is offered as the placeholder option. */
export const THINKING_LEVELS = ['off', 'low', 'medium', 'high', 'xhigh', 'max']

export interface ModelState {
  tone: Tone
  label: string
  /** The longer explanation, shown as a tooltip. */
  title: string
}

/** Where a model stands between the running instance and the config file. */
export function modelState(m: Model): ModelState {
  if (m.in_runtime && !m.in_file) {
    return { tone: 'warning', label: 'runtime only', title: 'Live on this instance but not in the config file: save to config to keep it across restarts.' }
  }
  if (!m.in_runtime && m.in_file) {
    return { tone: 'warning', label: 'file only', title: 'In the config file but not loaded on the running instance.' }
  }
  if (m.drift) {
    return { tone: 'warning', label: 'unsaved', title: 'The live model differs from the config file.' }
  }
  return { tone: 'success', label: 'saved', title: 'The live model matches the config file.' }
}

/** True when the model exists only in the config file, so nothing live can be removed or tested. */
export function fileOnly(m: Model): boolean {
  return !m.in_runtime && m.in_file
}

export function driftSummary(models: Model[]): { count: number; text: string } {
  const count = models.filter((m) => m.drift).length
  let text = 'all changes saved to config'
  if (count === 1) text = '1 unsaved model change'
  else if (count > 1) text = `${count} unsaved model changes`
  return { count, text }
}

/** A failed test as "class: message", the way the runtime classifies it. */
export function testFailure(res: ModelTest): string {
  const msg = res.error ?? ''
  return res.class ? `${res.class}: ${msg}` : msg
}

export interface ModelForm {
  name: string
  provider: string
  model: string
  apiKey: string
  baseURL: string
  timeout: string
  retry: string
  maxTokens: string
  thinking: string
  serverTools: string
}

export function formFor(existing: Model | null): ModelForm {
  if (!existing) {
    return { name: '', provider: 'openai', model: '', apiKey: '', baseURL: '', timeout: '', retry: '', maxTokens: '', thinking: '', serverTools: '' }
  }
  return {
    name: existing.name,
    provider: existing.provider || 'openai',
    model: existing.model ?? '',
    // Write-only: the stored key never comes back, and blank keeps it.
    apiKey: '',
    baseURL: existing.base_url ?? '',
    timeout: existing.timeout ?? '',
    retry: existing.retry > 0 ? String(existing.retry) : '',
    maxTokens: existing.max_tokens > 0 ? String(existing.max_tokens) : '',
    thinking: existing.thinking ?? '',
    serverTools: (existing.server_tools ?? []).join(', '),
  }
}

const INTEGER = /^[+-]?\d+$/

function count(text: string): number | null {
  const s = text.trim()
  if (s === '') return 0
  const n = Number(s)
  return INTEGER.test(s) && Number.isSafeInteger(n) && n >= 0 ? n : null
}

export type DraftResult = { name: string; draft: ModelDraft; error?: undefined } | { name?: undefined; draft?: undefined; error: string }

/** Checks the form and builds the draft; the runtime validates the rest. */
export function buildDraft(f: ModelForm): DraftResult {
  const name = f.name.trim()
  if (!MODEL_NAME.test(name)) return { error: 'name must be 1-64 of letters, digits, _ . : -' }
  if (f.provider === '') return { error: 'choose a provider' }
  const model = f.model.trim()
  if (model === '') return { error: 'model is required' }
  const retry = count(f.retry)
  if (retry === null) return { error: 'retry must be a non-negative number' }
  const maxTokens = count(f.maxTokens)
  if (maxTokens === null) return { error: 'max tokens must be a non-negative number' }
  return {
    name,
    draft: {
      provider: f.provider,
      model,
      api_key: f.apiKey.trim(),
      base_url: f.baseURL.trim(),
      timeout: f.timeout.trim(),
      retry,
      max_tokens: maxTokens,
      thinking: f.thinking,
      server_tools: splitList(f.serverTools),
    },
  }
}
