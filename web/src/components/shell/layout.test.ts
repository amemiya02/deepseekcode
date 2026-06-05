import { describe, it, expect, beforeEach } from 'vitest'
import { loadLayout, saveLayout, applyPreset, DEFAULT_LAYOUT, clampLeft, clampRight, type Layout } from './layout'

beforeEach(() => localStorage.clear())

describe('layout persistence', () => {
  it('returns defaults when nothing stored', () => {
    expect(loadLayout()).toEqual(DEFAULT_LAYOUT)
  })
  it('round-trips a saved layout', () => {
    const l: Layout = { left: 300, right: 500, leftCollapsed: true, rightCollapsed: false, preset: 'review' }
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
    expect(clampLeft(10)).toBe(160)
    expect(clampLeft(9999)).toBe(480)
    expect(clampRight(10)).toBe(240)
    expect(clampRight(9999)).toBe(720)
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
