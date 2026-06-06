import { describe, it, expect } from 'vitest'
import { isCmdK, matchShortcut } from './shortcuts'

function ev(init: Partial<KeyboardEvent>): KeyboardEvent {
  return new KeyboardEvent('keydown', init)
}

describe('shortcuts', () => {
  it('isCmdK detects Meta+K (mac)', () => {
    expect(isCmdK(ev({ key: 'k', metaKey: true }))).toBe(true)
  })

  it('isCmdK detects Ctrl+K (win/linux)', () => {
    expect(isCmdK(ev({ key: 'k', ctrlKey: true }))).toBe(true)
  })

  it('isCmdK is false for a bare k', () => {
    expect(isCmdK(ev({ key: 'k' }))).toBe(false)
  })

  it('matchShortcut matches case-insensitively with the meta flag', () => {
    expect(matchShortcut(ev({ key: 'K', metaKey: true }), { key: 'k', meta: true })).toBe(true)
    expect(matchShortcut(ev({ key: 'k', metaKey: false }), { key: 'k', meta: true })).toBe(false)
  })
})
