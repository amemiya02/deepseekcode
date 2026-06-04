import { describe, it, expect } from 'vitest'
import { formatPct, formatCNY, formatTokens } from './format'
import { relativeDay, groupSessionsByDay } from './format'
import type { Session } from './api'

describe('formatPct', () => {
  it('renders a 0..1 ratio as a rounded percent', () => {
    expect(formatPct(0.731)).toBe('73%')
    expect(formatPct(1)).toBe('100%')
    expect(formatPct(0)).toBe('0%')
  })
  it('returns an em dash for null/undefined/NaN', () => {
    expect(formatPct(null)).toBe('—')
    expect(formatPct(undefined)).toBe('—')
    expect(formatPct(NaN)).toBe('—')
  })
})

describe('formatCNY', () => {
  it('renders 4-dp yuan with a ¥ prefix', () => {
    expect(formatCNY(0.024)).toBe('¥0.0240')
    expect(formatCNY(0)).toBe('¥0.0000')
  })
  it('returns ¥— for null', () => {
    expect(formatCNY(null)).toBe('¥—')
  })
})

describe('formatTokens', () => {
  it('thousands-separates integers', () => {
    expect(formatTokens(16908)).toBe('16,908')
    expect(formatTokens(0)).toBe('0')
  })
  it('returns an em dash for null', () => {
    expect(formatTokens(null)).toBe('—')
  })
})

function sess(id: string, updatedMs: number): Session {
  return { id, title: id, turns: 1, updated_at: updatedMs, created_at: updatedMs }
}

// Build timezone-safe anchors using local-time setHours so tests are correct
// on any host timezone (UTC, UTC+8, UTC+9, etc.).
function localNoon(year: number, month: number, day: number): number {
  const d = new Date(year, month - 1, day, 12, 0, 0, 0)
  return d.getTime()
}
function localHour(year: number, month: number, day: number, hour: number): number {
  const d = new Date(year, month - 1, day, hour, 0, 0, 0)
  return d.getTime()
}

describe('relativeDay', () => {
  // Use local-time anchors so tests pass on any timezone.
  const now = localNoon(2026, 6, 4)
  it('labels same calendar day as today', () => {
    expect(relativeDay(localHour(2026, 6, 4, 1), now)).toBe('today')
  })
  it('labels the previous calendar day as yesterday', () => {
    expect(relativeDay(localHour(2026, 6, 3, 23), now)).toBe('yesterday')
  })
  it('labels anything older as earlier', () => {
    expect(relativeDay(localHour(2026, 5, 30, 10), now)).toBe('earlier')
  })

  // Explicit timezone-boundary test: a session at local 01:00 on the same
  // calendar day must be 'today' even when that hour is UTC-yesterday.
  // This would fail if startOfDay() used UTC midnight instead of local midnight.
  it('uses local midnight as the day boundary (timezone-safe)', () => {
    const nowMs = localNoon(2026, 6, 4)
    // Local 01:00 same day → today
    expect(relativeDay(localHour(2026, 6, 4, 1), nowMs)).toBe('today')
    // Local 23:00 previous day → yesterday
    expect(relativeDay(localHour(2026, 6, 3, 23), nowMs)).toBe('yesterday')
  })
})

describe('groupSessionsByDay', () => {
  // Use local-time anchors so tests pass on any timezone.
  const now = localNoon(2026, 6, 4)
  it('buckets sessions into today/yesterday/earlier, newest first within a bucket', () => {
    const sessions = [
      sess('old', localHour(2026, 5, 1, 10)),
      sess('today-early', localHour(2026, 6, 4, 2)),
      sess('today-late', localHour(2026, 6, 4, 10)),
      sess('yday', localHour(2026, 6, 3, 10)),
    ]
    const groups = groupSessionsByDay(sessions, now)
    expect(groups.map((g) => g.key)).toEqual(['today', 'yesterday', 'earlier'])
    expect(groups[0].sessions.map((s) => s.id)).toEqual(['today-late', 'today-early'])
    expect(groups[1].sessions.map((s) => s.id)).toEqual(['yday'])
    expect(groups[2].sessions.map((s) => s.id)).toEqual(['old'])
  })
  it('omits empty buckets', () => {
    const groups = groupSessionsByDay([sess('a', now)], now)
    expect(groups.map((g) => g.key)).toEqual(['today'])
  })
})
