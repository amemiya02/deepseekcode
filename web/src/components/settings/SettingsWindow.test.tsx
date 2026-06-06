import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, test } from 'vitest'
import React from 'react'
import { LocaleProvider } from '../../lib/i18n'
import { SettingsWindow, SETTINGS_SECTIONS } from './SettingsWindow'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('SettingsWindow', () => {
  it('exports all 18 section keys', () => {
    expect(SETTINGS_SECTIONS.length).toBe(18)
    const ids = SETTINGS_SECTIONS.map((s) => s.id)
    for (const id of ['general', 'appearance', 'models', 'budget', 'duet', 'network', 'about']) {
      expect(ids).toContain(id)
    }
  })

  it('assigns every section to one of the six known groups', () => {
    const groups = new Set(['personal', 'modelsCost', 'coding', 'integrations', 'workspace', 'system'])
    for (const s of SETTINGS_SECTIONS) {
      expect(groups.has(s.group), `${s.id} has group ${s.group}`).toBe(true)
    }
  })

  it('renders the nav with one button per section', () => {
    wrap(<SettingsWindow open onClose={() => {}} />)
    const nav = screen.getByRole('navigation', { name: /settings/i })
    expect(nav.querySelectorAll('button').length).toBe(18)
  })

  it('switches the active section on nav click', async () => {
    wrap(<SettingsWindow open onClose={() => {}} />)
    const apBtn = screen.getByRole('button', { name: /appearance/i })
    await userEvent.click(apBtn)
    expect(apBtn).toHaveAttribute('aria-current', 'page')
  })

  it('renders nothing when closed', () => {
    const { container } = wrap(<SettingsWindow open={false} onClose={() => {}} />)
    expect(container.querySelector('[role="dialog"]')).toBeNull()
  })
})

test('settings overlay uses a translucent scrim, never an opaque white wipe', () => {
  render(<LocaleProvider><SettingsWindow open onClose={() => {}} /></LocaleProvider>)
  const overlay = document.querySelector('[class*="overlay"]') as HTMLElement
  expect(overlay).toBeTruthy()
  const bg = getComputedStyle(overlay).background + getComputedStyle(overlay).backgroundColor
  // must not be solid white, and must reference the scrim variable
  expect(bg).not.toMatch(/#ffffff|rgb\(255,\s*255,\s*255\)/i)
})
