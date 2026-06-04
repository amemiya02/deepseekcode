import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchUpdate, type UpdateInfo } from '../../lib/system'
import { UpdateBanner } from '../UpdateBanner'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

export function UpdatesSection() {
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  async function check() {
    setLoading(true)
    setError('')
    try {
      setInfo(await fetchUpdate())
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => {
    void check()
  }, [])

  return (
    <div>
      <h2 className={styles.h2}>{t('settings.updates', 'Updates')}</h2>
      {loading && !info ? (
        <StateView kind="loading" message={t('settings.checkingUpdate', 'Checking for updates…')} />
      ) : error ? (
        <StateView kind="error" message={error} onRetry={check} />
      ) : info ? (
        <>
          {info.updateAvailable && <UpdateBanner info={info} />}
          <p className={styles.note}>
            {t('settings.currentVersion', 'Current version:')} <strong>{info.current}</strong>
          </p>
          {!info.updateAvailable && (
            <p className={styles.ok} role="status">
              {t('settings.upToDate', "You're up to date.")}
            </p>
          )}
          <button className={`${styles.button} ${styles.secondary}`} onClick={() => void check()} disabled={loading}>
            {t('settings.checkNow', 'Check now')}
          </button>
        </>
      ) : null}
    </div>
  )
}
