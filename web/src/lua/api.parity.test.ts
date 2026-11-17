// The prelude table exists twice: here and in the desktop app's
// internal/lua/api.go. This reads the Go source and fails when the two differ,
// so an entry added to one editor cannot be forgotten in the other.
import { describe, expect, it } from 'vitest'
import goSource from '../../../desktop/internal/lua/api.go?raw'
import { preludeEntries, type ApiEntry } from './api'

const GO_KINDS: Record<string, ApiEntry['kind']> = {
  SymbolFunction: 'function',
  SymbolTable: 'table',
  SymbolVariable: 'variable',
  SymbolSnippet: 'snippet',
}

const STR = '"((?:[^"\\\\]|\\\\.)*)"'

function goEntries(): ApiEntry[] {
  const out: ApiEntry[] = []
  for (const line of goSource.split('\n')) {
    if (!line.trimStart().startsWith('{Path: ')) continue
    const field = (name: string) => {
      const m = new RegExp(`${name}: ${STR}`).exec(line)
      return m ? (JSON.parse(`"${m[1]}"`) as string) : undefined
    }
    const kind = /Kind: (\w+)/.exec(line)
    const e: ApiEntry = { path: field('Path') ?? '', signature: field('Signature') ?? '', summary: field('Summary') ?? '', kind: GO_KINDS[kind ? kind[1] : ''] }
    const capability = field('Capability')
    if (capability !== undefined) e.capability = capability
    const insert = field('Insert')
    if (insert !== undefined) e.insert = insert
    if (line.includes('Yields: true')) e.yields = true
    if (line.includes('Maintenance: true')) e.maintenance = true
    out.push(e)
  }
  return out
}

describe('prelude table', () => {
  it('matches the desktop app entry for entry', () => {
    const desktop = goEntries()
    expect(desktop.length).toBeGreaterThan(50)
    expect(preludeEntries.map((e) => e.path)).toEqual(desktop.map((e) => e.path))
    expect(preludeEntries).toEqual(desktop)
  })
})
