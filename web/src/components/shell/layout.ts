// Single source of truth for the shell's resizable layout: column widths,
// collapse flags, and the active preset. Persisted under one key with a
// fallback read of the legacy widths-only key so existing users don't reset.

export type Preset = 'balanced' | 'focus' | 'review'

export interface Layout {
  left: number
  right: number
  leftCollapsed: boolean
  rightCollapsed: boolean
  preset: Preset
  // Adaptive review companion (spec §4): 'auto' reveals only when the working tree
  // has changes; 'open' pins it visible; 'closed' hides it (back to 2 columns).
  reviewPin: 'auto' | 'open' | 'closed'
}

export const LEFT_MIN = 236, LEFT_MAX = 420
export const RIGHT_MIN = 280, RIGHT_MAX = 760

export const DEFAULT_LAYOUT: Layout = {
  left: 268, right: 360, leftCollapsed: false, rightCollapsed: false, preset: 'balanced', reviewPin: 'auto',
}

const KEY = 'dsc.shell.layout'
const LEGACY_KEY = 'dsc.shell.splits'

export const clampLeft = (n: number) => Math.max(LEFT_MIN, Math.min(LEFT_MAX, n))
export const clampRight = (n: number) => Math.max(RIGHT_MIN, Math.min(RIGHT_MAX, n))

// isReviewOpen derives whether the review companion pane is visible: a pinned-open
// pane always shows, a closed one never does, and 'auto' follows whether the working
// tree has changes — driving the 2↔3-column adaptive layout (spec §4).
export function isReviewOpen(l: Layout, hasChanges: boolean): boolean {
  if (l.reviewPin === 'open') return true
  if (l.reviewPin === 'closed') return false
  return hasChanges
}

export function loadLayout(): Layout {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) return { ...DEFAULT_LAYOUT, ...(JSON.parse(raw) as Partial<Layout>) }
    const legacy = localStorage.getItem(LEGACY_KEY)
    if (legacy) {
      const { left, right } = JSON.parse(legacy) as Partial<Layout>
      return { ...DEFAULT_LAYOUT, ...(left != null ? { left } : {}), ...(right != null ? { right } : {}) }
    }
  } catch { /* ignore */ }
  return { ...DEFAULT_LAYOUT }
}

export function saveLayout(l: Layout): void {
  try { localStorage.setItem(KEY, JSON.stringify(l)) } catch { /* ignore */ }
}

// applyPreset derives collapse/width from a preset without losing the user's
// last balanced widths (kept on the object so toggling back restores them).
export function applyPreset(l: Layout, preset: Preset): Layout {
  switch (preset) {
    case 'focus':
      return { ...l, leftCollapsed: true, rightCollapsed: true, preset }
    case 'review':
      return { ...l, leftCollapsed: true, rightCollapsed: false, right: clampRight(Math.max(l.right, RIGHT_MAX)), preset }
    case 'balanced':
    default:
      return { ...l, leftCollapsed: false, rightCollapsed: false, preset: 'balanced' }
  }
}
