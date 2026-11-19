// The bundled documentation set: the repository's docs/ directory, the same
// files the desktop Studio embeds. There is no copy to keep in step. Pages are
// compiled into the app, so the viewer works offline and on every transport.
const pages = import.meta.glob('../../../docs/*.md', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const assets = import.meta.glob('../../../docs/**/*.{yaml,yml,luau,json}', { query: '?raw', import: 'default', eager: true }) as Record<string, string>

/** The glob keys' prefix: this file's path to the shared docs directory. */
const ROOT = '../../../docs/'

/** Every Markdown file in docs/, by file name. A page needs a manifest entry
 *  to be reachable; the tests compare the two. */
export function bundledDocFiles(): string[] {
  return Object.keys(pages).map((k) => k.slice(ROOT.length)).sort()
}

export interface DocPage {
  slug: string
  file: string
  title: string
  section: string
  order: number
}

export interface DocResult {
  page: DocPage
  snippet: string
}

export interface DocAsset {
  /** Path relative to the docs root, e.g. "openapi/users.yaml". */
  path: string
  title: string
  language: 'yaml' | 'lua' | 'json' | 'text'
}

const MANIFEST: DocPage[] = [
  { slug: 'getting-started', file: 'getting-started.md', title: 'Overview', section: 'Start here', order: 1 },
  { slug: 'architecture', file: 'architecture.md', title: 'Architecture & deployment', section: 'Start here', order: 2 },
  { slug: 'multi-region', file: 'multi-region.md', title: 'Multi-region', section: 'Start here', order: 3 },
  { slug: 'loops', file: 'loops.md', title: 'Luau loops', section: 'Build', order: 4 },
  { slug: 'memory', file: 'memory.md', title: 'Memory', section: 'Build', order: 5 },
  { slug: 'extending', file: 'extending.md', title: 'Extending', section: 'Build', order: 6 },
  { slug: 'configuration', file: 'configuration.md', title: 'Configuration', section: 'Reference', order: 7 },
  { slug: 'loops-reference', file: 'loops-reference.md', title: 'Loop API reference', section: 'Reference', order: 8 },
  { slug: 'openai-api', file: 'openai-api.md', title: 'OpenAI-compatible API', section: 'Reference', order: 9 },
]

export const DOC_SECTIONS = ['Start here', 'Build', 'Reference']

/** The contract files that ship beside the guides. */
export const DOC_ASSETS: DocAsset[] = [
  { path: 'openapi/users.yaml', title: 'User API (OpenAPI)', language: 'yaml' },
  { path: 'luau/af.d.luau', title: 'Luau type definitions', language: 'lua' },
]

const available = MANIFEST.filter((p) => typeof pages[ROOT + p.file] === 'string' && pages[ROOT + p.file] !== '')
const bySlug = new Map(available.map((p) => [p.slug, p]))

/** The manifest in reading order (only pages actually bundled). */
export function docPages(): DocPage[] {
  return available
}

export function docPage(slug: string): DocPage | undefined {
  return bySlug.get(slug)
}

export function readDoc(slug: string): string | undefined {
  const p = bySlug.get(slug)
  return p ? pages[ROOT + p.file] : undefined
}

export function readAsset(path: string): string | undefined {
  return assets[ROOT + path.replace(/^\/+/, '')]
}

/**
 * Resolves an in-document link to a page slug: "loops.md", "./loops.md#x" and
 * "docs/loops.md" all name the loops page. External links, anchors and
 * non-Markdown targets return undefined.
 */
export function slugFor(href: string): string | undefined {
  let h = href.trim()
  if (h === '' || h.startsWith('#') || h.includes('://') || h.startsWith('mailto:')) return undefined
  const cut = h.search(/[?#]/)
  if (cut >= 0) h = h.slice(0, cut)
  h = h.replace(/^\.\//, '')
  const base = h.slice(h.lastIndexOf('/') + 1)
  if (!base.endsWith('.md')) return undefined
  const slug = base.slice(0, -3)
  return bySlug.has(slug) ? slug : undefined
}

/** The asset an in-document link names ("openapi/users.yaml"), if bundled. */
export function assetFor(href: string): DocAsset | undefined {
  const h = href.trim().replace(/^\.\//, '').replace(/[?#].*$/, '')
  return DOC_ASSETS.find((a) => h === a.path || h.endsWith('/' + a.path))
}

/** Pages whose body or title contains the query, each with the matching line. */
export function searchDocs(query: string, limit = 20): DocResult[] {
  const q = query.trim().toLowerCase()
  if (q === '') return []
  const out: DocResult[] = []
  for (const p of available) {
    const body = readDoc(p.slug) ?? ''
    const idx = body.toLowerCase().indexOf(q)
    if (idx < 0 && !p.title.toLowerCase().includes(q)) continue
    let snippet = ''
    if (idx >= 0) {
      const start = body.lastIndexOf('\n', idx - 1) + 1
      let end = body.indexOf('\n', idx)
      if (end < 0) end = body.length
      snippet = body.slice(start, end).trim()
      const runes = Array.from(snippet)
      if (runes.length > 160) snippet = runes.slice(0, 157).join('') + '…'
    }
    out.push({ page: p, snippet })
    if (out.length >= limit) break
  }
  return out.sort((a, b) => a.page.order - b.page.order)
}

export interface DocHeading {
  level: number
  text: string
  id: string
}

/**
 * The id the Markdown renderer gives a heading: GitHub's slug, which the
 * guides' own #links are written against. Every space becomes a hyphen, so
 * "audit — message journal" is audit--message-journal, as on GitHub.
 */
export function headingID(text: string): string {
  return text
    .toLowerCase()
    .replace(/`/g, '')
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .trim()
    .replace(/\s/g, '-')
}

/** The h2/h3 outline of a page, for section navigation. */
export function docOutline(source: string): DocHeading[] {
  const out: DocHeading[] = []
  let fenced = false
  for (const line of source.split('\n')) {
    if (/^\s*(```|~~~)/.test(line)) {
      fenced = !fenced
      continue
    }
    if (fenced) continue
    const m = /^(#{2,3})\s+(.+?)\s*#*\s*$/.exec(line)
    if (m) {
      const text = m[2].replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').replace(/[*_`]/g, '')
      out.push({ level: m[1].length, text, id: headingID(text) })
    }
  }
  return out
}
