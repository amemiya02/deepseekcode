import { X } from 'lucide-react'
import { useT } from '../lib/i18n'

export interface ContextPill { id: string; label: string }

export function ContextPills({ items, onRemove }: { items: ContextPill[]; onRemove: (id: string) => void }) {
  const t = useT()
  if (items.length === 0) return null
  return (
    <div className="context-pills" aria-label={t('composer.contextItems', 'Context items')}>
      {items.map((p) => (
        <span className="context-pill" key={p.id}>
          <span className="context-pill__label">{p.label}</span>
          <button
            type="button"
            className="context-pill__remove"
            data-testid={`pill-remove-${p.id}`}
            onClick={() => onRemove(p.id)}
            aria-label={t('composer.removeReference', 'Remove')}
          >
            <X size={12} />
          </button>
        </span>
      ))}
    </div>
  )
}
