import { describe, expect, it } from 'vitest'
import { humanCount, humanDuration, orDash, shortID, splitList, utcDay } from '@/lib/format'

// The same cases the desktop app asserts (internal/ui/ui_test.go), so both
// apps print numbers identically.
describe('format', () => {
  it('humanCount', () => {
    const cases: [number, string][] = [
      [0, '0'],
      [999, '999'],
      [9999, '9999'],
      [12_345, '12.3k'],
      [2_000_000, '2M'],
      [1_500_000_000, '1.5B'],
      [-12_345, '-12.3k'],
    ]
    for (const [n, want] of cases) expect(humanCount(n)).toBe(want)
  })

  it('humanDuration', () => {
    expect(humanDuration(42)).toBe('42s')
    expect(humanDuration(3720)).toBe('1h 2m')
    expect(humanDuration(3 * 86400 + 2 * 3600)).toBe('3d 2h')
  })

  it('small helpers', () => {
    expect(orDash('  ')).toBe('—')
    expect(shortID('4f2a91c3d8b14e6f')).toBe('4f2a91c3')
    expect(splitList('pm, researcher\n\n ops ')).toEqual(['pm', 'researcher', 'ops'])
    expect(utcDay(new Date('2026-11-07T23:59:59Z'))).toBe('2026-11-07')
  })
})
