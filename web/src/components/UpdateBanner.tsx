// Adapted from deepseek-reasonix (MIT) — UpdateBanner.tsx (dismissible top banner; download/dismiss actions).
import { X } from 'lucide-react'
import { t } from '../lib/i18n'
import type { UpdateInfo } from '../lib/system'
import styles from './UpdateBanner.module.css'

export interface UpdateBannerProps {
  info: UpdateInfo
  onDismiss?: () => void
}

// openDownload opens the signature-verified download page. In the desktop build
// the Wails runtime is present (runtime.BrowserOpenURL); in a plain browser it
// falls back to window.open.
function openDownload(url: string) {
  const rt = (window as unknown as { runtime?: { BrowserOpenURL?: (u: string) => void } }).runtime
  if (rt?.BrowserOpenURL) {
    rt.BrowserOpenURL(url)
  } else {
    window.open(url, '_blank', 'noopener')
  }
}

export function UpdateBanner({ info, onDismiss }: UpdateBannerProps) {
  if (!info.updateAvailable) return null
  return (
    <div className={styles.banner} role="status">
      <span className={styles.text}>
        {t('update.available', 'A new version is available:')} <strong>{info.latest}</strong>
      </span>
      <button className={styles.action} onClick={() => openDownload(info.url)}>
        {t('update.download', 'Download')}
      </button>
      <button className={styles.dismiss} aria-label={t('update.dismiss', 'Dismiss')} onClick={() => onDismiss?.()}>
        <X size={14} />
      </button>
    </div>
  )
}
