import { describe, it, expect, beforeEach } from 'vitest'
import {
  useThemeStore,
  setThemeSettings,
  loadThemeSettings,
  persistThemeSettings,
  DEFAULT_THEME_SETTINGS,
} from './store'

beforeEach(() => {
  localStorage.clear()
  // reset the store to defaults for a clean test
  useThemeStore.setState({ settings: { ...DEFAULT_THEME_SETTINGS } })
})

describe('theme store', () => {
  it('loadThemeSettings returns defaults when localStorage is empty', () => {
    expect(loadThemeSettings()).toEqual(DEFAULT_THEME_SETTINGS)
  })

  it('persist then load round-trips the chosen settings', () => {
    persistThemeSettings({ theme: 'lumen', mode: 'light', density: 'compact', accent: 'emerald' })
    expect(loadThemeSettings()).toEqual({ theme: 'lumen', mode: 'light', density: 'compact', accent: 'emerald' })
  })

  it('falls back to defaults when stored JSON is corrupt', () => {
    localStorage.setItem('dsc.theme', '{not json')
    expect(loadThemeSettings()).toEqual(DEFAULT_THEME_SETTINGS)
  })

  it('the store starts at the default settings', () => {
    expect(useThemeStore.getState().settings).toEqual(DEFAULT_THEME_SETTINGS)
  })

  it('setThemeSettings patches the store and persists', () => {
    setThemeSettings({ accent: 'emerald' })
    expect(useThemeStore.getState().settings.accent).toBe('emerald')
    expect(loadThemeSettings().accent).toBe('emerald')
  })
})
