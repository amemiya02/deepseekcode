import type { Preset } from './layout'
import styles from './index.module.css'

const PRESETS: { id: Preset; label: string }[] = [
  { id: 'balanced', label: 'Balanced' },
  { id: 'focus', label: 'Focus' },
  { id: 'review', label: 'Review' },
]

export function LayoutPresets({ value, onChange }: { value: Preset; onChange: (p: Preset) => void }) {
  return (
    <div className={styles.presetGroup} role="group" aria-label="Layout preset">
      {PRESETS.map((p) => (
        <button
          key={p.id}
          type="button"
          data-testid={`preset-${p.id}`}
          className={styles.presetBtn}
          aria-pressed={value === p.id}
          onClick={() => onChange(p.id)}
        >
          {p.label}
        </button>
      ))}
    </div>
  )
}
