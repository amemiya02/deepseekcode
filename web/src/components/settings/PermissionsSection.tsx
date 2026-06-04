import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const PERMISSION_LEVELS = ['ask', 'auto', 'full'] as const

export function PermissionsSection() {
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
      <h2 className={styles.h2}>{t('settings.permissions', 'Permissions & Autonomy')}</h2>
      <label className={styles.field}>
        {t('settings.permissionDefault', 'Default autonomy level')}
        <select
          className={styles.select}
          value={cfg.permissionDefault || 'ask'}
          onChange={(e) => void patch({ permissionDefault: e.target.value })}
        >
          {PERMISSION_LEVELS.map((lv) => (
            <option key={lv} value={lv}>{lv}</option>
          ))}
        </select>
      </label>
      <label className={styles.toggle}>
        <input
          type="checkbox"
          checked={cfg.autoClarify}
          onChange={(e) => void patch({ autoClarify: e.target.checked })}
        />
        {t('settings.autoClarify', 'Auto clarify under-specified prompts')}
      </label>
      <p className={styles.note}>
        {t('settings.permissionsNote', '"ask" prompts before every tool call. "auto" approves safe reads and branches but asks for writes. "full" skips all confirmations — use only in trusted, isolated environments.')}
      </p>
    </div>
  )
}
