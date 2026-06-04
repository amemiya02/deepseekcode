// Adapted from deepseek-reasonix (MIT) — components/HistoryPanel.tsx (drawer
// shell, search box, day grouping, resume/delete).
import { useMemo, useState } from 'react'
import { Search, X } from 'lucide-react'
import { t } from '../lib/i18n'
import type { Session } from '../lib/api'
import { SessionList } from './SessionList'
import styles from './HistoryDrawer.module.css'

export function HistoryDrawer({
  open = false,
  sessions = [],
  now = Date.now(),
  onResume,
  onDelete,
  onClose,
}: {
  open?: boolean
  sessions?: Session[]
  now?: number
  onResume?: (id: string) => void
  onDelete?: (id: string) => void
  onClose?: () => void
}) {
  const [query, setQuery] = useState('')
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? sessions.filter((s) => s.title.toLowerCase().includes(q)) : sessions
  }, [query, sessions])

  if (!open) return null

  return (
    <>
      <button
        type="button"
        className={styles.backdrop}
        data-testid="history-backdrop"
        aria-label={t('common.close', 'Close')}
        onClick={() => onClose?.()}
      />
      <div className={styles.drawer} data-testid="history-drawer" role="dialog" aria-label={t('history.title', 'Session history')}>
        <header className={styles.head}>
          <h2>{t('history.title', 'Session history')}</h2>
          <button type="button" className={styles.close} aria-label={t('common.close', 'Close')} onClick={() => onClose?.()}>
            <X size={16} aria-hidden="true" />
          </button>
        </header>
        <label className={styles.search}>
          <Search size={14} aria-hidden="true" />
          <input
            data-testid="history-search"
            type="search"
            placeholder={t('history.search', 'Search all sessions')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </label>
        <div className={styles.scroll}>
          <SessionList sessions={filtered} activeId="" now={now} onSelect={onResume} onDelete={onDelete} />
        </div>
      </div>
    </>
  )
}
