import { IconGitBranch, IconSun, IconMoon, IconSettings, IconPanelLeft, IconPanelRight } from '../../lib/icons'
import { useT } from '../../lib/i18n'
import { useThemeStore, setThemeSettings } from '../../lib/theme/store'
import { useLayoutStore } from '../../lib/layoutStore'
import { isReviewOpen } from './layout'
import { Logo } from '../Logo'
import { GitBranchPicker } from '../GitBranchPicker'
import styles from './index.module.css'

export interface TitleBarProps {
  branch?: string
  onOpenPalette?: () => void
  onOpenSettings?: () => void
  /** Drives the review-pane toggle pressed state under reviewPin='auto'. */
  workspaceHasContent?: boolean
}

export function TitleBar({ branch = 'main', onOpenSettings, workspaceHasContent = false }: TitleBarProps) {
  const t = useT()
  const settings = useThemeStore((s) => s.settings)
  const isDark = settings.mode === 'dark'
  const layout = useLayoutStore((s) => s.layout)
  const toggleLeft = useLayoutStore((s) => s.toggleLeft)
  const toggleRight = useLayoutStore((s) => s.toggleRight)
  const reviewOpen = isReviewOpen(layout, workspaceHasContent)

  const toggleMode = () => setThemeSettings({ mode: isDark ? 'light' : 'dark' })

  return (
    <header className={`${styles.titlebar} titlebar`}>
      <div className={styles.titlebarLeft}>
        <Logo size={20} className={styles.titlebarLogo} />
        <span className={styles.appName}>DeepSeekCode</span>
        <GitBranchPicker
          current={branch}
          branches={[branch]}
          onSelect={() => {}}
        />
      </div>

      <div className={styles.titlebarRight}>
        <button className={styles.iconBtn} data-testid="toggle-mode" aria-label={t('titlebar.toggleMode')} onClick={toggleMode}>
          {isDark ? <IconSun size={16} /> : <IconMoon size={16} />}
        </button>
        <button
          className={styles.iconBtn}
          data-testid="open-settings"
          aria-label={t('settings.title', 'Settings')}
          title={t('settings.title', 'Settings')}
          onClick={() => onOpenSettings?.()}
        >
          <IconSettings size={16} />
        </button>
        <span className={styles.titlebarDivider} aria-hidden="true" />
        <div className={styles.paneToggles} role="group" aria-label={t('shell.layout', 'Layout')}>
          <button
            className={styles.paneToggle}
            data-testid="collapse-sessions"
            aria-pressed={!layout.leftCollapsed}
            aria-label={t('shell.collapseSessions')}
            onClick={toggleLeft}
          >
            <IconPanelLeft size={15} />
          </button>
          <button
            className={styles.paneToggle}
            data-testid="collapse-workspace"
            aria-pressed={reviewOpen}
            aria-label={t('shell.collapseWorkspace')}
            onClick={() => toggleRight(workspaceHasContent)}
          >
            <IconPanelRight size={15} />
          </button>
        </div>
      </div>
    </header>
  )
}
