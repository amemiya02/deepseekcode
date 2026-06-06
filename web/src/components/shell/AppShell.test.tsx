import { render, screen, fireEvent, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { AppShell } from './AppShell'
import { LocaleProvider } from '../../lib/i18n'
import { useLayoutStore } from '../../lib/layoutStore'
import { DEFAULT_LAYOUT } from './layout'

function renderShell(props: { workspaceHasContent?: boolean } = {}) {
  return render(
    <LocaleProvider>
      <AppShell
        sessions={<div data-testid="zone-sessions">SESSIONS</div>}
        conversation={<div data-testid="zone-conversation">CONVO</div>}
        workspace={<div data-testid="zone-workspace">WORK</div>}
        workspaceHasContent={props.workspaceHasContent}
      />
    </LocaleProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  useLayoutStore.setState({ layout: { ...DEFAULT_LAYOUT } })
})

describe('AppShell', () => {
  it('renders rail + conversation always; workspace only with content', () => {
    renderShell({ workspaceHasContent: true })
    expect(screen.getByTestId('zone-sessions')).toBeInTheDocument()
    expect(screen.getByTestId('zone-conversation')).toBeInTheDocument()
    expect(screen.getByTestId('zone-workspace')).toBeInTheDocument()
  })

  it('hides the review pane by default (auto, no changes) → 2 columns', () => {
    renderShell({ workspaceHasContent: false })
    expect(screen.queryByTestId('zone-workspace')).toBeNull()
    expect(screen.getByTestId('app-shell').getAttribute('data-workspace-collapsed')).toBe('true')
  })

  it('auto-reveals the review pane when changes appear', () => {
    renderShell({ workspaceHasContent: true })
    expect(screen.getByTestId('zone-workspace')).toBeInTheDocument()
    expect(screen.getByTestId('app-shell').getAttribute('data-workspace-collapsed')).toBe('false')
  })

  it('reflects sessions-collapsed from the store', () => {
    renderShell()
    expect(screen.getByTestId('app-shell').getAttribute('data-sessions-collapsed')).toBe('false')
    act(() => { useLayoutStore.getState().toggleLeft() })
    expect(screen.getByTestId('app-shell').getAttribute('data-sessions-collapsed')).toBe('true')
  })

  it('the workspace pin (store) reveals the pane even without changes', () => {
    renderShell({ workspaceHasContent: false })
    expect(screen.queryByTestId('zone-workspace')).toBeNull()
    act(() => { useLayoutStore.getState().toggleRight(false) })
    expect(screen.getByTestId('zone-workspace')).toBeInTheDocument()
  })

  it('no longer renders the floating collapse tabs (moved to the title bar)', () => {
    renderShell()
    expect(screen.queryByTestId('collapse-sessions')).toBeNull()
    expect(screen.queryByTestId('collapse-workspace')).toBeNull()
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
