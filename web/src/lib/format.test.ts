import { describe, expect, it } from 'vitest'
import { bytes, duration, percent, protocolName, shortId, timeAgo } from './format.ts'

describe('format', () => {
  it('formats durations', () => {
    expect(duration(null)).toBe('—')
    expect(duration(420)).toBe('420 ms')
    expect(duration(4200)).toBe('4.2 s')
    expect(duration(125_000)).toBe('2m 5s')
  })
  it('formats bytes', () => {
    expect(bytes(512)).toBe('512 B')
    expect(bytes(2048)).toBe('2.0 KB')
    expect(bytes(5 * 1024 * 1024)).toBe('5.0 MB')
  })
  it('shortens ids and percentages', () => {
    expect(shortId('exp_0123456789abcdef')).toBe('01234567')
    expect(percent(1, 4)).toBe('25%')
    expect(percent(1, 0)).toBe('—')
  })
  it('relative time', () => {
    const now = Date.parse('2026-10-09T12:00:00Z')
    expect(timeAgo('2026-10-09T11:59:50Z', now)).toBe('刚刚')
    expect(timeAgo(undefined, now)).toBe('—')
  })
  it('protocol names', () => {
    expect(protocolName('repair-once-v1')).toBe('repair-once-v1')
    expect(protocolName(null)).toBe('single-pass-v1')
  })
})
