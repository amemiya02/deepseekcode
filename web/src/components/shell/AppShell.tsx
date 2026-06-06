import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import {
  saveLayout, clampLeft, clampRight, isReviewOpen,
  LEFT_MIN, LEFT_MAX, RIGHT_MIN, RIGHT_MAX,
  type Layout,
} from './layout'
import { useLayoutStore } from '../../lib/layoutStore'
import styles from './index.module.css'

export interface AppShellProps {
  sessions?: ReactNode
  conversation?: ReactNode
  workspace?: ReactNode
  /** True when the working tree has changes — drives the 'auto' review reveal. */
  workspaceHasContent?: boolean
}

type Edge = 'left' | 'right'

const STEP = 16
const STEP_BIG = 64
const GUTTER = 24

export function AppShell({ sessions, conversation, workspace, workspaceHasContent }: AppShellProps) {
  const layout = useLayoutStore((s) => s.layout)
  const setLayout = useLayoutStore((s) => s.setLayout)
  const [dragging, setDragging] = useState(false)
  const reviewOpen = isReviewOpen(layout, !!workspaceHasContent)

  const drag = useRef<{ edge: Edge; startX: number; startVal: number } | null>(null)
  const leftHost = useRef<HTMLDivElement>(null)
  const rightHost = useRef<HTMLDivElement>(null)

  // ── Drag resize ──────────────────────────────────────────────────────────

  const onMove = useCallback((e: MouseEvent) => {
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.startX
    setLayout((prev) =>
      d.edge === 'left'
        ? { ...prev, left: clampLeft(d.startVal + dx) }
        : { ...prev, right: clampRight(d.startVal - dx) },
    )
  }, [])

  const onUp = useCallback(() => {
    drag.current = null
    setDragging(false)
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
    setLayout((cur) => {
      saveLayout(cur)
      return cur
    })
  }, [onMove])

  const onDown = useCallback(
    (edge: Edge, e: React.MouseEvent) => {
      drag.current = { edge, startX: e.clientX, startVal: edge === 'left' ? layout.left : layout.right }
      setDragging(true)
      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
    },
    [layout.left, layout.right, onMove, onUp],
  )

  // Teardown guard: remove window listeners if component unmounts mid-drag.
  useEffect(
    () => () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    },
    [onMove, onUp],
  )

  // ── Keyboard resize ──────────────────────────────────────────────────────

  const onKey = (edge: Edge) => (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const delta = (e.key === 'ArrowRight' ? 1 : -1) * (e.shiftKey ? STEP_BIG : STEP)
    setLayout((prev) => {
      const next =
        edge === 'left'
          ? { ...prev, left: clampLeft(prev.left + delta) }
          : { ...prev, right: clampRight(prev.right - delta) }
      saveLayout(next)
      return next
    })
  }

  // ── Double-click auto-fit ─────────────────────────────────────────────────

  const autoFit = (edge: Edge) => {
    const host = edge === 'left' ? leftHost.current : rightHost.current
    if (!host) return
    const content = host.scrollWidth + GUTTER
    setLayout((prev) => {
      const next =
        edge === 'left'
          ? { ...prev, left: clampLeft(content), leftCollapsed: false }
          : { ...prev, right: clampRight(content), reviewPin: 'open' as const }
      saveLayout(next)
      return next
    })
  }

  // ── Window keyboard shortcuts ─────────────────────────────────────────────

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey
      if (mod && e.key === '\\') {
        e.preventDefault()
        setLayout((prev) => {
          const open = isReviewOpen(prev, !!workspaceHasContent)
          const next = { ...prev, reviewPin: open ? 'closed' as const : 'open' as const }
          saveLayout(next)
          return next
        })
      }
      if (mod && e.key === 'b') {
        e.preventDefault()
        setLayout((prev) => {
          const next = { ...prev, leftCollapsed: !prev.leftCollapsed }
          saveLayout(next)
          return next
        })
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [workspaceHasContent])

  // ── CSS custom properties → grid ──────────────────────────────────────────

  const gridStyle: CSSProperties = {
    ['--col-left' as string]: layout.leftCollapsed ? '0px' : `${layout.left}px`,
    ['--col-right' as string]: reviewOpen ? `${layout.right}px` : '0px',
  }

  return (
    <div
      className={styles.appShell}
      data-testid="app-shell"
      data-sessions-collapsed={layout.leftCollapsed}
      data-workspace-collapsed={!reviewOpen}
      data-dragging={dragging || undefined}
      style={gridStyle}
    >
      <aside className={`${styles.zone} ${styles.zoneSessions} shell-rail`}>
        <div ref={leftHost} data-testid="zone-sessions-host" style={{ height: '100%' }}>
          {!layout.leftCollapsed && sessions}
        </div>
      </aside>

      <div
        className={styles.splitter}
        data-testid="splitter-left"
        role="separator"
        aria-orientation="vertical"
        aria-valuenow={layout.left}
        aria-valuemin={LEFT_MIN}
        aria-valuemax={LEFT_MAX}
        tabIndex={0}
        onMouseDown={(e) => onDown('left', e)}
        onDoubleClick={() => autoFit('left')}
        onKeyDown={onKey('left')}
      />

      <main className={`${styles.zone} ${styles.zoneConversation} shell-main`}>
        {conversation}
      </main>

      <div
        className={styles.splitter}
        data-testid="splitter-right"
        role="separator"
        aria-orientation="vertical"
        aria-valuenow={layout.right}
        aria-valuemin={RIGHT_MIN}
        aria-valuemax={RIGHT_MAX}
        tabIndex={0}
        onMouseDown={(e) => onDown('right', e)}
        onDoubleClick={() => autoFit('right')}
        onKeyDown={onKey('right')}
      />

      <aside className={`${styles.zone} ${styles.zoneWorkspace} shell-panel`}>
        <div ref={rightHost} data-testid="zone-workspace-host" style={{ height: '100%' }}>
          {reviewOpen && workspace}
        </div>
      </aside>
    </div>
  )
}
