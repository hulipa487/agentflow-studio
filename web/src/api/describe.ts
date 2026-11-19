import { t } from '@/i18n'
import { transportOf } from './client'

/** Says what an address will reach before connecting. ok=false means it cannot work. */
export function describeURL(raw: string, proxy: { available: boolean; platform: string }): { text: string; ok: boolean } {
  const s = raw.trim()
  if (s === '') {
    return { text: t('connection.describeEmpty'), ok: true }
  }
  const kind = transportOf(s)
  if (kind !== 'http' && !proxy.available) {
    return { text: t('connection.describeNeedsProxy'), ok: false }
  }
  if (kind === 'unix') {
    const path = s.slice('unix://'.length)
    if (!path.startsWith('/') && !path.startsWith('.') && !(path.length > 1 && path[1] === ':')) {
      return { text: t('connection.describeUnixNoPath'), ok: false }
    }
    return { text: t('connection.describeUnix', { path }), ok: true }
  }
  if (kind === 'npipe') {
    if (proxy.platform !== 'win32') {
      return { text: t('connection.describePipeWindowsOnly'), ok: false }
    }
    return { text: t('connection.describePipe'), ok: true }
  }
  let u: URL
  try {
    u = new URL(s.includes('://') ? s : 'http://' + s)
  } catch {
    return { text: t('connection.describeNotURL'), ok: false }
  }
  if (!u.host) return { text: t('connection.describeNotURL'), ok: false }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') {
    return { text: t('connection.describeBadScheme', { scheme: u.protocol.replace(/:$/, '') }), ok: false }
  }
  const host = u.hostname
  const local = host === 'localhost' || host.startsWith('127.') || host === '[::1]' || host === '::1'
  return {
    text: t('connection.describeHttp', { scheme: u.protocol === 'https:' ? 'HTTPS' : 'HTTP', host: u.host, where: local ? 'local' : 'remote' }),
    ok: true,
  }
}
