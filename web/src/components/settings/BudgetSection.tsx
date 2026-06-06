import { t } from '../../lib/i18n'
import { useConfig } from '../../lib/useConfig'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const MiB = 1024 * 1024

function toMiB(bytes: number): string {
  return (bytes / MiB).toFixed(0)
}

function fromMiB(s: string): number {
  const n = parseFloat(s)
  return isNaN(n) || n <= 0 ? MiB : Math.round(n * MiB)
}

export function BudgetSection() {
  const { cfg, error, patch, clearError } = useConfig()

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.budget', 'Budget & Cache')}</h2>
        <p className={styles.sub}>{t('settings.budgetSub', 'File read/write caps and cache management.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>×</button>
        </div>
      )}
      <div className={styles.group}>
        <label className={styles.field}>
          {t('settings.maxReadBytes', 'Max file read size (MiB)')}
          <input
            className={styles.input}
            type="number"
            min={1}
            max={512}
            value={toMiB(cfg.maxReadBytes)}
            onChange={(e) => void patch({ maxReadBytes: fromMiB(e.target.value) })}
            data-testid="budget-read"
          />
        </label>
        <label className={styles.field}>
          {t('settings.maxWriteBytes', 'Max file write size (MiB)')}
          <input
            className={styles.input}
            type="number"
            min={1}
            max={512}
            value={toMiB(cfg.maxWriteBytes)}
            onChange={(e) => void patch({ maxWriteBytes: fromMiB(e.target.value) })}
            data-testid="budget-write"
          />
        </label>
      </div>
      <p className={styles.note}>
        {t('settings.budgetNote', 'File read/write caps guard against accidental large-file operations. The prefix cache is managed automatically.')}
      </p>
    </div>
  )
}
