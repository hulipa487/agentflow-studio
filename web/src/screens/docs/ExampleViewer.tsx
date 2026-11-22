// A bundled example loop from examples/loops/<slug>.lua, shown read-only
// with quick actions to copy, download, or open in the Builder when connected.
import { ArrowRight, Download, X } from 'lucide-react'
import { exampleSummary, type ExampleLoop } from '@/docs/examples'
import { Button, Chip, CopyButton, EmptyState, IconButton } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { downloadText } from '@/lib/format'
import { t } from '@/i18n'
import { toast } from '@/state/ui'

export function ExampleViewer({
  example,
  slug,
  onClose,
  onOpenBuilder,
}: {
  example: ExampleLoop | undefined
  slug: string
  onClose: () => void
  onOpenBuilder?: (slug: string) => void
}) {
  if (!example) {
    return (
      <EmptyState
        title={t('docs.couldNotRead', { path: slug })}
        guidance={t('docs.exampleNotBundled')}
        actionLabel={t('docs.backToPage')}
        onAction={onClose}
      />
    )
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2 px-5 pb-4 pt-3">
      <div className="flex shrink-0 items-center gap-2">
        <h2 className="min-w-0 truncate font-mono text-lg font-semibold leading-tight">{example.title}</h2>
        <Chip>examples/loops/{example.file}</Chip>
        <span className="font-mono text-xs text-faint">{t('docs.assetLines', { n: example.source.split('\n').length })}</span>
        <span className="flex-1" />
        <CopyButton text={example.source} what={example.file} size="md" />
        <Button
          icon={Download}
          onClick={() => {
            void downloadText(example.file, example.source, 'text/plain').then(
              (saved) => saved && toast(t('docs.downloaded', { name: saved }), 'success'),
            )
          }}
        >
          {t('docs.download')}
        </Button>
        {onOpenBuilder && (
          <Button variant="primary" icon={ArrowRight} onClick={() => onOpenBuilder(example.slug)}>
            {t('docs.openInBuilder')}
          </Button>
        )}
        <IconButton icon={X} label={t('docs.closeEsc')} onClick={onClose} />
      </div>
      {exampleSummary(example) && <p className="shrink-0 text-sm text-muted">{exampleSummary(example)}</p>}
      <CodeEditor value={example.source} language="lua" readOnly className="min-h-0 flex-1" />
    </div>
  )
}
