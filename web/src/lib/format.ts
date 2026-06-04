// Pure, framework-agnostic formatting helpers shared by the status bar,
// cockpit, and session list. No DOM, no React — trivially unit-testable.

const DASH = '—'

/** Render a 0..1 ratio as a rounded integer percent, or — when missing. */
export function formatPct(ratio: number | null | undefined): string {
  if (ratio == null || Number.isNaN(ratio)) return DASH
  return `${Math.round(ratio * 100)}%`
}

/** Render yuan to 4 decimal places with a ¥ prefix; ¥— when missing. */
export function formatCNY(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return `¥${DASH}`
  return `¥${value.toFixed(4)}`
}

/** Thousands-separate an integer token count; — when missing. */
export function formatTokens(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return DASH
  return Math.round(value).toLocaleString('en-US')
}
