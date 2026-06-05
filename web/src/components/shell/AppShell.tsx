import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { IconPanelLeft, IconPanelRight } from '../../lib/icons'
import { useT } from '../../lib/i18n'
import {
  loadLayout, saveLayout, applyPreset, clampLeft, clampRight,
  LEFT_MIN, LEFT_MAX, RIGHT_MIN, RIGHT_MAX,
  type Layout,
} from './layout'
import { LayoutPresets } from './LayoutPresets'
import styles from './index.module.css'

export interface AppShellProps {
  sessions?: ReactNode
  conversation?: ReactNode
  workspace?: ReactNode
}

type Edge = 'left' | 'right'

const STEP = 16
const STEP_BIG = 64
const GUTTER = 24

export function AppShell({ sessions, conversation, workspace }: AppShellProps) {
  const t = useT()
  const [layout, setLayout] = useState<Layout>(() => loadLayout())
  const [dragging, setDragging] = useState(false)

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
          : { ...prev, right: clampRight(content), rightCollapsed: false }
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
          const next = { ...prev, rightCollapsed: !prev.rightCollapsed }
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
  }, [])

  // ── Collapse toggles ──────────────────────────────────────────────────────

  const toggleLeft = () =>
    setLayout((prev) => {
      const next = { ...prev, leftCollapsed: !prev.leftCollapsed }
      saveLayout(next)
      return next
    })

  const toggleRight = () =>
    setLayout((prev) => {
      const next = { ...prev, rightCollapsed: !prev.rightCollapsed }
      saveLayout(next)
      return next
    })

  // ── Preset handler ────────────────────────────────────────────────────────

  const onPreset = (p: Parameters<typeof applyPreset>[1]) => {
    setLayout((prev) => {
      const next = applyPreset(prev, p)
      saveLayout(next)
      return next
    })
  }

  // ── CSS custom properties → grid ──────────────────────────────────────────

  const gridStyle: CSSProperties = {
    ['--col-left' as string]: layout.leftCollapsed ? '0px' : `${layout.left}px`,
    ['--col-right' as string]: layout.rightCollapsed ? '0px' : `${layout.right}px`,
  }

  return (
    <div
      className={styles.appShell}
      data-testid="app-shell"
      data-sessions-collapsed={layout.leftCollapsed}
      data-workspace-collapsed={layout.rightCollapsed}
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
        <button
          className={`${styles.collapseTab} ${styles.collapseTabLeft}`}
          data-testid="collapse-sessions"
          aria-label={t('shell.collapseSessions')}
          onClick={toggleLeft}
        >
          <IconPanelLeft size={14} />
        </button>

        <div className={styles.presetRow}>
          <LayoutPresets value={layout.preset} onChange={onPreset} />
        </div>

        {conversation}

        <button
          className={`${styles.collapseTab} ${styles.collapseTabRight}`}
          data-testid="collapse-workspace"
          aria-label={t('shell.collapseWorkspace')}
          onClick={toggleRight}
        >
          <IconPanelRight size={14} />
        </button>
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
          {!layout.rightCollapsed && workspace}
        </div>
      </aside>
    </div>
  )
}
