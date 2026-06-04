import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function KeybindingsSection() {
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
      <h2 className={styles.h2}>{t('settings.keybindings', 'Keybindings')}</h2>
      <label className={styles.toggle}>
        <input
          type="checkbox"
          checked={cfg.vimKeybindings}
          onChange={(e) => void patch({ vimKeybindings: e.target.checked })}
        />
        {t('settings.vimKeybindings', 'Vim keybindings')}
      </label>
      <p className={styles.note}>
        {t('settings.vimKeybindingsNote', 'Enables modal vi/vim navigation in the transcript and composer. Restart may be required for TUI surfaces.')}
      </p>
    </div>
  )
}
