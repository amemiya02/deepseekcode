import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { EFFORT_LEVELS } from '../../lib/api'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function ModelsSection() {
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
      <h2 className={styles.h2}>{t('settings.models', 'Models & Routing')}</h2>
      <label className={styles.field}>
        {t('settings.model', 'Default model')}
        <input
          className={styles.input}
          type="text"
          value={cfg.model}
          onChange={(e) => void patch({ model: e.target.value })}
          placeholder="deepseek-v4-flash"
        />
      </label>
      <label className={styles.field}>
        {t('settings.reasoningEffort', 'Reasoning effort')}
        <select
          className={styles.select}
          value={cfg.reasoningEffort}
          onChange={(e) => void patch({ reasoningEffort: e.target.value })}
        >
          {EFFORT_LEVELS.map((lv) => (
            <option key={lv} value={lv}>{lv}</option>
          ))}
        </select>
      </label>
      <label className={styles.toggle}>
        <input
          type="checkbox"
          checked={cfg.autoRoute}
          onChange={(e) => void patch({ autoRoute: e.target.checked })}
        />
        {t('settings.autoRoute', 'Auto-route (flash-first)')}
      </label>
      {cfg.autoRoute && (
        <label className={styles.field}>
          {t('settings.escalationModel', 'Escalation model')}
          <input
            className={styles.input}
            type="text"
            value={cfg.escalationModel}
            onChange={(e) => void patch({ escalationModel: e.target.value })}
            placeholder="deepseek-v4-pro"
          />
        </label>
      )}
      <label className={styles.toggle}>
        <input
          type="checkbox"
          checked={cfg.autoReasoning}
          onChange={(e) => void patch({ autoReasoning: e.target.checked })}
        />
        {t('settings.autoReasoning', 'Auto reasoning')}
      </label>
      <p className={styles.note}>
        {t('settings.autoRouteNote', 'Auto-route sends mechanical turns to the flash model and escalates reasoning/ambiguity turns to the pro model, cutting cost.')}
      </p>
    </div>
  )
}
