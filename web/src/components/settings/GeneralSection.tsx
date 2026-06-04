import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function GeneralSection() {
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
      <h2 className={styles.h2}>{t('settings.general', 'General')}</h2>
      <label className={styles.field}>
        {t('settings.language', 'Language')}
        <select className={styles.select} value={cfg.language} onChange={(e) => void patch({ language: e.target.value })}>
          <option value="en">English</option>
          <option value="zh">中文</option>
          <option value="auto">{t('settings.languageAuto', 'Auto (system)')}</option>
        </select>
      </label>
      <label className={styles.field}>
        {t('settings.transcriptVerbosity', 'Transcript verbosity')}
        <select
          className={styles.select}
          value={cfg.transcriptVerbosity}
          onChange={(e) => void patch({ transcriptVerbosity: e.target.value as ConfigDTO['transcriptVerbosity'] })}
        >
          <option value="normal">{t('settings.verbosityNormal', 'Normal')}</option>
          <option value="verbose">{t('settings.verbosityVerbose', 'Verbose')}</option>
          <option value="summary">{t('settings.verbositySummary', 'Summary')}</option>
        </select>
      </label>
      <label className={styles.toggle}>
        <input type="checkbox" checked={cfg.autoReasoning} onChange={(e) => void patch({ autoReasoning: e.target.checked })} />
        {t('settings.autoReasoning', 'Auto reasoning')}
      </label>
      <label className={styles.toggle}>
        <input type="checkbox" checked={cfg.autoClarify} onChange={(e) => void patch({ autoClarify: e.target.checked })} />
        {t('settings.autoClarify', 'Auto clarify')}
      </label>
    </div>
  )
}
