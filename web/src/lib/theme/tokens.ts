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
  uiFont?: string
  codeFont?: string
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

/** UI font options (id → full font-family stack). 'plex' is the brand default. */
export const UI_FONTS: { id: string; stack: string }[] = [
  { id: 'plex', stack: SANS },
  { id: 'system', stack: 'system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", sans-serif' },
  { id: 'inter', stack: `'Inter', ${SANS}` },
]
/** Code font options. 'jetbrains' is the brand default. */
export const CODE_FONTS: { id: string; stack: string }[] = [
  { id: 'jetbrains', stack: MONO },
  { id: 'plex-mono', stack: "'IBM Plex Mono', ui-monospace, 'SF Mono', monospace" },
  { id: 'sf-mono', stack: "ui-monospace, 'SF Mono', Menlo, Consolas, monospace" },
]
function resolveFont(opts: { id: string; stack: string }[], id: string | undefined, fallback: string): string {
  return opts.find((o) => o.id === id)?.stack ?? fallback
}

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

// accentScale returns the accent token family for the chosen accent. The brand
// accent (indigo) stays PIXEL-EXACT (#4d6bfe family — the brand contract); other
// accents are OKLCH-derived from their ACCENTS def so the picker is genuinely
// live. Surfaces/neutrals are unaffected — this is an accent-only extension, not
// a token-generator rewrite.
//
// `mode` (not a boolean) is taken so HC is distinct from plain dark: HC shares
// the dark canvas but needs a BRIGHTER focus ring to clear WCAG 3:1 against the
// near-black HC bg, so `focusRing` is a separate field from `accent` (the brand
// accent token stays pixel-exact even in HC; only the ring brightens).
function accentScale(accent: Accent, mode: Mode) {
  const isLight = mode === 'light'
  const isHC = mode === 'hc'
  // The brand indigo is pixel-exact in every mode. An UNKNOWN accent (not in
  // ACCENTS) must ALSO route here — falling back to the brand contract (#4d6bfe),
  // never to ACCENTS[0]'s OKLCH approximation oklch(0.62 0.17 274), which would
  // render a different value despite the same indigo intent.
  if (accent === 'indigo' || !ACCENTS.some((x) => x.id === accent)) {
    return {
      accent: ACCENT,
      ink: ACCENT_INK,
      deep: ACCENT_DEEP,
      text: isLight ? ACCENT_INK : DARK.accentText,
      weak: isLight ? ACCENT_WEAK : 'color-mix(in oklch, #4d6bfe 22%, #11141d)',
      mist: isLight ? ACCENT_MIST : 'color-mix(in oklch, #4d6bfe 12%, #0b0d13)',
      // HC focus ring = the legible lighter brand blue (#7d97ff) on the dark HC
      // canvas; light/dark keep the saturated brand blue as the ring.
      focusRing: isHC ? DARK.accentText : ACCENT,
      glow: '0 6px 18px -6px rgba(77,107,254,.6)',
      glowHover: '0 8px 22px -8px rgba(77,107,254,.7)',
    }
  }
  // Non-brand accent: OKLCH-derived. Guaranteed present (the !some() guard above
  // routes any unknown id to the indigo branch), so `find` cannot miss.
  const a = ACCENTS.find((x) => x.id === accent)!
  const base = `oklch(0.62 ${a.c} ${a.h})`
  return {
    accent: base,
    ink: `oklch(0.52 ${a.c} ${a.h})`,
    deep: `oklch(0.42 ${a.c} ${a.h})`,
    text: isLight ? `oklch(0.48 ${a.c} ${a.h})` : `oklch(0.74 ${(a.c * 0.9).toFixed(3)} ${a.h})`,
    weak: isLight ? `oklch(0.96 ${(a.c * 0.18).toFixed(3)} ${a.h})` : `color-mix(in oklch, ${base} 22%, #11141d)`,
    mist: isLight ? `oklch(0.985 ${(a.c * 0.1).toFixed(3)} ${a.h})` : `color-mix(in oklch, ${base} 12%, #0b0d13)`,
    // HC: brighter ring (L 0.78) to clear 3:1 against the near-black HC canvas;
    // light/dark use the base (L 0.62) accent as the ring.
    focusRing: isHC ? `oklch(0.78 ${a.c} ${a.h})` : base,
    glow: `0 6px 18px -6px oklch(0.62 ${a.c} ${a.h} / 0.6)`,
    glowHover: `0 8px 22px -8px oklch(0.62 ${a.c} ${a.h} / 0.7)`,
  }
}

export function buildTokens(input: TokenInput): Record<string, string> {
  // The chrome reads a single brand accent (spec §3), so the chosen `accent`
  // and `theme` no longer steer the palette — they remain on TokenInput for the
  // settings UI + future per-surface scales. Mode (light default / dark island)
  // selects the brand palette below.
  const isHC = input.mode === 'hc'
  const isLight = input.mode === 'light'

  const ac = accentScale(input.accent, input.mode)
  const sans = resolveFont(UI_FONTS, input.uiFont, SANS)
  const mono = resolveFont(CODE_FONTS, input.codeFont, MONO)

  const step = spacingScale(input.density)
  const sp = (n: number) => `${step * n}px`

  // Dark + high-contrast share the dark terminal-island canvas; light is default.
  const isDarkish = input.mode !== 'light'
  const p = isLight ? LIGHT : DARK

  // High-contrast strengthens borders/text against the dark canvas.
  const border = isHC ? '#3a4254' : isLight ? LIGHT.line : DARK.line
  const borderStrong = isHC ? '#5b6478' : isLight ? '#cfd4dc' : '#2a3142'
  const lineSoft = isLight ? LIGHT.lineSoft : DARK.lineSoft

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
    '--focus-ring': ac.focusRing,
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
    // Accent — brand-indigo default (pixel-exact); other accents OKLCH-derived.
    '--accent': ac.accent,
    '--accent-ink': ac.ink,
    '--accent-deep': ac.deep,
    '--accent-text': ac.text,
    '--accent-weak': ac.weak,
    '--accent-mist': ac.mist,
    // The brand "gradient" is a glow, never a multi-hue ramp.
    '--glow-accent': ac.glow,
    '--glow-accent-hover': ac.glowHover,
    // Telemetry — cockpit signal colors re-pointed to the active accent
    '--cache': ac.accent, // cache → accent
    '--cost': p.ink, // cost → ink
    '--route': ac.text, // route → accent tint
    '--ring-track': isLight ? '#e4e7ec' : '#1d2230',
    // Semantic signal colors (true status ONLY — never decoration)
    '--success': '#3ecf8e',
    '--danger': '#e5484d',
    '--warning': '#e7b15b',
    '--info': ac.accent,
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
    '--type-sans': sans,
    '--type-display': '600 28px/34px -0.02em',
    '--type-title': '600 18px/24px -0.01em',
    '--type-body': '400 14px/22px 0',
    '--type-ui': '500 13px/18px 0',
    '--type-label': '600 11px/14px 0.04em',
    '--type-mono': `400 13px/20px 0 ${mono}`,
    '--type-mono-family': mono,
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
