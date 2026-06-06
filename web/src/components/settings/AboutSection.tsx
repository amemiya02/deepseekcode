import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchUpdate, type UpdateInfo } from '../../lib/system'
import { UpdateBanner } from '../UpdateBanner'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const REPO_URL = 'https://github.com/amemiya02/deepseekcode'

export function AboutSection() {
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  async function check() {
    setLoading(true); setError('')
    try { setInfo(await fetchUpdate()) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setLoading(false) }
  }
  useEffect(() => { void check() }, [])

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.about', 'About')}</h2>
        <p className={styles.sub}>{t('settings.aboutSub', 'Version, updates, and project links.')}</p>
      </div>

      {loading && !info ? (
        <StateView kind="loading" message={t('settings.checkingUpdate', 'Checking for updates…')} />
      ) : error ? (
        <StateView kind="error" message={error} onRetry={check} />
      ) : info ? (
        <div className={styles.group}>
          {info.updateAvailable && <UpdateBanner info={info} />}
          <p className={styles.note}>
            {t('settings.currentVersion', 'Current version:')} <strong>{info.current}</strong>
          </p>
          {!info.updateAvailable && (
            <p className={styles.ok} role="status">{t('settings.upToDate', "You're up to date.")}</p>
          )}
          <button className={`${styles.button} ${styles.secondary}`} onClick={() => void check()} disabled={loading}>
            {t('settings.checkNow', 'Check now')}
          </button>
        </div>
      ) : null}

      <div className={styles.group}>
        <p className={styles.groupTitle}>{t('settings.aboutLinks', 'Project')}</p>
        <p className={styles.note}>
          <a href={REPO_URL} target="_blank" rel="noreferrer">{t('settings.repository', 'GitHub repository')}</a>
        </p>
        <p className={styles.fieldHelp}>{t('settings.license', 'Licensed under the project LICENSE.')}</p>
      </div>
    </div>
  )
}
