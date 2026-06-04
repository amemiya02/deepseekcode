import { render, act } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ThemeProvider } from './ThemeProvider'
import { setThemeSettings, useThemeStore, DEFAULT_THEME_SETTINGS } from '../../lib/theme/store'

beforeEach(() => {
  localStorage.clear()
  useThemeStore.setState({ settings: { ...DEFAULT_THEME_SETTINGS } })
  document.documentElement.removeAttribute('style')
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-mode')
  document.documentElement.removeAttribute('data-density')
})

describe('ThemeProvider', () => {
  it('writes semantic CSS variables onto :root on mount', () => {
    render(<ThemeProvider><div>child</div></ThemeProvider>)
    const root = document.documentElement
    // Brand light is the default (spec §3.2): exact-hex surface + the one accent.
    expect(root.style.getPropertyValue('--bg')).toBe('#f5f6f8')
    expect(root.style.getPropertyValue('--accent')).toBe('#4d6bfe')
  })

  it('sets data-theme/-mode/-density attributes from the store', () => {
    render(<ThemeProvider><div>child</div></ThemeProvider>)
    const root = document.documentElement
    expect(root.getAttribute('data-theme')).toBe('graphite')
    expect(root.getAttribute('data-mode')).toBe('light')
    expect(root.getAttribute('data-density')).toBe('comfortable')
  })

  it('updates the applied tokens when settings change', () => {
    render(<ThemeProvider><div>child</div></ThemeProvider>)
    // The single brand accent no longer varies by picker, but mode swaps the
    // whole palette (light default → dark terminal island), so --bg re-applies.
    const before = document.documentElement.style.getPropertyValue('--bg')
    act(() => setThemeSettings({ mode: 'dark' }))
    const after = document.documentElement.style.getPropertyValue('--bg')
    expect(after).not.toBe(before)
    expect(after).toBe('#0b0d13')
  })

  it('renders its children', () => {
    const { getByText } = render(<ThemeProvider><span>hello</span></ThemeProvider>)
    expect(getByText('hello')).toBeInTheDocument()
  })
})
