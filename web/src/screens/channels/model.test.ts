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
    const r = buildDraft(qqForm({ scenes: ['guild', 'group_all', 'c2c'], path: '/webhook/qq/', secret: '${QQ_BOT_SECRET}' }))
    expect(r.error).toBeUndefined()
    expect(r.draft).toMatchObject({ type: 'qq', mode: 'websocket', app_id: '1024', scenes: ['c2c', 'group_all', 'guild'], secret: '${QQ_BOT_SECRET}' })
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

describe('email channel form', () => {
  const emailForm = (over: Partial<ChannelForm> = {}): ChannelForm => ({
    ...formFor(null, ['assistant']),
    name: 'support-mail',
    type: 'email',
    imapHost: 'imap.example.com',
    smtpHost: 'smtp.example.com',
    username: 'bot@example.com',
    ...over,
  })

  it('defaults to idle mode, standard ports, INBOX, and markSeen on', () => {
    const f = formFor(null, ['assistant'])
    expect(f.emailMode).toBe('idle')
    expect(f.imapPort).toBe('993')
    expect(f.smtpPort).toBe('465')
    expect(f.mailbox).toBe('INBOX')
    expect(f.markSeen).toBe(true)
  })

  it('builds an email draft with ports, allowlists, mark_seen, secrets, and no HTTP path', () => {
    const r = buildDraft(
      emailForm({
        path: '/ignored/',
        emailMode: 'polling',
        imapPort: '143',
        smtpPort: '587',
        password: '${IMAP_PW}',
        smtpUsername: 'mailer@example.com',
        smtpPassword: '${SMTP_PW}',
        fromAddress: 'Support <support@example.com>',
        mailbox: 'Support',
        pollInterval: '45s',
        markSeen: false,
        allowFrom: 'alice@partner.org, bob@partner.org',
        allowDomains: 'example.com, trusted.io',
        mediaAllow: 'image/*, application/pdf',
      }),
    )
    expect(r.error).toBeUndefined()
    expect(r.draft?.path).toBeUndefined()
    expect(r.draft).toMatchObject({
      name: 'support-mail',
      type: 'email',
      mode: 'polling',
      imap_host: 'imap.example.com',
      imap_port: 143,
      smtp_host: 'smtp.example.com',
      smtp_port: 587,
      username: 'bot@example.com',
      password: '${IMAP_PW}',
      smtp_username: 'mailer@example.com',
      smtp_password: '${SMTP_PW}',
      from_address: 'Support <support@example.com>',
      mailbox: 'Support',
      poll_interval: '45s',
      mark_seen: false,
      allow_from: ['alice@partner.org', 'bob@partner.org'],
      allow_domains: ['example.com', 'trusted.io'],
      media_allow: ['image/*', 'application/pdf'],
    })
  })

  it('validates required host/username fields and TCP ports', () => {
    expect(buildDraft(emailForm({ imapHost: ' ' })).error).toMatch(/IMAP host/)
    expect(buildDraft(emailForm({ smtpHost: ' ' })).error).toMatch(/SMTP host/)
    expect(buildDraft(emailForm({ username: ' ' })).error).toMatch(/username/)
    expect(buildDraft(emailForm({ imapPort: '70000' })).error).toMatch(/1 and 65535/)
  })

  it('hydrates an existing email channel into the form cleanly', () => {
    const f = formFor(
      {
        ...base,
        name: 'mail',
        path: '',
        type: 'email',
        mode: 'polling',
        imap_host: 'imap.corp.net',
        imap_port: 993,
        smtp_host: 'smtp.corp.net',
        smtp_port: 465,
        username: 'desk@corp.net',
        smtp_username: 'relay@corp.net',
        from_address: 'Desk <desk@corp.net>',
        mailbox: 'SupportBox',
        poll_interval: '60s',
        mark_seen: false,
        allow_from: ['vip@client.org'],
        allow_domains: ['corp.net'],
      },
      ['assistant'],
    )
    expect(f).toMatchObject({
      type: 'email',
      emailMode: 'polling',
      imapHost: 'imap.corp.net',
      imapPort: '993',
      smtpHost: 'smtp.corp.net',
      smtpPort: '465',
      username: 'desk@corp.net',
      smtpUsername: 'relay@corp.net',
      fromAddress: 'Desk <desk@corp.net>',
      mailbox: 'SupportBox',
      pollInterval: '60s',
      markSeen: false,
      allowFrom: 'vip@client.org',
      allowDomains: 'corp.net',
    })
  })
})
