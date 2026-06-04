import { useEffect } from 'react'
import { useCockpitStore } from '../../lib/cockpitStore'
import { CacheCard } from './CacheCard'
import { CostCard } from './CostCard'
import { RoutingCard } from './RoutingCard'
import { LedgerDrillDown } from './LedgerDrillDown'
import styles from './Cockpit.module.css'

export function Cockpit({ sessionId = '' }: { sessionId?: string }) {
  const liveCache = useCockpitStore((s) => s.liveCache)
  const liveCost = useCockpitStore((s) => s.liveCost)
  const routing = useCockpitStore((s) => s.routing)
  const balance = useCockpitStore((s) => s.balance)
  const currency = useCockpitStore((s) => s.currency)
  const connect = useCockpitStore((s) => s.connect)
  const disconnect = useCockpitStore((s) => s.disconnect)
  const refreshBalance = useCockpitStore((s) => s.refreshBalance)

  useEffect(() => {
    if (!sessionId) return
    connect(sessionId)
    void refreshBalance()
    return () => disconnect()
  }, [sessionId, connect, disconnect, refreshBalance])

  return (
    <div className={styles.cockpit}>
      <CacheCard turnPct={liveCache.turn_pct} avgPct={liveCache.avg_pct} prefixes={liveCache.prefixes} eviction={liveCache.eviction} />
      <CostCard turnCny={liveCost.turn_cny} sessionCny={liveCost.session_cny} outputTokens={liveCost.output_tokens} balance={balance} currency={currency} />
      <RoutingCard hops={routing} />
      <LedgerDrillDown sessionId={sessionId} />
    </div>
  )
}
