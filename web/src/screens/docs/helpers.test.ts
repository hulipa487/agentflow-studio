import { describe, expect, it } from 'vitest'
import { docOutline, docPages, readDoc, searchDocs } from '@/docs/library'
import { docPath, sectionForMatch, splitMatch } from './helpers'

describe('docPath', () => {
  it('addresses a heading with ?h= because the hash is the route', () => {
    expect(docPath('loops')).toBe('/docs/loops')
    expect(docPath('loops', '6-api-reference')).toBe('/docs/loops?h=6-api-reference')
    expect(docPath('loops', 'a b&c')).toBe('/docs/loops?h=a%20b%26c')
  })
})

describe('sectionForMatch', () => {
  const src = ['# Title', 'intro needle-zero', '## First part', 'text', '```lua', '## not a heading', 'needle-one', '```', '### Sub `part`', 'needle-two'].join('\n')

  it('names the last heading at or before the hit', () => {
    expect(sectionForMatch(src, 'needle-zero')).toBe('')
    expect(sectionForMatch(src, 'NEEDLE-ONE')).toBe('first-part')
    expect(sectionForMatch(src, 'needle-two')).toBe('sub-part')
    expect(sectionForMatch(src, 'First part')).toBe('first-part')
    expect(sectionForMatch(src, 'absent')).toBe('')
    expect(sectionForMatch(src, '  ')).toBe('')
  })

  it('returns ids the outline of every bundled page has', () => {
    for (const p of docPages()) {
      const body = readDoc(p.slug) ?? ''
      const ids = new Set(docOutline(body).map((h) => h.id))
      for (const q of ['agent', 'runtime', 'memory', 'config']) {
        const id = sectionForMatch(body, q)
        if (id !== '') expect(ids.has(id), `${p.slug}: ${q} -> ${id}`).toBe(true)
      }
    }
  })
})

describe('splitMatch', () => {
  it('splits around the first case-insensitive occurrence', () => {
    expect(splitMatch('The Runtime boots', 'runtime')).toEqual(['The ', 'Runtime', ' boots'])
    expect(splitMatch('nothing here', 'zzz')).toEqual(['nothing here', '', ''])
    expect(splitMatch('anything', ' ')).toEqual(['anything', '', ''])
  })
})

describe('the bundled set', () => {
  it('has pages in reading order and search hits with snippets', () => {
    const pages = docPages()
    expect(pages.length).toBeGreaterThan(0)
    expect(pages.map((p) => p.order)).toEqual([...pages.map((p) => p.order)].sort((a, b) => a - b))
    const hits = searchDocs('cors_origins', 30)
    expect(hits.length).toBeGreaterThan(0)
    expect(hits[0].snippet.toLowerCase()).toContain('cors_origins')
  })
})
