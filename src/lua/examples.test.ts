// Port of the desktop Studio's internal/lua/examples_test.go.
import { describe, expect, it } from 'vitest'
import { analyzeSource } from './analysis'
import { SeverityError } from './token'

// testdata/ mirrors the core repository's examples/loops (by way of the
// desktop Studio's testdata); refresh it when those change. The examples must
// all parse and pass the entry checks; warnings are logged so a rule that is
// too eager shows up here.
const examples = import.meta.glob<string>('./testdata/*.lua', { query: '?raw', import: 'default', eager: true })

describe('example loops', () => {
  const files = Object.keys(examples).sort()

  it('finds the examples', () => {
    expect(files.length).toBeGreaterThan(0)
  })

  it.each(files)('%s lints without errors', (path) => {
    const { diagnostics } = analyzeSource(examples[path], {
      preludeVersion: 1,
      capabilities: null, // the examples do not carry capability metadata
    })
    const errors: string[] = []
    for (const d of diagnostics) {
      const line = `line ${d.line + 1}: ${d.message} (${d.code})`
      if (d.severity === SeverityError) errors.push(line)
      else console.info(`${path}: warning ${line}`)
    }
    expect(errors).toEqual([])
  })
})
