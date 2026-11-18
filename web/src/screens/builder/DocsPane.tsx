// The builder's inline documentation pane. The guide selector stays pinned at
// the top of the card while the reference body scrolls below.
import { useEffect, useRef, useState } from 'react'
import { BookOpen } from 'lucide-react'
import { Button, Select } from '@/components/ui'
import { Markdown } from '@/components/ui/Markdown'
import { docPage, docPages, readDoc } from '@/docs/library'
import { docPath } from '@/screens/docs/helpers'

const BUILDER_DOC_PAGES = [
  { slug: 'loops-reference', label: 'Loop API reference' },
  { slug: 'loops', label: 'Luau coding tutorial' },
]

export function DocsPane({ onOpenDocs }: { onOpenDocs: (path: string) => void }) {
  const [slug, setSlug] = useState(BUILDER_DOC_PAGES[0].slug)
  const scroll = useRef<HTMLDivElement>(null)

  useEffect(() => {
    scroll.current?.scrollTo({ top: 0 })
  }, [slug])

  let body
  if (docPages().length === 0) {
    body = <p className="text-sm text-muted">Documentation is not bundled in this build.</p>
  } else if (!docPage(slug)) {
    body = <p className="text-sm text-muted">This guide is not bundled in this build.</p>
  } else {
    const source = readDoc(slug)
    body =
      source === undefined ? (
        <p className="text-sm text-muted">This guide could not be read.</p>
      ) : (
        <Markdown
          compact
          source={source}
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
          <h2 className="min-w-0 flex-1 truncate text-md font-semibold leading-tight">Luau Reference</h2>
          <Button size="sm" variant="ghost" icon={BookOpen} title="Open this guide in the documentation viewer" onClick={() => onOpenDocs('/docs/' + slug)}>
            Full Docs
          </Button>
        </div>
        <Select value={slug} onChange={setSlug} aria-label="Guide" options={BUILDER_DOC_PAGES.map((p) => ({ value: p.slug, label: p.label }))} />
      </div>
      <div ref={scroll} className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        {body}
      </div>
    </aside>
  )
}
