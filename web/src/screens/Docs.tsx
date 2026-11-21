// Documentation: the bundled guides, readable offline and on every
// transport. Pages live at /docs/<slug>; hash routing has no fragment left
// to spend on a heading, so one is addressed as ?h=<heading-id>.
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ArrowLeft, ArrowRight } from 'lucide-react'
import { findExample } from '@/docs/examples'
import { DOC_ASSETS, docHeadingIDs, docOutline, docPages, docTitle, readDoc, type DocPage } from '@/docs/library'
import { Chip, EmptyState } from '@/components/ui'
import { Markdown } from '@/components/ui/Markdown'
import { useScreenActions } from '@/lib/hooks'
import { useSession } from '@/state/session'
import { t, useLang } from '@/i18n'
import { AssetViewer } from './docs/AssetViewer'
import { ExampleViewer } from './docs/ExampleViewer'
import { Outline } from './docs/Outline'
import { Sidebar } from './docs/Sidebar'
import { docPath, findHeading, offsetIn, outlineElements } from './docs/helpers'
import { newAgentPath } from './NewAgentDialog'

/** How far below the top of the reading column a heading comes to rest. */
const TOP_GAP = 16

function PagerLink({ page, dir }: { page: DocPage; dir: 'prev' | 'next' }) {
  const next = dir === 'next'
  return (
    <Link
      to={docPath(page.slug)}
      className={
        'flex min-w-0 flex-col gap-0.5 rounded-card border border-line bg-surface px-3 py-2 transition-colors duration-100 hover:border-accent/50 hover:bg-hover ' +
        (next ? 'items-end text-right' : 'items-start')
      }
    >
      <span className="inline-flex items-center gap-1 text-xs text-faint">
        {!next && <ArrowLeft className="size-3" />}
        {next ? t('docs.next') : t('docs.previous')}
        {next && <ArrowRight className="size-3" />}
      </span>
      <span className="max-w-full truncate text-base font-semibold text-text">{docTitle(page)}</span>
    </Link>
  )
}

function Viewer({ pages }: { pages: DocPage[] }) {
  const params = useParams()
  const [search, setSearch] = useSearchParams()
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(-1)
  const scroller = useRef<HTMLDivElement>(null)
  // Set when the outline has already scrolled to the heading it is about to
  // put in the URL, so the URL change does not jump there a second time.
  const skipScroll = useRef(false)

  const client = useSession((s) => s.client)
  // An unknown slug shows the first page, as the desktop app does.
  const index = Math.max(0, pages.findIndex((p) => p.slug === params.slug))
  const page = pages[index]
  const anchor = search.get('h') ?? ''
  const assetPath = search.get('asset') ?? ''
  const exampleSlug = assetPath === '' ? (search.get('example') ?? '') : ''
  const overlayOpen = assetPath !== '' || exampleSlug !== ''
  // The guide in the current language (English where it has no translation);
  // a translation's headings take the English ids, so anchors still resolve.
  const lang = useLang()
  const source = useMemo(() => readDoc(page.slug, lang), [page.slug, lang])
  const headingIds = useMemo(() => docHeadingIDs(page.slug, lang), [page.slug, lang])
  const englishOnly = lang !== 'en' && headingIds === undefined
  const outline = useMemo(() => (source ? docOutline(source, headingIds) : []), [source, headingIds])

  // A new page starts at the top, or at the heading the link named.
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el) return
    if (skipScroll.current) {
      skipScroll.current = false
      return
    }
    const target = findHeading(el, anchor)
    el.scrollTop = target ? Math.max(0, offsetIn(target, el) - TOP_GAP) : 0
    // Hand the keyboard to the page (Space, PageDown) unless a field has it.
    const focused = document.activeElement
    if (!(focused instanceof HTMLInputElement)) el.focus({ preventScroll: true })
  }, [page.slug, anchor])

  // Mark the section being read: the last heading at or above the top edge.
  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const heads = outlineElements(el, outline)
    let frame = 0
    const update = () => {
      frame = 0
      const line = el.scrollTop + TOP_GAP + 8
      let current = -1
      for (let i = 0; i < heads.length; i++) {
        const h = heads[i]
        if (!h) continue
        if (offsetIn(h, el) > line) break
        current = i
      }
      // The last sections of a page can never reach the top edge.
      if (el.scrollTop > 0 && el.scrollTop + el.clientHeight >= el.scrollHeight - 2) {
        for (let i = heads.length - 1; i > current; i--) {
          if (heads[i]) {
            current = i
            break
          }
        }
      }
      setActive(current)
    }
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(update)
    }
    update()
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => {
      el.removeEventListener('scroll', onScroll)
      cancelAnimationFrame(frame)
    }
  }, [outline, page.slug])

  const pickHeading = (i: number) => {
    const el = scroller.current
    const target = el ? outlineElements(el, outline)[i] : null
    if (!el || !target) return
    const calm = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    el.scrollTo({ top: Math.max(0, offsetIn(target, el) - TOP_GAP), behavior: calm ? 'auto' : 'smooth' })
    const id = outline[i].id
    if (id !== anchor) {
      skipScroll.current = true
      const next = new URLSearchParams(search)
      next.set('h', id)
      setSearch(next, { replace: true })
    }
  }

  const openAsset = (path: string) => {
    const next = new URLSearchParams(search)
    next.delete('example')
    next.set('asset', path)
    setSearch(next)
  }
  const closeAsset = () => {
    const next = new URLSearchParams(search)
    next.delete('asset')
    setSearch(next, { replace: true })
  }
  const openExample = (slug: string) => {
    const next = new URLSearchParams(search)
    next.delete('asset')
    next.set('example', slug)
    setSearch(next)
  }
  const closeExample = () => {
    const next = new URLSearchParams(search)
    next.delete('example')
    setSearch(next, { replace: true })
  }

  useScreenActions({
    back: () => {
      if (assetPath !== '') closeAsset()
      else if (exampleSlug !== '') closeExample()
      // Esc returns to the previous view; a page opened by its link has none.
      else if (((window.history.state as { idx?: number } | null)?.idx ?? 0) > 0) navigate(-1)
    },
  })

  const prev = index > 0 ? pages[index - 1] : undefined
  const next = index < pages.length - 1 ? pages[index + 1] : undefined

  return (
    <div className="flex h-full min-h-0">
      <Sidebar current={page} asset={assetPath} example={exampleSlug} query={query} onQuery={setQuery} onAsset={openAsset} onExample={openExample} />
      <div className="relative flex min-w-0 flex-1">
        {/* The page stays mounted under a contract file or example so closing it returns to the same place. */}
        <div className="flex min-w-0 flex-1" inert={overlayOpen}>
          <div ref={scroller} tabIndex={-1} className="min-w-0 flex-1 overflow-y-auto outline-none">
            <article className="mx-auto w-full max-w-[860px] px-8 pb-16 pt-5">
              <div className="mb-4 flex items-center gap-2">
                <Chip tone="accent">{t(page.sectionKey)}</Chip>
                <span className="min-w-0 truncate text-xs text-muted">{docTitle(page)}</span>
              </div>
              {englishOnly && <p className="mb-3 text-xs text-muted">{t('docs.englishOnly')}</p>}
              {source === undefined ? (
                <p className="text-sm text-muted">{t('docs.unreadable')}</p>
              ) : (
                <Markdown source={source} headingIds={headingIds} onNavigate={(slug, a) => navigate(docPath(slug, a))} onAsset={openAsset} />
              )}
              {(prev || next) && (
                <nav className="mt-10 grid grid-cols-2 gap-3 border-t border-line pt-4" aria-label={t('docs.pager')}>
                  {prev ? <PagerLink page={prev} dir="prev" /> : <span />}
                  {next ? <PagerLink page={next} dir="next" /> : <span />}
                </nav>
              )}
            </article>
          </div>
          <Outline outline={outline} active={active} onPick={pickHeading} />
        </div>
        {assetPath !== '' && (
          <div className="absolute inset-0 z-10 flex animate-fade-in flex-col bg-bg">
            <AssetViewer asset={DOC_ASSETS.find((a) => a.path === assetPath)} path={assetPath} onClose={closeAsset} />
          </div>
        )}
        {exampleSlug !== '' && (
          <div className="absolute inset-0 z-10 flex animate-fade-in flex-col bg-bg">
            <ExampleViewer
              example={findExample(exampleSlug)}
              slug={exampleSlug}
              onClose={closeExample}
              onOpenBuilder={client ? (slug) => navigate(newAgentPath(undefined, slug)) : undefined}
            />
          </div>
        )}
      </div>
    </div>
  )
}

export default function Docs() {
  const pages = docPages()
  if (pages.length === 0) {
    return <EmptyState title={t('docs.notBundled')} guidance={t('docs.noPages')} className="h-full" />
  }
  return <Viewer pages={pages} />
}
