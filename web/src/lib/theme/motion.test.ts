import { describe, it, expect } from 'vitest'
import { MOTION } from './motion'

describe('motion tokens', () => {
  it('matches DeepSeek-GUI timing contract', () => {
    expect(MOTION.micro).toBe(140)
    expect(MOTION.standard).toBe(150)
    expect(MOTION.deep).toBe(300)
    expect(MOTION.pulse).toBe(1800)
    expect(MOTION.shiny).toBe(2400)
  })
  it('defines functional transforms', () => {
    expect(MOTION.cardLift).toBe('translateY(-1px)')
    expect(MOTION.press).toBe('scale(0.985)')
  })
})
