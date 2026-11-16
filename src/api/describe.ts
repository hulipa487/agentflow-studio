import { transportOf } from './client'

/** Says what an address will reach before connecting. ok=false means it cannot work. */
export function describeURL(raw: string, proxy: { available: boolean; platform: string }): { text: string; ok: boolean } {
  const s = raw.trim()
  if (s === '') {
    return { text: "The runtime's HTTP address (gateway.listen, default :8080); the admin API is served there under /admin/.", ok: true }
  }
  const kind = transportOf(s)
  if (kind !== 'http' && !proxy.available) {
    return { text: 'Sockets and pipes need Studio’s local proxy. Run this app with `npm run dev` or `npm start`, or use HTTP.', ok: false }
  }
  if (kind === 'unix') {
    const path = s.slice('unix://'.length)
    if (!path.startsWith('/') && !path.startsWith('.') && !(path.length > 1 && path[1] === ':')) {
      return { text: 'A Unix socket URL needs a path, e.g. unix:///run/agentflow/admin.sock.', ok: false }
    }
    return { text: `Unix socket ${path} · runtime.admin.socket. Owner-only; the token is still required.`, ok: true }
  }
  if (kind === 'npipe') {
    if (proxy.platform !== 'win32') {
      return { text: 'Named pipes are only available on Windows; use HTTP or a Unix socket here.', ok: false }
    }
    return { text: 'Named pipe on this machine · runtime.admin.socket. Owner-only; the token is still required.', ok: true }
  }
  let u: URL
  try {
    u = new URL(s.includes('://') ? s : 'http://' + s)
  } catch {
    return { text: 'Not a URL. Use http://host:port, unix:///path or npipe://name.', ok: false }
  }
  if (!u.host) return { text: 'Not a URL. Use http://host:port, unix:///path or npipe://name.', ok: false }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') {
    return { text: `Unknown scheme ${u.protocol}//. Use http, https, unix or npipe.`, ok: false }
  }
  const host = u.hostname
  const local = host === 'localhost' || host.startsWith('127.') || host === '[::1]' || host === '::1'
  const where = local ? 'this machine' : 'a remote host: keep it on TLS or a private network'
  return { text: `${u.protocol === 'https:' ? 'HTTPS' : 'HTTP'} to ${u.host} · ${where}.`, ok: true }
}
