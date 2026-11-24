// Example loops are no longer bundled separately; starter templates live in
// the runtime catalog instead.
import { hasMessage, t } from '@/i18n'

export interface ExampleLoop {
  slug: string
  file: string
  title: string
  summary: string
  source: string
}

const ALL_EXAMPLES: ExampleLoop[] = []

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
