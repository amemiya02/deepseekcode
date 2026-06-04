// Adapted from deepseek-reasonix (MIT) — components/WorkspacePanel.tsx
// (Files/Changed tab shell + tree ‖ preview split). Simplified to consume our
// HTTP workspace client and mount into the App workspace zone (Contract 4).
import { useEffect, useState } from 'react'
import { FileText, GitBranch } from 'lucide-react'
import { FileTree } from './FileTree'
import { ChangedFiles } from './ChangedFiles'
import { CodeViewer } from './CodeViewer'
import { fetchFiles, fetchFile, addToChat, type FileEntry } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './WorkspacePanel.module.css'

type Tab = 'files' | 'changed'

export interface WorkspacePanelProps {
  // Bumped by the conversation after each turn so ChangedFiles re-fetches.
  refreshKey?: number
  // Called with the formatted pill content when the user adds a file/folder to
  // chat; App appends it to the composer.
  onAddToChat?: (content: string) => void
}

interface OpenFile {
  path: string
  content: string
  binary: boolean
  truncated: boolean
}

const EMPTY_FILE: OpenFile = { path: '', content: '', binary: false, truncated: false }

// WorkspacePanel is the tabbed Files/Changed drawer for the App workspace zone.
// Files tab: a filtered FileTree over the root listing + a CodeViewer preview.
// Changed tab: the git ChangedFiles list, also previewing into the CodeViewer.
export function WorkspacePanel({ refreshKey = 0, onAddToChat }: WorkspacePanelProps) {
  const [tab, setTab] = useState<Tab>('files')
  const [entries, setEntries] = useState<FileEntry[]>([])
  const [open, setOpen] = useState<OpenFile>(EMPTY_FILE)

  // Load the root tree once on mount.
  useEffect(() => {
    let live = true
    fetchFiles('')
      .then((r) => {
        if (live) setEntries(r.entries)
      })
      .catch(() => {
        if (live) setEntries([])
      })
    return () => {
      live = false
    }
  }, [])

  async function openPath(path: string) {
    try {
      const r = await fetchFile(path)
      setOpen({ path: r.path, content: r.content, binary: r.binary, truncated: r.truncated })
    } catch {
      setOpen({ ...EMPTY_FILE, path })
    }
  }

  async function add(entry: FileEntry) {
    const r = await addToChat({ path: entry.path, is_dir: entry.is_dir })
    onAddToChat?.(r.content)
  }

  return (
    <div className={styles.workspace}>
      <div className={styles.tabs} role="tablist">
        <button
          role="tab"
          aria-selected={tab === 'files'}
          className={`${styles.tab} ${tab === 'files' ? styles.tabActive : ''}`}
          data-testid="tab-files"
          onClick={() => setTab('files')}
        >
          <FileText size={13} aria-hidden />
          {t('workspace.filesTab', 'Files')}
        </button>
        <button
          role="tab"
          aria-selected={tab === 'changed'}
          className={`${styles.tab} ${tab === 'changed' ? styles.tabActive : ''}`}
          data-testid="tab-changed"
          onClick={() => setTab('changed')}
        >
          <GitBranch size={13} aria-hidden />
          {t('workspace.changedTab', 'Changed')}
        </button>
      </div>

      <div className={styles.body}>
        <div className={styles.list}>
          {tab === 'files' ? (
            <FileTree entries={entries} onOpen={openPath} onAddToChat={add} />
          ) : (
            <ChangedFiles refreshKey={refreshKey} onOpen={openPath} />
          )}
        </div>
        <div className={styles.viewer}>
          <CodeViewer
            path={open.path}
            content={open.content}
            binary={open.binary}
            truncated={open.truncated}
          />
        </div>
      </div>
    </div>
  )
}
