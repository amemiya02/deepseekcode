import { useMemo, useState } from 'react'
import { Plus, Search, History } from 'lucide-react'
import { t } from '../lib/i18n'
import type { Session } from '../lib/api'
import { SessionList } from './SessionList'
import styles from './SessionRail.module.css'

export function SessionRail({
  sessions = [],
  activeId = '',
  now = Date.now(),
  onNew,
  onSelect,
  onDelete,
  onOpenHistory,
}: {
  sessions?: Session[]
  activeId?: string
  now?: number
  onNew?: () => void
  onSelect?: (id: string) => void
  onDelete?: (id: string) => void
  onOpenHistory?: () => void
}) {
  const [query, setQuery] = useState('')
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? sessions.filter((s) => s.title.toLowerCase().includes(q)) : sessions
  }, [query, sessions])

  return (
    <aside className={styles.rail}>
      <button type="button" className={styles.new} data-testid="session-new" onClick={() => onNew?.()}>
        <Plus size={16} aria-hidden="true" />
        <span>{t('session.new', 'New session')}</span>
      </button>

      <label className={styles.search}>
        <Search size={14} aria-hidden="true" />
        <input
          data-testid="session-search"
          type="search"
          placeholder={t('session.search', 'Search sessions')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </label>

      <div className={styles.scroll}>
        <SessionList sessions={filtered} activeId={activeId} now={now} onSelect={onSelect} onDelete={onDelete} />
      </div>

      <button type="button" className={styles.history} data-testid="session-history" onClick={() => onOpenHistory?.()}>
        <History size={14} aria-hidden="true" />
        <span>{t('session.allHistory', 'All history')}</span>
      </button>
    </aside>
  )
}
