// Port of the desktop Studio's internal/lua/examples_test.go.
import { describe, expect, it } from 'vitest'
import { analyzeSource } from './analysis'
import { SeverityError } from './token'

// Validate the live repository examples directly so loop syntax checks stay
// in sync with every shipped loop and scenario.
const examples = import.meta.glob<string>(['../../../examples/loops/*.lua', '../../../examples/*/loops/*.lua'], { query: '?raw', import: 'default', eager: true })

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
