// RuntimeBanner.tsx — slim inline banner surfacing runtime status (model, version, etc.).
import styles from './RuntimeBanner.module.css'

export interface RuntimeBannerProps {
  /** Model name, e.g. "deepseek-chat" */
  model?: string
  /** Version string, e.g. "v1.2.3" */
  version?: string
  /** Optional extra info text */
  info?: string
}

export function RuntimeBanner({ model, version, info }: RuntimeBannerProps) {
  if (!model && !version && !info) return null

  const parts: string[] = []
  if (model) parts.push(model)
  if (version) parts.push(version)
  if (info) parts.push(info)

  return (
    <div className={styles.banner} role="status" aria-label="Runtime status">
      {parts.map((p, i) => (
        <span key={i} className={styles.part}>
          {p}
        </span>
      ))}
    </div>
  )
}
