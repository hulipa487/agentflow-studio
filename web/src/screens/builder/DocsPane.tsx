// The builder's inline documentation pane. The guide selector stays pinned at
// the top of the card while the reference body scrolls below.
import { useEffect, useMemo, useRef, useState } from 'react'
import { BookOpen } from 'lucide-react'
import { Button, Select } from '@/components/ui'
import { Markdown } from '@/components/ui/Markdown'
import { t, useLang } from '@/i18n'
import { docHeadingIDs, docPage, docPages, readDoc } from '@/docs/library'
import { docPath } from '@/screens/docs/helpers'

const BUILDER_DOC_PAGES = [
  { slug: 'loops-reference', labelKey: 'builder.docLoopsReference' },
  { slug: 'loops', labelKey: 'builder.docLoopsTutorial' },
]

export function DocsPane({ onOpenDocs }: { onOpenDocs: (path: string) => void }) {
  const [slug, setSlug] = useState(BUILDER_DOC_PAGES[0].slug)
  const scroll = useRef<HTMLDivElement>(null)
  // Memoised per page and language: a fresh list would re-parse the guide on
  // every render of the builder.
  const lang = useLang()
  const headingIds = useMemo(() => docHeadingIDs(slug, lang), [slug, lang])

  useEffect(() => {
    scroll.current?.scrollTo({ top: 0 })
  }, [slug])

  let body
  if (docPages().length === 0) {
    body = <p className="text-sm text-muted">{t('builder.docsNotBundled')}</p>
  } else if (!docPage(slug)) {
    body = <p className="text-sm text-muted">{t('builder.guideNotBundled')}</p>
  } else {
    const source = readDoc(slug, lang)
    body =
      source === undefined ? (
        <p className="text-sm text-muted">{t('builder.guideUnreadable')}</p>
      ) : (
        <Markdown
          compact
          source={source}
          headingIds={headingIds}
          onNavigate={(next, anchor) => {
            // The two builder guides link to each other; those stay in the
            // pane. Any other guide opens in the documentation viewer.
            if (BUILDER_DOC_PAGES.some((p) => p.slug === next)) setSlug(next)
            else onOpenDocs(docPath(next, anchor))
          }}
          onAsset={() => onOpenDocs('/docs/' + slug)}
        />
      )
  }

  return (
    <aside className="flex min-h-0 flex-1 flex-col rounded-card border border-line bg-surface">
      <div className="flex shrink-0 flex-col gap-2 border-b border-line p-3">
        <div className="flex items-center gap-2">
          <h2 className="min-w-0 flex-1 truncate text-md font-semibold leading-tight">{t('builder.luauReference')}</h2>
          <Button size="sm" variant="ghost" icon={BookOpen} title={t('builder.fullDocsTitle')} onClick={() => onOpenDocs('/docs/' + slug)}>
            {t('builder.fullDocs')}
          </Button>
        </div>
        <Select value={slug} onChange={setSlug} aria-label={t('builder.guide')} options={BUILDER_DOC_PAGES.map((p) => ({ value: p.slug, label: t(p.labelKey) }))} />
      </div>
      <div ref={scroll} className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        {body}
      </div>
    </aside>
  )
}
