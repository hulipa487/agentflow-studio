import { beforeEach, describe, expect, it } from 'vitest'
import { importProfiles, useConnections } from '@/state/connections'

// The desktop shell's first run carries over the old desktop app's saved
// connections.
describe('importProfiles', () => {
  beforeEach(() => useConnections.setState({ profiles: [], active: '' }))

  it('adds the saved connections, through the proxy, keeping only remembered tokens', () => {
    const n = importProfiles(
      [
        { name: 'prod', url: 'http://10.0.0.5:8080', token: 't1' },
        { name: 'local', url: 'unix:///run/agentflow/admin.sock', token: 'gone', forget_token: true },
        { url: 'http://10.0.0.5:8080', token: 'duplicate' },
      ],
      'local',
    )
    const { profiles, active } = useConnections.getState()
    expect(n).toBe(2)
    expect(profiles.map((p) => p.name)).toEqual(['prod', 'local'])
    expect(profiles.every((p) => p.viaProxy)).toBe(true)
    expect(profiles[0]).toMatchObject({ token: 't1', forgetToken: false })
    expect(profiles[1]).toMatchObject({ token: undefined, forgetToken: true })
    expect(active).toBe('local')
  })

  it('names a connection the old app left unnamed, and leaves an existing list alone', () => {
    importProfiles([{ url: 'http://runtime.lan:8080' }])
    expect(useConnections.getState().profiles[0].name).toBe('runtime.lan:8080')
    expect(importProfiles([{ url: 'http://other:8080' }])).toBe(0)
    expect(useConnections.getState().profiles).toHaveLength(1)
  })
})
