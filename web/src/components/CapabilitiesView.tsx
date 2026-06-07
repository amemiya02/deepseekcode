// CapabilitiesView.tsx — settings-style section listing runtime feature flags.
// Each flag is rendered as a row with an on/off chip.
import styles from './CapabilitiesView.module.css'

export interface CapabilitiesViewProps {
  /** Feature flags keyed by name, e.g. { mcp: true, web: true, skills: false } */
  caps: Record<string, boolean>
}

export function CapabilitiesView({ caps }: CapabilitiesViewProps) {
  const entries = Object.entries(caps)
  if (entries.length === 0) return null

  return (
    <section className={styles.root} aria-label="Capabilities">
      <h3 className={styles.heading}>Capabilities</h3>
      <ul className={styles.list}>
        {entries.map(([key, enabled]) => (
          <li key={key} className={styles.row}>
            <span className={styles.name}>{key}</span>
            <span
              className={`${styles.chip} ${enabled ? styles.on : styles.off}`}
              data-cap={key}
            >
              {enabled ? 'on' : 'off'}
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}
