import { describe, it, expect, beforeEach } from 'vitest'
import { DEFAULT_THEME_SETTINGS, setThemeSettings, useThemeStore } from './store'

beforeEach(() => { localStorage.clear(); setThemeSettings({ ...DEFAULT_THEME_SETTINGS }) })

describe('theme store transcriptVerbosity', () => {
  it('defaults to "normal"', () => {
    expect(DEFAULT_THEME_SETTINGS.transcriptVerbosity).toBe('normal')
  })
  it('persists a change', () => {
    setThemeSettings({ transcriptVerbosity: 'verbose' })
    expect(useThemeStore.getState().settings.transcriptVerbosity).toBe('verbose')
  })
})
