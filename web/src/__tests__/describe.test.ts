import { describe, expect, it } from 'vitest'
import { transportOf } from '@/api/client'
import { describeURL } from '@/api/describe'
import { paletteMatch } from '@/components/shell/CommandPalette'

const proxy = { available: true, platform: 'linux' }

// The cases the Fyne desktop app asserted (TestDescribeURL), kept since its removal.
describe('describeURL', () => {
  const cases: [string, boolean, string, string][] = [
    ['http://127.0.0.1:8080', true, 'this machine', 'http'],
    ['127.0.0.1:8080', true, 'this machine', 'http'],
    ['https://agentflow.example.com', true, 'HTTPS', 'http'],
    ['ftp://x', false, 'Unknown scheme', 'http'],
    ['unix:///run/agentflow/admin.sock', true, '/run/agentflow/admin.sock', 'unix'],
    ['unix://run/agentflow.sock', false, 'needs a path', 'unix'],
    ['', true, 'gateway.listen', 'http'],
  ]
  for (const [input, ok, contains, transport] of cases) {
    it(JSON.stringify(input), () => {
      const got = describeURL(input, proxy)
      expect(got.ok).toBe(ok)
      expect(got.text).toContain(contains)
      expect(transportOf(input)).toBe(transport)
    })
  }

  it('named pipes are Windows only', () => {
    expect(describeURL('npipe://agentflow-admin', proxy).ok).toBe(false)
    expect(describeURL('npipe://agentflow-admin', { available: true, platform: 'win32' }).ok).toBe(true)
  })

  it('sockets need the local proxy', () => {
    const got = describeURL('unix:///run/agentflow/admin.sock', { available: false, platform: '' })
    expect(got.ok).toBe(false)
    expect(got.text).toContain('local proxy')
  })
})

describe('paletteMatch', () => {
  const users = { title: 'Go to Users', hint: 'Profiles' }
  it('matches a destination by prefix', () => expect(paletteMatch(users, 'use')).toBe(0))
  it('matches the hint', () => expect(paletteMatch(users, 'prof')).toBe(2))
  it('rejects an unrelated query', () => expect(paletteMatch(users, 'zzz')).toBe(-1))
})
