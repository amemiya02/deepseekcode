import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { fetchDoctor, type DoctorReport } from '../../lib/system'
import { StateView } from '../StateViews'
import styles from './DoctorView.module.css'

export function DoctorView() {
  const [report, setReport] = useState<DoctorReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  async function run() {
    setLoading(true)
    setError('')
    try {
      setReport(await fetchDoctor())
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => {
    void run()
  }, [])

  return (
    <div>
      <div className={styles.head}>
        <h2 className={styles.h2}>{t('settings.doctor', 'Doctor')}</h2>
        <button className={styles.button} onClick={() => void run()} disabled={loading}>
          {t('settings.doctorRun', 'Run again')}
        </button>
      </div>

      {loading && !report ? (
        <StateView kind="loading" message={t('settings.doctorRunning', 'Running diagnostics…')} />
      ) : error ? (
        <StateView kind="error" message={error} onRetry={run} />
      ) : report ? (
        <ul className={styles.checks}>
          {report.checks.map((c) => (
            <li key={c.name} data-ok={c.ok}>
              <span className={c.ok ? `${styles.dot} ${styles.ok}` : `${styles.dot} ${styles.bad}`} aria-hidden="true" />
              <span className={styles.name}>{c.name}</span>
              <span className={styles.detail}>{c.detail}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
