// The local proxy: the browser talks to this origin, and this process talks
// to the runtime. That removes the need for a runtime.admin.cors_origins
// entry and makes Unix sockets and Windows named pipes reachable from a
// browser tab.
//
//   browser  ──  /__af/admin/api/...   X-AF-Target: http://127.0.0.1:8080
//   proxy    ──  http://127.0.0.1:8080/admin/api/...
//
// It is deliberately narrow:
//   - only paths under /admin/ are forwarded, to the target named per request;
//   - the X-AF-Target header is required. A page on another origin cannot set
//     it without a CORS preflight, and this handler never answers one, so the
//     proxy cannot be driven by a site you happen to have open;
//   - requests whose Host is not a loopback name are refused (DNS rebinding);
//   - nothing is stored: the bearer token passes through and is never logged.
import http from 'node:http'
import https from 'node:https'

export const PROXY_PREFIX = '/__af'

const LOOPBACK = /^(localhost|127(?:\.\d{1,3}){3}|\[::1\])(:\d+)?$/i
const FORWARD_REQ = ['authorization', 'content-type', 'accept']
const FORWARD_RES = ['content-type', 'cache-control', 'content-disposition']

function refuse(res, status, error) {
  res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
  res.end(JSON.stringify({ ok: false, error }))
}

// parseTarget turns the address the operator typed into request options.
export function parseTarget(raw) {
  const t = String(raw || '').trim()
  if (!t) throw new Error('instance URL is required')
  if (t.startsWith('unix://')) {
    const socketPath = t.slice('unix://'.length)
    if (!socketPath) throw new Error('a Unix socket URL needs a path, e.g. unix:///run/agentflow/admin.sock')
    return { kind: 'unix', socketPath, display: t }
  }
  if (t.startsWith('npipe://') || t.startsWith('pipe://') || t.startsWith('\\\\.\\pipe\\')) {
    let name = t
    if (t.startsWith('npipe://')) name = t.slice('npipe://'.length)
    else if (t.startsWith('pipe://')) name = t.slice('pipe://'.length)
    const socketPath = name.startsWith('\\\\.\\pipe\\') ? name : '\\\\.\\pipe\\' + name.replace(/^\/+/, '')
    return { kind: 'npipe', socketPath, display: t }
  }
  const u = new URL(t.includes('://') ? t : 'http://' + t)
  if (u.protocol !== 'http:' && u.protocol !== 'https:') {
    throw new Error(`unknown scheme ${u.protocol}//; use http, https, unix or npipe`)
  }
  return { kind: 'tcp', url: u, display: u.origin }
}

export function createProxyHandler({ allowHosts = [] } = {}) {
  const hostOK = (host) => !host || LOOPBACK.test(host) || allowHosts.includes(host.replace(/:\d+$/, ''))

  // Returns true when the request was handled here.
  return function handle(req, res) {
    const url = req.url || ''
    if (!url.startsWith(PROXY_PREFIX + '/')) return false

    if (!hostOK(req.headers.host)) {
      refuse(res, 403, 'the local proxy only answers loopback hosts')
      return true
    }
    const site = req.headers['sec-fetch-site']
    if (site && site !== 'same-origin' && site !== 'none') {
      refuse(res, 403, 'cross-site requests are refused')
      return true
    }
    if (req.method === 'OPTIONS') {
      refuse(res, 403, 'preflight refused: this proxy is same-origin only')
      return true
    }

    const rest = url.slice(PROXY_PREFIX.length)
    if (rest === '/__ping') {
      res.writeHead(200, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
      res.end(JSON.stringify({ ok: true, proxy: true, platform: process.platform }))
      return true
    }
    if (!rest.startsWith('/admin/')) {
      refuse(res, 404, 'only /admin/ is proxied')
      return true
    }
    const rawTarget = req.headers['x-af-target']
    if (!rawTarget || Array.isArray(rawTarget)) {
      refuse(res, 400, 'X-AF-Target header is required')
      return true
    }
    let target
    try {
      target = parseTarget(rawTarget)
    } catch (err) {
      refuse(res, 400, err.message)
      return true
    }

    const headers = {}
    for (const h of FORWARD_REQ) if (req.headers[h]) headers[h] = req.headers[h]
    const opts = { method: req.method, path: rest, headers }
    let client = http
    if (target.kind === 'tcp') {
      opts.protocol = target.url.protocol
      opts.hostname = target.url.hostname
      opts.port = target.url.port || (target.url.protocol === 'https:' ? 443 : 80)
      headers.host = target.url.host
      if (target.url.protocol === 'https:') client = https
    } else {
      opts.socketPath = target.socketPath
      headers.host = 'localhost'
    }

    const upstream = client.request(opts, (up) => {
      const out = { 'Cache-Control': 'no-store' }
      for (const h of FORWARD_RES) if (up.headers[h]) out[h] = up.headers[h]
      // Streams (the log tail, the playground turn) must not be buffered.
      out['X-Accel-Buffering'] = 'no'
      res.writeHead(up.statusCode || 502, out)
      if (typeof res.flushHeaders === 'function') res.flushHeaders()
      up.on('data', (chunk) => res.write(chunk))
      up.on('end', () => res.end())
      up.on('error', () => res.end())
    })
    upstream.on('error', (err) => {
      if (res.headersSent) return res.end()
      const why = err.code === 'ECONNREFUSED' ? 'connection refused'
        : err.code === 'ENOENT' ? 'no such socket'
        : err.code === 'EACCES' ? 'permission denied on the socket'
        : err.code === 'ENOTFOUND' ? 'host not found'
        : err.code === 'EINVAL' && target.kind !== 'tcp' ? 'not a usable socket path (too long, or not a socket)'
        : err.message
      refuse(res, 502, `cannot reach ${target.display}: ${why}`)
    })
    // The browser going away (navigation, Stop, a closed tab) ends the
    // upstream call too, so an abandoned stream does not linger.
    res.on('close', () => upstream.destroy())
    req.pipe(upstream)
    return true
  }
}
