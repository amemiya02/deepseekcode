import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { SettingsWindow, SETTINGS_SECTIONS } from './SettingsWindow'

describe('SettingsWindow', () => {
  it('exports all 18 section keys', () => {
    expect(SETTINGS_SECTIONS.length).toBe(18)
    const ids = SETTINGS_SECTIONS.map((s) => s.id)
    for (const id of ['general', 'appearance', 'models', 'budget', 'duet', 'network', 'about']) {
      expect(ids).toContain(id)
    }
  })

  it('renders the nav with one button per section', () => {
    render(<SettingsWindow open onClose={() => {}} />)
    const nav = screen.getByRole('navigation', { name: /settings/i })
    expect(nav.querySelectorAll('button').length).toBe(18)
  })

  it('switches the active section on nav click', async () => {
    render(<SettingsWindow open onClose={() => {}} />)
    const apBtn = screen.getByRole('button', { name: /appearance/i })
    await userEvent.click(apBtn)
    expect(apBtn).toHaveAttribute('aria-current', 'page')
  })

  it('renders nothing when closed', () => {
    const { container } = render(<SettingsWindow open={false} onClose={() => {}} />)
    expect(container.querySelector('[role="dialog"]')).toBeNull()
  })
})
