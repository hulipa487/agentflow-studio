// Keeps the three catalogues in step, and the code in step with them.
import { describe, expect, it } from 'vitest'
import { CATALOGUES, LANGS, catalogueFiles, detectLang, translate, type Lang } from './index'
import { formatMessage, messageParams } from './format'

const OTHERS: Lang[] = ['zh-Hans', 'zh-Hant']

describe('catalogues', () => {
  it('has every language', () => {
    for (const { id } of LANGS) expect(Object.keys(CATALOGUES[id]).length).toBeGreaterThan(0)
  })

  it('holds the same keys in every language', () => {
    const en = Object.keys(CATALOGUES.en).sort()
    for (const lang of OTHERS) {
      const keys = new Set(Object.keys(CATALOGUES[lang]))
      expect({ lang, missing: en.filter((k) => !keys.has(k)) }).toEqual({ lang, missing: [] })
      const source = new Set(en)
      expect({ lang, extra: [...keys].filter((k) => !source.has(k)).sort() }).toEqual({ lang, extra: [] })
    }
  })

  it('keeps each key in the file named after its namespace', () => {
    const misplaced: string[] = []
    for (const [file, messages] of Object.entries(catalogueFiles())) {
      const ns = file.split('/')[1]
      for (const key of Object.keys(messages)) if (!key.startsWith(ns + '.')) misplaced.push(`${file}: ${key}`)
    }
    expect(misplaced).toEqual([])
  })

  it('reads the same parameters in every language', () => {
    const wrong: string[] = []
    for (const [key, msg] of Object.entries(CATALOGUES.en)) {
      const want = messageParams(msg).join(',')
      for (const lang of OTHERS) {
        const other = CATALOGUES[lang][key]
        if (other !== undefined && messageParams(other).join(',') !== want) wrong.push(`${lang} ${key}`)
      }
    }
    expect(wrong).toEqual([])
  })

  it('leaves no message empty', () => {
    const empty: string[] = []
    for (const { id } of LANGS) for (const [key, msg] of Object.entries(CATALOGUES[id])) if (msg.trim() === '') empty.push(`${id} ${key}`)
    expect(empty).toEqual([])
  })

  it('keeps Simplified and Traditional in their own scripts', () => {
    // A few characters that exist in one script only: enough to catch a file
    // pasted into the wrong directory.
    const hans = Object.entries(CATALOGUES['zh-Hans']).filter(([, m]) => /[這個為們時說沒設擊關閉刪讀啟動錄帳應連線碼檔]/.test(m)).map(([k]) => k)
    const hant = Object.entries(CATALOGUES['zh-Hant']).filter(([, m]) => /[这为们时说没设击关闭删读启动录账应连线码档]/.test(m)).map(([k]) => k)
    expect({ hans, hant }).toEqual({ hans: [], hant: [] })
  })
})

describe('code', () => {
  const sources = import.meta.glob(['../**/*.{ts,tsx}', '!../**/*.test.{ts,tsx}'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>

  it('only asks for keys that exist', () => {
    const missing: string[] = []
    const namespaces = new Set(Object.keys(CATALOGUES.en).map((k) => k.split('.')[0]))
    // t('key'…), tr('key'…), and fields that hold a key: fooKey: 'key', key: 'key'.
    const call = /(?:\bt|\btr|\btranslate\([^,]+,|Key:|\bkey:)\s*\(?\s*'([a-z][A-Za-z0-9]*\.[A-Za-z0-9_.]+)'/g
    for (const [file, text] of Object.entries(sources)) {
      for (const m of text.matchAll(call)) {
        // A prefix ('usage.col.' + name) is checked where its names are listed;
        // a dotted value outside every namespace (a config path) is not a key.
        if (m[1].endsWith('.') || !namespaces.has(m[1].split('.')[0])) continue
        if (!(m[1] in CATALOGUES.en)) missing.push(`${file}: ${m[1]}`)
      }
    }
    expect(missing).toEqual([])
  })
})

describe('messages', () => {
  const en = (n: number) => (n === 1 ? 'one' : 'other')

  it('fills parameters', () => {
    expect(formatMessage('Copy {what}', { what: 'id' }, en)).toBe('Copy id')
    expect(formatMessage('{a} and {b}', { a: 1, b: 'two' }, en)).toBe('1 and two')
    expect(formatMessage('keeps {missing}', {}, en)).toBe('keeps {missing}')
  })

  it('chooses plural forms', () => {
    const msg = '{n, plural, =0 {no agents} one {# agent} other {# agents}}'
    expect(formatMessage(msg, { n: 0 }, en)).toBe('no agents')
    expect(formatMessage(msg, { n: 1 }, en)).toBe('1 agent')
    expect(formatMessage(msg, { n: 12 }, en)).toBe('12 agents')
    expect(formatMessage('{n, plural, other {# 个智能体}}', { n: 1 }, () => 'other')).toBe('1 个智能体')
  })

  it('chooses select branches and nests parameters', () => {
    const msg = '{mode, select, file {Saved to {path}} other {Applied}}'
    expect(formatMessage(msg, { mode: 'file', path: 'a.yaml' }, en)).toBe('Saved to a.yaml')
    expect(formatMessage(msg, { mode: 'live' }, en)).toBe('Applied')
  })

  it('leaves other braces alone', () => {
    expect(formatMessage('Send {"ok": true} or ${ENV}', {}, en)).toBe('Send {"ok": true} or ${ENV}')
    expect(formatMessage('a } b { c', {}, en)).toBe('a } b { c')
  })

  it('falls back to English, then to the key', () => {
    expect(translate('zh-Hant', 'common.save')).toBe('儲存')
    expect(translate('zh-Hans', 'no.such.key')).toBe('no.such.key')
  })

  it('detects the language from the browser', () => {
    expect(detectLang(['fr-FR', 'zh-HK', 'en'])).toBe('zh-Hant')
    expect(detectLang(['zh-CN'])).toBe('zh-Hans')
    expect(detectLang(['zh'])).toBe('zh-Hans')
    expect(detectLang(['zh-Hant-CN'])).toBe('zh-Hant')
    expect(detectLang(['zh-Hans-HK'])).toBe('zh-Hans')
    expect(detectLang(['zh-TW'])).toBe('zh-Hant')
    expect(detectLang(['en-GB'])).toBe('en')
    expect(detectLang(['ja', 'de'])).toBe('en')
  })
})
