// Adapted from deepseek-reasonix (MIT) — components/HistoryPanel.tsx (row +
// turn pluralization + delete affordance).
import { Trash2 } from 'lucide-react'
import { t } from '../lib/i18n'
import type { Session } from '../lib/api'
import styles from './SessionItem.module.css'

export function SessionItem({
  session,
  active = false,
  onSelect,
  onDelete,
}: {
  session: Session
  active?: boolean
  onSelect?: (id: string) => void
  onDelete?: (id: string) => void
}) {
  const turnLabel =
    session.turns === 1
      ? t('session.turnOne', '{n} turn', { n: session.turns })
      : t('session.turnOther', '{n} turns', { n: session.turns })

  return (
    <button
      type="button"
      className={`${styles.item} ${active ? styles.active : ''}`}
      data-testid="session-item"
      aria-current={active ? 'true' : undefined}
      onClick={() => onSelect?.(session.id)}
    >
      <span className={styles.title}>{session.title}</span>
      <span className={styles.meta}>{turnLabel}</span>
      <span
        className={styles.del}
        data-testid="session-delete"
        role="button"
        tabIndex={0}
        aria-label={t('session.delete', 'Delete session')}
        onClick={(e) => {
          e.stopPropagation()
          onDelete?.(session.id)
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.stopPropagation()
            onDelete?.(session.id)
          }
        }}
      >
        <Trash2 size={14} aria-hidden="true" />
      </span>
    </button>
  )
}
