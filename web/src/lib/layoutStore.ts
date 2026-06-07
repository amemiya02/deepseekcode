import { create } from 'zustand'
import { loadLayout, saveLayout, isReviewOpen, type Layout } from '../components/shell/layout'

export type Mode = 'code' | 'write'

interface LayoutState {
  layout: Layout
  mode: Mode
  setLayout: (next: Layout | ((prev: Layout) => Layout)) => void
  setMode: (mode: Mode) => void
  toggleLeft: () => void
  toggleRight: (hasChanges: boolean) => void
}

// The shell's resizable layout lives here so both AppShell (resize logic) and
// TitleBar (the toggle cluster) read/write one source. setLayout does NOT persist
// (AppShell's drag/keyboard/auto-fit call saveLayout explicitly, matching the
// pre-lift behavior); the toggle actions persist.
export const useLayoutStore = create<LayoutState>((set) => ({
  layout: loadLayout(),
  mode: 'code',
  setLayout: (next) =>
    set((s) => ({ layout: typeof next === 'function' ? (next as (p: Layout) => Layout)(s.layout) : next })),
  setMode: (mode) => set({ mode }),
  toggleLeft: () =>
    set((s) => {
      const layout = { ...s.layout, leftCollapsed: !s.layout.leftCollapsed }
      saveLayout(layout)
      return { layout }
    }),
  toggleRight: (hasChanges) =>
    set((s) => {
      const open = isReviewOpen(s.layout, hasChanges)
      const layout = { ...s.layout, reviewPin: open ? ('closed' as const) : ('open' as const) }
      saveLayout(layout)
      return { layout }
    }),
}))
