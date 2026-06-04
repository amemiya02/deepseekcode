import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function ContextSection() {
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
      <h2 className={styles.h2}>{t('settings.context', 'Context & Memory')}</h2>
      <label className={styles.toggle}>
        <input
          type="checkbox"
          checked={cfg.autoClarify}
          onChange={(e) => void patch({ autoClarify: e.target.checked })}
        />
        {t('settings.autoClarify', 'Auto clarify under-specified prompts')}
      </label>
      <label className={styles.toggle}>
        <input
          type="checkbox"
          checked={cfg.autoReasoning}
          onChange={(e) => void patch({ autoReasoning: e.target.checked })}
        />
        {t('settings.autoReasoning', 'Auto reasoning')}
      </label>
      <p className={styles.note}>
        {t('settings.contextNote', 'Context is bounded by the model window. Auto clarify asks one question on under-specified prompts to reduce wasted turns. Auto reasoning activates extended thinking on complex queries.')}
      </p>
    </div>
  )
}
