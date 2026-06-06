import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { LocaleProvider } from '../../lib/i18n'
import { SettingsView, SETTINGS_SECTIONS, SETTINGS_GROUPS } from './SettingsWindow'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('SettingsView', () => {
  it('exports all 18 section keys', () => {
    expect(SETTINGS_SECTIONS.length).toBe(18)
    const ids = SETTINGS_SECTIONS.map((s) => s.id)
    for (const id of ['general', 'appearance', 'models', 'budget', 'duet', 'network', 'about']) {
      expect(ids).toContain(id)
    }
  })

  it('assigns every section to one of the six known groups', () => {
    const groups = new Set(SETTINGS_GROUPS.map((g) => g.id))
    for (const s of SETTINGS_SECTIONS) expect(groups.has(s.group)).toBe(true)
  })

  it('is a full-page view, not a modal dialog (no role=dialog / aria-modal)', () => {
    const { container } = wrap(<SettingsView onClose={() => {}} />)
    expect(container.querySelector('[role="dialog"]')).toBeNull()
    expect(container.querySelector('[aria-modal]')).toBeNull()
  })

  it('renders a nav button per section plus the group headers', () => {
    wrap(<SettingsView onClose={() => {}} />)
    const nav = screen.getByRole('navigation', { name: /settings/i })
    expect(nav.querySelectorAll('[data-testid="settings-nav-item"]').length).toBe(18)
    for (const g of SETTINGS_GROUPS) {
      expect(screen.getByText(new RegExp(`^${g.fallback}$`, 'i'))).toBeInTheDocument()
    }
  })

  it('switches the active section on nav click', async () => {
    wrap(<SettingsView onClose={() => {}} />)
    const apBtn = screen.getByRole('button', { name: /appearance/i })
    await userEvent.click(apBtn)
    expect(apBtn).toHaveAttribute('aria-current', 'page')
  })

  it('search filters the nav and hides empty groups', async () => {
    wrap(<SettingsView onClose={() => {}} />)
    await userEvent.type(screen.getByTestId('settings-search'), 'sandbox')
    expect(screen.getByRole('button', { name: /sandbox/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^general$/i })).toBeNull()
    expect(screen.queryByText(/^Personal$/)).toBeNull()
  })

  it('the back button calls onClose', async () => {
    const onClose = vi.fn()
    wrap(<SettingsView onClose={onClose} />)
    await userEvent.click(screen.getByTestId('settings-back'))
    expect(onClose).toHaveBeenCalledOnce()
  })
})
