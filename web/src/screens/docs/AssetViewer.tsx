// A bundled contract file (the OpenAPI document, the Lua 5.1 type definitions)
// shown read-only, to copy or save (a save dialog in the desktop app).
import { Download, X } from 'lucide-react'
import { readAsset, type DocAsset } from '@/docs/library'
import { Button, Chip, CopyButton, EmptyState, IconButton } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import { downloadText } from '@/lib/format'
import { t } from '@/i18n'
import { toast } from '@/state/ui'

const MIME: Record<DocAsset['language'], string> = {
  yaml: 'application/yaml',
  json: 'application/json',
  lua: 'text/plain',
  text: 'text/plain',
}

export function AssetViewer({ asset, path, onClose }: { asset: DocAsset | undefined; path: string; onClose: () => void }) {
  const text = asset ? readAsset(asset.path) : undefined
  const name = path.slice(path.lastIndexOf('/') + 1)
  if (!asset || text === undefined) {
    return <EmptyState title={t('docs.couldNotRead', { path })} guidance={t('docs.assetNotBundled')} actionLabel={t('docs.backToPage')} onAction={onClose} />
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2 px-5 pb-4 pt-3">
      <div className="flex shrink-0 items-center gap-2">
        <h2 className="min-w-0 truncate text-lg font-semibold leading-tight">{t(asset.titleKey)}</h2>
        <Chip>{asset.path}</Chip>
        <span className="font-mono text-xs text-faint">{t('docs.assetLines', { n: text.split('\n').length })}</span>
        <span className="flex-1" />
        <CopyButton text={text} what={name} size="md" />
        <Button
          icon={Download}
          onClick={() => {
            void downloadText(name, text, MIME[asset.language]).then(
              (saved) => saved && toast(t('docs.downloaded', { name: saved }), 'success'),
            )
          }}
        >
          {t('docs.download')}
        </Button>
        <IconButton icon={X} label={t('docs.closeEsc')} onClick={onClose} />
      </div>
      <CodeEditor value={text} language={asset.language} readOnly className="min-h-0 flex-1" />
    </div>
  )
}
