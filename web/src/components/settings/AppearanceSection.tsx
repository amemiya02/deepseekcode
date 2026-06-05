import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { setThemeSettings, type ThemeSettings } from '../../lib/theme/store'
import { type Accent, type Theme, type Density } from '../../lib/theme/tokens'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const THEMES: Theme[] = ['graphite', 'lumen', 'halo']
const ACCENTS: Accent[] = ['indigo', 'terracotta', 'emerald', 'amber', 'rose', 'cyan', 'violet', 'slate']
const DENSITIES: Density[] = ['comfortable', 'compact']

export function AppearanceSection() {
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
    // Live-apply ONLY the visual keys that actually changed — never spread
    // undefined siblings, which would wipe theme/density in the theme store.
    const visual: Partial<ThemeSettings> = {}
    if (p.accent !== undefined) visual.accent = p.accent as Accent
    if (p.density !== undefined) visual.density = p.density as Density
    if (Object.keys(visual).length > 0) setThemeSettings(visual)
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
      <h2 className={styles.h2}>{t('settings.appearance', 'Appearance')}</h2>
      <label className={styles.field}>
        {t('settings.theme', 'Theme')}
        <select className={styles.select} value={cfg.theme} onChange={(e) => void patch({ theme: e.target.value })}>
          {THEMES.map((th) => (
            <option key={th} value={th}>{th}</option>
          ))}
        </select>
      </label>
      <label className={styles.field}>
        {t('settings.accent', 'Accent')}
        <select className={styles.select} value={cfg.accent} onChange={(e) => void patch({ accent: e.target.value })}>
          {ACCENTS.map((a) => (
            <option key={a} value={a}>{a}</option>
          ))}
        </select>
      </label>
      <label className={styles.field}>
        {t('settings.density', 'Density')}
        <select className={styles.select} value={cfg.density} onChange={(e) => void patch({ density: e.target.value })}>
          {DENSITIES.map((d) => (
            <option key={d} value={d}>{d}</option>
          ))}
        </select>
      </label>
    </div>
  )
}
