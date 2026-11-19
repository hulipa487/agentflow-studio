// The documentation sidebar: search over every bundled guide, the pages in
// reading order under their sections, and the contract files that ship with
// them.
import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { FileCode } from 'lucide-react'
import { DOC_ASSETS, DOC_SECTIONS, docPages, readDoc, searchDocs, type DocPage } from '@/docs/library'
import { SearchBox, cx } from '@/components/ui'
import { t } from '@/i18n'
import { docPath, sectionForMatch, splitMatch } from './helpers'

const CAPTION = 'px-2.5 pb-1 pt-3 text-xs font-bold text-faint'
const ITEM = 'flex min-h-[30px] w-full items-center gap-2 rounded-[7px] px-2.5 text-left text-base transition-colors duration-100'
const IDLE = 'text-muted hover:bg-hover hover:text-text'
const ACTIVE = 'bg-surface2 font-semibold text-text'

function Marked({ text, query }: { text: string; query: string }) {
  const [before, hit, after] = splitMatch(text, query)
  if (hit === '') return <>{text}</>
  return (
    <>
      {before}
      <mark className="rounded-sm bg-accent-dim text-text">{hit}</mark>
      {after}
    </>
  )
}

export function Sidebar({
  current,
  asset,
  query,
  onQuery,
  onAsset,
}: {
  current: DocPage
  /** The contract file being viewed, if any. */
  asset: string
  query: string
  onQuery: (q: string) => void
  onAsset: (path: string) => void
}) {
  const pages = docPages()
  const searching = query.trim() !== ''
  const results = useMemo(
    () =>
      searchDocs(query, 30).map((r) => ({
        ...r,
        // Land on the section the hit is in, not at the top of a long page.
        anchor: sectionForMatch(readDoc(r.page.slug) ?? '', query),
      })),
    [query],
  )
  // Sections in their declared order, then any a page names that the list does not.
  // They hold catalogue keys; the headings are translated where rendered.
  const sections = useMemo(() => {
    const keys = [...DOC_SECTIONS]
    for (const p of pages) if (!keys.includes(p.sectionKey)) keys.push(p.sectionKey)
    return keys.map((key) => ({ key, pages: pages.filter((p) => p.sectionKey === key) })).filter((s) => s.pages.length > 0)
  }, [pages])

  return (
    <aside className="flex w-[264px] shrink-0 flex-col border-r border-line bg-surface" aria-label={t('docs.title')}>
      <div className="shrink-0 border-b border-line px-3 pb-3 pt-3">
        <h1 className="mb-2 px-0.5 text-md font-semibold leading-tight">{t('docs.title')}</h1>
        <SearchBox value={query} onChange={onQuery} placeholder={t('docs.search')} />
      </div>
      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {searching ? (
          <>
            <div className={CAPTION}>{t('docs.results', { n: results.length })}</div>
            {results.length === 0 && <div className="px-2.5 py-6 text-center text-sm text-muted">{t('docs.noMatches')}</div>}
            {results.map((r) => (
              <Link
                key={r.page.slug}
                to={docPath(r.page.slug, r.anchor)}
                className={cx(ITEM, 'flex-col !items-start gap-0.5 py-1.5', r.page.slug === current.slug && asset === '' ? ACTIVE : IDLE)}
              >
                <span className="w-full truncate text-text">
                  <Marked text={r.page.title} query={query} />
                </span>
                {r.snippet !== '' && (
                  <span className="line-clamp-2 w-full break-words text-sm font-normal leading-snug text-muted">
                    <Marked text={r.snippet} query={query} />
                  </span>
                )}
              </Link>
            ))}
          </>
        ) : (
          <>
            {sections.map((s) => (
              <div key={s.key}>
                <div className={CAPTION}>{t(s.key)}</div>
                {s.pages.map((p) => {
                  const active = p.slug === current.slug && asset === ''
                  return (
                    <Link key={p.slug} to={docPath(p.slug)} aria-current={active ? 'page' : undefined} className={cx(ITEM, 'relative', active ? ACTIVE : IDLE)}>
                      {active && <span className="absolute left-0 top-1/2 h-4 w-[3px] -translate-y-1/2 rounded-full bg-accent" />}
                      <span className="min-w-0 flex-1 truncate">{p.title}</span>
                    </Link>
                  )
                })}
              </div>
            ))}
            <div className={CAPTION}>{t('docs.contracts')}</div>
            {DOC_ASSETS.map((a) => (
              <button key={a.path} type="button" onClick={() => onAsset(a.path)} title={a.path} className={cx(ITEM, asset === a.path ? ACTIVE : IDLE)}>
                <FileCode className="size-[15px] shrink-0" strokeWidth={1.9} />
                <span className="min-w-0 flex-1 truncate">{t(a.titleKey)}</span>
              </button>
            ))}
          </>
        )}
      </nav>
    </aside>
  )
}
