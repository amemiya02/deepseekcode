// Adapted from deepseek-reasonix (MIT) — components/HistoryPanel.tsx (row +
// turn pluralization + delete affordance). Extended with inline rename.
import { useRef, useState } from 'react'
import { Trash2, Pencil, Check, X } from 'lucide-react'
import { t } from '../lib/i18n'
import type { Session } from '../lib/api'
import styles from './SessionItem.module.css'

export function SessionItem({
  session,
  active = false,
  onSelect,
  onDelete,
  onRename,
}: {
  session: Session
  active?: boolean
  onSelect?: (id: string) => void
  onDelete?: (id: string) => void
  onRename?: (id: string, title: string) => void
}) {
  const [renaming, setRenaming] = useState(false)
  const [draft, setDraft] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const turnLabel =
    session.turns === 1
      ? t('session.turnOne', '{n} turn', { n: session.turns })
      : t('session.turnOther', '{n} turns', { n: session.turns })

  function startRename(e: React.MouseEvent | React.KeyboardEvent) {
    e.stopPropagation()
    setDraft(session.title)
    setRenaming(true)
    // Focus after state update.
    setTimeout(() => inputRef.current?.select(), 0)
  }

  function commitRename() {
    const trimmed = draft.trim()
    if (trimmed && trimmed !== session.title) {
      onRename?.(session.id, trimmed)
    }
    setRenaming(false)
  }

  function cancelRename() {
    setRenaming(false)
  }

  if (renaming) {
    return (
      <div
        className={`${styles.item} ${active ? styles.active : ''} ${styles.renaming}`}
        data-testid="session-item"
        aria-current={active ? 'true' : undefined}
      >
        <input
          ref={inputRef}
          className={styles.renameInput}
          data-testid="session-rename-input"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') commitRename()
            else if (e.key === 'Escape') cancelRename()
          }}
          autoFocus
        />
        <button
          type="button"
          className={styles.renameAction}
          data-testid="session-rename-commit"
          aria-label={t('session.renameCommit', 'Confirm rename')}
          onClick={commitRename}
        >
          <Check size={13} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={styles.renameAction}
          data-testid="session-rename-cancel"
          aria-label={t('common.cancel', 'Cancel')}
          onClick={cancelRename}
        >
          <X size={13} aria-hidden="true" />
        </button>
      </div>
    )
  }

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
        className={styles.rename}
        data-testid="session-rename"
        role="button"
        tabIndex={0}
        aria-label={t('session.rename', 'Rename session')}
        onClick={startRename}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') startRename(e)
        }}
      >
        <Pencil size={13} aria-hidden="true" />
      </span>
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
