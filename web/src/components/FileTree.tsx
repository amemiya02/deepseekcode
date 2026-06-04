// Adapted from deepseek-reasonix (MIT) — components/WorkspacePanel.tsx
// (filter + filtered rows + per-entry add-to-chat affordances).
import { useMemo, useState } from 'react'
import { File, Folder, MessageSquarePlus } from 'lucide-react'
import type { FileEntry } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './FileTree.module.css'

export interface FileTreeProps {
  entries?: FileEntry[]
  onOpen?: (path: string) => void
  onAddToChat?: (entry: FileEntry) => void
}

// FileTree renders a single directory level (the parent loads children when a
// dir is opened) with a case-insensitive name filter. Clicking a row fires
// onOpen(path); the per-row "+" fires onAddToChat(entry). Pure-props for tests.
export function FileTree({ entries = [], onOpen, onAddToChat }: FileTreeProps) {
  const [filter, setFilter] = useState('')

  const visible = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (q === '') return entries
    return entries.filter((e) => e.name.toLowerCase().includes(q))
  }, [entries, filter])

  return (
    <div className={styles.tree}>
      <input
        className={styles.filter}
        data-testid="file-filter"
        placeholder={t('workspace.filter', 'Filter files…')}
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />
      {visible.length === 0 ? (
        <p className={styles.empty}>{t('workspace.noFiles', 'No files')}</p>
      ) : (
        <ul className={styles.list}>
          {visible.map((entry) => (
            <li key={entry.path} className={`${styles.row} ${entry.is_dir ? styles.dir : ''}`}>
              <button className={styles.name} onClick={() => onOpen?.(entry.path)}>
                {entry.is_dir ? <Folder size={14} aria-hidden /> : <File size={14} aria-hidden />}
                {entry.name}
                {entry.is_dir ? '/' : ''}
              </button>
              <button
                className={styles.add}
                data-testid={`add-${entry.name}`}
                title={t('workspace.addToChat', 'Add to chat')}
                onClick={() => onAddToChat?.(entry)}
              >
                <MessageSquarePlus size={14} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
