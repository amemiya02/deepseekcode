import { useMemo } from 'react'
import { groupSessionsByDay, type DayKey } from '../lib/format'
import { t } from '../lib/i18n'
import type { Session } from '../lib/api'
import { SessionItem } from './SessionItem'
import styles from './SessionList.module.css'

export function SessionList({
  sessions = [],
  activeId = '',
  now = Date.now(),
  onSelect,
  onDelete,
  onRename,
}: {
  sessions?: Session[]
  activeId?: string
  now?: number
  onSelect?: (id: string) => void
  onDelete?: (id: string) => void
  onRename?: (id: string, title: string) => void
}) {
  const labels: Record<DayKey, string> = {
    today: t('session.group.today', 'Today'),
    yesterday: t('session.group.yesterday', 'Yesterday'),
    week: t('session.group.week', 'Previous 7 Days'),
    month: t('session.group.month', 'Previous 30 Days'),
    older: t('session.group.older', 'Older'),
  }
  const groups = useMemo(() => groupSessionsByDay(sessions, now), [sessions, now])

  if (sessions.length === 0) {
    return (
      <p className={styles.empty} data-testid="sessions-empty">
        {t('session.empty', 'No sessions yet.')}
      </p>
    )
  }

  return (
    <div className={styles.list}>
      {groups.map((group) => (
        <section className={styles.group} data-testid={`group-${group.key}`} key={group.key}>
          <h3 className={styles.groupLabel}>{labels[group.key]}</h3>
          {group.sessions.map((s) => (
            <SessionItem
              key={s.id}
              session={s}
              now={now}
              active={s.id === activeId}
              onSelect={onSelect}
              onDelete={onDelete}
              onRename={onRename}
            />
          ))}
        </section>
      ))}
    </div>
  )
}
