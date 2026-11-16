import { describe, expect, it } from 'vitest'
import { LogBuffer, logLevelOf } from './buffer'

const texts = (b: LogBuffer) => b.visible.map((l) => l.text)

describe('logLevelOf', () => {
  it('reads the level from the second field', () => {
    expect(logLevelOf('15:04:05 DEV x')).toBe(0)
    expect(logLevelOf('15:04:05 DBG x')).toBe(1)
    expect(logLevelOf('15:04:05 INF listening addr=:8080')).toBe(2)
    expect(logLevelOf('15:04:05 WRN slow')).toBe(3)
    expect(logLevelOf('15:04:05   ERR boom err=x')).toBe(4)
  })

  it('treats anything unrecognized as INF', () => {
    expect(logLevelOf('')).toBe(2)
    expect(logLevelOf('oneword')).toBe(2)
    expect(logLevelOf('ERR first field is the time')).toBe(2)
    expect(logLevelOf('15:04:05 err lowercase')).toBe(2)
  })
})

describe('LogBuffer', () => {
  it('shows nothing until a commit, then everything at or above the floor', () => {
    const b = new LogBuffer()
    b.push('t DEV trace')
    b.push('t DBG debug')
    b.push('t INF info')
    expect(b.visible).toHaveLength(0)
    expect(b.commit()).toBe(true)
    expect(b.commit()).toBe(false)
    // The default floor is "Debug and up".
    expect(texts(b)).toEqual(['t DBG debug', 't INF info'])
    expect(b.total).toBe(3)
    b.setMinLevel(0)
    expect(b.visible).toHaveLength(3)
    b.setMinLevel(4)
    expect(b.visible).toHaveLength(0)
  })

  it('filters by case-insensitive substring', () => {
    const b = new LogBuffer()
    b.push('t INF turn done agent=assistant')
    b.push('t ERR turn failed agent=echo err=timeout')
    b.commit()
    b.setFilter('  AGENT=Assistant ')
    expect(texts(b)).toEqual(['t INF turn done agent=assistant'])
    b.setFilter('err=')
    expect(texts(b)).toEqual(['t ERR turn failed agent=echo err=timeout'])
    b.push('t INF later agent=echo')
    b.commit()
    expect(b.visible).toHaveLength(1)
    b.setFilter('')
    expect(b.visible).toHaveLength(3)
  })

  it('keeps a bounded buffer and reports what rolled off the view', () => {
    const b = new LogBuffer(5)
    for (let i = 0; i < 5; i++) b.push(`t INF ${i}`)
    b.commit()
    expect(b.takeDropped()).toBe(0)
    for (let i = 5; i < 8; i++) b.push(`t INF ${i}`)
    b.commit()
    expect(b.total).toBe(5)
    expect(texts(b)).toEqual(['t INF 3', 't INF 4', 't INF 5', 't INF 6', 't INF 7'])
    expect(b.takeDropped()).toBe(3)
    expect(b.takeDropped()).toBe(0)
    // Row keys stay unique and ordered across trimming.
    const seqs = b.visible.map((l) => l.seq)
    expect(seqs).toEqual([...seqs].sort((x, y) => x - y))
    expect(new Set(seqs).size).toBe(seqs.length)
  })

  it('bounds the backlog while nothing commits (hidden tab)', () => {
    const b = new LogBuffer(5)
    for (let i = 0; i < 100; i++) b.push(`t INF ${i}`)
    b.commit()
    expect(b.total).toBe(5)
    expect(texts(b).at(-1)).toBe('t INF 99')
  })

  it('pause freezes the view and counts what arrives; resume flushes', () => {
    const b = new LogBuffer()
    b.push('t INF a')
    b.pause()
    // Pausing folds in what had already arrived.
    expect(texts(b)).toEqual(['t INF a'])
    b.push('t INF b')
    b.push('t WRN c')
    b.commit()
    expect(b.pending).toBe(2)
    expect(texts(b)).toEqual(['t INF a'])
    // A filter change while paused searches the snapshot, not the backlog.
    b.setFilter('t')
    expect(texts(b)).toEqual(['t INF a'])
    b.setFilter('')
    b.resume()
    expect(b.pending).toBe(0)
    expect(texts(b)).toEqual(['t INF a', 't INF b', 't WRN c'])
  })

  it('reset starts over for a reconnect but leaves a paused view alone', () => {
    const b = new LogBuffer()
    b.push('t INF old')
    b.commit()
    b.reset()
    expect(b.visible).toHaveLength(0)
    expect(b.total).toBe(0)

    b.push('t INF kept')
    b.pause()
    b.reset()
    b.push('t INF replayed')
    b.commit()
    expect(texts(b)).toEqual(['t INF kept'])
    expect(b.pending).toBe(1)
    b.resume()
    expect(texts(b)).toEqual(['t INF replayed'])
  })

  it('clear forgets everything, paused or not', () => {
    const b = new LogBuffer()
    b.push('t INF a')
    b.pause()
    b.push('t INF b')
    b.commit()
    b.clear()
    expect(b.visible).toHaveLength(0)
    expect(b.pending).toBe(0)
    b.resume()
    expect(b.visible).toHaveLength(0)
  })
})
