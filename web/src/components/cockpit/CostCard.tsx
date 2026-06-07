import { t } from '../../lib/i18n'
import { formatCNY, formatTokens } from '../../lib/format'
import styles from './CostCard.module.css'

export function CostCard({
  turnCny = 0,
  sessionCny = 0,
  outputTokens = 0,
  balance = null,
  currency = 'CNY',
}: {
  turnCny?: number
  sessionCny?: number
  outputTokens?: number
  balance?: number | null
  currency?: string
}) {
  return (
    <section className={`${styles.card} ds-card`} data-testid="cost-card">
      <h3 className={styles.title}>{t('cockpit.cost.title', 'Cost')}</h3>
      <dl className={styles.rows}>
        <div className={styles.row}>
          <dt>{t('cockpit.cost.turn', 'this turn')}</dt>
          <dd data-testid="cost-turn">{formatCNY(turnCny)}</dd>
        </div>
        <div className={styles.row}>
          <dt>{t('cockpit.cost.session', 'session')}</dt>
          <dd data-testid="cost-session">{formatCNY(sessionCny)}</dd>
        </div>
        <div className={styles.row}>
          <dt>{t('cockpit.cost.output', 'output tokens')}</dt>
          <dd data-testid="cost-output">{formatTokens(outputTokens)}</dd>
        </div>
        <div className={styles.row}>
          <dt>{t('cockpit.cost.balance', 'balance')}</dt>
          <dd data-testid="cost-balance">{balance == null ? '—' : `${balance} ${currency}`}</dd>
        </div>
      </dl>
    </section>
  )
}
