import { ArrowRight } from 'lucide-react'

export function RoutingActivity({ from, to, reason }: { from: string; to: string; reason: string }) {
  return (
    <div className="routing" role="note">
      <span className="routing__from">{from}</span>
      <ArrowRight className="routing__arrow" size={12} />
      <span className="routing__to">{to}</span>
      {reason && <span className="routing__reason">{reason}</span>}
    </div>
  )
}
