import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const MiB = 1024 * 1024

function toMiB(bytes: number): string {
  return (bytes / MiB).toFixed(0)
}

function fromMiB(s: string): number {
  const n = parseFloat(s)
  return isNaN(n) || n <= 0 ? MiB : Math.round(n * MiB)
}

export function BudgetSection() {
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
      <h2 className={styles.h2}>{t('settings.budget', 'Budget & Cache')}</h2>
      <label className={styles.field}>
        {t('settings.maxReadBytes', 'Max file read size (MiB)')}
        <input
          className={styles.input}
          type="number"
          min={1}
          max={512}
          value={toMiB(cfg.maxReadBytes)}
          onChange={(e) => void patch({ maxReadBytes: fromMiB(e.target.value) })}
        />
      </label>
      <label className={styles.field}>
        {t('settings.maxWriteBytes', 'Max file write size (MiB)')}
        <input
          className={styles.input}
          type="number"
          min={1}
          max={512}
          value={toMiB(cfg.maxWriteBytes)}
          onChange={(e) => void patch({ maxWriteBytes: fromMiB(e.target.value) })}
        />
      </label>
      <p className={styles.note}>
        {t('settings.budgetNote', 'File read/write caps guard against accidental large-file operations. The prefix cache is managed automatically; cache metrics are visible in the Cockpit tab.')}
      </p>
    </div>
  )
}
