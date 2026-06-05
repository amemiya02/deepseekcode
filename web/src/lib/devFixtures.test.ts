import { describe, it, expect } from 'vitest'
import { fixtures, demoCommands, demoModels, sessionsFixture } from './devFixtures'
import { groupSessionsByDay } from './format'

describe('devFixtures', () => {
  it('exports a chat fixture', () => {
    expect(fixtures['chat']).toBeDefined()
    expect(Array.isArray(fixtures['chat'])).toBe(true)
  })

  it('demoCommands is a non-empty array with SlashCommand shape', () => {
    expect(Array.isArray(demoCommands)).toBe(true)
    expect(demoCommands.length).toBeGreaterThan(0)
    const cmd = demoCommands[0]
    expect(typeof cmd.name).toBe('string')
    expect(typeof cmd.description).toBe('string')
    expect(['builtin', 'custom', 'mcp', 'skill']).toContain(cmd.kind)
  })

  it('demoModels has at least two entries with id+label', () => {
    expect(Array.isArray(demoModels)).toBe(true)
    expect(demoModels.length).toBeGreaterThanOrEqual(2)
    for (const m of demoModels) {
      expect(typeof m.id).toBe('string')
      expect(typeof m.label).toBe('string')
    }
  })

  it('demoModels includes deepseek-chat and deepseek-reasoner', () => {
    const ids = demoModels.map((m) => m.id)
    expect(ids).toContain('deepseek-chat')
    expect(ids).toContain('deepseek-reasoner')
  })

  it('sessionsFixture returns 7 sessions', () => {
    const now = new Date('2026-06-05T12:00:00Z').getTime()
    const sessions = sessionsFixture(now)
    expect(sessions).toHaveLength(7)
  })

  it('sessionsFixture spans all day buckets', () => {
    const now = new Date('2026-06-05T12:00:00Z').getTime()
    const sessions = sessionsFixture(now)
    const groups = groupSessionsByDay(sessions, now)
    const keys = groups.map((g) => g.key)
    expect(keys).toContain('today')
    expect(keys).toContain('yesterday')
    expect(keys).toContain('week')
    expect(keys).toContain('month')
    expect(keys).toContain('older')
  })
})
