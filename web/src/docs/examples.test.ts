import { describe, expect, it } from 'vitest'
import { bundledExamples, findExample } from './examples'

describe('bundled example loops', () => {
  it('loads examples from examples/loops with summaries', () => {
    const all = bundledExamples()
    expect(all.length).toBeGreaterThanOrEqual(11)
    const echo = findExample('echo')
    expect(echo).toBeDefined()
    expect(echo?.file).toBe('echo.ts')
    expect(echo?.summary.length).toBeGreaterThan(0)
    expect(echo?.source).toContain('function loop()')
    expect(findExample('does-not-exist')).toBeUndefined()
  })
})
