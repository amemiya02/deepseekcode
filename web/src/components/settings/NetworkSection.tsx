// Adapted from deepseek-reasonix (MIT) — SettingsPanel.tsx NetworkSection (proxy modes + custom fields).
import { useState } from 'react'
import { t } from '../../lib/i18n'
import styles from './sections.module.css'

const MODES = ['auto', 'env', 'custom', 'off'] as const
const SCHEMES = ['http', 'https', 'socks5', 'socks5h'] as const

export function NetworkSection() {
  const [mode, setMode] = useState<(typeof MODES)[number]>('auto')
  const [scheme, setScheme] = useState<(typeof SCHEMES)[number]>('http')
  const [url, setUrl] = useState('')
  const [noProxy, setNoProxy] = useState('')

  return (
    <div>
      <h2 className={styles.h2}>{t('settings.network', 'Network / Proxy')}</h2>
      <label className={styles.field}>
        {t('settings.proxyMode', 'Proxy mode')}
        <select className={styles.select} value={mode} onChange={(e) => setMode(e.target.value as (typeof MODES)[number])}>
          {MODES.map((m) => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
      </label>

      {mode === 'custom' && (
        <>
          <label className={styles.field}>
            {t('settings.proxyScheme', 'Proxy scheme')}
            <select className={styles.select} value={scheme} onChange={(e) => setScheme(e.target.value as (typeof SCHEMES)[number])}>
              {SCHEMES.map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </label>
          <label className={styles.field}>
            {t('settings.proxyUrl', 'Proxy URL')}
            <input className={styles.input} type="text" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="host:port" />
          </label>
          <label className={styles.field}>
            no_proxy
            <input className={styles.input} type="text" value={noProxy} onChange={(e) => setNoProxy(e.target.value)} placeholder="localhost,127.0.0.1,.internal" />
          </label>
        </>
      )}

      <p className={styles.note}>{t('settings.proxyEnvNote', 'In env mode, DEEPSEEKCODE_PROXY / HTTPS_PROXY / NO_PROXY from the environment are used.')}</p>
      <p className={styles.note}>{t('settings.cjkNote', 'Non-UTF-8 files (GBK / GB18030) are detected and round-tripped so CJK content is preserved on read and write.')}</p>
    </div>
  )
}
