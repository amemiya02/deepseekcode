import { useCallback, useRef, useState } from 'react'
import { ChangedFiles, type FileReviewStatus } from './ChangedFiles'
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

const LIST_MIN = 160, LIST_MAX = 420

export function ReviewPanel({ refreshKey = 0 }: { refreshKey?: number }) {
  const [sel, setSel] = useState('')
  const [preview, setPreview] = useState<Preview | null>(null)
  const [listW, setListW] = useState(220)
  const [reviewStatus, setReviewStatus] = useState<Record<string, FileReviewStatus>>({})
  const drag = useRef<{ startX: number; startW: number } | null>(null)
  // Monotonic request token: a slow earlier fetch must not overwrite a newer
  // selection's preview (rapid row switches → last-click-wins, not last-resolve).
  const reqRef = useRef(0)

  const onApprove = useCallback((path: string) => {
    setReviewStatus((prev) => ({ ...prev, [path]: 'approved' }))
  }, [])
  const onReject = useCallback((path: string) => {
    setReviewStatus((prev) => ({ ...prev, [path]: 'rejected' }))
  }, [])

  const onMove = useCallback((e: MouseEvent) => {
    const d = drag.current
    if (!d) return
    const next = Math.min(LIST_MAX, Math.max(LIST_MIN, d.startW + (e.clientX - d.startX)))
    setListW(next)
  }, [])
  const onUp = useCallback(() => {
    drag.current = null
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }, [onMove])
  const onDown = (e: React.MouseEvent) => {
    drag.current = { startX: e.clientX, startW: listW }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

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
    <div className={styles.review} style={{ gridTemplateColumns: `${listW}px 6px 1fr` }}>
      <div className={styles.list}>
        <ChangedFiles refreshKey={refreshKey} selected={sel} onOpen={open} onApprove={onApprove} onReject={onReject} reviewStatus={reviewStatus} />
      </div>
      <div
        className={styles.splitter}
        data-testid="review-splitter"
        role="separator"
        aria-orientation="vertical"
        tabIndex={0}
        onMouseDown={onDown}
      />
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
