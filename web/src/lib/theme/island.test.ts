import { describe, it, expect } from 'vitest'
import { buildTokens } from './tokens'

describe('theme-adaptive code island', () => {
  it('light mode uses a LIGHT code surface (not obsidian)', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(t['--island-bg']).not.toBe('#0b0d13')
  })
  it('dark mode keeps a dark code surface', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'indigo' })
    expect(t['--island-bg']).toBe('#0b0d13')
  })
})
