// Adapted from deepseek-reasonix (MIT) — components/WorkspacePanel.tsx
// (Changed-files list ‖ preview split). Simplified to consume our HTTP workspace
// client and mount into the App workspace zone (Contract 4). The Files tab was
// removed: in `dsc serve` the gateway is built without a workspace root, so
// /v1/files?path= returns 400 on every empty-path listing. The panel now shows
// only the git Changed list (GET /v1/changed, which returns 200 even with no
// root) and never calls /v1/files with an empty path.
import { useState } from 'react'
import { GitBranch } from 'lucide-react'
import { ChangedFiles } from './ChangedFiles'
import { CodeViewer } from './CodeViewer'
import { fetchFile } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './WorkspacePanel.module.css'

export interface WorkspacePanelProps {
  // Bumped by the conversation after each turn so ChangedFiles re-fetches.
  refreshKey?: number
  // Retained for API compatibility with App.tsx (add-to-chat from the workspace
  // surface). The Files tree that produced these pills was removed; ChangedFiles
  // does not add to chat, so this is currently unused but kept to avoid churning
  // the App wiring that a sibling phase owns.
  onAddToChat?: (content: string) => void
}

interface OpenFile {
  path: string
  content: string
  binary: boolean
  truncated: boolean
}

const EMPTY_FILE: OpenFile = { path: '', content: '', binary: false, truncated: false }

// WorkspacePanel is the Changed-files drawer for the App workspace zone: the git
// ChangedFiles list previewing into a CodeViewer. Clicking a changed row reads
// that file via /v1/file (a concrete path, never empty) and previews it.
export function WorkspacePanel({ refreshKey = 0 }: WorkspacePanelProps) {
  const [open, setOpen] = useState<OpenFile>(EMPTY_FILE)

  async function openPath(path: string) {
    try {
      const r = await fetchFile(path)
      setOpen({ path: r.path, content: r.content, binary: r.binary, truncated: r.truncated })
    } catch {
      setOpen({ ...EMPTY_FILE, path })
    }
  }

  return (
    <div className={styles.workspace}>
      <div className={styles.tabs} role="tablist">
        <button
          role="tab"
          aria-selected={true}
          className={`${styles.tab} ${styles.tabActive}`}
          data-testid="tab-changed"
        >
          <GitBranch size={13} aria-hidden />
          {t('workspace.changedTab', 'Changed')}
        </button>
      </div>

      <div className={styles.body}>
        <div className={styles.list}>
          <ChangedFiles refreshKey={refreshKey} onOpen={openPath} />
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
