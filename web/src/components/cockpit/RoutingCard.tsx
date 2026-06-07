import { ArrowRight } from 'lucide-react'
import { t } from '../../lib/i18n'
import type { RoutingHop } from '../../lib/api'
import styles from './RoutingCard.module.css'

export function RoutingCard({ hops = [] }: { hops?: RoutingHop[] }) {
  return (
    <section className={`${styles.card} ds-card`} data-testid="routing-card">
      <h3 className={styles.title}>{t('cockpit.routing.title', 'Routing')}</h3>
      {hops.length === 0 ? (
        <p className={styles.empty} data-testid="routing-empty">
          {t('cockpit.routing.empty', 'No re-routing this session.')}
        </p>
      ) : (
        <ol className={styles.hops}>
          {hops.map((hop, i) => (
            <li className={styles.hop} key={i}>
              <span className={styles.node}>{hop.from}</span>
              <ArrowRight size={13} aria-hidden="true" />
              <span className={styles.node}>{hop.to}</span>
              <span className={styles.reason}>{hop.reason}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}
