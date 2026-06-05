import { useRef, useState } from 'react'
import { ChangedFiles } from './ChangedFiles'
import { DiffView } from './DiffView'
import { CodeViewer } from './CodeViewer'
import { fetchDiff, fetchFile } from '../lib/workspace'
import { t } from '../lib/i18n'
import styles from './ReviewPanel.module.css'

interface Preview {
  path: string
  patch: string
  content: string
  binary: boolean
  truncated: boolean
  mode: 'diff' | 'file'
}

export function ReviewPanel({ refreshKey = 0 }: { refreshKey?: number }) {
  const [sel, setSel] = useState('')
  const [preview, setPreview] = useState<Preview | null>(null)
  // Monotonic request token: a slow earlier fetch must not overwrite a newer
  // selection's preview (rapid row switches → last-click-wins, not last-resolve).
  const reqRef = useRef(0)

  async function open(path: string) {
    const my = ++reqRef.current
    setSel(path)
    const d = await fetchDiff(path).catch(() => ({ path, patch: '' }))
    if (reqRef.current !== my) return
    if (d.patch.trim()) {
      setPreview({ path, patch: d.patch, content: '', binary: false, truncated: false, mode: 'diff' })
      return
    }
    const f = await fetchFile(path).catch(() => null)
    if (reqRef.current !== my) return
    setPreview({
      path,
      patch: '',
      mode: 'file',
      content: f?.content ?? '',
      binary: f?.binary ?? false,
      truncated: f?.truncated ?? false,
    })
  }

  return (
    <div className={styles.review}>
      <div className={styles.list}>
        <ChangedFiles refreshKey={refreshKey} selected={sel} onOpen={open} />
      </div>
      <div className={styles.hero} data-testid="review-hero">
        {!preview ? (
          <p className={styles.placeholder} data-testid="review-placeholder">
            {t('review.pick', 'Select a changed file to review its diff.')}
          </p>
        ) : preview.mode === 'diff' ? (
          <DiffView path={preview.path} patch={preview.patch} readOnly />
        ) : (
          <CodeViewer
            path={preview.path}
            content={preview.content}
            binary={preview.binary}
            truncated={preview.truncated}
          />
        )}
      </div>
    </div>
  )
}
