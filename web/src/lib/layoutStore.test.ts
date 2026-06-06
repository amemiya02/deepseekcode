import { describe, it, expect, beforeEach } from 'vitest'
import { useLayoutStore } from './layoutStore'
import { DEFAULT_LAYOUT } from '../components/shell/layout'

beforeEach(() => {
  localStorage.clear()
  useLayoutStore.setState({ layout: { ...DEFAULT_LAYOUT } })
})

describe('layoutStore', () => {
  it('toggleLeft flips leftCollapsed and persists', () => {
    useLayoutStore.getState().toggleLeft()
    expect(useLayoutStore.getState().layout.leftCollapsed).toBe(true)
    expect(localStorage.getItem('dsc.shell.layout')).toContain('"leftCollapsed":true')
  })

  it('toggleRight(false) opens then closes the review pane', () => {
    useLayoutStore.getState().toggleRight(false) // auto+no-changes (closed) → open
    expect(useLayoutStore.getState().layout.reviewPin).toBe('open')
    useLayoutStore.getState().toggleRight(false) // open → closed
    expect(useLayoutStore.getState().layout.reviewPin).toBe('closed')
  })

  it('setLayout accepts a functional updater', () => {
    useLayoutStore.getState().setLayout((prev) => ({ ...prev, left: 333 }))
    expect(useLayoutStore.getState().layout.left).toBe(333)
  })
})
