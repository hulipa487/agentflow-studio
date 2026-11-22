// Bundled example loops from the repository root examples/loops/*.lua.
// Compiled in just like docs/, so developers can inspect examples offline
// and open any example loop directly in the Builder.
import { hasMessage, t } from '@/i18n'

const rawExamples = import.meta.glob<string>('../../../examples/loops/*.lua', {
  query: '?raw',
  import: 'default',
  eager: true,
})

const ROOT = '../../../examples/loops/'

export interface ExampleLoop {
  slug: string
  file: string
  title: string
  summary: string
  source: string
}

function extractSummary(source: string): string {
  for (const rawLine of source.replace(/\r\n?/g, '\n').split('\n')) {
    const trimmed = rawLine.trim()
    if (trimmed === '') continue
    if (!trimmed.startsWith('--')) break
    const body = trimmed.slice(2).trim()
    if (body === '' || body.startsWith('af-prelude-version:') || body.startsWith('af-import:')) continue
    return body.replace(/^Example loop:\s*/i, '')
  }
  return ''
}

const ALL_EXAMPLES: ExampleLoop[] = Object.keys(rawExamples)
  .filter((key) => key.startsWith(ROOT))
  .sort()
  .map((key) => {
    const file = key.slice(ROOT.length)
    const slug = file.replace(/\.lua$/i, '')
    const source = rawExamples[key].replace(/\r\n?/g, '\n')
    return {
      slug,
      file,
      title: file,
      summary: extractSummary(source),
      source,
    }
  })

const BY_SLUG = new Map(ALL_EXAMPLES.map((ex) => [ex.slug, ex]))

export function bundledExamples(): readonly ExampleLoop[] {
  return ALL_EXAMPLES
}

export function findExample(slug: string): ExampleLoop | undefined {
  return BY_SLUG.get(slug)
}

export function exampleSummary(ex: ExampleLoop): string {
  const key = 'docs.example.' + ex.slug
  return hasMessage(key) ? t(key) : ex.summary
}
