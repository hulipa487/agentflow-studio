// New agent: start from a template or a blank definition. Everything can be
// edited in the builder afterwards.
import { useNavigate } from 'react-router-dom'
import { ArrowRight } from 'lucide-react'
import { Button, Chip, Modal } from '@/components/ui'
import { useCatalog } from '@/state/session'
import { useUI } from '@/state/ui'
import { t } from '@/i18n'

/** The builder route for a new agent, optionally seeded from a template. */
export function newAgentPath(template?: string): string {
  return '/new-agent' + (template ? '?template=' + encodeURIComponent(template) : '')
}

export function NewAgentDialog() {
  const open = useUI((s) => s.newAgentOpen)
  const setOpen = useUI((s) => s.setNewAgent)
  const catalog = useCatalog()
  const navigate = useNavigate()
  const use = (template?: string) => {
    setOpen(false)
    navigate(newAgentPath(template))
  }
  const card = (key: string, title: string, tag: string, tone: 'neutral' | 'accent', description: string, template?: string) => (
    <div key={key} className="flex items-center gap-3 rounded-ctl border border-line bg-surface2 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-md font-semibold">{title}</span>
          <Chip tone={tone}>{tag}</Chip>
        </div>
        <p className="mt-0.5 text-sm leading-snug text-muted">{description}</p>
      </div>
      <Button variant="primary" icon={ArrowRight} onClick={() => use(template)}>
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
        {(catalog.templates ?? []).map((tpl) => card(tpl.name, tpl.title, tpl.name, 'accent', tpl.description, tpl.name))}
      </div>
    </Modal>
  )
}
