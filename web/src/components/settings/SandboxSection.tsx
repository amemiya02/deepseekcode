import { t } from '../../lib/i18n'
import { useConfig } from '../../lib/useConfig'
import { Switch } from '../Switch'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function SandboxSection() {
  const { cfg, error, patch, clearError } = useConfig()

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.sandbox', 'Sandbox')}</h2>
        <p className={styles.sub}>{t('settings.sandboxSub', 'Workspace confinement and network egress control.')}</p>
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
            <div className={styles.rowLabel}>{t('settings.sandboxEnable', 'Enable sandbox')}</div>
            <div className={styles.rowDesc}>{t('settings.sandboxNote', 'Workspace confinement plus a macOS Seatbelt bash jail.')}</div>
          </div>
          <Switch checked={cfg.sandboxEnabled} onChange={(v) => void patch({ sandboxEnabled: v })} label={t('settings.sandboxEnable', 'Enable sandbox')} testid="sandbox-enable" />
        </div>
        <div className={styles.row}>
          <div className={styles.rowText}>
            <div className={styles.rowLabel}>{t('settings.sandboxNetwork', 'Allow network egress')}</div>
            <div className={styles.rowDesc}>{t('settings.sandboxNetworkNote', 'When disabled, sandboxed commands cannot make network requests.')}</div>
          </div>
          <Switch checked={cfg.sandboxNetwork} disabled={!cfg.sandboxEnabled} onChange={(v) => void patch({ sandboxNetwork: v })} label={t('settings.sandboxNetwork', 'Allow network egress')} testid="sandbox-network" />
        </div>
      </div>
    </div>
  )
}
