import { t } from '../../lib/i18n'
import { useConfig } from '../../lib/useConfig'
import { Switch } from '../Switch'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function DuetSection() {
  const { cfg, error, patch, clearError } = useConfig()

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.duet', 'Duet')}</h2>
        <p className={styles.sub}>{t('settings.duetSub', 'Second-model validation for destructive operations.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>x</button>
        </div>
      )}
      <div className={styles.group}>
        <div className={styles.row}>
          <div className={styles.rowText}>
            <div className={styles.rowLabel}>{t('settings.duetEnable', 'Enable Duet validation')}</div>
            <div className={styles.rowDesc}>{t('settings.duetNote', 'Duet runs a second model pass to validate destructive operations before they execute.')}</div>
          </div>
          <Switch checked={cfg.duetEnabled} onChange={(v) => void patch({ duetEnabled: v })} label={t('settings.duetEnable', 'Enable Duet validation')} testid="duet-enable" />
        </div>
      </div>
    </div>
  )
}
