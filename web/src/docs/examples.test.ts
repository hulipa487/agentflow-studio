import { describe, expect, it } from 'vitest'
import { bundledExamples, findExample } from './examples'

describe('bundled example loops', () => {
  it('ships no standalone example loops', () => {
    expect(bundledExamples()).toEqual([])
    expect(findExample('echo')).toBeUndefined()
    expect(findExample('does-not-exist')).toBeUndefined()
  })
})
