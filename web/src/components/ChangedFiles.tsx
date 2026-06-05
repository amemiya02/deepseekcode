// Adapted from deepseek-reasonix (MIT) — components/WorkspacePanel.tsx
// (change-row rendering: git status badge + deleted marker).
import { useEffect, useState, type ComponentType } from 'react'
import { File, FileCode, FileJson, FileText, Folder, Image } from 'lucide-react'
import { fetchChanged, type ChangedEntry } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './ChangedFiles.module.css'

export interface ChangedFilesProps {
  refreshKey?: number
  selected?: string
  onOpen?: (path: string) => void
}

const IMAGE_EXT = new Set(['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp', 'avif'])
const JSON_EXT = new Set(['json', 'jsonc'])
const CODE_EXT = new Set([
  'ts', 'tsx', 'js', 'jsx', 'mjs', 'cjs', 'go', 'rs', 'py', 'rb', 'php', 'sh', 'bash',
  'c', 'cc', 'cpp', 'h', 'hpp', 'java', 'kt', 'swift', 'css', 'scss', 'html', 'vue', 'sql',
])
const TEXT_EXT = new Set(['md', 'mdx', 'txt', 'rst', 'yml', 'yaml', 'toml', 'ini', 'env'])

// iconFor picks a file-type glyph from the extension so the change row reads at a
// glance (a real .png/.json icon, not a raw "??" git code the user mistook for a
// broken file). Directories get a folder glyph.
function iconFor(path: string, isDir: boolean): ComponentType<{ size?: number; className?: string }> {
  if (isDir) return Folder
  const dot = path.lastIndexOf('.')
  const ext = dot >= 0 ? path.slice(dot + 1).toLowerCase() : ''
  if (IMAGE_EXT.has(ext)) return Image
  if (JSON_EXT.has(ext)) return FileJson
  if (CODE_EXT.has(ext)) return FileCode
  if (TEXT_EXT.has(ext)) return FileText
  return File
}

// splitPath normalizes a porcelain path into a parent dir + leaf name. A trailing
// slash (untracked directory) is stripped first so the leaf is the folder's own
// name ("pkg/sub/" → dir "pkg/", name "sub") rather than an empty string rendered
// through a now-removed rtl trick that produced garbage like "/vite.".
function splitPath(path: string): { dir: string; name: string; isDir: boolean } {
  const isDir = path.endsWith('/')
  const clean = isDir ? path.slice(0, -1) : path
  const slash = clean.lastIndexOf('/')
  return {
    dir: slash >= 0 ? clean.slice(0, slash + 1) : '',
    name: slash >= 0 ? clean.slice(slash + 1) : clean,
    isDir,
  }
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
          const { dir, name, isDir } = splitPath(entry.path)
          const Icon = iconFor(entry.path, isDir)
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
                <span className={styles.icon} data-testid="change-icon" aria-hidden="true">
                  <Icon size={14} />
                </span>
                {dir && <span className={styles.dir}>{dir}</span>}
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
