import { t } from '../lib/i18n'
import styles from './PermissionTier.module.css'

export type Tier = 'read-only' | 'workspace-write' | 'full'

const TIERS: { value: Tier; label: string }[] = [
  { value: 'read-only', label: t('tier.readOnly', 'read-only') },
  { value: 'workspace-write', label: t('tier.workspaceWrite', 'workspace-write') },
  { value: 'full', label: t('tier.full', 'full') },
]

export interface PermissionTierProps {
  value: Tier
  onChange: (tier: Tier) => void
  disabled?: boolean
}

export function PermissionTier({ value, onChange, disabled }: PermissionTierProps) {
  return (
    <div className={styles.group} role="radiogroup" aria-label={t('tier.aria', 'Permission tier')}>
      {TIERS.map((tier) => (
        <button
          key={tier.value}
          className={`${styles.btn} ${value === tier.value ? styles.active : ''}`}
          role="radio"
          aria-checked={value === tier.value}
          disabled={disabled}
          onClick={() => onChange(tier.value)}
          data-testid={`tier-${tier.value}`}
        >
          {tier.label}
        </button>
      ))}
    </div>
  )
}
