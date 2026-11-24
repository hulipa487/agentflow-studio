import { afterEach, describe, expect, it, vi } from 'vitest'
import { Client } from './client'

describe('Studio Telegram test & Event Center client APIs', () => {
  const originalFetch = globalThis.fetch

  afterEach(() => {
    globalThis.fetch = originalFetch
  })

  it('calls POST /admin/api/studio/channels/telegram/test and unwraps bot info', async () => {
    const mockFetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ok: true,
          bot: {
            bot_id: 9901,
            username: 'helper_bot',
            first_name: 'Helper',
            can_join_groups: true,
            can_read_all_group_messages: false,
            privacy_mode_enabled: true,
          },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    globalThis.fetch = mockFetch as unknown as typeof fetch

    const client = new Client({ url: 'http://127.0.0.1:8080', token: 'tok', viaProxy: false })
    const bot = await client.testTelegramToken({ index: 0, token: 'abc' })
    expect(bot.username).toBe('helper_bot')
    expect(bot.privacy_mode_enabled).toBe(true)

    const [url, init] = mockFetch.mock.calls[0]
    expect(String(url)).toBe('http://127.0.0.1:8080/admin/api/studio/channels/telegram/test')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('queries studioEvents and studioSessionTrace with URL parameters', async () => {
    const mockFetch = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            ok: true,
            events: [
              {
                id: 'e1',
                ts: 1700000000,
                direction: 'out',
                status: 'ignored',
                channel: 'telegram',
                chat: '-1001',
                sender: '-1001',
                agent: 'group_bot',
                session_id: 'group_bot|telegram:-1001',
                text: 'not_mentioned',
                has_trace: true,
                outcome: 'ignored',
              },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            ok: true,
            session: 'group_bot|telegram:-1001',
            events: [
              { seq: 1, ts: '2026-01-01T00:00:00Z', agent: 'group_bot', session: 'group_bot|telegram:-1001', kind: 'turn.skip', text: 'not_mentioned', ok: true },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
    globalThis.fetch = mockFetch as unknown as typeof fetch

    const client = new Client({ url: 'http://127.0.0.1:8080', token: 'tok', viaProxy: false })
    const events = await client.studioEvents({ channel: 'telegram', status: 'ignored', limit: 50 })
    expect(events).toHaveLength(1)
    expect(events[0].outcome).toBe('ignored')
    expect(String(mockFetch.mock.calls[0][0])).toBe('http://127.0.0.1:8080/admin/api/studio/events?channel=telegram&status=ignored&limit=50')

    const trace = await client.studioSessionTrace('group_bot|telegram:-1001')
    expect(trace.session).toBe('group_bot|telegram:-1001')
    expect(trace.events[0].kind).toBe('turn.skip')
    expect(String(mockFetch.mock.calls[1][0])).toContain('/admin/api/studio/session-trace?session=group_bot')
  })
})
