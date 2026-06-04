import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { App } from './App'
import { useThemeStore, DEFAULT_THEME_SETTINGS } from './lib/theme/store'

beforeEach(() => {
  localStorage.clear()
  useThemeStore.setState({ settings: { ...DEFAULT_THEME_SETTINGS } })
})

describe('App shell composition', () => {
  it('renders the three workspace zones', () => {
    render(<App />)
    expect(screen.getByTestId('zone-sessions')).toBeInTheDocument()
    expect(screen.getByTestId('zone-conversation')).toBeInTheDocument()
    expect(screen.getByTestId('zone-workspace')).toBeInTheDocument()
  })

  it('applies theme tokens to :root via ThemeProvider', () => {
    render(<App />)
    expect(document.documentElement.style.getPropertyValue('--bg')).toContain('oklch')
  })

  it('opens the command palette when the titlebar trigger is clicked', async () => {
    const user = userEvent.setup()
    render(<App />)
    expect(screen.queryByPlaceholderText(/Search commands/)).toBeNull()
    await user.click(screen.getByTestId('open-palette'))
    expect(screen.getByPlaceholderText(/Search commands/)).toBeInTheDocument()
  })

  it('opens the command palette on Cmd+K', () => {
    render(<App />)
    fireEvent.keyDown(window, { key: 'k', metaKey: true })
    expect(screen.getByPlaceholderText(/Search commands/)).toBeInTheDocument()
  })
})
