// Pieces the Safety list and its two editors share: the chain drawn as the
// dispatcher will run it, the live/restart chip, and the validate/save
// plumbing of an editor dialog.
import { Fragment, useEffect, useRef, useState, type RefObject } from 'react'
import { Check, RotateCw, Upload } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { SafetyList } from '@/api/types'
import { Banner, Button, Chip, StatusChip, cx, type Tone } from '@/components/ui'
import { t } from '@/i18n'
import { chainText, emptyChain, isClassifierFilter, isScriptFilter } from './helpers'

/** True until the component unmounts, so a late answer is not applied. */
export function useMounted(): RefObject<boolean> {
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  return mounted
}

export function LiveChip({ live }: { live: boolean }) {
  return live ? <StatusChip tone="success">{t('safety.live')}</StatusChip> : <StatusChip tone="warning">{t('safety.restartRequired')}</StatusChip>
}

/** A filter chain, left to right; classifier and script filters stand out from builtins. */
export function ChainView({ filters, className }: { filters: string[]; className?: string }) {
  if (filters.length === 0) return <span className={cx('font-mono text-sm text-faint', className)}>{emptyChain()}</span>
  return (
    <span className={cx('flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1', className)} title={chainText(filters)}>
      {filters.map((f, i) => (
        <Fragment key={`${i}:${f}`}>
          {i > 0 && (
            <span className="font-mono text-xs text-faint" aria-hidden>
              →
            </span>
          )}
          <Chip tone={isClassifierFilter(f) ? 'accent' : isScriptFilter(f) ? 'success' : 'neutral'}>{f}</Chip>
        </Fragment>
      ))}
    </span>
  )
}

interface Notice {
  tone: Tone
  text: string
  /** The config changed on disk (409): offer to reload without losing the draft. */
  conflict?: boolean
}

type Busy = '' | 'validate' | 'save' | 'reload'

/**
 * The validate / save flow both editors run. `build` throws with the message
 * to show when the form cannot become a draft; nothing is sent in that case.
 */
export function useEditorActions<D, L = SafetyList>(opts: {
  build: () => D
  validate: (draft: D) => Promise<string[]>
  save: (draft: D) => Promise<L>
  /** Shown when the runtime finds nothing wrong. */
  validText: (draft: D) => string
  onSaved: (list: L) => void
  /** Reloads the list (and with it the mtime the next save sends). */
  reload: () => Promise<void>
  /** What the editor edits, for the reload notice: 'memory' or 'safety' (the default). */
  what?: 'memory' | 'safety'
}) {
  const mounted = useMounted()
  const [busy, setBusy] = useState<Busy>('')
  const [notice, setNotice] = useState<Notice | null>(null)

  const draft = (): D | null => {
    try {
      return opts.build()
    } catch (err) {
      setNotice({ tone: 'danger', text: errorMessage(err) })
      return null
    }
  }

  const run = async (what: Busy, work: () => Promise<void>) => {
    if (busy !== '') return
    setBusy(what)
    try {
      await work()
    } catch (err) {
      if (mounted.current) {
        setNotice({ tone: 'danger', text: errorMessage(err), conflict: err instanceof ApiError && err.status === 409 })
      }
    } finally {
      if (mounted.current) setBusy('')
    }
  }

  const validate = () => {
    const d = draft()
    if (d === null) return
    void run('validate', async () => {
      const errs = await opts.validate(d)
      if (!mounted.current) return
      setNotice(errs.length > 0 ? { tone: 'danger', text: errs.join('\n') } : { tone: 'success', text: opts.validText(d) })
    })
  }

  const save = () => {
    const d = draft()
    if (d === null) return
    void run('save', async () => {
      const list = await opts.save(d)
      if (mounted.current) opts.onSaved(list)
    })
  }

  const reload = () =>
    void run('reload', async () => {
      await opts.reload()
      if (mounted.current) {
        setNotice({ tone: 'warning', text: t('safety.reloadedNotice', { what: opts.what ?? 'safety' }) })
      }
    })

  return { busy, notice, validate, save, reload }
}

export type EditorActions = ReturnType<typeof useEditorActions>

/** The editor's banner: local errors, the runtime's verdict, or a conflict. */
export function EditorNotice({ actions }: { actions: Pick<EditorActions, 'notice' | 'busy' | 'reload'> }) {
  const { notice, busy, reload } = actions
  if (!notice) return null
  return (
    <Banner tone={notice.tone} className="mb-3">
      {notice.text}
      {notice.conflict && (
        <div className="mt-2">
          <Button size="sm" icon={RotateCw} loading={busy === 'reload'} disabled={busy !== ''} onClick={reload}>
            {t('safety.reload')}
          </Button>
        </div>
      )}
    </Banner>
  )
}

export function EditorFooter({ actions, onCancel }: { actions: Pick<EditorActions, 'busy' | 'validate' | 'save'>; onCancel: () => void }) {
  const { busy, validate, save } = actions
  return (
    <>
      <Button onClick={onCancel} disabled={busy !== ''}>
        {t('common.cancel')}
      </Button>
      <Button icon={Check} loading={busy === 'validate'} disabled={busy !== ''} onClick={validate}>
        {t('safety.validate')}
      </Button>
      <Button variant="primary" icon={Upload} loading={busy === 'save'} disabled={busy !== ''} onClick={save}>
        {t('common.save')}
      </Button>
    </>
  )
}
