// Adapted from deepseek-reasonix (MIT) — components/WorkspacePanel.tsx
// (filter + filtered rows + per-entry add-to-chat affordances).
// Extended with recursive directory expand/collapse (lazy child loading).
import { useMemo, useState } from 'react'
import { File, Folder, FolderOpen, ChevronRight, ChevronDown, MessageSquarePlus } from 'lucide-react'
import { fetchFiles, type FileEntry } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './FileTree.module.css'

export interface FileTreeProps {
  entries?: FileEntry[]
  onOpen?: (path: string) => void
  onAddToChat?: (entry: FileEntry) => void
  // Internal: depth level for indented child rows (default 0 = root).
  _depth?: number
}

// FileTreeRows renders a flat list of entries, handling recursive dir expansion.
// It is separated so it can be reused for child directories without re-rendering
// the filter input.
function FileTreeRows({
  entries,
  onOpen,
  onAddToChat,
  depth = 0,
}: {
  entries: FileEntry[]
  onOpen?: (path: string) => void
  onAddToChat?: (entry: FileEntry) => void
  depth?: number
}) {
  // Map<path, children|null> — null means "expanded but not yet loaded"
  const [expanded, setExpanded] = useState<Map<string, FileEntry[] | null>>(new Map())

  async function toggleDir(entry: FileEntry) {
    if (!entry.is_dir) return
    if (expanded.has(entry.path)) {
      // Collapse: remove from map.
      setExpanded((prev) => {
        const next = new Map(prev)
        next.delete(entry.path)
        return next
      })
      return
    }
    // Expand: set null as "loading" sentinel then fetch.
    setExpanded((prev) => new Map(prev).set(entry.path, null))
    try {
      const res = await fetchFiles(entry.path)
      setExpanded((prev) => new Map(prev).set(entry.path, res.entries))
    } catch {
      setExpanded((prev) => {
        const next = new Map(prev)
        next.delete(entry.path)
        return next
      })
    }
  }

  return (
    <ul className={styles.list} style={depth > 0 ? { paddingLeft: depth * 12 } : undefined}>
      {entries.map((entry) => {
        const isOpen = expanded.has(entry.path)
        const children = expanded.get(entry.path) ?? null
        return (
          <li key={entry.path} className={`${styles.row} ${entry.is_dir ? styles.dir : ''}`}>
            <button
              className={styles.name}
              data-testid={entry.is_dir ? `dir-${entry.name}` : undefined}
              onClick={() => {
                if (entry.is_dir) {
                  void toggleDir(entry)
                } else {
                  onOpen?.(entry.path)
                }
              }}
            >
              {entry.is_dir ? (
                <>
                  {isOpen ? <ChevronDown size={12} aria-hidden /> : <ChevronRight size={12} aria-hidden />}
                  {isOpen ? <FolderOpen size={14} aria-hidden /> : <Folder size={14} aria-hidden />}
                </>
              ) : (
                <File size={14} aria-hidden />
              )}
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
            {isOpen && children === null && (
              <p className={styles.loadingChild}>{t('common.loading', 'Loading…')}</p>
            )}
            {isOpen && children !== null && children.length > 0 && (
              <FileTreeRows entries={children} onOpen={onOpen} onAddToChat={onAddToChat} depth={depth + 1} />
            )}
          </li>
        )
      })}
    </ul>
  )
}

// FileTree renders a filterable file tree with recursive directory expansion.
// Top-level entries are passed as props; child entries are fetched lazily when
// a directory is expanded. Clicking a file fires onOpen(path); the per-row "+"
// fires onAddToChat(entry). Pure-props for tests (filter lives here).
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
        <FileTreeRows entries={visible} onOpen={onOpen} onAddToChat={onAddToChat} depth={0} />
      )}
    </div>
  )
}
