// Adapted from deepseek-reasonix (MIT) — SettingsPanel.tsx KeyField (password input + set/save button).
import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { connectKey, fetchConfig } from '../../lib/system'
import styles from './sections.module.css'

type Status = 'idle' | 'checking' | 'ok' | 'error'

export function ProvidersSection() {
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('https://api.deepseek.com')
  const [model, setModel] = useState('deepseek-v4-flash')
  const [status, setStatus] = useState<Status>('idle')
  const [message, setMessage] = useState('')

  useEffect(() => {
    void fetchConfig().then((c) => {
      setBaseUrl(c.baseUrl || 'https://api.deepseek.com')
      setModel(c.model || 'deepseek-v4-flash')
    }).catch(() => {})
  }, [])

  async function validate() {
    setStatus('checking')
    setMessage('')
    try {
      await connectKey({ apiKey, baseUrl, model })
      setStatus('ok')
      setMessage(t('settings.keyConnected', 'Connected — key is valid.'))
    } catch (e) {
      setStatus('error')
      setMessage(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div>
      <h2 className={styles.h2}>{t('settings.providers', 'Providers & Keys')}</h2>
      <label className={styles.field}>
        {t('settings.apiKey', 'API key')}
        <input className={`${styles.input} ${styles.mono}`} type="password" autoComplete="off" value={apiKey} onChange={(e) => { setApiKey(e.target.value); if (status === 'ok') setStatus('idle') }} />
      </label>
      <label className={styles.field}>
        {t('settings.baseUrl', 'Base URL')}
        <input className={styles.input} type="url" value={baseUrl} onChange={(e) => { setBaseUrl(e.target.value); if (status === 'ok') setStatus('idle') }} />
      </label>
      <label className={styles.field}>
        {t('settings.model', 'Model')}
        <input className={styles.input} type="text" value={model} onChange={(e) => { setModel(e.target.value); if (status === 'ok') setStatus('idle') }} />
      </label>
      <button className={styles.button} onClick={() => void validate()} disabled={status === 'checking' || !apiKey || !baseUrl}>
        {status === 'checking' ? t('settings.validating', 'Validating…') : status === 'ok' ? t('settings.saved', 'Saved') : t('settings.validateConnect', 'Validate & connect')}
      </button>
      {status === 'ok' && <p className={styles.ok} role="status">{message}</p>}
      {status === 'error' && <p className={styles.err} role="alert">{message}</p>}
    </div>
  )
}
