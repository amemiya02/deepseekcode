import { describe, it, expect } from 'vitest'
import { buildTokens } from './tokens'

describe('accent discipline', () => {
  it('keeps brand accent #4d6bfe (not DeepSeek-GUI #0088ff)', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(t['--accent']).toBe('#4d6bfe')
  })
})
