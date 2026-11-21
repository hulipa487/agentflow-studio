// New agent: start from a template, a blank definition, or a bundled example loop.
// Everything can be edited in the builder afterwards.
import { useNavigate } from 'react-router-dom'
import { ArrowRight } from 'lucide-react'
import { bundledExamples } from '@/docs/examples'
import { Button, Chip, Modal } from '@/components/ui'
import { useCatalog } from '@/state/session'
import { useUI } from '@/state/ui'
import { t } from '@/i18n'

/** The builder route for a new agent, optionally seeded from a template or example loop. */
export function newAgentPath(template?: string, example?: string): string {
  const qs = new URLSearchParams()
  if (template) qs.set('template', template)
  if (example) qs.set('example', example)
  const suffix = qs.toString()
  return '/new-agent' + (suffix ? '?' + suffix : '')
}

export function NewAgentDialog() {
  const open = useUI((s) => s.newAgentOpen)
  const setOpen = useUI((s) => s.setNewAgent)
  const catalog = useCatalog()
  const navigate = useNavigate()
  const examples = bundledExamples()
  const use = (template?: string, example?: string) => {
    setOpen(false)
    navigate(newAgentPath(template, example))
  }
  const card = (
    key: string,
    title: string,
    tag: string,
    tone: 'neutral' | 'accent',
    description: string,
    opts?: { template?: string; example?: string },
  ) => (
    <div key={key} className="flex items-center gap-3 rounded-ctl border border-line bg-surface2 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-md font-semibold">{title}</span>
          <Chip tone={tone}>{tag}</Chip>
        </div>
        {description && <p className="mt-0.5 text-sm leading-snug text-muted">{description}</p>}
      </div>
      <Button variant="primary" icon={ArrowRight} onClick={() => use(opts?.template, opts?.example)}>
        {t('agents.use')}
      </Button>
    </div>
  )
  return (
    <Modal
      open={open}
      onClose={() => setOpen(false)}
      title={t('agents.newTitle')}
      subtitle={t('agents.newSubtitle')}
      width={680}
      footer={<Button onClick={() => setOpen(false)}>{t('common.cancel')}</Button>}
    >
      <div className="flex flex-col gap-2">
        {card('blank', t('agents.blankTitle'), t('agents.blankTag'), 'neutral', t('agents.blankDescription'))}
        {(catalog.templates ?? []).map((tpl) => card(`tpl:${tpl.name}`, tpl.title, tpl.name, 'accent', tpl.description, { template: tpl.name }))}
      </div>
      {examples.length > 0 && (
        <div className="mt-5">
          <div className="mb-2">
            <h3 className="text-xs font-semibold text-faint">{t('agents.exampleLoopsTitle')}</h3>
            <p className="text-xs text-muted">{t('agents.exampleLoopsHint')}</p>
          </div>
          <div className="flex flex-col gap-2">
            {examples.map((ex) => card(`ex:${ex.slug}`, ex.title, ex.slug, 'neutral', ex.summary, { example: ex.slug }))}
          </div>
        </div>
      )}
    </Modal>
  )
}
