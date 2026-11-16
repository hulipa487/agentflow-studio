import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, Client, encodeBody, featureOff, normalizeURL, parseBody, readSSE } from '@/api/client'
import type { ChatEvent } from '@/api/types'

function sse(chunks: string[]): Response {
  const enc = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      for (const ch of chunks) c.enqueue(enc.encode(ch))
      c.close()
    },
  })
  return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

function json(status: number, body: unknown): Response {
  return new Response(typeof body === 'string' ? body : JSON.stringify(body), { status })
}

afterEach(() => vi.unstubAllGlobals())

describe('client', () => {
  it('sends the bearer token straight to the runtime in direct mode', async () => {
    const fetchMock = vi.fn(async () => json(200, { ok: true, agents: [{ name: 'echo' }] }))
    vi.stubGlobal('fetch', fetchMock)
    const c = new Client({ url: '127.0.0.1:8080/', token: ' secret ', viaProxy: false })
    const agents = await c.agents()
    expect(agents[0].name).toBe('echo')
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('http://127.0.0.1:8080/admin/api/studio/agents')
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer secret')
    expect(init.credentials).toBe('omit')
  })

  it('routes through the local proxy and names the target', async () => {
    const fetchMock = vi.fn(async () => json(200, { ok: true }))
    vi.stubGlobal('fetch', fetchMock)
    const c = new Client({ url: 'unix:///run/agentflow/admin.sock', token: 't', viaProxy: false })
    // A socket can only be reached through the proxy, whatever was asked.
    expect(c.viaProxy).toBe(true)
    expect(c.local).toBe(true)
    await c.state()
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/__af/admin/api/state')
    expect((init.headers as Record<string, string>)['X-AF-Target']).toBe('unix:///run/agentflow/admin.sock')
  })

  it('turns a nil list into an empty one', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { ok: true, agents: null })))
    expect(await new Client({ url: 'http://x', token: 't', viaProxy: false }).agents()).toEqual([])
  })

  it('reports a rejected token', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(401, 'unauthorized\n')))
    const err = await new Client({ url: 'http://x', token: 'bad', viaProxy: false }).ping().catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(401)
    expect(err.message).toBe('admin token rejected: unauthorized')
  })

  it('joins validation errors', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(400, { ok: false, errors: ['name is required', 'unknown agent'] })))
    const err = await new Client({ url: 'http://x', token: 't', viaProxy: false }).channels().catch((e) => e)
    expect(err.message).toBe('name is required; unknown agent')
  })

  it('explains an unreachable runtime, and CORS in direct mode', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch') }))
    vi.stubGlobal('location', { origin: 'http://127.0.0.1:5173' })
    const err = await new Client({ url: 'http://127.0.0.1:1', token: 't', viaProxy: false }).ping().catch((e) => e)
    expect(err.status).toBe(0)
    expect(err.network).toBe(true)
    expect(err.message).toContain('cannot reach http://127.0.0.1:1')
    expect(err.message).toContain('runtime.admin.cors_origins')
  })

  it('reads a playground turn as trace, reply and done events', async () => {
    const stream = [
      ': keepalive\n\n',
      'event: trace\ndata: {"trace":{"seq":1,"kind":"turn.start"}}\n\n',
      // A chunk boundary in the middle of an event must not split it.
      'event: reply\ndata: {"reply":{"text":"hel',
      'lo","status":"final"}}\n\n',
      'event: done\r\ndata: {"type":"done","done":{"thread":"t1","events":2,"timed_out":false}}\r\n\r\n',
    ]
    vi.stubGlobal('fetch', vi.fn(async () => sse(stream)))
    const events: ChatEvent[] = []
    await new Client({ url: 'http://x', token: 't', viaProxy: false }).chat({ agent: 'echo', thread: 't1', text: 'hi' }, (ev) => events.push(ev))
    expect(events.map((e) => e.type)).toEqual(['trace', 'reply', 'done'])
    expect(events[0].trace?.kind).toBe('turn.start')
    expect(events[1].reply?.text).toBe('hello')
    expect(events[2].done?.thread).toBe('t1')
  })

  it('reads the log tail line by line', async () => {
    const lines: string[] = []
    await readSSE(sse(['data: 10:00:00 INF up\n\ndata: 10:00:01 ERR boom\n\n', 'data: tail without blank line\n']), (_e, d) => lines.push(d))
    expect(lines).toEqual(['10:00:00 INF up', '10:00:01 ERR boom', 'tail without blank line'])
  })

  it('sends an empty tenant when removing an every-member grant', async () => {
    const fetchMock = vi.fn(async () => json(200, { ok: true }))
    vi.stubGlobal('fetch', fetchMock)
    await new Client({ url: 'http://x', token: 't', viaProxy: false }).removeGrant('p 1', 'echo', '')
    expect((fetchMock.mock.calls[0] as unknown as [string])[0]).toBe('http://x/admin/api/pools/p%201/grants?agent=echo&tenant=')
  })

  it('recognizes a switched-off subsystem', () => {
    expect(featureOff(new ApiError(503, 'identity layer not enabled'))).toContain('runtime.identity.enabled')
    expect(featureOff(new ApiError(500, 'boom'))).toBeNull()
  })

  it('normalizes addresses', () => {
    expect(normalizeURL(' 127.0.0.1:8080/ ')).toBe('http://127.0.0.1:8080')
    expect(normalizeURL('unix:///run/a.sock')).toBe('unix:///run/a.sock')
  })
})

describe('nanosecond mtimes', () => {
  // The runtime stamps channels.yaml and the safety config in nanoseconds and
  // refuses a write whose stamp differs by a single digit.
  const exact = '1794839575017614971'

  it('survives the round trip digit for digit', async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) =>
      init?.method === 'GET'
        ? new Response(`{"ok":true,"mtime": ${exact},"channels":[]}`, { status: 200 })
        : new Response(`{"ok":true,"mtime":${exact},"channels":[]}`, { status: 200 }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const c = new Client({ url: 'http://x', token: 't', viaProxy: false })
    const list = await c.channels()
    expect(list.mtime).toBe(exact)
    // A plain JSON.parse would have rounded it.
    expect(String(JSON.parse(`{"mtime":${exact}}`).mtime)).not.toBe(exact)

    await c.saveChannel(0, { name: 'local', type: 'webhook', agent: 'echo' }, list.mtime)
    const put = fetchMock.mock.calls[1] as unknown as [string, RequestInit]
    // Sent as a bare number: Go's int64 field rejects a quoted value.
    expect(put[1].body).toContain(`"mtime":${exact}}`)
    expect(put[1].body).not.toContain(`"${exact}"`)

    await c.deleteChannel(2, list.mtime)
    expect((fetchMock.mock.calls[2] as unknown as [string])[0]).toBe(`http://x/admin/api/studio/channels/2?mtime=${exact}`)
  })

  it('leaves second-resolution stamps as numbers', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"ok":true,"mtime":1794839575,"raw":"a: 1","path":"/x","view":{}}', { status: 200 })))
    const f = await new Client({ url: 'http://x', token: 't', viaProxy: false }).config()
    expect(f.mtime).toBe(1794839575)
  })

  it('does not rewrite a look-alike inside a string value', () => {
    const body = encodeBody({ raw: 'note: "mtime":"123"', mtime: exact })
    expect(JSON.parse(body.replace(/"mtime":\d+\}$/, '"mtime":0}')).raw).toBe('note: "mtime":"123"')
    expect(parseBody<{ text: string }>('{"text":"\\"mtime\\": 1794839575017614971"}').text).toBe('"mtime": 1794839575017614971')
  })
})

describe('timeouts', () => {
  it('gives up on a runtime that never answers', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
          }),
      ),
    )
    const pending = new Client({ url: 'http://x', token: 't', viaProxy: false }).state().catch((e) => e)
    await vi.advanceTimersByTimeAsync(61_000)
    const err = await pending
    vi.useRealTimers()
    expect(err).toBeInstanceOf(ApiError)
    expect(err.message).toBe('no answer from http://x within 60s')
  })
})
