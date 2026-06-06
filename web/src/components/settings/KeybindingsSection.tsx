import { t } from '../../lib/i18n'
import { useConfig } from '../../lib/useConfig'
import { Switch } from '../Switch'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function KeybindingsSection() {
  const { cfg, error, patch, clearError } = useConfig()

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.keybindings', 'Keybindings')}</h2>
        <p className={styles.sub}>{t('settings.keybindingsSub', 'Keyboard navigation mode.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>×</button>
        </div>
      )}
      <div className={styles.group}>
        <div className={styles.row}>
          <div className={styles.rowText}>
            <div className={styles.rowLabel}>{t('settings.vimKeybindings', 'Vim keybindings')}</div>
            <div className={styles.rowDesc}>{t('settings.vimKeybindingsNote', 'Enables modal vi/vim navigation in the transcript and composer.')}</div>
          </div>
          <Switch checked={cfg.vimKeybindings} onChange={(v) => void patch({ vimKeybindings: v })} label={t('settings.vimKeybindings', 'Vim keybindings')} testid="keys-vim" />
        </div>
      </div>
    </div>
  )
}
