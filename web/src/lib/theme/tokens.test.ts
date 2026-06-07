import { describe, it, expect } from 'vitest'
import { buildTokens, THEMES, ACCENTS, UI_FONTS, CODE_FONTS, type Theme, type Mode, type Density } from './tokens'

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
  '--island-bg', '--island-header', '--island-ink', '--island-accent', '--island-dot',
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

  it('brand default: indigo accent stays pixel-exact #4d6bfe', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(t['--accent']).toBe('#4d6bfe')
    expect(t['--accent-ink']).toBe('#2b46d4')
  })

  it('non-brand accent changes the accent token (emerald ≠ brand blue)', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'emerald' })
    expect(t['--accent']).not.toBe('#4d6bfe')
    // emerald hue (158) → an oklch accent anchored at that hue (anchored to the
    // hue position in the oklch(L C H) syntax, not just any substring '158').
    expect(t['--accent']).toMatch(/oklch\([^)]*\s158[\s/)]/)
  })

  it('dark mode non-indigo accent: derives the dark accent family (color-mix + reduced chroma)', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'emerald' })
    // base accent stays the OKLCH base at the emerald hue
    expect(t['--accent']).toMatch(/oklch\([^)]*\s158[\s/)]/)
    // dark text variant: reduced chroma (0.15 * 0.9 = 0.135) at lighter L
    expect(t['--accent-text']).toBe('oklch(0.74 0.135 158)')
    // weak/mist on dark are color-mix toward the island body, anchored on the base
    expect(t['--accent-weak']).toBe('color-mix(in oklch, oklch(0.62 0.15 158) 22%, #11141d)')
    expect(t['--accent-mist']).toBe('color-mix(in oklch, oklch(0.62 0.15 158) 12%, #0b0d13)')
  })

  it('high-contrast non-indigo accent: brighter focus ring than dark (WCAG-safe on the HC canvas)', () => {
    const hc = buildTokens({ theme: 'graphite', mode: 'hc', density: 'comfortable', accent: 'emerald' })
    const dark = buildTokens({ theme: 'graphite', mode: 'dark', density: 'comfortable', accent: 'emerald' })
    // HC gets a brighter focus ring (higher OKLCH lightness) than plain dark so it
    // clears the 3:1 minimum against the very dark HC canvas.
    expect(hc['--focus-ring']).toBe('oklch(0.78 0.15 158)')
    expect(hc['--focus-ring']).not.toBe(dark['--focus-ring'])
  })

  it('high-contrast indigo keeps the pixel-exact brand accent but brightens the focus ring', () => {
    const hc = buildTokens({ theme: 'graphite', mode: 'hc', density: 'comfortable', accent: 'indigo' })
    // brand accent stays pixel-exact even in HC
    expect(hc['--accent']).toBe('#4d6bfe')
    // focus ring is the legible lighter brand blue on the dark HC canvas
    expect(hc['--focus-ring']).toBe('#7d97ff')
  })

  it('unknown accent routes to the pixel-exact brand indigo (NOT an oklch approximation)', () => {
    // An accent id that is not in ACCENTS must fall back to the brand contract
    // (#4d6bfe), not to ACCENTS[0]'s OKLCH approximation oklch(0.62 0.17 274).
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'bogus' as never })
    expect(t['--accent']).toBe('#4d6bfe')
    expect(t['--accent-ink']).toBe('#2b46d4')
  })

  it('fonts resolve from uiFont/codeFont (default = brand fonts)', () => {
    // sanity: the font maps are exported with brand defaults first
    expect(UI_FONTS[0].id).toBe('plex')
    expect(CODE_FONTS[0].id).toBe('jetbrains')
    const def = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(def['--type-sans']).toContain('IBM Plex Sans')
    expect(def['--type-mono-family']).toContain('JetBrains Mono')
    const sys = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo', uiFont: 'system', codeFont: 'sf-mono' })
    expect(sys['--type-sans']).toContain('system-ui')
    expect(sys['--type-mono-family']).toContain('SF Mono')
  })

  it('anchors the brand light palette to the exact spec §3.2 hex', () => {
    const t = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(t['--bg']).toBe('#f8f9fb')
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
    expect(t['--bg']).toBe('#0e1018')
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

  it('dark/HC island keeps obsidian; light island uses a light surface', () => {
    const DARK_ISLAND: Record<string, string> = {
      '--island-bg': '#0b0d13',
      '--island-header': '#0f1320',
      '--island-card': '#11141d',
      '--island-ink': '#d6dae4',
      '--island-ink-2': '#9aa3b4',
      '--island-ink-3': '#6b7385',
      '--island-line': '#1d2230',
      '--island-line-soft': '#161b27',
      '--island-accent': '#7d97ff',
      '--island-dot': '#4d6bfe',
      '--island-glass-edge': 'oklch(1 0 0 / 0.08)',
      '--island-add-fg': '#7fe3b4',
      '--island-del-fg': '#f2a3a1',
    }
    // dark and HC keep the obsidian palette
    for (const mode of ['dark', 'hc']) {
      const tok = buildTokens({ theme: 'graphite', mode, density: 'comfortable', accent: 'indigo' })
      for (const [k, v] of Object.entries(DARK_ISLAND)) {
        expect(tok[k], `${mode} ${k}`).toBe(v)
      }
    }
    // light uses a light code surface (not obsidian)
    const lightTok = buildTokens({ theme: 'graphite', mode: 'light', density: 'comfortable', accent: 'indigo' })
    expect(lightTok['--island-bg']).toBe('#f7f8fa')
    expect(lightTok['--island-header']).toBe('#eef0f4')
    expect(lightTok['--island-ink']).toBe('#0d1016')
    expect(lightTok['--island-line']).toBe('#e4e7ec')
    expect(lightTok['--island-dot']).toBe('#4d6bfe')
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
