// Adapted from deepseek-reasonix (MIT) — components/StatusBar.tsx (busy dot,
// model/effort/ctx/cache segments, ¥ cost + wallet balance chips, jobs/retry).
import { AlertTriangle, RotateCw, Loader, Wallet, Coins } from 'lucide-react'
import { t } from '../lib/i18n'
import { formatPct, formatCNY } from '../lib/format'
import styles from './StatusBar.module.css'

export type AgentStatus = 'idle' | 'streaming' | 'waiting' | 'error'

export function StatusBar({
  status = 'idle',
  model = '',
  effort = '',
  ctxPct = 0,
  turnCachePct = 0,
  avgCachePct = 0,
  eviction = false,
  turnCny = 0,
  sessionCny = 0,
  balance = null,
  currency = 'CNY',
  jobs = 0,
  retryAttempt = 0,
  retryMax = 0,
  linesAdded = 0,
  linesRemoved = 0,
}: {
  status?: AgentStatus
  model?: string
  effort?: string
  ctxPct?: number
  turnCachePct?: number
  avgCachePct?: number
  eviction?: boolean
  turnCny?: number
  sessionCny?: number
  balance?: number | null
  currency?: string
  jobs?: number
  retryAttempt?: number
  retryMax?: number
  linesAdded?: number
  linesRemoved?: number
}) {
  const ctxClass =
    ctxPct >= 0.8 ? styles.ctxDanger : ctxPct >= 0.55 ? styles.ctxWarn : ''

  return (
    <footer className={`${styles.statusBar} status-bar`} role="status">
      <span className={`${styles.dot} ds-status-dot`} data-testid="status-dot" data-status={status} aria-label={t(`status.${status}`, status)} />

      <span className={`${styles.seg} ${styles.model}`} title={t('status.model', 'Model')}>{model}</span>
      <span className={styles.seg} title={t('status.effort', 'Effort')}>{effort}</span>

      <span className={`${styles.seg} ${ctxClass}`} data-testid="ctx-pct" title={t('status.context', 'Context used')}>
        <span
          className={styles.ctxRing}
          data-testid="ctx-ring"
          style={{ ['--p' as string]: `${Math.round(ctxPct * 100)}%` }}
          aria-hidden="true"
        />
        {t('status.ctx', 'ctx')} {formatPct(ctxPct)}
      </span>

      <span className={`${styles.seg} ${styles.cache}`} data-testid="turn-cache" title={t('status.turnCache', 'This-turn cache hit')}>
        {t('status.cacheTurn', 'cache')} {formatPct(turnCachePct)}
      </span>
      <span className={`${styles.seg} ${styles.cache}`} data-testid="avg-cache" title={t('status.avgCache', 'Session average cache hit')}>
        {t('status.cacheAvg', 'avg')} {formatPct(avgCachePct)}
      </span>

      {eviction && (
        <span className={`${styles.seg} ${styles.warn}`} data-testid="eviction-warning" title={t('status.eviction', 'Full-body cache eviction')}>
          <AlertTriangle size={13} aria-hidden="true" />
          {t('status.evicted', 'evicted')}
        </span>
      )}

      <span className={styles.spacer} />

      {(linesAdded > 0 || linesRemoved > 0) && (
        <span className={styles.seg} data-testid="diff-lines" title={t('status.lines', 'Lines changed this turn')}>
          <span className={styles.add}>+{linesAdded}</span>
          <span className={styles.del}>−{linesRemoved}</span>
        </span>
      )}

      <span className={styles.seg} data-testid="turn-cost" title={t('status.turnCost', 'This-turn cost')}>
        <Coins size={11} aria-hidden="true" />
        {formatCNY(turnCny)}
      </span>
      <span className={styles.seg} data-testid="session-cost" title={t('status.sessionCost', 'Session cost')}>{formatCNY(sessionCny)}</span>
      <span className={styles.seg} data-testid="balance" title={t('status.balance', 'Wallet balance')}>
        <Wallet size={11} aria-hidden="true" />
        {balance == null ? '—' : `${balance} ${currency}`}
      </span>

      {jobs > 0 && (
        <span className={styles.seg} data-testid="jobs" title={t('status.jobs', 'Background jobs')}>
          <Loader size={13} aria-hidden="true" />
          {jobs}
        </span>
      )}

      {retryMax > 0 && (
        <span className={`${styles.seg} ${styles.retry}`} data-testid="retry" title={t('status.retry', 'Retrying')}>
          <RotateCw size={13} aria-hidden="true" />
          {retryAttempt}/{retryMax}
        </span>
      )}
    </footer>
  )
}
