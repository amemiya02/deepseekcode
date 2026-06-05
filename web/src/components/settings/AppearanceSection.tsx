import { useEffect, useState } from 'react'
import { useT } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { setThemeSettings, useThemeStore, type ThemeSettings } from '../../lib/theme/store'
import { ACCENTS, UI_FONTS, CODE_FONTS, type Accent, type Density, type Mode } from '../../lib/theme/tokens'
import { BrandedSelect } from '../BrandedSelect'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

// Single source of truth for the accent ids — the canonical ACCENTS def from
// tokens.ts (was duplicated here as a local array that could drift).
const DENSITIES: Density[] = ['comfortable', 'compact']
const MODES: Mode[] = ['light', 'dark', 'hc']

export function AppearanceSection() {
  const t = useT()
  const [cfg, setCfg] = useState<ConfigDTO | null>(null)
  const [error, setError] = useState('')

  // Mode and fonts live in localStorage-only theme store (never in ConfigDTO).
  const { mode, uiFont, codeFont } = useThemeStore((s) => s.settings)

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

      {/* Mode — localStorage only, never through ConfigDTO */}
      <label className={styles.field}>
        {t('settings.mode', 'Appearance mode')}
        <BrandedSelect
          value={mode}
          options={MODES.map((m) => ({ value: m, label: t(`mode.${m}`, m) }))}
          onChange={(m) => setThemeSettings({ mode: m as Mode })}
          ariaLabel={t('settings.mode', 'Appearance mode')}
          testid="appearance-mode"
        />
      </label>

      {/* Accent — round-trips to Go config */}
      <label className={styles.field}>
        {t('settings.accent', 'Accent')}
        <BrandedSelect
          value={cfg.accent}
          options={ACCENTS.map((a) => ({ value: a.id, label: t(`accent.${a.id}`, a.id) }))}
          onChange={(v) => void patch({ accent: v })}
          ariaLabel={t('settings.accent', 'Accent')}
          testid="appearance-accent"
        />
      </label>

      {/* Density — round-trips to Go config */}
      <label className={styles.field}>
        {t('settings.density', 'Density')}
        <BrandedSelect
          value={cfg.density}
          options={DENSITIES.map((d) => ({ value: d, label: t(`density.${d}`, d) }))}
          onChange={(v) => void patch({ density: v })}
          ariaLabel={t('settings.density', 'Density')}
          testid="appearance-density"
        />
      </label>

      {/* UI font — localStorage only */}
      <label className={styles.field}>
        {t('settings.fontUi', 'UI font')}
        <BrandedSelect
          value={uiFont}
          options={UI_FONTS.map((f) => ({ value: f.id, label: t(`font.${f.id}`, f.id) }))}
          onChange={(v) => setThemeSettings({ uiFont: v })}
          ariaLabel={t('settings.fontUi', 'UI font')}
          testid="appearance-uifont"
        />
      </label>

      {/* Code font — localStorage only */}
      <label className={styles.field}>
        {t('settings.fontCode', 'Code font')}
        <BrandedSelect
          value={codeFont}
          options={CODE_FONTS.map((f) => ({ value: f.id, label: t(`font.${f.id}`, f.id) }))}
          onChange={(v) => setThemeSettings({ codeFont: v })}
          ariaLabel={t('settings.fontCode', 'Code font')}
          testid="appearance-codefont"
        />
      </label>
    </div>
  )
}
