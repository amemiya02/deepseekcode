// Adapted from deepseek-reasonix (MIT) — components/WorkspacePanel.tsx
// (change-row rendering: git status badge + deleted marker).
import { useEffect, useState } from 'react'
import { fetchChanged, type ChangedEntry } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './ChangedFiles.module.css'

export interface ChangedFilesProps {
  refreshKey?: number
  selected?: string
  onOpen?: (path: string) => void
}

// ChangedFiles lists git working-tree changes (served by Wave 1's /v1/changed).
// It re-fetches on mount and whenever refreshKey changes — the conversation
// bumps refreshKey after each turn so the working-tree view stays current. A
// per-render request guard drops a stale response if refreshKey changed again
// before the previous fetch resolved.
export function ChangedFiles({ refreshKey = 0, selected, onOpen }: ChangedFilesProps) {
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
        {entries.map((entry) => {
          const slash = entry.path.lastIndexOf('/')
          const dir = slash >= 0 ? entry.path.slice(0, slash + 1) : ''
          const name = slash >= 0 ? entry.path.slice(slash + 1) : entry.path
          const code = (entry.status ?? '').trim() || '?'
          const isSel = entry.path === selected
          return (
            <li key={entry.path}>
              <button
                className={styles.row}
                aria-current={isSel ? 'true' : undefined}
                onClick={() => onOpen?.(entry.path)}
              >
                <span
                  className={styles.status}
                  data-testid="change-status"
                  data-code={code[0]}
                >
                  {code}
                </span>
                <span className={styles.dir}>{dir}</span>
                <span className={styles.name}>{name}</span>
                {entry.deleted && (
                  <span className={styles.deleted} data-testid={`deleted-${entry.path}`}>
                    {t('workspace.deleted', 'deleted')}
                  </span>
                )}
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
