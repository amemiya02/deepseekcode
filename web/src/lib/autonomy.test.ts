import { describe, it, expect } from 'vitest'
import { ORDER, MODES, nextMode } from './autonomy'

describe('autonomy', () => {
  it('has 4 modes in order', () => expect(ORDER).toEqual(['ask', 'auto-edit', 'plan', 'yolo']))
  it('cycles', () => expect(nextMode('ask')).toBe('auto-edit'))
  it('wraps', () => expect(nextMode('yolo')).toBe('ask'))
  it('has labels+descriptions', () => expect(MODES['plan']).toMatchObject({ label: expect.any(String), desc: expect.any(String) }))
})
