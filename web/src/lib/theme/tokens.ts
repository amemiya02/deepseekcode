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

function spacingScale(density: Density): number {
  return density === 'compact' ? 3 : 4
}

// ── Brand anchors (spec §3.2 light / §3.3 dark) ──────────────────────────────
// These hex values are the brand contract; the surface + accent anchors are
// pixel-exact, never OKLCH-derived. The OKLCH ramp still drives accent *scales*
// and density elsewhere, but the chrome reads pure brand.
const SANS =
  "'IBM Plex Sans', 'IBM Plex Sans SC', 'PingFang SC', 'Microsoft YaHei', -apple-system, system-ui, sans-serif"
const MONO = "'JetBrains Mono', 'IBM Plex Mono', ui-monospace, 'SF Mono', monospace"

/** The single brand accent. One blue, everywhere. */
const ACCENT = '#4d6bfe'
const ACCENT_INK = '#2b46d4' // hover
const ACCENT_DEEP = '#1f33a8' // pressed
const ACCENT_WEAK = '#eef1ff' // wash / active fill (light)
const ACCENT_MIST = '#f5f7ff' // faintest tint (light)

/** Light palette (§3.2). */
const LIGHT = {
  bg: '#f5f6f8',
  bg2: '#eef0f4', // recessed / hover fill
  surface: '#ffffff',
  ink: '#0d1016',
  inkSoft: '#4d5560',
  inkFaint: '#8a929e',
  line: '#e4e7ec',
  lineSoft: '#eef0f3',
} as const

/** Dark "terminal island" palette (§3.3). */
const DARK = {
  bg: '#0b0d13',
  bg2: '#0f1320',
  surface: '#11141d',
  ink: '#d6dae4',
  inkSoft: '#9aa3b4',
  inkFaint: '#6b7385',
  line: '#1d2230',
  lineSoft: '#161b27',
  accentText: '#7d97ff', // lighter accent for legibility on dark
} as const

export function buildTokens(input: TokenInput): Record<string, string> {
  // The chrome reads a single brand accent (spec §3), so the chosen `accent`
  // and `theme` no longer steer the palette — they remain on TokenInput for the
  // settings UI + future per-surface scales. Mode (light default / dark island)
  // selects the brand palette below.
  const isHC = input.mode === 'hc'
  const isLight = input.mode === 'light'

  const step = spacingScale(input.density)
  const sp = (n: number) => `${step * n}px`

  // Dark + high-contrast share the dark terminal-island canvas; light is default.
  const isDarkish = input.mode !== 'light'
  const p = isLight ? LIGHT : DARK

  // High-contrast strengthens borders/text against the dark canvas.
  const border = isHC ? '#3a4254' : isLight ? LIGHT.line : DARK.line
  const borderStrong = isHC ? '#5b6478' : isLight ? '#cfd4dc' : '#2a3142'
  const lineSoft = isLight ? LIGHT.lineSoft : DARK.lineSoft

  // Accent text: lighter blue on dark for legibility; brand-ink on light.
  const accentText = isLight ? ACCENT_INK : DARK.accentText
  const accentWeak = isLight ? ACCENT_WEAK : 'color-mix(in oklch, #4d6bfe 22%, #11141d)'

  return {
    // Surface ladder (§3.2 / §3.3 brand anchors)
    '--bg': p.bg,
    '--surface': p.surface,
    '--elevated': p.bg2,
    '--card': p.surface,
    '--overlay': isLight ? '#ffffff' : '#0f1320',
    // Full-screen modal backdrop. Translucent in BOTH modes so opening a modal
    // dims the app behind a contained panel instead of wiping it to a flat color.
    '--scrim': isLight ? 'oklch(0.2 0.02 264 / 0.38)' : 'oklch(0 0 0 / 0.55)',
    // Lines
    '--border': border,
    '--border-strong': borderStrong,
    '--border-hair': lineSoft,
    '--line-soft': lineSoft,
    '--focus-ring': ACCENT,
    // Panel
    '--panel-pad': sp(4),
    // Glass edge — a 1px top highlight so panels read as lifted, not painted-on.
    '--glass-edge': isDarkish ? 'oklch(1 0 0 / 0.05)' : 'oklch(1 0 0 / 0.9)',
    // Elevation shadows (light §3.6; dark code islands deeper)
    '--shadow-1': isDarkish
      ? '0 1px 2px rgba(5,8,16,.5)'
      : '0 1px 2px rgba(13,16,22,.05)',
    '--shadow-2': isDarkish
      ? '0 24px 60px -18px rgba(5,8,16,.6)'
      : '0 4px 14px rgba(13,16,22,.06)',
    '--shadow-pop': isDarkish
      ? '0 40px 80px -28px rgba(15,22,55,.55)'
      : '0 24px 60px -18px rgba(20,28,60,.30)',
    // Text
    '--text': p.ink,
    '--text-2': p.inkSoft,
    '--text-3': p.inkFaint,
    '--text-on-accent': '#ffffff',
    // Accent — the ONE brand blue (glow, not gradient)
    '--accent': ACCENT,
    '--accent-ink': ACCENT_INK,
    '--accent-deep': ACCENT_DEEP,
    '--accent-text': accentText,
    '--accent-weak': accentWeak,
    '--accent-mist': isLight ? ACCENT_MIST : 'color-mix(in oklch, #4d6bfe 12%, #0b0d13)',
    // The brand "gradient" is a glow, never a multi-hue ramp.
    '--glow-accent': '0 6px 18px -6px rgba(77,107,254,.6)',
    '--glow-accent-hover': '0 8px 22px -8px rgba(77,107,254,.7)',
    // Telemetry — cockpit signal colors re-pointed to brand (no cyan/amber/violet)
    '--cache': ACCENT, // cache → brand blue
    '--cost': p.ink, // cost → ink
    '--route': isLight ? ACCENT_INK : DARK.accentText, // route → brand tint
    '--ring-track': isLight ? '#e4e7ec' : '#1d2230',
    // Semantic signal colors (true status ONLY — never decoration)
    '--success': '#3ecf8e',
    '--danger': '#e5484d',
    '--warning': '#e7b15b',
    '--info': ACCENT,
    '--add-bg': isLight ? '#eaf7f0' : 'color-mix(in oklch, #3ecf8e 18%, #0b0d13)',
    '--add-fg': isLight ? '#1f7a52' : '#7fe3b4',
    '--del-bg': isLight ? '#fdeceb' : 'color-mix(in oklch, #e5484d 18%, #0b0d13)',
    '--del-fg': isLight ? '#b4322f' : '#f2a3a1',
    // ── Code/diff "island" — ALWAYS obsidian, identical in every mode (§3 material rule).
    // A fixed dark material: light mode = paper-vs-obsidian contrast; dark mode = island
    // one step deeper than the #11141d surface + a brand glass edge. Mode-independent on
    // purpose, so `.island` (app.css) can re-scope surface/ink/border to these anywhere.
    '--island-bg': DARK.bg, // #0b0d13 — island body
    '--island-header': DARK.bg2, // #0f1320 — brand header strip
    '--island-card': DARK.surface, // #11141d — inset rows
    '--island-ink': DARK.ink, // #d6dae4
    '--island-ink-2': DARK.inkSoft, // #9aa3b4
    '--island-ink-3': DARK.inkFaint, // #6b7385
    '--island-line': DARK.line, // #1d2230
    '--island-line-soft': DARK.lineSoft, // #161b27
    '--island-accent': DARK.accentText, // #7d97ff — legible label/keyword text on dark
    '--island-dot': ACCENT, // #4d6bfe — saturated brand dot in the header
    '--island-glass-edge': 'oklch(1 0 0 / 0.08)',
    '--island-add-bg': 'color-mix(in oklch, #3ecf8e 18%, #0b0d13)',
    '--island-add-fg': '#7fe3b4',
    '--island-del-bg': 'color-mix(in oklch, #e5484d 18%, #0b0d13)',
    '--island-del-fg': '#f2a3a1',
    // Type roles: "weight size/line-height tracking" bundled per role.
    '--type-sans': SANS,
    '--type-display': '600 28px/34px -0.02em',
    '--type-title': '600 18px/24px -0.01em',
    '--type-body': '400 14px/22px 0',
    '--type-ui': '500 13px/18px 0',
    '--type-label': '600 11px/14px 0.04em',
    '--type-mono': `400 13px/20px 0 ${MONO}`,
    '--type-mono-family': MONO,
    // Spacing (4px grid, compact = 3px)
    '--s-1': sp(1),
    '--s-2': sp(2),
    '--s-3': sp(3),
    '--s-4': sp(4),
    '--s-5': sp(5),
    '--s-6': sp(6),
    '--s-7': sp(7),
    '--s-8': sp(8),
    // Radius (§3.6)
    '--r-sm': '9px',
    '--r-md': '14px',
    '--r-lg': '22px',
    '--r-xl': '28px',
    // Chat surface (SP2) — mode-independent layout rhythm
    '--measure': '760px',
    '--avatar-size': '26px',
    '--turn-gap': '24px',
    '--turn-pad': '6px',
    // Motion
    '--ease-standard': 'cubic-bezier(0.2, 0, 0, 1)',
    '--dur-fast': '120ms',
    '--dur-base': '200ms',
    '--dur-slow': '320ms',
  }
}
