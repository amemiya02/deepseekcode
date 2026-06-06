import styles from './Switch.module.css'

export interface SwitchProps {
  checked: boolean
  onChange: (next: boolean) => void
  label: string            // accessible name (aria-label)
  disabled?: boolean
  testid?: string
}

// A boolean toggle styled as a track + knob. role="switch" with aria-checked so
// it is keyboard- and screen-reader-accessible. Visual contract matches the MCP
// row toggle (34x20 track, 16px knob, 14px travel).
export function Switch({ checked, onChange, label, disabled, testid }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      data-testid={testid}
      disabled={disabled}
      className={checked ? `${styles.track} ${styles.on}` : styles.track}
      onClick={() => { if (!disabled) onChange(!checked) }}
    >
      <span className={styles.knob} />
    </button>
  )
}
