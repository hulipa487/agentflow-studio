import { describe, expect, it } from 'vitest'
import { analyzeSource } from './analysis'
import { SeverityError } from './token'

// Validate the live repository examples directly so loop syntax checks stay
// in sync with every shipped chatbot starter.
const examples = import.meta.glob<string>(['../../../examples/*/loops/*.ts'], {
  query: '?raw',
  import: 'default',
  eager: true,
})

describe('example loops', () => {
  const files = Object.keys(examples).sort()

  it('finds all four chatbot example loops', () => {
    expect(files).toEqual([
      '../../../examples/qq-group-bot-fts/loops/qq-group-bot.ts',
      '../../../examples/qq-group-bot-vec/loops/qq-group-bot.ts',
      '../../../examples/telegram-group-bot-fts/loops/telegram-group-bot.ts',
      '../../../examples/telegram-group-bot-vec/loops/telegram-group-bot.ts',
    ])
  })

  it.each(files)('%s lints without errors', (path) => {
    const { diagnostics } = analyzeSource(examples[path], {
      preludeVersion: 2,
      capabilities: null,
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
