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
    const s = { theme: 'lumen', mode: 'light', density: 'compact', accent: 'emerald', uiFont: 'system', codeFont: 'sf-mono', transcriptVerbosity: 'normal' } as const
    persistThemeSettings(s)
    expect(loadThemeSettings()).toEqual(s)
  })

  it('defaults include the brand fonts (plex / jetbrains)', () => {
    expect(DEFAULT_THEME_SETTINGS.uiFont).toBe('plex')
    expect(DEFAULT_THEME_SETTINGS.codeFont).toBe('jetbrains')
  })

  it('migrates older stored settings (missing fonts) by spreading defaults first', () => {
    // A pre-P3.5 user has no uiFont/codeFont in localStorage.
    localStorage.setItem('dsc.theme', JSON.stringify({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'rose' }))
    const loaded = loadThemeSettings()
    expect(loaded.uiFont).toBe('plex')
    expect(loaded.codeFont).toBe('jetbrains')
    expect(loaded.accent).toBe('rose') // stored value preserved
    expect(loaded.mode).toBe('dark')
  })

  it('setThemeSettings can patch a font without wiping mode/accent', () => {
    setThemeSettings({ mode: 'dark', accent: 'emerald' })
    setThemeSettings({ uiFont: 'system' })
    const s = useThemeStore.getState().settings
    expect(s.uiFont).toBe('system')
    expect(s.mode).toBe('dark')
    expect(s.accent).toBe('emerald')
    expect(s.codeFont).toBe('jetbrains') // untouched default
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
