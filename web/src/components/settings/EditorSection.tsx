import { t } from '../../lib/i18n'
import { useThemeStore, setThemeSettings, type TranscriptVerbosity } from '../../lib/theme/store'
import { useConfig } from '../../lib/useConfig'
import { BrandedSelect } from '../BrandedSelect'
import { Switch } from '../Switch'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

const VERBOSITY: TranscriptVerbosity[] = ['normal', 'verbose', 'summary']

export function EditorSection() {
  const verbosity = useThemeStore((s) => s.settings.transcriptVerbosity)
  const { cfg, error, patch, clearError } = useConfig()

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.editor', 'Editor & Diff')}</h2>
        <p className={styles.sub}>{t('settings.editorSub', 'Transcript rendering and diff panel appearance.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>×</button>
        </div>
      )}
      {!cfg ? (
        <StateView kind="loading" message={t('settings.loading', 'Loading…')} />
      ) : (
        <div className={styles.group}>
          <label className={styles.field}>
            {t('settings.transcriptVerbosity', 'Transcript verbosity')}
            <BrandedSelect
              value={verbosity}
              options={VERBOSITY.map((v) => ({ value: v, label: t(`settings.verbosity${v[0].toUpperCase()}${v.slice(1)}`, v) }))}
              onChange={(v) => setThemeSettings({ transcriptVerbosity: v as TranscriptVerbosity })}
              ariaLabel={t('settings.transcriptVerbosity', 'Transcript verbosity')}
              testid="editor-verbosity"
            />
          </label>
          <div className={styles.row}>
            <div className={styles.rowText}>
              <div className={styles.rowLabel}>{t('settings.transparentBackground', 'Transparent panel backgrounds')}</div>
              <div className={styles.rowDesc}>{t('settings.transparentNote', 'Removes filled backgrounds from tool-result and diff panels.')}</div>
            </div>
            <Switch
              checked={cfg.transparentBackground}
              onChange={(v) => void patch({ transparentBackground: v })}
              label={t('settings.transparentBackground', 'Transparent panel backgrounds')}
              testid="editor-transparent"
            />
          </div>
        </div>
      )}
    </div>
  )
}
