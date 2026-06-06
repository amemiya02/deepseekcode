// Adapted from deepseek-reasonix (MIT) — empty/banner state idioms (className="empty"/"banner banner--error").
// Reusable designed states. One component, three kinds, token-styled.
import { Loader2, AlertCircle } from 'lucide-react'
import { t } from '../lib/i18n'
import styles from './StateViews.module.css'

export interface StateViewProps {
  kind: 'loading' | 'empty' | 'error'
  message?: string
  title?: string
  hint?: string
  onRetry?: () => void
}

export function StateView({ kind, message, title, hint, onRetry }: StateViewProps) {
  if (kind === 'loading') {
    return (
      <div className={styles.state} role="status" aria-live="polite">
        <Loader2 className={styles.spinner} size={18} aria-hidden="true" />
        <span className={styles.msg}>{message || t('state.loading', 'Loading…')}</span>
      </div>
    )
  }
  if (kind === 'empty') {
    return (
      <div className={`${styles.state} ${styles.empty}`}>
        <p className={styles.title}>{title}</p>
        {hint && <p className={styles.hint}>{hint}</p>}
      </div>
    )
  }
  return (
    <div className={`${styles.state} ${styles.error}`} role="alert">
      <AlertCircle size={18} aria-hidden="true" />
      <span className={styles.msg}>{message || t('state.error', 'Something went wrong.')}</span>
      {onRetry && (
        <button className={styles.retry} onClick={onRetry}>
          {t('state.retry', 'Retry')}
        </button>
      )}
    </div>
  )
}
