import { describe, it, expect } from 'vitest'
import { formatPct, formatCNY, formatTokens } from './format'

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
