import { useToasts, dismissToast } from '../../lib/toasts'
import { IconX } from '../../lib/icons'
import { useT } from '../../lib/i18n'
import styles from './index.module.css'

export function Toasts() {
  const items = useToasts()
  const t = useT()
  return (
    <div className={styles.toasts} role="status" aria-live="polite">
      {items.map((toast) => (
        <div key={toast.id} className={`${styles.toast} ${styles[`toast_${toast.kind}`]}`} data-testid="toast">
          <span className={styles.toastMsg}>{toast.message}</span>
          <button className={styles.toastDismiss} aria-label={t('toast.dismiss')} onClick={() => dismissToast(toast.id)}>
            <IconX size={14} />
          </button>
        </div>
      ))}
    </div>
  )
}
