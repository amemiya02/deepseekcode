import { useEffect } from 'react'
import { useCockpitStore } from '../lib/cockpitStore'
import { StatusBar, type AgentStatus } from './StatusBar'

export function StatusBarLive({
  sessionId = '',
  model = '',
  effort = '',
  status = 'idle',
  ctxPct = 0,
}: {
  sessionId?: string
  model?: string
  effort?: string
  status?: AgentStatus
  ctxPct?: number
}) {
  const liveCache = useCockpitStore((s) => s.liveCache)
  const liveCost = useCockpitStore((s) => s.liveCost)
  const jobs = useCockpitStore((s) => s.jobs)
  const retry = useCockpitStore((s) => s.retry)
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

  // ±lines are best-effort: wired to 0 until a per-turn diff-count source is available.
  // When the ReviewPanel diff source exposes per-turn add/del counts, wire them here.
  return (
    <StatusBar
      status={status}
      model={model}
      effort={effort}
      ctxPct={ctxPct}
      turnCachePct={liveCache.turn_pct}
      avgCachePct={liveCache.avg_pct}
      eviction={liveCache.eviction}
      turnCny={liveCost.turn_cny}
      sessionCny={liveCost.session_cny}
      balance={balance}
      currency={currency}
      jobs={jobs}
      retryAttempt={retry.attempt}
      retryMax={retry.max}
      linesAdded={0}
      linesRemoved={0}
    />
  )
}
