import { useCallback, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { IconPanelLeft, IconPanelRight } from '../../lib/icons'
import { useT } from '../../lib/i18n'
import styles from './index.module.css'

interface Splits {
  left: number // sessions rail px
  right: number // workspace px
}

const KEY = 'dsc.shell.splits'
const DEFAULTS: Splits = { left: 260, right: 420 }

function loadSplits(): Splits {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) return { ...DEFAULTS, ...(JSON.parse(raw) as Partial<Splits>) }
  } catch {
    /* ignore */
  }
  return { ...DEFAULTS }
}

export interface AppShellProps {
  sessions?: ReactNode
  conversation?: ReactNode
  workspace?: ReactNode
}

type Edge = 'left' | 'right'

export function AppShell({ sessions, conversation, workspace }: AppShellProps) {
  const t = useT()
  const [splits, setSplits] = useState<Splits>(() => loadSplits())
  const [sessionsCollapsed, setSessionsCollapsed] = useState(false)
  const [workspaceCollapsed, setWorkspaceCollapsed] = useState(false)

  const drag = useRef<{ edge: Edge; startX: number; startVal: number } | null>(null)

  const persist = useCallback((s: Splits) => {
    try {
      localStorage.setItem(KEY, JSON.stringify(s))
    } catch {
      /* ignore */
    }
  }, [])

  const onMove = useCallback((e: MouseEvent) => {
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.startX
    setSplits((prev) =>
      d.edge === 'left'
        ? { ...prev, left: Math.max(160, Math.min(480, d.startVal + dx)) }
        : { ...prev, right: Math.max(240, Math.min(720, d.startVal - dx)) },
    )
  }, [])

  const onUp = useCallback(() => {
    drag.current = null
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
    setSplits((cur) => {
      persist(cur)
      return cur
    })
  }, [onMove, persist])

  const onDown = useCallback(
    (edge: Edge, e: React.MouseEvent) => {
      drag.current = { edge, startX: e.clientX, startVal: edge === 'left' ? splits.left : splits.right }
      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
    },
    [splits.left, splits.right, onMove, onUp],
  )

  const gridStyle: CSSProperties = {
    // CSS custom properties drive the grid template (see index.module.css).
    ['--col-left' as string]: sessionsCollapsed ? '0px' : `${splits.left}px`,
    ['--col-right' as string]: workspaceCollapsed ? '0px' : `${splits.right}px`,
  }

  return (
    <div
      className={styles.appShell}
      data-testid="app-shell"
      data-sessions-collapsed={sessionsCollapsed}
      data-workspace-collapsed={workspaceCollapsed}
      style={gridStyle}
    >
      <aside className={`${styles.zone} ${styles.zoneSessions}`}>{!sessionsCollapsed && sessions}</aside>

      <div
        className={styles.splitter}
        data-testid="splitter-left"
        role="separator"
        aria-orientation="vertical"
        onMouseDown={(e) => onDown('left', e)}
      />

      <main className={`${styles.zone} ${styles.zoneConversation}`}>
        <button
          className={`${styles.collapseTab} ${styles.collapseTabLeft}`}
          data-testid="collapse-sessions"
          aria-label={t('shell.collapseSessions')}
          onClick={() => setSessionsCollapsed((v) => !v)}
        >
          <IconPanelLeft size={14} />
        </button>
        {conversation}
        <button
          className={`${styles.collapseTab} ${styles.collapseTabRight}`}
          data-testid="collapse-workspace"
          aria-label={t('shell.collapseWorkspace')}
          onClick={() => setWorkspaceCollapsed((v) => !v)}
        >
          <IconPanelRight size={14} />
        </button>
      </main>

      <div
        className={styles.splitter}
        data-testid="splitter-right"
        role="separator"
        aria-orientation="vertical"
        onMouseDown={(e) => onDown('right', e)}
      />

      <aside className={`${styles.zone} ${styles.zoneWorkspace}`}>{!workspaceCollapsed && workspace}</aside>
    </div>
  )
}
