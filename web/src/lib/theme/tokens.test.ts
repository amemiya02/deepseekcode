import { describe, it, expect } from 'vitest'
import { buildTokens, THEMES, ACCENTS, type Theme, type Mode, type Density } from './tokens'

// Every semantic token key from spec §4.2 must be emitted for any combination.
const REQUIRED_KEYS = [
  '--bg', '--surface', '--elevated', '--card', '--overlay',
  '--border', '--border-strong', '--focus-ring',
  '--text', '--text-2', '--text-3', '--text-on-accent',
  '--accent', '--accent-text', '--accent-weak',
  '--success', '--danger', '--warning', '--info',
  '--add-bg', '--add-fg', '--del-bg', '--del-fg',
  '--type-display', '--type-title', '--type-body', '--type-ui', '--type-label', '--type-mono',
  '--s-1', '--s-2', '--s-3', '--s-4', '--s-5', '--s-6', '--s-7', '--s-8',
  '--r-sm', '--r-md', '--r-lg', '--r-xl',
  '--ease-standard', '--dur-fast', '--dur-base', '--dur-slow',
] as const

const themes: Theme[] = ['graphite', 'lumen', 'halo']
const modes: Mode[] = ['light', 'dark', 'hc']
const densities: Density[] = ['comfortable', 'compact']

describe('buildTokens', () => {
  it('exposes 3 themes and 8 named accents', () => {
    expect(THEMES.map((t) => t.id)).toEqual(themes)
    expect(ACCENTS).toHaveLength(8)
  })

  it('emits every required §4.2 token for every theme/mode/density/accent', () => {
    for (const theme of themes)
      for (const mode of modes)
        for (const density of densities)
          for (const accent of ACCENTS) {
            const tok = buildTokens({ theme, mode, density, accent: accent.id })
            for (const key of REQUIRED_KEYS) {
              expect(tok[key], `${theme}/${mode}/${density}/${accent.id} missing ${key}`).toBeTruthy()
            }
          }
  })

  it('uses the single brand accent #4d6bfe regardless of the chosen accent (spec §3)', () => {
    const indigo = buildTokens({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'indigo' })
    const terracotta = buildTokens({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'terracotta' })
    // The brand has exactly ONE accent; the accent picker no longer recolors the chrome.
    expect(indigo['--accent']).toBe('#4d6bfe')
    expect(terracotta['--accent']).toBe('#4d6bfe')
  })

  it('anchors the brand light palette to the exact spec §3.2 hex', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(t['--bg']).toBe('#f5f6f8')
    expect(t['--surface']).toBe('#ffffff')
    expect(t['--text']).toBe('#0d1016')
    expect(t['--border']).toBe('#e4e7ec')
    expect(t['--accent']).toBe('#4d6bfe')
    expect(t['--accent-weak']).toBe('#eef1ff')
    expect(t['--accent-mist']).toBe('#f5f7ff')
    expect(t['--glow-accent']).toBe('0 6px 18px -6px rgba(77,107,254,.6)')
  })

  it('anchors the dark terminal-island palette to the exact spec §3.3 hex', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'indigo' })
    expect(t['--bg']).toBe('#0b0d13')
    expect(t['--surface']).toBe('#11141d')
    expect(t['--text']).toBe('#d6dae4')
    expect(t['--border']).toBe('#1d2230')
    expect(t['--accent-text']).toBe('#7d97ff')
  })

  it('compact density yields smaller spacing than comfortable', () => {
    const comf = buildTokens({ theme: 'halo', mode: 'light', density: 'comfortable', accent: 'indigo' })
    const comp = buildTokens({ theme: 'halo', mode: 'light', density: 'compact', accent: 'indigo' })
    expect(parseFloat(comp['--s-4'])).toBeLessThan(parseFloat(comf['--s-4']))
  })

  it('high-contrast mode strengthens the border token vs dark', () => {
    const dark = buildTokens({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'indigo' })
    const hc = buildTokens({ theme: 'graphite', mode: 'hc', density: 'comfortable', accent: 'indigo' })
    expect(hc['--border']).not.toBe(dark['--border'])
  })

  it('exposes a hairline divider token and a panel padding token', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(t['--border-hair']).toBeTruthy()
    expect(t['--panel-pad']).toBeTruthy()
    expect(t['--border-hair']).not.toBe(t['--border-strong'])
  })
})
