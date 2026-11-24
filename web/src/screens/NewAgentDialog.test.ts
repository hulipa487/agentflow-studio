import { afterEach, describe, expect, it } from 'vitest'
import type { Template } from '@/api/types'
import { useI18n } from '@/i18n'
import { REMOVED_TEMPLATES, templateDescription, templateTitle, visibleTemplates } from './NewAgentDialog'

function tplOf(name: string, title = name, description = 'Runtime fallback description'): Template {
  return {
    name,
    title,
    description,
    loop: '// af-prelude-version: 2\nfunction loop() {}\n',
  }
}

describe('NewAgentDialog template filtering and i18n', () => {
  afterEach(() => {
    useI18n.getState().setLang('en')
  })

  it('hides retired legacy templates even when an older runtime reports them', () => {
    const legacyNames = [...REMOVED_TEMPLATES]
    const currentNames = ['qq-group-bot-fts', 'qq-group-bot-vec', 'telegram-group-bot-fts', 'telegram-group-bot-vec']
    const mixed = [...legacyNames, ...currentNames].map((n) => tplOf(n))
    expect(visibleTemplates(mixed).map((t) => t.name)).toEqual(currentNames)
    expect(visibleTemplates(null)).toEqual([])
  })

  it('translates template titles and descriptions in en, zh-Hans, and zh-Hant', () => {
    const names = [
      'qq-group-bot-fts',
      'qq-group-bot-vec',
      'telegram-group-bot-fts',
      'telegram-group-bot-vec',
      'qq-group-bot',
      'telegram-group-bot',
    ]

    for (const name of names) {
      const tpl = tplOf(name, 'RAW TITLE', 'RAW DESCRIPTION')
      useI18n.getState().setLang('en')
      const enTitle = templateTitle(tpl)
      const enDesc = templateDescription(tpl)
      expect(enTitle).not.toBe('RAW TITLE')
      expect(enDesc).not.toBe('RAW DESCRIPTION')

      useI18n.getState().setLang('zh-Hans')
      const hansTitle = templateTitle(tpl)
      const hansDesc = templateDescription(tpl)
      expect(hansTitle).not.toBe('RAW TITLE')
      expect(hansTitle).not.toBe(enTitle)
      expect(hansDesc).not.toBe('RAW DESCRIPTION')
      expect(hansDesc).not.toBe(enDesc)

      useI18n.getState().setLang('zh-Hant')
      const hantTitle = templateTitle(tpl)
      const hantDesc = templateDescription(tpl)
      expect(hantTitle).not.toBe('RAW TITLE')
      expect(hantTitle).not.toBe(enTitle)
      expect(hantDesc).not.toBe('RAW DESCRIPTION')
      expect(hantDesc).not.toBe(enDesc)
    }
  })

  it('falls back to runtime title and description for unknown template names', () => {
    useI18n.getState().setLang('zh-Hant')
    const unknown = tplOf('custom-starter', 'Custom Starter', 'Custom starter description')
    expect(templateTitle(unknown)).toBe('Custom Starter')
    expect(templateDescription(unknown)).toBe('Custom starter description')
  })
})
