import type { CSSProperties } from 'react'
import { CheckCircle2, AlertTriangle } from 'lucide-react'
import { t } from '../../lib/i18n'
import { formatPct } from '../../lib/format'
import styles from './CacheCard.module.css'

export function CacheCard({
  turnPct = 0,
  avgPct = 0,
  prefixes = 1,
  eviction = false,
}: {
  turnPct?: number
  avgPct?: number
  prefixes?: number
  eviction?: boolean
}) {
  const pct = Math.max(0, Math.min(1, turnPct))
  const state = eviction ? 'evict' : turnPct >= 0.6 ? 'good' : 'low'

  return (
    <section className={styles.card} data-testid="cache-card">
      <h3 className={styles.title}>{t('cockpit.cache.title', 'Cache')}</h3>

      <div className={styles.gaugeRow}>
        <div className={styles.gauge} data-state={state} style={{ '--pct': pct } as CSSProperties}>
          <span className={styles.gaugeVal} data-testid="cache-turn">{formatPct(turnPct)}</span>
          <span className={styles.gaugeCap}>{t('cockpit.cache.turn', 'this turn')}</span>
        </div>
        <div className={styles.side}>
          <span className={styles.avg} data-testid="cache-avg">{formatPct(avgPct)}</span>
          <span className={styles.lbl}>{t('cockpit.cache.avg', 'session avg')}</span>
        </div>
      </div>

      {prefixes === 1 ? (
        <p className={`${styles.proof} ${styles.ok}`} data-testid="prefix-stable">
          <CheckCircle2 size={14} aria-hidden="true" />
          {t('cockpit.cache.stable', 'prefixes')} = {prefixes} · {t('cockpit.cache.stableNote', 'stable')}
        </p>
      ) : (
        <p className={`${styles.proof} ${styles.warn}`} data-testid="prefix-unstable">
          <AlertTriangle size={14} aria-hidden="true" />
          {t('cockpit.cache.unstable', 'prefixes')} = {prefixes} · {t('cockpit.cache.unstableNote', 'unstable')}
        </p>
      )}

      {eviction && (
        <p className={`${styles.proof} ${styles.warn}`} data-testid="cache-eviction">
          <AlertTriangle size={14} aria-hidden="true" />
          {t('cockpit.cache.eviction', 'Full-body eviction this turn')}
        </p>
      )}
    </section>
  )
}
