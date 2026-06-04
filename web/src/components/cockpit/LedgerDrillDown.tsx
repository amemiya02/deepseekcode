import { useEffect, useState } from 'react'
import { fetchCacheLedger, type CacheLedgerRow } from '../../lib/api'
import { t } from '../../lib/i18n'
import { formatTokens } from '../../lib/format'
import styles from './LedgerDrillDown.module.css'

export function LedgerDrillDown({ sessionId = '', turn }: { sessionId?: string; turn?: number }) {
  const [rows, setRows] = useState<CacheLedgerRow[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    if (!sessionId) return
    let cancelled = false
    setLoading(true)
    setError('')
    fetchCacheLedger(sessionId, turn)
      .then((r) => {
        if (cancelled) return
        setRows(r)
        setLoaded(true)
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [sessionId, turn])

  return (
    <section className={styles.card} data-testid="ledger-card">
      <h3 className={styles.title}>{t('cockpit.ledger.title', 'Per-turn ledger')}</h3>

      {!sessionId ? (
        <p className={styles.muted}>{t('cockpit.ledger.noSession', 'No active session.')}</p>
      ) : loading ? (
        <p className={styles.muted}>{t('common.loading', 'Loading…')}</p>
      ) : error ? (
        <p className={styles.error}>{error}</p>
      ) : loaded && rows.length === 0 ? (
        <p className={styles.muted} data-testid="ledger-empty">{t('cockpit.ledger.empty', 'No ledger rows yet.')}</p>
      ) : rows.length > 0 ? (
        <table className={styles.ledger} data-testid="ledger-table">
          <thead>
            <tr>
              <th>{t('cockpit.ledger.turn', 'Turn')}</th>
              <th>{t('cockpit.ledger.hit', 'Hit')}</th>
              <th>{t('cockpit.ledger.miss', 'Miss')}</th>
              <th>{t('cockpit.ledger.evicted', 'Evicted')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.turn} data-testid="ledger-row" data-evicted={row.evicted ? 'true' : 'false'} className={row.evicted ? styles.evicted : undefined}>
                <td>{row.turn}</td>
                <td>{formatTokens(row.hit_tokens)}</td>
                <td>
                  {row.evicted ? (
                    <span data-testid="ledger-row-evicted">{formatTokens(row.miss_tokens)}</span>
                  ) : (
                    formatTokens(row.miss_tokens)
                  )}
                </td>
                <td>{row.evicted ? t('common.yes', 'yes') : ''}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </section>
  )
}
