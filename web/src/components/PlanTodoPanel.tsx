// Adapted from deepseek-reasonix (MIT) — components/TodoPanel.tsx
import { useState } from 'react'
import { Check, ChevronDown, ChevronRight, Circle, CircleDot, X } from 'lucide-react'
import { useLocale } from '../lib/i18n'
import type { PlanItem, PlanStatus } from '../lib/api'
import styles from './PlanTodoPanel.module.css'

// PlanTodoPanel is the live plan pinned above the composer: the kernel's latest
// plan_update drives it, ticking items pending → in_progress → done in place.
// It auto-clears when empty or all-done, collapses to a one-line summary, and
// can be dismissed; a fresh plan_update brings it back.
export function PlanTodoPanel({
  items = [],
  onDismiss,
}: {
  items?: PlanItem[]
  onDismiss?: () => void
}) {
  const { t } = useLocale()
  const [open, setOpen] = useState(true)

  const total = items.length
  const doneCount = items.filter((i) => i.status === 'done').length
  // Visible only while there is at least one not-yet-done item (auto-clear).
  const visible = total > 0 && doneCount < total
  if (!visible) return null

  return (
    <div className={styles.panel} data-testid="plan-panel" role="region" aria-label={t('plan.aria', 'plan todo')}>
      <div className={styles.head}>
        <button
          type="button"
          className={styles.toggle}
          data-testid="plan-toggle"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
          <span className={styles.title}>{t('plan.title', 'Plan')}</span>
          <span className={styles.progress} data-testid="plan-progress">
            {doneCount}/{total}
          </span>
        </button>
        <button
          type="button"
          className={styles.dismiss}
          data-testid="plan-dismiss"
          aria-label={t('plan.dismiss', 'dismiss plan')}
          onClick={() => onDismiss?.()}
        >
          <X size={13} />
        </button>
      </div>
      {open && (
        <ul className={styles.list}>
          {items.map((item, i) => (
            <li className={styles.item} data-status={item.status} key={i}>
              <StatusIcon status={item.status} />
              <span className={`${styles.text}${item.status === 'done' ? ` ${styles.textDone}` : ''}`}>
                {item.text}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function StatusIcon({ status }: { status: PlanStatus }) {
  if (status === 'done') return <Check size={14} className={styles.icoDone} aria-hidden="true" />
  if (status === 'in_progress') return <CircleDot size={14} className={styles.icoActive} aria-hidden="true" />
  return <Circle size={14} className={styles.icoPending} aria-hidden="true" />
}
