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
    expect(root.style.getPropertyValue('--bg')).toContain('oklch')
    expect(root.style.getPropertyValue('--accent')).toContain('oklch')
  })

  it('sets data-theme/-mode/-density attributes from the store', () => {
    render(<ThemeProvider><div>child</div></ThemeProvider>)
    const root = document.documentElement
    expect(root.getAttribute('data-theme')).toBe('graphite')
    expect(root.getAttribute('data-mode')).toBe('dark')
    expect(root.getAttribute('data-density')).toBe('comfortable')
  })

  it('updates the applied tokens when settings change', () => {
    render(<ThemeProvider><div>child</div></ThemeProvider>)
    const before = document.documentElement.style.getPropertyValue('--accent')
    act(() => setThemeSettings({ accent: 'emerald' }))
    const after = document.documentElement.style.getPropertyValue('--accent')
    expect(after).not.toBe(before)
  })

  it('renders its children', () => {
    const { getByText } = render(<ThemeProvider><span>hello</span></ThemeProvider>)
    expect(getByText('hello')).toBeInTheDocument()
  })
})
