import { describe, expect, it } from 'vitest'
import type { Channel } from '@/api/types'
import { buildDraft, channelAgents, formFor, keysPlaceholder, type ChannelForm } from './model'

const base: Channel = {
  index: 0,
  name: 'compat',
  type: 'openai',
  agent: '',
  mode: '',
  path: '/v1/',
  timeout: '',
  async: false,
  deliver_url: '',
  allow_users: null,
  media_allow: null,
  media_max_bytes: 0,
  has_token: false,
  has_secret: false,
  has_secret_token: false,
  token_ref: '',
  secret_ref: '',
  secret_token_ref: '',
  live: true,
}

describe('openai channel form', () => {
  it('starts a new channel on shared-key auth and the metadata journal', () => {
    const f = formFor(null, ['coder'])
    expect(f.auth).toBe('key')
    expect(f.journal).toBe('metadata')
  })

  it('builds a draft with the agent list and no single agent', () => {
    const f = { ...formFor(null, ['coder']), name: 'compat', type: 'openai', agents: 'coder, helper', keys: '${COMPAT_API_KEY}', maxRequestBytes: '1048576', exposeReasoning: true, timeout: '90s' }
    expect(buildDraft(f).draft).toEqual({
      name: 'compat',
      type: 'openai',
      agent: '',
      agents: ['coder', 'helper'],
      auth: 'key',
      keys: ['${COMPAT_API_KEY}'],
      journal: 'metadata',
      expose_reasoning: true,
      timeout: '90s',
      max_request_bytes: 1048576,
    })
  })

  it('does not ask for an agent, unlike the other types', () => {
    const f = { ...formFor(null, []), name: 'x' }
    expect(buildDraft({ ...f, type: 'webhook' }).error).toBe('choose an agent')
    expect(buildDraft({ ...f, type: 'openai' }).error).toBeUndefined()
  })

  it('refuses a request limit that is not a whole number', () => {
    const f = { ...formFor(null, ['a']), name: 'x', type: 'openai', maxRequestBytes: '1.5' }
    expect(buildDraft(f).error).toBe('max request bytes must be a whole number, 0 or more')
  })

  it('opens the agent: shorthand as a one-agent list', () => {
    const f = formFor({ ...base, agent: 'coder', auth: 'user_token' }, ['coder'])
    expect(f.agents).toBe('coder')
    expect(f.auth).toBe('user_token')
    // The file states no journal: a blank draft keeps whatever is stored.
    expect(f.journal).toBe('')
    expect(buildDraft(f).draft).toEqual({ name: 'compat', type: 'openai', agent: '', path: '/v1/', agents: ['coder'], auth: 'user_token' })
  })

  it('names the stored key references without their values', () => {
    expect(keysPlaceholder(null)).toBe('${COMPAT_API_KEY}')
    expect(keysPlaceholder({ ...base, has_keys: true })).toBe('leave blank to keep the stored keys')
    expect(keysPlaceholder({ ...base, has_keys: true, key_refs: ['${A}', 'cred:b'] })).toBe('stored ${A}, cred:b — leave blank to keep')
  })

  it('lists every agent a channel routes to', () => {
    expect(channelAgents({ ...base, agents: ['a', 'b'] })).toBe('a, b')
    expect(channelAgents({ ...base, type: 'webhook', agent: 'echo' })).toBe('echo')
  })
})

describe('qq channel form', () => {
  const qqForm = (over: Partial<ChannelForm> = {}): ChannelForm => ({
    ...formFor(null, ['assistant']),
    name: 'qq',
    type: 'qq',
    appID: '1024',
    ...over,
  })

  it('starts on the websocket gateway with the default scenes', () => {
    const f = formFor(null, ['assistant'])
    expect(f.qqMode).toBe('websocket')
    expect(f.scenes).toEqual(['c2c', 'group', 'guild'])
  })

  it('builds a draft with app id, mode and scenes in display order, and no path in websocket mode', () => {
    const r = buildDraft(qqForm({ scenes: ['guild', 'c2c'], path: '/webhook/qq/', secret: '${QQ_BOT_SECRET}' }))
    expect(r.error).toBeUndefined()
    expect(r.draft).toMatchObject({ type: 'qq', mode: 'websocket', app_id: '1024', scenes: ['c2c', 'guild'], secret: '${QQ_BOT_SECRET}' })
    expect(r.draft?.path).toBeUndefined()
  })

  it('sends the markdown switch only when it is on', () => {
    expect(buildDraft(qqForm({ markdown: true })).draft?.markdown).toBe(true)
    expect(buildDraft(qqForm()).draft?.markdown).toBeUndefined()
    expect(formFor({ ...base, name: 'qq', path: '', type: 'qq', markdown: true }, ['assistant']).markdown).toBe(true)
  })

  it('keeps the path in webhook mode', () => {
    const r = buildDraft(qqForm({ qqMode: 'webhook', path: '/webhook/qq/main/' }))
    expect(r.draft?.path).toBe('/webhook/qq/main/')
  })

  it('asks for an app id and at least one scene', () => {
    expect(buildDraft(qqForm({ appID: ' ' })).error).toMatch(/AppID/)
    expect(buildDraft(qqForm({ scenes: [] })).error).toMatch(/scene/)
  })

  it('opens a stored channel with its own scenes and mode', () => {
    const f = formFor(
      { ...base, name: 'qq', path: '', type: 'qq', mode: 'webhook', app_id: '42', scenes: ['DM'], api_base: 'https://proxy.example.com' },
      ['assistant'],
    )
    expect(f).toMatchObject({ qqMode: 'webhook', appID: '42', scenes: ['dm'], apiBase: 'https://proxy.example.com' })
  })
})
