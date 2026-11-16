// The add / edit dialog for one LLM endpoint. Saving applies the model to the
// running instance at once; the API key is write-only.
import { useState } from 'react'
import { Play } from 'lucide-react'
import { errorMessage } from '@/api/client'
import type { Model } from '@/api/types'
import { Banner, Button, Field, Input, Modal, Select, type Tone } from '@/components/ui'
import { useClient } from '@/state/session'
import { useAlive } from '../channels/useAlive'
import { MODEL_PROVIDERS, THINKING_LEVELS, buildDraft, fileOnly, formFor, testFailure, type ModelForm } from './model'

/** Keeps a stored value selectable even when the form would not offer it. */
function withCurrent(options: string[], current: string): string[] {
  return current === '' || options.includes(current) ? options : [...options, current]
}

export function ModelEditor({
  existing,
  onClose,
  onSaved,
}: {
  /** null adds a model. */
  existing: Model | null
  onClose: () => void
  onSaved: (name: string) => void
}) {
  const client = useClient()
  const alive = useAlive()
  const [form, setForm] = useState<ModelForm>(() => formFor(existing))
  const [notice, setNotice] = useState<{ tone: Tone; text: string } | null>(null)
  const [busy, setBusy] = useState<'save' | 'test' | null>(null)

  const text = (key: keyof ModelForm) => ({
    value: form[key],
    onChange: (e: { target: { value: string } }) => setForm((f) => ({ ...f, [key]: e.target.value })),
  })

  // The runtime keeps the key of a model it is running. One that only the
  // config file holds has no live key to keep, so blank would save it keyless.
  const unloaded = existing !== null && fileOnly(existing)

  const save = async () => {
    if (busy) return
    const r = buildDraft(form)
    if (r.error !== undefined) {
      setNotice({ tone: 'danger', text: r.error })
      return
    }
    setBusy('save')
    try {
      await client.putModel(r.name, r.draft)
      if (alive.current) onSaved(r.name)
    } catch (err) {
      if (!alive.current) return
      setNotice({ tone: 'danger', text: errorMessage(err) })
      setBusy(null)
    }
  }

  // Exercises the model as the instance runs it now, not the form's edits;
  // the answer stays in this dialog instead of stacking a second one on it.
  const test = async () => {
    if (busy || !existing) return
    setBusy('test')
    setNotice(null)
    try {
      const res = await client.testModel(existing.name)
      if (!alive.current) return
      setNotice(res.ok ? { tone: 'success', text: `${existing.name} ok · ${res.latency_ms}ms` } : { tone: 'danger', text: 'Model test failed\n' + testFailure(res) })
    } catch (err) {
      if (alive.current) setNotice({ tone: 'danger', text: errorMessage(err) })
    } finally {
      if (alive.current) setBusy(null)
    }
  }

  return (
    <Modal
      open
      // Stays open while a request is in flight; the answer decides what happens next.
      onClose={() => {
        if (!busy) onClose()
      }}
      title={existing ? 'Edit ' + existing.name : 'New model'}
      subtitle="Secrets are write-only: leave the API key blank to keep the stored value."
      width={640}
      footer={
        <>
          {existing && (
            <Button className="mr-auto" icon={Play} onClick={test} loading={busy === 'test'} disabled={busy !== null}>
              Test live
            </Button>
          )}
          <Button onClick={onClose} disabled={busy !== null}>
            Cancel
          </Button>
          <Button variant="primary" onClick={save} loading={busy === 'save'} disabled={busy !== null}>
            {busy === 'save' ? 'Saving…' : 'Save'}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {notice && <Banner tone={notice.tone}>{notice.text}</Banner>}
        {unloaded && (
          <Banner tone="warning">
            This model is only in the config file. Saving loads it on the running instance without the stored key, so enter the API key again, or use Revert to
            config to load it as the file has it.
          </Banner>
        )}
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label="Name">
            <Input mono placeholder="assistant-default" disabled={existing !== null} {...text('name')} />
          </Field>
          <Field label="Provider">
            <Select value={form.provider} onChange={(v) => setForm((f) => ({ ...f, provider: v }))} options={withCurrent(MODEL_PROVIDERS, form.provider)} />
          </Field>
          <Field label="Model">
            {/* When editing, the name is locked, so typing starts here. */}
            <Input mono placeholder="gpt-4o-mini" autoFocus={existing !== null} {...text('model')} />
          </Field>
          <Field label="API key">
            {/* new-password keeps a password manager from filling in an unrelated login. */}
            <Input
              type="password"
              autoComplete="new-password"
              mono
              placeholder={existing && !unloaded ? 'leave blank to keep the stored key' : 'sk-…'}
              {...text('apiKey')}
            />
          </Field>
          <Field label="Base URL">
            <Input mono placeholder="optional" {...text('baseURL')} />
          </Field>
          <Field label="Timeout">
            <Input mono placeholder="60s" {...text('timeout')} />
          </Field>
          <Field label="Retry">
            <Input mono inputMode="numeric" placeholder="0" {...text('retry')} />
          </Field>
          <Field label="Max tokens">
            <Input mono inputMode="numeric" placeholder="0" {...text('maxTokens')} />
          </Field>
          <Field label="Thinking">
            <Select
              value={form.thinking}
              onChange={(v) => setForm((f) => ({ ...f, thinking: v }))}
              options={withCurrent(THINKING_LEVELS, form.thinking)}
              placeholder="provider default"
            />
          </Field>
          <Field label="Server tools">
            <Input mono placeholder="web_search, google_search" {...text('serverTools')} />
          </Field>
        </div>
      </div>
    </Modal>
  )
}
