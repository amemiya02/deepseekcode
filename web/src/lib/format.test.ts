import { describe, it, expect } from 'vitest'
import { formatPct, formatCNY, formatTokens } from './format'
import { relativeDay, groupSessionsByDay, formatRelativeTime } from './format'

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
  const now = localNoon(2026, 6, 4)
  it('labels timestamps from today as today', () => {
    expect(relativeDay(localHour(2026, 6, 4, 1), now)).toBe('today')
  })
  it('labels timestamps from yesterday as yesterday', () => {
    expect(relativeDay(localHour(2026, 6, 3, 23), now)).toBe('yesterday')
  })
  it('labels within the last 7 days as week', () => {
    expect(relativeDay(localHour(2026, 5, 30, 10), now)).toBe('week')
  })
  it('labels within the last 30 days as month', () => {
    expect(relativeDay(localHour(2026, 5, 18, 10), now)).toBe('month')
  })
  it('labels anything older as older', () => {
    expect(relativeDay(localHour(2026, 3, 1, 10), now)).toBe('older')
  })
})

describe('groupSessionsByDay', () => {
  const now = localNoon(2026, 6, 4)
  const sess = (id: string, updated: number) => ({ id, title: id, turns: 1, updated_at: updated, created_at: updated })
  it('buckets into today/yesterday/week/month/older, newest first within a bucket', () => {
    const sessions = [
      sess('old', localHour(2026, 3, 1, 9)),
      sess('t1', localHour(2026, 6, 4, 9)),
      sess('t2', localHour(2026, 6, 4, 11)),
      sess('y', localHour(2026, 6, 3, 9)),
      sess('w', localHour(2026, 5, 31, 9)),
      sess('m', localHour(2026, 5, 18, 9)),
    ]
    const groups = groupSessionsByDay(sessions, now)
    expect(groups.map((g) => g.key)).toEqual(['today', 'yesterday', 'week', 'month', 'older'])
    expect(groups[0].sessions.map((s) => s.id)).toEqual(['t2', 't1'])
  })
  it('omits empty buckets', () => {
    const groups = groupSessionsByDay([sess('a', now)], now)
    expect(groups.map((g) => g.key)).toEqual(['today'])
  })
})

describe('formatRelativeTime', () => {
  const now = localNoon(2026, 6, 4)
  it('renders sub-minute as now', () => {
    expect(formatRelativeTime(now - 5_000, now)).toBe('now')
  })
  it('renders minutes', () => {
    expect(formatRelativeTime(now - 5 * 60_000, now)).toBe('5m')
  })
  it('renders hours', () => {
    expect(formatRelativeTime(now - 3 * 3_600_000, now)).toBe('3h')
  })
  it('renders days', () => {
    expect(formatRelativeTime(now - 3 * 86_400_000, now)).toBe('3d')
  })
  it('renders a same-year month-day for older timestamps', () => {
    expect(formatRelativeTime(localNoon(2026, 3, 4), now)).toBe('Mar 4')
  })
  it('includes the year when not the current year', () => {
    expect(formatRelativeTime(localNoon(2024, 3, 4), now)).toBe('Mar 4 2024')
  })
  it('never renders a future negative', () => {
    expect(formatRelativeTime(now + 10_000, now)).toBe('now')
  })
})
