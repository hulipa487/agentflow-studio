import { describe, expect, it } from 'vitest'
import type { MemoryBackend } from '@/api/types'
import { backendForm, backendInUse, backendTarget, buildBackendDraft, buildProfileDraft, parseCount, profileForm, storeSummary } from './helpers'

const backend = (b: Partial<MemoryBackend>): MemoryBackend => ({
  name: 'b',
  provider: 'sqlite',
  has_url: false,
  has_api_key: false,
  other_keys: [],
  profiles: [],
  agents: [],
  implicit: false,
  live: true,
  ...b,
})

describe('backend drafts', () => {
  it('sends only the keys the provider reads', () => {
    const f = { ...backendForm(null), name: ' vec ', provider: 'sqlite', path: './v.db', url: 'postgres://x', dim: '1536', vectorScanLimit: '5000' }
    expect(buildBackendDraft(f)).toEqual({ name: 'vec', provider: 'sqlite', path: './v.db', vector_scan_limit: 5000 })
    const q = { ...backendForm(null), name: 'qd', provider: 'qdrant', url: '${QDRANT_URL}', apiKey: 'cred:qdrant', collection: 'docs', dim: '768', path: './ignored' }
    expect(buildBackendDraft(q)).toEqual({ name: 'qd', provider: 'qdrant', url: '${QDRANT_URL}', api_key: 'cred:qdrant', collection: 'docs', dim: 768 })
  })

  it('keeps a stored secret unless it is replaced or cleared', () => {
    const f = { ...backendForm(backend({ name: 'qd', provider: 'qdrant', has_url: true, has_api_key: true })) }
    expect(buildBackendDraft(f)).toEqual({ name: 'qd', provider: 'qdrant' })
    expect(buildBackendDraft({ ...f, clearApiKey: true })).toEqual({ name: 'qd', provider: 'qdrant', clear_api_key: true })
  })

  it('refuses a count that is not a whole number', () => {
    expect(() => buildBackendDraft({ ...backendForm(null), provider: 'volatile', maxEntries: '1e3' })).toThrow('Max entries must be a whole number')
    expect(parseCount(' 12 ', 'x')).toBe(12)
    expect(parseCount('', 'x')).toBe(0)
  })

  it('says where a backend points without its secrets', () => {
    expect(backendTarget(backend({}))).toBe('./data/agentflow.db')
    expect(backendTarget(backend({ provider: 'postgres', has_url: true, url_display: 'postgres://app:xxxxx@db/mem' }))).toBe('postgres://app:xxxxx@db/mem')
    expect(backendTarget(backend({ provider: 'qdrant', has_url: true, url_ref: '${Q}', collection: 'docs' }))).toBe('${Q} · docs')
    expect(backendTarget(backend({ provider: 'volatile', max_entries: 10 }))).toBe('max 10 entries')
  })

  it('counts the built-in profile as a user only while an agent is on it', () => {
    const main = backend({ name: 'main_db', profiles: ['conversational'], agents: ['chat'] })
    expect(backendInUse(main, [])).toBe(true)
    expect(backendInUse({ ...main, agents: [] }, [])).toBe(false)
    expect(backendInUse({ ...main, agents: [] }, ['chat'])).toBe(true)
  })
})

describe('profile drafts', () => {
  it('starts a new profile like the built-in one, on main_db', () => {
    const f = profileForm(null, ['vectors', 'main_db'])
    expect(f.stores.map((s) => [s.name, s.backend])).toEqual([
      ['dialogue', 'main_db'],
      ['facts', 'main_db'],
    ])
    expect(buildProfileDraft({ ...f, name: 'chat' })).toEqual({
      name: 'chat',
      stores: [
        { name: 'dialogue', backend: 'main_db', table: 'dialogue', retention: '30d', window: 1000, requires: ['kv'] },
        { name: 'facts', backend: 'main_db', table: 'facts', retention: 'forever', requires: ['kv', 'text_search'] },
      ],
      write: ['plugin:routing_table'],
      recall: 'plugin:recency',
    })
  })

  it('lets a pool replace the scope and splits the write list', () => {
    const f = profileForm(null, ['main_db'])
    f.stores = [{ ...f.stores[1], scope: 'agent', pool: ' team ' }]
    f.write = ['plugin:routing_table, ./plugins/custom_tap.ts']
    const d = buildProfileDraft({ ...f, name: 'p' })
    expect(d.stores[0]).toMatchObject({ pool: 'team' })
    expect(d.stores[0].scope).toBeUndefined()
    expect(d.write).toEqual(['plugin:routing_table', './plugins/custom_tap.ts'])
    expect(() => buildProfileDraft({ ...f, stores: [{ ...f.stores[0], window: 'lots' }] })).toThrow('Store "facts": window must be a whole number')
  })

  it('summarises a store in one line', () => {
    expect(storeSummary({ name: 'dialogue', backend: 'main_db', table: 'd', retention: '30d', window: 200, requires: ['kv'] })).toBe('main_db.d · 30d · window 200 · kv')
    expect(storeSummary({ name: 's', backend: 'v', pool: 'team', scope: 'agent' })).toBe('v · pool team')
  })
})
