import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { TitleBar } from './TitleBar'
import { LocaleProvider } from '../../lib/i18n'
import { useThemeStore, setThemeSettings, DEFAULT_THEME_SETTINGS } from '../../lib/theme/store'

beforeEach(() => {
  localStorage.clear()
  setThemeSettings(DEFAULT_THEME_SETTINGS)
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

  it('fires onOpenPalette when the palette trigger is clicked', async () => {
    const user = userEvent.setup()
    const onOpenPalette = vi.fn()
    renderTitleBar({ onOpenPalette })
    await user.click(screen.getByTestId('open-palette'))
    expect(onOpenPalette).toHaveBeenCalledOnce()
  })

  it('toggles light/dark mode via the mode button', async () => {
    const user = userEvent.setup()
    renderTitleBar()
    expect(useThemeStore.getState().settings.mode).toBe('dark')
    await user.click(screen.getByTestId('toggle-mode'))
    expect(useThemeStore.getState().settings.mode).toBe('light')
  })
})
