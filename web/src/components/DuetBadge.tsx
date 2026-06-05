// DuetBadge — surfaces the existing Duet pro-validation result (spec §5.1). A quiet
// note on the turn: ✓ Validated by Pro (decision "allow") or ⚠ Pro proposed changes
// (decision "deny"), with the pro model's reasoning. Driven by the `duet` SSE event.
import { ShieldCheck, ShieldAlert } from 'lucide-react'
import { useLocale } from '../lib/i18n'

export function DuetBadge({ decision, reason }: { decision: string; reason: string }) {
  const { t } = useLocale()
  const denied = decision === 'deny'
  return (
    <div className={`duet${denied ? ' duet--warn' : ''}`} role="note" data-testid="duet-badge" title={reason}>
      {denied ? <ShieldAlert size={13} aria-hidden="true" /> : <ShieldCheck size={13} aria-hidden="true" />}
      <span className="duet__label">
        {denied ? t('duet.blocked', 'Pro proposed changes') : t('duet.ok', 'Validated by Pro')}
      </span>
      {reason && <span className="duet__reason">{reason}</span>}
    </div>
  )
}
