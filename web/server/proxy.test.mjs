import http from 'node:http'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { createProxyHandler, parseTarget } from './proxy.mjs'

describe('parseTarget', () => {
  it('reads a TCP origin, adding http://', () => {
    const t = parseTarget('127.0.0.1:8080')
    expect(t.kind).toBe('tcp')
    expect(t.display).toBe('http://127.0.0.1:8080')
  })
  it('reads a Unix socket', () => {
    expect(parseTarget('unix:///run/agentflow/admin.sock')).toMatchObject({ kind: 'unix', socketPath: '/run/agentflow/admin.sock' })
  })
  it('reads the three named-pipe spellings', () => {
    for (const raw of ['npipe://agentflow-admin', 'pipe://agentflow-admin', '\\\\.\\pipe\\agentflow-admin']) {
      expect(parseTarget(raw)).toMatchObject({ kind: 'npipe', socketPath: '\\\\.\\pipe\\agentflow-admin' })
    }
  })
  it('refuses other schemes and empty input', () => {
    expect(() => parseTarget('ftp://x')).toThrow(/unknown scheme/)
    expect(() => parseTarget('  ')).toThrow(/required/)
    expect(() => parseTarget('unix://')).toThrow(/needs a path/)
  })
})

describe('proxy handler', () => {
  let upstream, front, upstreamURL, frontURL
  const seen = []

  beforeAll(async () => {
    upstream = http.createServer((req, res) => {
      let body = ''
      req.on('data', (c) => (body += c))
      req.on('end', () => {
        seen.push({ method: req.method, url: req.url, auth: req.headers.authorization, target: req.headers['x-af-target'], body })
        if (req.url === '/admin/api/logs') {
          res.writeHead(200, { 'Content-Type': 'text/event-stream' })
          res.write('data: one\n\n')
          setTimeout(() => res.end('data: two\n\n'), 30)
          return
        }
        res.writeHead(req.url === '/admin/api/missing' ? 404 : 200, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({ ok: true }))
      })
    })
    await new Promise((r) => upstream.listen(0, '127.0.0.1', r))
    upstreamURL = `http://127.0.0.1:${upstream.address().port}`
    const handle = createProxyHandler()
    front = http.createServer((req, res) => {
      if (!handle(req, res)) res.writeHead(418).end('not proxied')
    })
    await new Promise((r) => front.listen(0, '127.0.0.1', r))
    frontURL = `http://127.0.0.1:${front.address().port}`
  })

  afterAll(async () => {
    await new Promise((r) => front.close(r))
    await new Promise((r) => upstream.close(r))
  })

  const call = (path, init = {}) => fetch(frontURL + path, init)

  it('leaves everything outside its prefix alone', async () => {
    expect((await call('/index.html')).status).toBe(418)
  })

  it('answers the availability probe', async () => {
    const body = await (await call('/__af/__ping')).json()
    expect(body).toMatchObject({ ok: true, proxy: true })
  })

  it('forwards method, path, query, body and the bearer token, but not its own header', async () => {
    seen.length = 0
    const res = await call('/__af/admin/api/studio/agents/echo?persist=true', {
      method: 'PUT',
      headers: { 'X-AF-Target': upstreamURL, Authorization: 'Bearer t0ken', 'Content-Type': 'application/json' },
      body: '{"apply":true}',
    })
    expect(res.status).toBe(200)
    expect(seen[0]).toEqual({ method: 'PUT', url: '/admin/api/studio/agents/echo?persist=true', auth: 'Bearer t0ken', target: undefined, body: '{"apply":true}' })
  })

  it('passes the upstream status through', async () => {
    expect((await call('/__af/admin/api/missing', { headers: { 'X-AF-Target': upstreamURL } })).status).toBe(404)
  })

  it('streams without buffering', async () => {
    const res = await call('/__af/admin/api/logs', { headers: { 'X-AF-Target': upstreamURL } })
    expect(res.headers.get('content-type')).toBe('text/event-stream')
    expect(await res.text()).toBe('data: one\n\ndata: two\n\n')
  })

  it('requires the target header', async () => {
    const res = await call('/__af/admin/api/state')
    expect(res.status).toBe(400)
    expect((await res.json()).error).toMatch(/X-AF-Target/)
  })

  it('proxies only /admin/', async () => {
    expect((await call('/__af/webhook/local/', { headers: { 'X-AF-Target': upstreamURL } })).status).toBe(404)
  })

  it('refuses a preflight, so another origin can never set the target header', async () => {
    const res = await call('/__af/admin/api/state', {
      method: 'OPTIONS',
      headers: { Origin: 'http://evil.example', 'Access-Control-Request-Method': 'GET', 'Access-Control-Request-Headers': 'x-af-target' },
    })
    expect(res.status).toBe(403)
    expect(res.headers.get('access-control-allow-origin')).toBeNull()
  })

  it('refuses cross-site fetches and non-loopback hosts', async () => {
    const cross = await call('/__af/admin/api/state', { headers: { 'X-AF-Target': upstreamURL, 'Sec-Fetch-Site': 'cross-site' } })
    expect(cross.status).toBe(403)
    const rebound = await new Promise((resolve) => {
      const u = new URL(frontURL)
      http.get({ host: u.hostname, port: u.port, path: '/__af/admin/api/state', headers: { Host: 'evil.example', 'X-AF-Target': upstreamURL } }, (r) => {
        r.resume()
        resolve(r.statusCode)
      })
    })
    expect(rebound).toBe(403)
  })

  it('reports an unreachable runtime as 502 with the reason', async () => {
    const res = await call('/__af/admin/api/state', { headers: { 'X-AF-Target': 'http://127.0.0.1:1' } })
    expect(res.status).toBe(502)
    expect((await res.json()).error).toMatch(/cannot reach http:\/\/127\.0\.0\.1:1: connection refused/)
  })
})
