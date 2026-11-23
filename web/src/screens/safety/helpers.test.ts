import { describe, expect, it } from 'vitest'
import { normalizeCategoryAction, parseCategoriesEntry } from './helpers'

describe('classifier category actions', () => {
  it('preserves model case in redirect actions while lowercasing keywords', () => {
    expect(normalizeCategoryAction(' ALLOW ')).toBe('allow')
    expect(normalizeCategoryAction(' REDIRECT: DeepSeek-Safe ')).toBe('redirect:DeepSeek-Safe')
    expect(
      parseCategoriesEntry(`
        safe: ALLOW
        harassment: Review
        medical: REDIRECT: DeepSeek-Safe
      `),
    ).toEqual({
      safe: 'allow',
      harassment: 'review',
      medical: 'redirect:DeepSeek-Safe',
    })
  })
})
