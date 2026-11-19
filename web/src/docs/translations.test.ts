// The translated guides must match their English source in structure: the
// same headings (a translated heading takes the English heading's anchor by
// position), the same code blocks (only prose comments may be translated),
// the same link targets and the same table shapes. The desktop app has the
// same check (desktop/docs_i18n_test.go).
import { describe, expect, it } from 'vitest'
import { bundledDocFiles, bundledTranslations, docHeadingIDs, docOutline, englishSource, readDoc, translatedSource } from './library'

interface Shape {
  headings: number[]
  blocks: { info: string; code: string[] }[]
  links: string[]
  tables: number[][]
}

function parse(text: string): Shape {
  const g: Shape = { headings: [], blocks: [], links: [], tables: [] }
  let inFence = false
  let fence = ''
  let block = { info: '', code: [] as string[] }
  let table: number[] | null = null
  const flush = () => {
    if (table) g.tables.push(table)
    table = null
  }
  for (const line of text.split('\n')) {
    const m = /^(\s*)(`{3,}|~{3,})(.*)$/.exec(line)
    if (inFence) {
      if (m && m[2][0] === fence[0] && m[2].length >= fence.length && m[3].trim() === '') {
        g.blocks.push(block)
        inFence = false
      } else block.code.push(line)
      continue
    }
    if (m) {
      inFence = true
      fence = m[2]
      block = { info: m[3].trim(), code: [] }
      continue
    }
    const h = /^(#{1,6})\s+\S/.exec(line)
    if (h) g.headings.push(h[1].length)
    for (const l of line.matchAll(/\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g)) g.links.push(l[1])
    if (line.trimStart().startsWith('|')) {
      table ??= []
      table.push(line.trim().replace(/^\|+|\|+$/g, '').split('|').length)
    } else flush()
  }
  flush()
  g.links.sort()
  return g
}

/** Drops a #, -- or // comment (directives #!, #{ and -- af-… stay), as the Go test does. */
export function stripComment(line: string): string {
  for (let i = 0; i < line.length; i++) {
    if (i > 0 && line[i - 1] !== ' ' && line[i - 1] !== '\t') continue
    const rest = line.slice(i)
    const hash = rest.startsWith('#') && !rest.startsWith('#!') && !rest.startsWith('#{')
    const dash = rest.startsWith('--') && !rest.slice(2).trimStart().startsWith('af-')
    if (hash || dash || rest.startsWith('//')) return line.slice(0, i)
  }
  return line
}

const codeLines = (code: string[]) => code.map((l) => stripComment(l).trim()).filter(Boolean)

describe('translated guides', () => {
  const files = bundledDocFiles()

  for (const lang of ['zh-Hans', 'zh-Hant'] as const) {
    it(`every guide has a ${lang} translation and nothing else is there`, () => {
      const missing = files.filter((f) => translatedSource(lang, f) === undefined)
      const extra = bundledTranslations().filter((f) => f.startsWith(lang + '/') && !files.includes(f.slice(lang.length + 1)))
      expect({ missing, extra }).toEqual({ missing: [], extra: [] })
    })

    it(`${lang} guides keep the English headings, code, links and tables`, () => {
      const wrong: string[] = []
      for (const f of files) {
        const tr = translatedSource(lang, f)
        if (tr === undefined) continue
        const a = parse(englishSource(f) ?? '')
        const b = parse(tr)
        const name = `${lang}/${f}`
        if (JSON.stringify(a.headings) !== JSON.stringify(b.headings)) wrong.push(`${name}: headings (${b.headings.length} vs ${a.headings.length})`)
        if (a.blocks.length !== b.blocks.length) wrong.push(`${name}: ${b.blocks.length} code blocks vs ${a.blocks.length}`)
        else
          a.blocks.forEach((x, i) => {
            const y = b.blocks[i]
            if (x.info !== y.info) wrong.push(`${name}: code block ${i + 1} is ${y.info}, English ${x.info}`)
            else if (JSON.stringify(codeLines(x.code)) !== JSON.stringify(codeLines(y.code))) wrong.push(`${name}: code block ${i + 1} (${x.info}) differs beyond comments`)
          })
        if (JSON.stringify(a.links) !== JSON.stringify(b.links)) wrong.push(`${name}: link targets`)
        if (JSON.stringify(a.tables) !== JSON.stringify(b.tables)) wrong.push(`${name}: table shapes`)
      }
      expect(wrong).toEqual([])
    })
  }

  it('gives a translated page the English heading ids, so anchors resolve', () => {
    const slug = 'configuration'
    const en = docOutline(readDoc(slug, 'en') ?? '')
    for (const lang of ['zh-Hans', 'zh-Hant'] as const) {
      const ids = docHeadingIDs(slug, lang)
      if (!ids) continue
      const zh = docOutline(readDoc(slug, lang) ?? '', ids)
      expect(zh.map((h) => h.id)).toEqual(en.map((h) => h.id))
      expect(zh.map((h) => h.text)).not.toEqual(en.map((h) => h.text))
    }
    expect(docHeadingIDs(slug, 'en')).toBeUndefined()
  })

  it('strips comments as the Go test does', () => {
    expect(stripComment('key: value  # a note')).toBe('key: value  ')
    expect(stripComment('#!/bin/sh')).toBe('#!/bin/sh')
    expect(stripComment('-- af-prelude-version: 1')).toBe('-- af-prelude-version: 1')
    expect(stripComment('url: http://host/x')).toBe('url: http://host/x')
    expect(stripComment('color: "#fff"')).toBe('color: "#fff"')
  })
})
