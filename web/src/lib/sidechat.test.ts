import { describe, it, expect } from 'vitest'
import { buildReviewPrompt, buildBtwPrompt, isSideChatCommand, SIDECHAT_COMMANDS } from './sidechat'

describe('sidechat helpers', () => {
  it('lists /review and /btw as recognized commands', () => {
    expect(SIDECHAT_COMMANDS.map((c) => c.name)).toEqual(expect.arrayContaining(['/review', '/btw']))
    expect(isSideChatCommand('/review')).toBe(true)
    expect(isSideChatCommand('/btw')).toBe(true)
    expect(isSideChatCommand('/foo')).toBe(false)
  })

  it('buildReviewPrompt frames a diff critique and marks it read-only', () => {
    const p = buildReviewPrompt('look at the auth change')
    expect(p.readOnly).toBe(true)
    expect(p.text.toLowerCase()).toContain('review')
    expect(p.text).toContain('look at the auth change')
  })

  it('buildBtwPrompt produces a read-only side question that forbids edits', () => {
    const p = buildBtwPrompt('what does this function do?')
    expect(p.readOnly).toBe(true)
    expect(p.text).toContain('what does this function do?')
    expect(p.text.toLowerCase()).toMatch(/do not (edit|modify|change)/)
  })
})
