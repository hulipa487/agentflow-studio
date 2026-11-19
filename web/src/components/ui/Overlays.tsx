// The app-wide overlay host: toasts in the corner and the confirm / prompt /
// info dialogs opened through state/ui.ts.
import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useUI, type ConfirmOptions, type PromptOptions } from '@/state/ui'
import { t } from '@/i18n'
import { tr } from '@/i18n/rich'
import { Banner, Button, ComboInput, Field, Input, Modal, Textarea, cx, toneBg, toneText } from './index'

function ConfirmDialog({ opts, resolve }: { opts: ConfirmOptions; resolve: (ok: boolean) => void }) {
  const [typed, setTyped] = useState('')
  // Trimmed, so an id pasted with a stray space or newline still matches.
  const blocked = opts.typeToConfirm !== undefined && typed.trim() !== opts.typeToConfirm
  return (
    <Modal
      open
      onClose={() => resolve(false)}
      title={opts.title}
      width={460}
      footer={
        <>
          <Button onClick={() => resolve(false)}>{opts.cancelLabel ?? t('common.cancel')}</Button>
          <Button variant={opts.danger ? 'danger' : 'primary'} disabled={blocked} onClick={() => resolve(true)} autoFocus={opts.typeToConfirm === undefined}>
            {opts.confirmLabel ?? t('common.confirm')}
          </Button>
        </>
      }
    >
      <div className="whitespace-pre-wrap text-base leading-relaxed">{opts.message}</div>
      {opts.typeToConfirm !== undefined && (
        <Field label={tr('common.typeToConfirm', { text: opts.typeToConfirm }, { code: (c) => <code className="text-text">{c}</code> })} className="mt-4">
          <Input
            mono
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !blocked) resolve(true)
            }}
          />
        </Field>
      )}
    </Modal>
  )
}

function PromptDialog({ opts, resolve }: { opts: PromptOptions; resolve: (v: string | null) => void }) {
  const [value, setValue] = useState(opts.initial ?? '')
  const [error, setError] = useState('')
  const submit = () => {
    const v = value.trim()
    const problem = opts.validate?.(v)
    if (problem) {
      setError(problem)
      return
    }
    resolve(v)
  }
  return (
    <Modal
      open
      onClose={() => resolve(null)}
      title={opts.title}
      subtitle={opts.subtitle}
      width={520}
      footer={
        <>
          <Button onClick={() => resolve(null)}>{t('common.cancel')}</Button>
          <Button variant="primary" onClick={submit}>
            {opts.okLabel ?? t('common.ok')}
          </Button>
        </>
      }
    >
      <Field label={opts.label}>
        {opts.multiline ? (
          <Textarea autoFocus rows={6} value={value} placeholder={opts.placeholder} onChange={(e) => { setValue(e.target.value); setError('') }} />
        ) : opts.options ? (
          <ComboInput
            autoFocus
            mono
            options={opts.options}
            value={value}
            placeholder={opts.placeholder}
            onChange={(e) => { setValue(e.target.value); setError('') }}
            onKeyDown={(e) => { if (e.key === 'Enter') submit() }}
          />
        ) : (
          <Input
            autoFocus
            value={value}
            placeholder={opts.placeholder}
            onChange={(e) => { setValue(e.target.value); setError('') }}
            onKeyDown={(e) => { if (e.key === 'Enter') submit() }}
          />
        )}
      </Field>
      {error && <Banner tone="danger" className="mt-3">{error}</Banner>}
    </Modal>
  )
}

export function Overlays() {
  const toasts = useUI((s) => s.toasts)
  const dismiss = useUI((s) => s.dismissToast)
  const dialog = useUI((s) => s.dialog)

  // A dialog left open across a route change would answer a question nobody
  // is asking any more. The router's location (not the raw hash) is watched:
  // a navigation that is being held for a confirmation has not happened yet.
  const location = useLocation()
  const here = location.pathname + location.search
  const last = useRef(here)
  useEffect(() => {
    if (last.current === here) return
    last.current = here
    const d = useUI.getState().dialog
    if (!d) return
    if (d.kind === 'confirm') d.resolve(false)
    else if (d.kind === 'prompt') d.resolve(null)
    else d.resolve()
  }, [here])

  return (
    <>
      {dialog?.kind === 'confirm' && <ConfirmDialog opts={dialog.opts} resolve={dialog.resolve} />}
      {dialog?.kind === 'prompt' && <PromptDialog opts={dialog.opts} resolve={dialog.resolve} />}
      {dialog?.kind === 'info' && (
        <Modal
          open
          onClose={dialog.resolve}
          title={dialog.opts.title}
          width={480}
          footer={
            <Button variant="primary" onClick={dialog.resolve} autoFocus>
              {t('common.done')}
            </Button>
          }
        >
          <div className="whitespace-pre-wrap text-base leading-relaxed">{dialog.opts.message}</div>
        </Modal>
      )}
      <div className="pointer-events-none fixed bottom-9 right-4 z-[60] flex w-[min(420px,calc(100vw-2rem))] flex-col items-end gap-2">
        {toasts.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => dismiss(item.id)}
            className={cx(
              'pointer-events-auto max-w-full animate-pop-in rounded-ctl border border-line px-3 py-2 text-left text-xs shadow-float',
              toneBg(item.tone),
              item.tone === 'neutral' ? 'text-text' : toneText(item.tone),
            )}
          >
            {item.text}
          </button>
        ))}
      </div>
    </>
  )
}
