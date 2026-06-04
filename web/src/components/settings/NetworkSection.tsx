// Adapted from deepseek-reasonix (MIT) — SettingsPanel.tsx NetworkSection (proxy modes + custom fields).
import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchConfig, saveConfig, type ConfigDTO } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const MODES = ['auto', 'env', 'custom', 'off'] as const
const SCHEMES = ['http', 'https', 'socks5', 'socks5h'] as const

export function NetworkSection() {
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
      <h2 className={styles.h2}>{t('settings.network', 'Network / Proxy')}</h2>
      <label className={styles.field}>
        {t('settings.proxyMode', 'Proxy mode')}
        <select
          className={styles.select}
          value={cfg.proxyMode}
          onChange={(e) => void patch({ proxyMode: e.target.value as (typeof MODES)[number] })}
        >
          {MODES.map((m) => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
      </label>

      {cfg.proxyMode === 'custom' && (
        <>
          <label className={styles.field}>
            {t('settings.proxyScheme', 'Proxy scheme')}
            <select
              className={styles.select}
              value={cfg.proxyScheme}
              onChange={(e) => void patch({ proxyScheme: e.target.value as (typeof SCHEMES)[number] })}
            >
              {SCHEMES.map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </label>
          <label className={styles.field}>
            {t('settings.proxyUrl', 'Proxy URL')}
            <input
              className={styles.input}
              type="text"
              value={cfg.proxyUrl}
              onChange={(e) => void patch({ proxyUrl: e.target.value })}
              placeholder="host:port"
            />
          </label>
          <label className={styles.field}>
            no_proxy
            <input
              className={styles.input}
              type="text"
              value={cfg.noProxy}
              onChange={(e) => void patch({ noProxy: e.target.value })}
              placeholder="localhost,127.0.0.1,.internal"
            />
          </label>
        </>
      )}

      <p className={styles.note}>{t('settings.proxyEnvNote', 'In env mode, DEEPSEEKCODE_PROXY / HTTPS_PROXY / NO_PROXY from the environment are used.')}</p>
      <p className={styles.note}>{t('settings.cjkNote', 'Non-UTF-8 files (GBK / GB18030) are detected and round-tripped so CJK content is preserved on read and write.')}</p>
    </div>
  )
}
