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
    expect(localStorage.getItem('dsc.shell.splits')).toBeTruthy()
  })
})
