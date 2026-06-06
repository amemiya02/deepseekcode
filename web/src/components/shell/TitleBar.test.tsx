import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { TitleBar } from './TitleBar'
import { LocaleProvider } from '../../lib/i18n'
import { useThemeStore, setThemeSettings, DEFAULT_THEME_SETTINGS } from '../../lib/theme/store'
import { useLayoutStore } from '../../lib/layoutStore'
import { DEFAULT_LAYOUT, isReviewOpen } from './layout'

beforeEach(() => {
  localStorage.clear()
  setThemeSettings(DEFAULT_THEME_SETTINGS)
  useLayoutStore.setState({ layout: { ...DEFAULT_LAYOUT } })
})

function renderTitleBar(props: Partial<React.ComponentProps<typeof TitleBar>> = {}) {
  return render(
    <LocaleProvider>
      <TitleBar branch="main" {...props} />
    </LocaleProvider>,
  )
}

describe('TitleBar', () => {
  it('renders the branch label', () => {
    renderTitleBar({ branch: 'feat/x' })
    expect(screen.getByText('feat/x')).toBeInTheDocument()
  })

  it('no longer renders the command-palette pill (Cmd+K replaces it)', () => {
    renderTitleBar()
    expect(screen.queryByTestId('open-palette')).toBeNull()
  })

  it('fires onOpenSettings when the settings gear is clicked', async () => {
    const user = userEvent.setup()
    const onOpenSettings = vi.fn()
    renderTitleBar({ onOpenSettings })
    await user.click(screen.getByTestId('open-settings'))
    expect(onOpenSettings).toHaveBeenCalledOnce()
  })

  it('toggles light/dark mode via the mode button', async () => {
    const user = userEvent.setup()
    renderTitleBar()
    // Light is the brand default (spec §3.2); the toggle flips to the dark island.
    expect(useThemeStore.getState().settings.mode).toBe('light')
    await user.click(screen.getByTestId('toggle-mode'))
    expect(useThemeStore.getState().settings.mode).toBe('dark')
  })

  it('does not render the dead theme-cycle button', () => {
    renderTitleBar()
    expect(screen.queryByTestId('cycle-theme')).toBeNull()
    expect(screen.getByTestId('toggle-mode')).toBeInTheDocument() // the working light/dark toggle stays
  })

  it('renders the segmented rail + panel toggles', () => {
    renderTitleBar()
    expect(screen.getByTestId('collapse-sessions')).toBeInTheDocument()
    expect(screen.getByTestId('collapse-workspace')).toBeInTheDocument()
  })

  it('rail toggle flips leftCollapsed in the store', async () => {
    const user = userEvent.setup()
    renderTitleBar()
    expect(useLayoutStore.getState().layout.leftCollapsed).toBe(false)
    await user.click(screen.getByTestId('collapse-sessions'))
    expect(useLayoutStore.getState().layout.leftCollapsed).toBe(true)
  })

  it('panel toggle pins the review pane open', async () => {
    const user = userEvent.setup()
    renderTitleBar({ workspaceHasContent: false })
    await user.click(screen.getByTestId('collapse-workspace'))
    expect(isReviewOpen(useLayoutStore.getState().layout, false)).toBe(true)
  })
})
