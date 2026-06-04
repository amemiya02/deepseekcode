import { IconGitBranch, IconSun, IconMoon, IconPalette, IconSettings } from '../../lib/icons'
import { useT } from '../../lib/i18n'
import { useThemeStore, setThemeSettings } from '../../lib/theme/store'
import { THEMES } from '../../lib/theme/tokens'
import { Logo } from '../Logo'
import styles from './index.module.css'

export interface TitleBarProps {
  branch?: string
  /** Kept so callers can still wire the Cmd+K palette; no titlebar pill renders it. */
  onOpenPalette?: () => void
  onOpenSettings?: () => void
}

export function TitleBar({ branch = 'main', onOpenSettings }: TitleBarProps) {
  const t = useT()
  const settings = useThemeStore((s) => s.settings)
  const isDark = settings.mode === 'dark'

  const toggleMode = () => setThemeSettings({ mode: isDark ? 'light' : 'dark' })
  const cycleTheme = () => {
    const ids = THEMES.map((td) => td.id)
    const cur = ids.indexOf(settings.theme)
    setThemeSettings({ theme: ids[(cur + 1) % ids.length] })
  }

  return (
    <header className={`${styles.titlebar} titlebar`}>
      <div className={styles.titlebarLeft}>
        <Logo size={20} className={styles.titlebarLogo} />
        <span className={styles.appName}>DeepSeekCode</span>
        <span className={styles.branch} title={t('titlebar.branch')}>
          <IconGitBranch size={14} />
          {branch}
        </span>
      </div>

      <div className={styles.titlebarRight}>
        <button
          className={styles.iconBtn}
          data-testid="open-settings"
          aria-label={t('settings.title', 'Settings')}
          title={t('settings.title', 'Settings')}
          onClick={() => onOpenSettings?.()}
        >
          <IconSettings size={16} />
        </button>
        <button className={styles.iconBtn} aria-label={t('titlebar.theme')} onClick={cycleTheme} data-testid="cycle-theme">
          <IconPalette size={16} />
        </button>
        <button className={styles.iconBtn} data-testid="toggle-mode" aria-label={t('titlebar.toggleMode')} onClick={toggleMode}>
          {isDark ? <IconSun size={16} /> : <IconMoon size={16} />}
        </button>
      </div>
    </header>
  )
}
