// SddPanel — right-inspector panel for spec-driven development.
// Renders a requirement draft editor (textarea) and a "generate spec" submit button.
import styles from './SddPanel.module.css'

export interface SddPanelProps {
  draft: string
  onChange: (draft: string) => void
  onSubmit: () => void
}

export function SddPanel({ draft, onChange, onSubmit }: SddPanelProps) {
  return (
    <div className={styles.panel} data-testid="sdd-panel">
      <h2 className={styles.heading}>SDD — New Requirement</h2>
      <textarea
        className={styles.textarea}
        value={draft}
        onChange={(e) => onChange(e.target.value)}
        placeholder="Describe the requirement…"
        aria-label="Requirement draft"
        rows={8}
      />
      <button
        className={styles.button}
        type="button"
        onClick={onSubmit}
      >
        Generate Spec
      </button>
    </div>
  )
}
