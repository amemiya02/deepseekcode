// OKLCH semantic token generator (spec §4.2 / §4.3).
// Themes are DATA: a base lightness ramp + per-mode hue/chroma, combined with a
// chosen accent and density, produce a flat record of CSS custom properties.

export type Theme = 'graphite' | 'lumen' | 'halo'
export type Mode = 'light' | 'dark' | 'hc'
export type Density = 'comfortable' | 'compact'
export type Accent =
  | 'indigo' | 'terracotta' | 'emerald' | 'amber'
  | 'rose' | 'cyan' | 'violet' | 'slate'

export interface AccentDef {
  id: Accent
  /** OKLCH hue in degrees and chroma for the accent. */
  h: number
  c: number
}

export const ACCENTS: AccentDef[] = [
  { id: 'indigo', h: 274, c: 0.17 },
  { id: 'terracotta', h: 42, c: 0.14 },
  { id: 'emerald', h: 158, c: 0.15 },
  { id: 'amber', h: 75, c: 0.16 },
  { id: 'rose', h: 12, c: 0.16 },
  { id: 'cyan', h: 215, c: 0.13 },
  { id: 'violet', h: 300, c: 0.17 },
  { id: 'slate', h: 250, c: 0.03 },
]

export interface ThemeDef {
  id: Theme
  /** Base hue + chroma for surfaces (near-neutral). */
  baseHue: number
  baseChroma: number
  /** Default accent when none chosen. */
  defaultAccent: Accent
}

export const THEMES: ThemeDef[] = [
  { id: 'graphite', baseHue: 264, baseChroma: 0.012, defaultAccent: 'indigo' },
  { id: 'lumen', baseHue: 70, baseChroma: 0.02, defaultAccent: 'terracotta' },
  { id: 'halo', baseHue: 0, baseChroma: 0.0, defaultAccent: 'slate' },
]

export interface TokenInput {
  theme: Theme
  mode: Mode
  density: Density
  accent: Accent
}

/** Lightness ramp per mode. index 0 = bg .. 4 = overlay (surface ladder). */
function surfaceLightness(mode: Mode): number[] {
  switch (mode) {
    case 'light':
      return [0.985, 0.965, 0.94, 0.99, 0.92]
    case 'dark':
      // bg .. card climb for elevation; overlay is the dim scrim (darkest).
      return [0.155, 0.19, 0.225, 0.205, 0.1]
    case 'hc':
      return [0.08, 0.12, 0.16, 0.14, 0.2]
  }
}

/** Text lightness triple [primary, secondary, dim] per mode. */
function textLightness(mode: Mode): number[] {
  switch (mode) {
    case 'light':
      return [0.2, 0.42, 0.58]
    case 'dark':
      return [0.96, 0.74, 0.56]
    case 'hc':
      return [1.0, 0.86, 0.7]
  }
}

function oklch(l: number, c: number, h: number): string {
  return `oklch(${l.toFixed(3)} ${c.toFixed(3)} ${h.toFixed(1)})`
}

function spacingScale(density: Density): number {
  return density === 'compact' ? 3 : 4
}

export function buildTokens(input: TokenInput): Record<string, string> {
  const theme = THEMES.find((t) => t.id === input.theme) ?? THEMES[0]
  const accent = ACCENTS.find((a) => a.id === input.accent) ?? ACCENTS[0]
  const sl = surfaceLightness(input.mode)
  const tl = textLightness(input.mode)
  const bh = theme.baseHue
  const bc = theme.baseChroma
  const isHC = input.mode === 'hc'
  const isLight = input.mode === 'light'

  // Border contrast: HC pushes the border lightness away from bg for visibility.
  const borderL = isHC ? (isLight ? 0.55 : 0.62) : isLight ? 0.86 : 0.34
  const borderStrongL = isHC ? (isLight ? 0.4 : 0.78) : isLight ? 0.74 : 0.46

  const accentL = isHC ? 0.68 : input.mode === 'dark' ? 0.62 : 0.55

  const step = spacingScale(input.density)
  const sp = (n: number) => `${step * n}px`

  // Dark + high-contrast share a dark canvas; light is the exception.
  const isDarkish = input.mode !== 'light'
  // Surfaces carry a faint blue undertone on dark canvases (deep-space, not dead gray).
  const sc = isDarkish ? bc + 0.012 : bc
  const accentCss = oklch(accentL, accent.c, accent.h)

  return {
    // Surface ladder (faint blue undertone on dark canvases for depth)
    '--bg': oklch(sl[0], sc, bh),
    '--surface': oklch(sl[1], sc, bh),
    '--elevated': oklch(sl[2], sc, bh),
    '--card': oklch(sl[3], sc, bh),
    '--overlay': oklch(sl[4], sc, bh),
    // Lines
    '--border': oklch(borderL, sc, bh),
    '--border-strong': oklch(borderStrongL, sc, bh),
    '--focus-ring': accentCss,
    // Glass edge — a 1px top highlight so panels read as lifted, not painted-on.
    '--glass-edge': isDarkish ? 'oklch(1 0 0 / 0.055)' : 'oklch(1 0 0 / 0.9)',
    // Elevation shadows
    '--shadow-1': isDarkish ? '0 1px 2px oklch(0 0 0 / 0.4)' : '0 1px 2px oklch(0 0 0 / 0.08)',
    '--shadow-2': isDarkish ? '0 10px 34px -8px oklch(0 0 0 / 0.55)' : '0 10px 30px -10px oklch(0 0 0 / 0.18)',
    '--shadow-pop': isDarkish ? '0 24px 64px -16px oklch(0 0 0 / 0.7)' : '0 24px 64px -16px oklch(0 0 0 / 0.25)',
    // Text
    '--text': oklch(tl[0], bc, bh),
    '--text-2': oklch(tl[1], bc, bh),
    '--text-3': oklch(tl[2], bc, bh),
    '--text-on-accent': accentL > 0.6 ? oklch(0.16, 0, 0) : oklch(0.99, 0, 0),
    // Accent (+ a secondary for the signature indigo→cyan gradient + glow)
    '--accent': accentCss,
    '--accent-2': oklch(isDarkish ? 0.8 : 0.62, 0.12, 200),
    '--accent-text': oklch(isLight ? 0.42 : 0.82, accent.c, accent.h),
    '--accent-weak': oklch(isLight ? 0.94 : 0.26, accent.c * 0.55, accent.h),
    '--accent-grad': `linear-gradient(135deg, ${accentCss}, ${oklch(isDarkish ? 0.8 : 0.62, 0.12, 200)})`,
    '--glow-accent': isDarkish ? `0 0 24px -2px ${accentCss.replace(')', ' / 0.45)')}` : 'none',
    // Telemetry — the cockpit signal colors (cache=cyan, cost=amber, route=violet)
    '--cache': oklch(isDarkish ? 0.82 : 0.55, 0.13, 195),
    '--cost': oklch(isDarkish ? 0.84 : 0.6, 0.12, 85),
    '--route': oklch(isDarkish ? 0.78 : 0.58, 0.15, 300),
    '--ring-track': oklch(isDarkish ? 0.27 : 0.9, sc, bh),
    // Semantic
    '--success': oklch(0.62, 0.15, 152),
    '--danger': oklch(0.6, 0.18, 26),
    '--warning': oklch(0.7, 0.15, 75),
    '--info': oklch(0.62, 0.13, 230),
    '--add-bg': oklch(isLight ? 0.94 : 0.26, 0.06, 152),
    '--add-fg': oklch(isLight ? 0.4 : 0.8, 0.13, 152),
    '--del-bg': oklch(isLight ? 0.95 : 0.26, 0.06, 26),
    '--del-fg': oklch(isLight ? 0.45 : 0.8, 0.15, 26),
    // Type roles: "weight size/line-height tracking" bundled per role.
    '--type-display': '650 28px/34px -0.02em',
    '--type-title': '600 18px/24px -0.01em',
    '--type-body': '400 14px/22px 0',
    '--type-ui': '500 13px/18px 0',
    '--type-label': '600 11px/14px 0.04em',
    '--type-mono': "400 13px/20px 0 'JetBrains Mono', 'Geist Mono', 'Noto Sans Mono CJK SC', monospace",
    // Spacing (4px grid, compact = 3px)
    '--s-1': sp(1),
    '--s-2': sp(2),
    '--s-3': sp(3),
    '--s-4': sp(4),
    '--s-5': sp(5),
    '--s-6': sp(6),
    '--s-7': sp(7),
    '--s-8': sp(8),
    // Radius
    '--r-sm': '4px',
    '--r-md': '8px',
    '--r-lg': '12px',
    '--r-xl': '16px',
    // Motion
    '--ease-standard': 'cubic-bezier(0.2, 0, 0, 1)',
    '--dur-fast': '120ms',
    '--dur-base': '200ms',
    '--dur-slow': '320ms',
  }
}
