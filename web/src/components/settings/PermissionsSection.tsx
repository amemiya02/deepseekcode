import { t } from '../../lib/i18n'
import { useConfig } from '../../lib/useConfig'
import { BrandedSelect } from '../BrandedSelect'
import { Switch } from '../Switch'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const PERMISSION_LEVELS = ['ask', 'auto', 'full'] as const

export function PermissionsSection() {
  const { cfg, error, patch, clearError } = useConfig()

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.permissions', 'Permissions & Autonomy')}</h2>
        <p className={styles.sub}>{t('settings.permissionsSub', 'Tool-call approval and intent clarification.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>×</button>
        </div>
      )}
      <div className={styles.group}>
        <label className={styles.field}>
          {t('settings.permissionDefault', 'Default autonomy level')}
          <BrandedSelect
            value={cfg.permissionDefault || 'ask'}
            options={PERMISSION_LEVELS.map((lv) => ({ value: lv, label: lv }))}
            onChange={(v) => void patch({ permissionDefault: v })}
            ariaLabel={t('settings.permissionDefault', 'Default autonomy level')}
            testid="perm-default"
          />
        </label>
        <div className={styles.row}>
          <div className={styles.rowText}>
            <div className={styles.rowLabel}>{t('settings.autoClarify', 'Auto clarify under-specified prompts')}</div>
            <div className={styles.rowDesc}>{t('settings.autoClarifyNote', 'Asks one clarifying question before spending a model turn on an ambiguous request.')}</div>
          </div>
          <Switch checked={cfg.autoClarify} onChange={(v) => void patch({ autoClarify: v })} label={t('settings.autoClarify', 'Auto clarify under-specified prompts')} testid="perm-autoclarify" />
        </div>
      </div>
      <p className={styles.note}>
        {t('settings.permissionsNote', '"ask" prompts before every tool call. "auto" approves safe reads and branches but asks for writes. "full" skips all confirmations — use only in trusted, isolated environments.')}
      </p>
    </div>
  )
}
