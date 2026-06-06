import { t } from '../../lib/i18n'
import { useConfig } from '../../lib/useConfig'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function SessionsSection() {
  const { cfg, error, patch, clearError } = useConfig()

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.sessions', 'Sessions & Storage')}</h2>
        <p className={styles.sub}>{t('settings.sessionsSub', 'Retention, snapshots, and auto-resume settings.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>×</button>
        </div>
      )}
      <div className={styles.group}>
        <label className={styles.field}>
          {t('settings.sessionTTL', 'Session retention (days)')}
          <input
            className={styles.input}
            type="number"
            min={1}
            max={3650}
            value={cfg.sessionTTLDays}
            onChange={(e) => {
              const v = parseInt(e.target.value, 10)
              if (v > 0) void patch({ sessionTTLDays: v })
            }}
            data-testid="sessions-ttl"
          />
        </label>
        <label className={styles.field}>
          {t('settings.sessionSnapshotKeep', 'Snapshots to keep per session')}
          <input
            className={styles.input}
            type="number"
            min={1}
            max={500}
            value={cfg.sessionSnapshotKeep}
            onChange={(e) => {
              const v = parseInt(e.target.value, 10)
              if (v > 0) void patch({ sessionSnapshotKeep: v })
            }}
            data-testid="sessions-snapshot"
          />
        </label>
        <label className={styles.field}>
          {t('settings.sessionAutoResumeAge', 'Auto-resume window (hours)')}
          <input
            className={styles.input}
            type="number"
            min={1}
            max={8760}
            value={cfg.sessionAutoResumeAge}
            onChange={(e) => {
              const v = parseInt(e.target.value, 10)
              if (v > 0) void patch({ sessionAutoResumeAge: v })
            }}
            data-testid="sessions-resume"
          />
        </label>
      </div>
      <p className={styles.note}>
        {t('settings.sessionsNote', 'Sessions older than the retention period are pruned on next launch. Snapshots enable /undo and /rewind. Auto-resume re-attaches to sessions younger than the window.')}
      </p>
    </div>
  )
}
