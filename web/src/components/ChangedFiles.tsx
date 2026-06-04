// Adapted from deepseek-reasonix (MIT) — components/WorkspacePanel.tsx
// (change-row rendering: git status badge + deleted marker).
import { useEffect, useState } from 'react'
import { fetchChanged, type ChangedEntry } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './ChangedFiles.module.css'

export interface ChangedFilesProps {
  refreshKey?: number
  onOpen?: (path: string) => void
}

// ChangedFiles lists git working-tree changes (served by Wave 1's /v1/changed).
// It re-fetches on mount and whenever refreshKey changes — the conversation
// bumps refreshKey after each turn so the working-tree view stays current. A
// per-render request guard drops a stale response if refreshKey changed again
// before the previous fetch resolved.
export function ChangedFiles({ refreshKey = 0, onOpen }: ChangedFilesProps) {
  const [entries, setEntries] = useState<ChangedEntry[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    let live = true
    fetchChanged()
      .then((res) => {
        if (!live) return
        setEntries(res.entries)
        setError('')
      })
      .catch((e) => {
        if (!live) return
        setError(String(e))
        setEntries([])
      })
    return () => {
      live = false
    }
  }, [refreshKey])

  if (error) {
    return (
      <div className={styles.changed}>
        <p className={styles.error}>{error}</p>
      </div>
    )
  }
  if (entries.length === 0) {
    return (
      <div className={styles.changed}>
        <p className={styles.empty}>{t('workspace.noChanges', 'No changes')}</p>
      </div>
    )
  }
  return (
    <div className={styles.changed}>
      <ul className={styles.list}>
        {entries.map((entry) => (
          <li key={entry.path} className={styles.row}>
            <span className={styles.status}>{(entry.status ?? '').trim() || '·'}</span>
            <button className={styles.path} onClick={() => onOpen?.(entry.path)}>
              {entry.path}
            </button>
            {entry.deleted && (
              <span className={styles.deleted} data-testid={`deleted-${entry.path}`}>
                {t('workspace.deleted', 'deleted')}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}
