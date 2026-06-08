// Adapted from deepseek-reasonix (MIT) — UpdateBanner.tsx (dismissible top banner; download/dismiss actions).
import { X } from 'lucide-react'
import { t } from '../lib/i18n'
import { openDownload } from '../lib/desktopBridge'
import type { UpdateInfo } from '../lib/system'
import styles from './UpdateBanner.module.css'

const DISMISSED_KEY = (v: string) => `deepseek_update_dismissed_${v}`

export interface UpdateBannerProps {
  info: UpdateInfo
  onDismiss?: () => void
}

export function UpdateBanner({ info, onDismiss }: UpdateBannerProps) {
  if (!info.updateAvailable) return null
  if (typeof localStorage !== 'undefined' && localStorage.getItem(DISMISSED_KEY(info.latest))) return null

  function handleDismiss() {
    try { localStorage.setItem(DISMISSED_KEY(info.latest), '1') } catch { /* quota / private mode */ }
    onDismiss?.()
  }

  return (
    <div className={styles.banner} role="status">
      <span className={styles.text}>
        {t('update.available', 'A new version is available:')} <strong>{info.latest}</strong>
      </span>
      <button className={styles.action} onClick={() => openDownload(info.url)}>
        {t('update.download', 'Download')}
      </button>
      <button className={styles.dismiss} aria-label={t('update.dismiss', 'Dismiss')} onClick={handleDismiss}>
        <X size={14} />
      </button>
    </div>
  )
}
