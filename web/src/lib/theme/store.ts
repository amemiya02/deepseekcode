import { create } from 'zustand'
import type { Theme, Mode, Density, Accent } from './tokens'

export interface ThemeSettings {
  theme: Theme
  mode: Mode
  density: Density
  accent: Accent
  /** UI font option id (see UI_FONTS in tokens.ts). 'plex' = brand default. */
  uiFont: string
  /** Code font option id (see CODE_FONTS in tokens.ts). 'jetbrains' = brand default. */
  codeFont: string
}

export const DEFAULT_THEME_SETTINGS: ThemeSettings = {
  theme: 'graphite',
  // Light-first: the brand default is the light palette (spec §3.2); the dark
  // terminal-island palette ships but is opt-in.
  mode: 'light',
  density: 'comfortable',
  accent: 'indigo',
  // Brand fonts (IBM Plex Sans / JetBrains Mono) — see UI_FONTS/CODE_FONTS.
  uiFont: 'plex',
  codeFont: 'jetbrains',
}

const KEY = 'dsc.theme'

export function loadThemeSettings(): ThemeSettings {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULT_THEME_SETTINGS }
    const parsed = JSON.parse(raw) as Partial<ThemeSettings>
    return { ...DEFAULT_THEME_SETTINGS, ...parsed }
  } catch {
    return { ...DEFAULT_THEME_SETTINGS }
  }
}

export function persistThemeSettings(s: ThemeSettings): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(s))
  } catch {
    /* storage unavailable (private mode) — ignore */
  }
}

interface ThemeStore {
  settings: ThemeSettings
}

// Zustand store usable from both components (via the hook) and plain code
// (via getState/setState). Initial value reads localStorage once at module init.
export const useThemeStore = create<ThemeStore>(() => ({
  settings: loadThemeSettings(),
}))

export function setThemeSettings(next: Partial<ThemeSettings>): void {
  const merged = { ...useThemeStore.getState().settings, ...next }
  useThemeStore.setState({ settings: merged })
  persistThemeSettings(merged)
}
