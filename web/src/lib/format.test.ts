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

describe('relativeDay', () => {
  const now = new Date('2026-06-04T12:00:00Z').getTime()
  it('labels same calendar day as today', () => {
    expect(relativeDay(new Date('2026-06-04T01:00:00Z').getTime(), now)).toBe('today')
  })
  it('labels the previous calendar day as yesterday', () => {
    expect(relativeDay(new Date('2026-06-03T23:00:00Z').getTime(), now)).toBe('yesterday')
  })
  it('labels anything older as earlier', () => {
    expect(relativeDay(new Date('2026-05-30T10:00:00Z').getTime(), now)).toBe('earlier')
  })
})

describe('groupSessionsByDay', () => {
  const now = new Date('2026-06-04T12:00:00Z').getTime()
  it('buckets sessions into today/yesterday/earlier, newest first within a bucket', () => {
    const sessions = [
      sess('old', new Date('2026-05-01T10:00:00Z').getTime()),
      sess('today-early', new Date('2026-06-04T02:00:00Z').getTime()),
      sess('today-late', new Date('2026-06-04T10:00:00Z').getTime()),
      sess('yday', new Date('2026-06-03T10:00:00Z').getTime()),
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
