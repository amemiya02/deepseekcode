import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function SandboxSection() {
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
      <h2 className={styles.h2}>{t('settings.sandbox', 'Sandbox')}</h2>
      <p className={styles.note}>
        {t('settings.sandboxNote', 'Workspace confinement plus a macOS Seatbelt bash jail; the egress gate blocks network access for sandboxed commands.')}
      </p>
      <label className={styles.toggle}>
        <input type="checkbox" checked={cfg.sandboxEnabled} onChange={(e) => void patch({ sandboxEnabled: e.target.checked })} />
        {t('settings.sandboxEnable', 'Enable sandbox')}
      </label>
      <label className={styles.toggle}>
        <input
          type="checkbox"
          checked={cfg.sandboxNetwork}
          disabled={!cfg.sandboxEnabled}
          onChange={(e) => void patch({ sandboxNetwork: e.target.checked })}
        />
        {t('settings.sandboxNetwork', 'Allow network egress')}
      </label>
    </div>
  )
}
