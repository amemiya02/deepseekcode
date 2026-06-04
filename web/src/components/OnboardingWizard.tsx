// Adapted from deepseek-reasonix (MIT) — OnboardingOverlay.tsx (key paste → validate via Go → onComplete).
import { useState } from 'react'
import { t } from '../lib/i18n'
import { connectKey, saveConfig } from '../lib/system'
import { setThemeSettings } from '../lib/theme'
import styles from './OnboardingWizard.module.css'

type Step = 'key' | 'theme' | 'permission'

const THEMES = ['graphite', 'lumen', 'halo']
const PERMS = ['ask', 'auto-edit', 'plan', 'yolo']

export interface OnboardingWizardProps {
  open: boolean
  onComplete?: () => void
}

export function OnboardingWizard({ open, onComplete }: OnboardingWizardProps) {
  const [step, setStep] = useState<Step>('key')
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('https://api.deepseek.com')
  const [model, setModel] = useState('deepseek-v4')
  const [chosenTheme, setChosenTheme] = useState('graphite')
  const [permissionDefault, setPermissionDefault] = useState('ask')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  if (!open) return null

  async function nextFromKey() {
    setBusy(true)
    setError('')
    try {
      await connectKey({ apiKey, baseUrl, model })
      setStep('theme')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  function nextFromTheme() {
    setThemeSettings({ theme: chosenTheme })
    setStep('permission')
  }

  async function finish() {
    setBusy(true)
    setError('')
    try {
      await saveConfig({ theme: chosenTheme })
      onComplete?.()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={styles.overlay} role="dialog" aria-modal="true" aria-label={t('onboarding.title', 'Welcome')}>
      <div className={styles.card}>
        {step === 'key' && (
          <>
            <h2 className={styles.h2}>{t('onboarding.connectKey', 'Connect your API key')}</h2>
            <label className={styles.field}>
              {t('settings.apiKey', 'API key')}
              <input className={styles.input} type="password" autoComplete="off" value={apiKey} onChange={(e) => setApiKey(e.target.value)} />
            </label>
            <label className={styles.field}>
              {t('settings.baseUrl', 'Base URL')}
              <input className={styles.input} type="url" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} />
            </label>
            <label className={styles.field}>
              {t('settings.model', 'Model')}
              <input className={styles.input} type="text" value={model} onChange={(e) => setModel(e.target.value)} />
            </label>
            {error && <p className={styles.err} role="alert">{error}</p>}
            <button className={styles.button} onClick={() => void nextFromKey()} disabled={busy || !apiKey || !baseUrl}>
              {t('onboarding.next', 'Next')}
            </button>
          </>
        )}

        {step === 'theme' && (
          <>
            <h2 className={styles.h2}>{t('onboarding.chooseTheme', 'Choose a theme')}</h2>
            <div className={styles.choices}>
              {THEMES.map((th) => (
                <button
                  key={th}
                  className={chosenTheme === th ? `${styles.choice} ${styles.sel}` : styles.choice}
                  onClick={() => setChosenTheme(th)}
                >
                  {th}
                </button>
              ))}
            </div>
            <button className={styles.button} onClick={nextFromTheme} disabled={busy}>
              {t('onboarding.next', 'Next')}
            </button>
          </>
        )}

        {step === 'permission' && (
          <>
            <h2 className={styles.h2}>{t('onboarding.permissionDefault', 'Default autonomy')}</h2>
            <div className={styles.choices}>
              {PERMS.map((p) => (
                <button
                  key={p}
                  className={permissionDefault === p ? `${styles.choice} ${styles.sel}` : styles.choice}
                  onClick={() => setPermissionDefault(p)}
                >
                  {p}
                </button>
              ))}
            </div>
            {error && <p className={styles.err} role="alert">{error}</p>}
            <button className={styles.button} onClick={() => void finish()} disabled={busy}>
              {t('onboarding.finish', 'Finish')}
            </button>
          </>
        )}
      </div>
    </div>
  )
}
