import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { AppShell } from './AppShell'
import { LocaleProvider } from '../../lib/i18n'

function renderShell() {
  return render(
    <LocaleProvider>
      <AppShell
        sessions={<div data-testid="zone-sessions">SESSIONS</div>}
        conversation={<div data-testid="zone-conversation">CONVO</div>}
        workspace={<div data-testid="zone-workspace">WORK</div>}
      />
    </LocaleProvider>,
  )
}

beforeEach(() => localStorage.clear())

describe('AppShell', () => {
  it('renders all three named zones', () => {
    renderShell()
    expect(screen.getByTestId('zone-sessions')).toBeInTheDocument()
    expect(screen.getByTestId('zone-conversation')).toBeInTheDocument()
    expect(screen.getByTestId('zone-workspace')).toBeInTheDocument()
  })

  it('collapsing the sessions zone sets the collapsed flag', async () => {
    const user = userEvent.setup()
    renderShell()
    const grid = screen.getByTestId('app-shell')
    await user.click(screen.getByTestId('collapse-sessions'))
    expect(grid.getAttribute('data-sessions-collapsed')).toBe('true')
  })

  it('persists split sizes to localStorage on resize', () => {
    renderShell()
    const splitter = screen.getByTestId('splitter-left')
    fireEvent.mouseDown(splitter, { clientX: 300 })
    fireEvent.mouseMove(window, { clientX: 360 })
    fireEvent.mouseUp(window)
    // New key is dsc.shell.layout (migrated from dsc.shell.splits)
    expect(localStorage.getItem('dsc.shell.layout')).toBeTruthy()
  })

  it('splitters are focusable and resize with arrow keys', () => {
    renderShell()
    const left = screen.getByTestId('splitter-left')
    expect(left).toHaveAttribute('tabindex', '0')
    expect(left).toHaveAttribute('aria-orientation', 'vertical')
    const shell = screen.getByTestId('app-shell')
    const before = shell.style.getPropertyValue('--col-left')
    left.focus()
    fireEvent.keyDown(left, { key: 'ArrowRight' })
    expect(shell.style.getPropertyValue('--col-left')).not.toBe(before)
  })

  it('double-clicking the left splitter fits the left pane to content width', () => {
    renderShell()
    const aside = screen.getByTestId('zone-sessions-host')
    Object.defineProperty(aside, 'scrollWidth', { configurable: true, value: 333 })
    const shell = screen.getByTestId('app-shell')
    fireEvent.doubleClick(screen.getByTestId('splitter-left'))
    expect(shell.style.getPropertyValue('--col-left')).not.toBe('260px') // tracked content, not default
  })

  it('sets data-dragging on the shell during a splitter drag', () => {
    renderShell()
    const shell = screen.getByTestId('app-shell')
    fireEvent.mouseDown(screen.getByTestId('splitter-left'))
    expect(shell).toHaveAttribute('data-dragging', 'true')
    fireEvent.mouseUp(window)
    expect(shell).not.toHaveAttribute('data-dragging', 'true')
  })

  it('conversation no longer renders floating layout presets', () => {
    renderShell()
    expect(screen.queryByTestId('preset-balanced')).toBeNull()
    expect(screen.queryByRole('group', { name: /layout preset/i })).toBeNull()
  })
})
