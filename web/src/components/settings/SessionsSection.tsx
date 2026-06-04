import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function SessionsSection() {
  const [cfg, setCfg] = useState<ConfigDTO | null>(null)
  const [error, setError] = useState('')

  async function load() {
    setError('')
    setCfg(null)
    try {
      setCfg(await fetchConfig())
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }
  useEffect(() => {
    void load()
  }, [])

  async function patch(p: Partial<ConfigDTO>) {
    setCfg((prev) => (prev ? { ...prev, ...p } : prev))
    try {
      setCfg(await saveConfig(p))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  if (error) return <StateView kind="error" message={error} onRetry={load} />
  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <h2 className={styles.h2}>{t('settings.sessions', 'Sessions & Storage')}</h2>
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
        />
      </label>
      <p className={styles.note}>
        {t('settings.sessionsNote', 'Sessions older than the retention period are pruned on next launch. Snapshots enable /undo and /rewind. Auto-resume re-attaches to sessions younger than the window.')}
      </p>
    </div>
  )
}
