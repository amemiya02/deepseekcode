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
}

export const LEFT_MIN = 160, LEFT_MAX = 480
export const RIGHT_MIN = 240, RIGHT_MAX = 720

export const DEFAULT_LAYOUT: Layout = {
  left: 260, right: 420, leftCollapsed: false, rightCollapsed: false, preset: 'balanced',
}

const KEY = 'dsc.shell.layout'
const LEGACY_KEY = 'dsc.shell.splits'

export const clampLeft = (n: number) => Math.max(LEFT_MIN, Math.min(LEFT_MAX, n))
export const clampRight = (n: number) => Math.max(RIGHT_MIN, Math.min(RIGHT_MAX, n))

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
