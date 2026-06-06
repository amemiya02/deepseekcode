import { useMemo, useState } from 'react'
import { Plus, Search, MessageSquareDashed } from 'lucide-react'
import { t } from '../lib/i18n'
import type { Session } from '../lib/api'
import { SessionList } from './SessionList'
import styles from './SessionRail.module.css'

export function SessionRail({
  sessions = [],
  activeId = '',
  now = Date.now(),
  loading = false,
  onNew,
  onSelect,
  onDelete,
  onRename,
}: {
  sessions?: Session[]
  activeId?: string
  now?: number
  loading?: boolean
  onNew?: () => void
  onSelect?: (id: string) => void
  onDelete?: (id: string) => void
  onRename?: (id: string, title: string) => void
}) {
  const [query, setQuery] = useState('')
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? sessions.filter((s) => s.title.toLowerCase().includes(q)) : sessions
  }, [query, sessions])
  const count = sessions.length

  return (
    <aside className={styles.rail}>
      <div className={styles.searchRow}>
        <label className={styles.search}>
          <Search size={14} aria-hidden="true" />
          <input
            data-testid="session-search"
            type="search"
            placeholder={t('session.search', 'Search sessions')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Escape') setQuery('') }}
          />
        </label>
        <button type="button" className={styles.new} data-testid="session-new" onClick={() => onNew?.()} aria-label={t('session.new', 'New session')}>
          <Plus size={16} aria-hidden="true" />
        </button>
      </div>

      <div className={styles.scroll}>
        {loading ? (
          <div className={styles.skeleton} data-testid="session-skeleton" aria-hidden="true">
            {Array.from({ length: 5 }).map((_, i) => <div key={i} className={styles.skelRow} />)}
          </div>
        ) : count === 0 ? (
          <div className={styles.empty} data-testid="sessions-empty">
            <MessageSquareDashed size={22} aria-hidden="true" />
            <p className={styles.emptyTitle}>{t('session.empty', 'No sessions yet.')}</p>
            <p className={styles.emptyHint}>{t('session.emptyHint', 'Start a new session above.')}</p>
          </div>
        ) : (
          <SessionList
            sessions={filtered}
            activeId={activeId}
            now={now}
            onSelect={onSelect}
            onDelete={onDelete}
            onRename={onRename}
          />
        )}
      </div>

      {count > 0 && (
        <footer className={styles.footer} data-testid="session-count">
          {count === 1
            ? t('session.countOne', '{n} session', { n: count })
            : t('session.countOther', '{n} sessions', { n: count })}
        </footer>
      )}
    </aside>
  )
}
