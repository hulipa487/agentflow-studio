// Helpers for the documentation viewer: where things are in a rendered page.
import { headingID, type DocHeading } from '@/docs/library'

/** Where a page (and optionally a heading in it) lives in the hash router. */
export function docPath(slug: string, anchor?: string): string {
  return '/docs/' + encodeURIComponent(slug) + (anchor ? '?h=' + encodeURIComponent(anchor) : '')
}

/**
 * The heading element an anchor names. Links written for GitHub carry
 * GitHub's slug, which is not always the renderer's, so the anchor is also
 * tried as heading text.
 */
export function findHeading(root: HTMLElement, anchor: string, occurrence = 0): HTMLElement | null {
  if (anchor === '') return null
  const byID = (id: string) => root.querySelectorAll<HTMLElement>(`[id="${CSS.escape(id)}"]`)
  let hits = byID(anchor)
  if (hits.length === 0) hits = byID(headingID(anchor))
  return hits[occurrence] ?? hits[0] ?? null
}

/**
 * The rendered element for each outline entry. Two headings with the same
 * text get the same id, so entries are matched to elements by occurrence.
 */
export function outlineElements(root: HTMLElement, outline: DocHeading[]): (HTMLElement | null)[] {
  const seen = new Map<string, number>()
  return outline.map((h) => {
    const k = seen.get(h.id) ?? 0
    seen.set(h.id, k + 1)
    return root.querySelectorAll<HTMLElement>(`[id="${CSS.escape(h.id)}"]`)[k] ?? null
  })
}

/** An element's distance from the top of the scroll container's content. */
export function offsetIn(el: HTMLElement, scroller: HTMLElement): number {
  return el.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop
}

/**
 * The id of the section a search hit falls in: the last h2/h3 at or before
 * the first line containing the query. Empty when the hit precedes every
 * heading (or is only in the page title).
 */
export function sectionForMatch(source: string, query: string): string {
  const q = query.trim().toLowerCase()
  if (q === '') return ''
  let fenced = false
  let last = ''
  for (const line of source.split('\n')) {
    if (/^\s*(```|~~~)/.test(line)) {
      fenced = !fenced
    } else if (!fenced) {
      // The same heading rule as docOutline, so the id is one the page has.
      const m = /^(#{2,3})\s+(.+?)\s*#*\s*$/.exec(line)
      if (m) last = headingID(m[2].replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').replace(/[*_`]/g, ''))
    }
    if (line.toLowerCase().includes(q)) return last
  }
  return ''
}

/** Splits text around the first case-insensitive occurrence of the query. */
export function splitMatch(text: string, query: string): [string, string, string] {
  const q = query.trim().toLowerCase()
  const i = q === '' ? -1 : text.toLowerCase().indexOf(q)
  if (i < 0) return [text, '', '']
  return [text.slice(0, i), text.slice(i, i + q.length), text.slice(i + q.length)]
}
