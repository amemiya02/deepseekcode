import { describe, it, expect, beforeEach } from 'vitest'
import { loadLayout, saveLayout, applyPreset, DEFAULT_LAYOUT, clampLeft, clampRight, isReviewOpen, type Layout } from './layout'

beforeEach(() => localStorage.clear())

describe('layout persistence', () => {
  it('returns defaults when nothing stored', () => {
    expect(loadLayout()).toEqual(DEFAULT_LAYOUT)
  })
  it('round-trips a saved layout', () => {
    const l: Layout = { left: 300, right: 500, leftCollapsed: true, rightCollapsed: false, preset: 'review', reviewPin: 'open' }
    saveLayout(l)
    expect(loadLayout()).toEqual(l)
  })
  it('migrates the old dsc.shell.splits key (widths only) into the new shape', () => {
    localStorage.setItem('dsc.shell.splits', JSON.stringify({ left: 280, right: 460 }))
    const l = loadLayout()
    expect(l.left).toBe(280)
    expect(l.right).toBe(460)
    expect(l.preset).toBe('balanced')
    expect(l.leftCollapsed).toBe(false)
  })
  it('clamps widths to bounds', () => {
    expect(clampLeft(10)).toBe(236)
    expect(clampLeft(9999)).toBe(420)
    expect(clampRight(10)).toBe(280)
    expect(clampRight(9999)).toBe(760)
  })
  it('applyPreset focus collapses both rails; review widens right + collapses left', () => {
    const base = DEFAULT_LAYOUT
    expect(applyPreset(base, 'focus')).toMatchObject({ leftCollapsed: true, rightCollapsed: true, preset: 'focus' })
    const review = applyPreset(base, 'review')
    expect(review).toMatchObject({ leftCollapsed: true, rightCollapsed: false, preset: 'review' })
    expect(review.right).toBeGreaterThanOrEqual(base.right)
    expect(applyPreset(base, 'balanced')).toMatchObject({ leftCollapsed: false, rightCollapsed: false, preset: 'balanced' })
  })
})

const L = (pin: Layout['reviewPin']): Layout => ({ ...DEFAULT_LAYOUT, reviewPin: pin })

describe('isReviewOpen', () => {
  it('auto follows whether there are changes', () => {
    expect(isReviewOpen(L('auto'), true)).toBe(true)
    expect(isReviewOpen(L('auto'), false)).toBe(false)
  })
  it('open is always visible; closed is never', () => {
    expect(isReviewOpen(L('open'), false)).toBe(true)
    expect(isReviewOpen(L('closed'), true)).toBe(false)
  })
  it('defaults to auto', () => {
    expect(DEFAULT_LAYOUT.reviewPin).toBe('auto')
  })
})
