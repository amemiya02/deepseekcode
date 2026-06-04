import { IconGitBranch, IconCommand, IconSun, IconMoon, IconPalette } from '../../lib/icons'
import { useT } from '../../lib/i18n'
import { useThemeStore, setThemeSettings } from '../../lib/theme/store'
import { THEMES } from '../../lib/theme/tokens'
import styles from './index.module.css'

export interface TitleBarProps {
  branch?: string
  onOpenPalette?: () => void
}

export function TitleBar({ branch = 'main', onOpenPalette }: TitleBarProps) {
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
    <header className={styles.titlebar}>
      <div className={styles.titlebarLeft}>
        <span className={styles.branch} title={t('titlebar.branch')}>
          <IconGitBranch size={14} />
          {branch}
        </span>
      </div>

      <button className={styles.paletteTrigger} data-testid="open-palette" onClick={() => onOpenPalette?.()}>
        <IconCommand size={14} />
        <span>{t('app.openPalette')}</span>
        <kbd>⌘K</kbd>
      </button>

      <div className={styles.titlebarRight}>
        <button className={styles.iconBtn} aria-label={t('titlebar.theme')} onClick={cycleTheme}>
          <IconPalette size={16} />
        </button>
        <button className={styles.iconBtn} data-testid="toggle-mode" aria-label={t('titlebar.toggleMode')} onClick={toggleMode}>
          {isDark ? <IconSun size={16} /> : <IconMoon size={16} />}
        </button>
      </div>
    </header>
  )
}
