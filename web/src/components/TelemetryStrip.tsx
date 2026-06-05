import { useEffect, useState } from 'react'
import { ChevronUp, ChevronDown } from 'lucide-react'
import { useCockpitStore } from '../lib/cockpitStore'
import { Cockpit } from './cockpit/Cockpit'
import { formatPct, formatCNY } from '../lib/format'
import { t } from '../lib/i18n'
import styles from './TelemetryStrip.module.css'

export function TelemetryStrip({ sessionId = '' }: { sessionId?: string }) {
  const [open, setOpen] = useState(false)
  const liveCache = useCockpitStore((s) => s.liveCache)
  const liveCost = useCockpitStore((s) => s.liveCost)
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
    <div className={styles.strip} data-expanded={open}>
      <button
        type="button"
        className={styles.bar}
        data-testid="telemetry-toggle"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <span className={styles.compact} data-testid="telemetry-compact">
          <span className={styles.metric}>{t('telemetry.cache', 'cache')} {formatPct(liveCache.turn_pct)}</span>
          <span className={styles.metric}>{formatCNY(liveCost.session_cny)}</span>
        </span>
        {open ? <ChevronDown size={14} aria-hidden /> : <ChevronUp size={14} aria-hidden />}
      </button>
      {open && (
        <div className={styles.expanded} data-testid="telemetry-expanded">
          <Cockpit sessionId={sessionId} manageConnection={false} />
        </div>
      )}
    </div>
  )
}
