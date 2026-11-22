import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetDesktopProbesForTests } from '@/lib/desktop'
import {
  hydrateDesktopTokens,
  isKeychainEnabled,
  resetKeychainStateForTests,
  tokenFor,
  useConnections,
} from '@/state/connections'

interface MockDesktopBridgeOptions {
  available: boolean
  backend?: string
  initialSecrets?: Record<string, string>
}

function installDesktopFetchMock(opts: MockDesktopBridgeOptions) {
  const store = new Map<string, string>(Object.entries(opts.initialSecrets ?? {}))
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
    if (url.endsWith('/__af/__ping')) {
      return new Response(
        JSON.stringify({ proxy: 'agentflow-studio', version: '0.1.0', platform: 'windows', shell: 'wails', frameless: true }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    }
    if (url.endsWith('/__af/__desktop/secrets/status')) {
      return new Response(
        JSON.stringify({ ok: true, available: opts.available, backend: opts.backend ?? (opts.available ? 'windows-credman' : 'none') }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    }
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    if (url.endsWith('/__af/__desktop/secrets/get')) {
      if (!opts.available) return new Response(JSON.stringify({ error: 'unavailable' }), { status: 503 })
      const secret = store.get(body.account)
      return new Response(
        JSON.stringify(secret !== undefined ? { ok: true, found: true, secret } : { ok: true, found: false }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    }
    if (url.endsWith('/__af/__desktop/secrets/set')) {
      if (!opts.available) return new Response(JSON.stringify({ error: 'unavailable' }), { status: 503 })
      store.set(body.account, body.secret)
      return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    if (url.endsWith('/__af/__desktop/secrets/delete')) {
      if (!opts.available) return new Response(JSON.stringify({ error: 'unavailable' }), { status: 503 })
      store.delete(body.account)
      return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    return new Response(JSON.stringify({ error: 'unexpected url ' + url }), { status: 404 })
  })
  vi.stubGlobal('fetch', fetchMock)
  return { store, fetchMock }
}

describe('desktop OS keychain token storage', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    resetDesktopProbesForTests()
    resetKeychainStateForTests()
    useConnections.setState({ profiles: [], active: '' })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('migrates legacy plaintext localStorage tokens into OS keychain and scrubs localStorage', async () => {
    const { store } = installDesktopFetchMock({ available: true })
    useConnections.setState({
      profiles: [{ name: 'local:8080', url: 'http://127.0.0.1:8080', token: 'legacy-secret', forgetToken: false, viaProxy: true }],
      active: 'local:8080',
    })

    await hydrateDesktopTokens()

    expect(isKeychainEnabled()).toBe(true)
    expect(store.get('local:8080')).toBe('legacy-secret')
    const profile = useConnections.getState().profiles[0]
    expect(profile.token).toBeUndefined()
    expect(profile.tokenInKeychain).toBe(true)
    expect(tokenFor(profile)).toBe('legacy-secret')

    const persisted = JSON.parse(localStorage.getItem('agentflow-studio:connections') || '{}')
    expect(JSON.stringify(persisted)).not.toContain('legacy-secret')
  })

  it('hydrates existing keychain profiles and clears missing keychain flags', async () => {
    installDesktopFetchMock({ available: true, initialSecrets: { prod: 'prod-secret' } })
    useConnections.setState({
      profiles: [
        { name: 'prod', url: 'https://prod.example:8080', tokenInKeychain: true, forgetToken: false, viaProxy: true },
        { name: 'stale', url: 'https://stale.example:8080', tokenInKeychain: true, forgetToken: false, viaProxy: true },
      ],
      active: 'prod',
    })

    await hydrateDesktopTokens()

    const [prod, stale] = useConnections.getState().profiles
    expect(tokenFor(prod)).toBe('prod-secret')
    expect(prod.tokenInKeychain).toBe(true)
    expect(tokenFor(stale)).toBe('')
    expect(stale.tokenInKeychain).toBeUndefined()
  })

  it('stores newly remembered tokens in OS keychain and deletes them on lock or remove', async () => {
    const { store } = installDesktopFetchMock({ available: true })
    await hydrateDesktopTokens()

    const name = useConnections.getState().remember('http://127.0.0.1:8080', 'fresh-secret', true, true)
    await vi.waitFor(() => expect(store.get(name)).toBe('fresh-secret'))

    const saved = useConnections.getState().profiles[0]
    expect(saved.token).toBeUndefined()
    expect(saved.tokenInKeychain).toBe(true)
    expect(tokenFor(saved)).toBe('fresh-secret')
    expect(localStorage.getItem('agentflow-studio:connections') || '').not.toContain('fresh-secret')

    useConnections.getState().forgetActiveToken()
    await vi.waitFor(() => expect(store.has(name)).toBe(false))
    expect(tokenFor(useConnections.getState().profiles[0])).toBe('')

    // Save again and test profile removal.
    useConnections.getState().remember('http://127.0.0.1:8080', 'second-secret', true, true)
    await vi.waitFor(() => expect(store.get(name)).toBe('second-secret'))
    useConnections.getState().remove(name)
    await vi.waitFor(() => expect(store.has(name)).toBe(false))
    expect(useConnections.getState().profiles).toHaveLength(0)
  })
})
