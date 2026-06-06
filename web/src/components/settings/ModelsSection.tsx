import { useEffect, useState } from 'react'
import { t } from '../../lib/i18n'
import { EFFORT_LEVELS } from '../../lib/api'
import { useConfig } from '../../lib/useConfig'
import { BrandedSelect } from '../BrandedSelect'
import { Switch } from '../Switch'
import { StateView } from '../StateViews'
import styles from './sections.module.css'

interface ModelOpt { value: string; label: string }

export function ModelsSection() {
  const { cfg, error, patch, clearError } = useConfig()
  const [models, setModels] = useState<ModelOpt[]>([])

  useEffect(() => {
    void (async () => {
      try {
        const res = await fetch('/v1/models', { headers: { Accept: 'application/json' } })
        if (!res.ok) return
        const body = await res.json()
        const list = (body.models ?? []) as Array<string | { id: string; label?: string }>
        setModels(list.map((m) => typeof m === 'string' ? { value: m, label: m } : { value: m.id, label: m.label ?? m.id }))
      } catch { /* leave empty -> free-text fallback */ }
    })()
  }, [])

  if (!cfg) return <StateView kind="loading" message={t('settings.loading', 'Loading…')} />

  const modelField = (value: string, onChange: (v: string) => void, testid: string, placeholder: string) =>
    models.length > 0
      ? <BrandedSelect value={value} options={models} onChange={onChange} ariaLabel={placeholder} testid={testid} />
      : <input className={styles.input} type="text" value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} data-testid={testid} />

  return (
    <div>
      <div className={styles.header}>
        <h2 className={styles.h2}>{t('settings.models', 'Models & Routing')}</h2>
        <p className={styles.sub}>{t('settings.modelsSub', 'Default model, effort, and flash-first routing.')}</p>
      </div>
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button onClick={clearError} aria-label={t('settings.dismiss', 'Dismiss')}>×</button>
        </div>
      )}
      <div className={styles.group}>
        <label className={styles.field}>
          {t('settings.model', 'Default model')}
          {modelField(cfg.model, (v) => void patch({ model: v }), 'models-model', 'deepseek-v4-flash')}
        </label>
        <label className={styles.field}>
          {t('settings.reasoningEffort', 'Reasoning effort')}
          <BrandedSelect
            value={cfg.reasoningEffort}
            options={EFFORT_LEVELS.map((lv) => ({ value: lv, label: lv }))}
            onChange={(v) => void patch({ reasoningEffort: v })}
            ariaLabel={t('settings.reasoningEffort', 'Reasoning effort')}
            testid="models-effort"
          />
        </label>
      </div>
      <div className={styles.group}>
        <div className={styles.row}>
          <div className={styles.rowText}>
            <div className={styles.rowLabel}>{t('settings.autoRoute', 'Auto-route (flash-first)')}</div>
            <div className={styles.rowDesc}>{t('settings.autoRouteNote', 'Mechanical turns stay on flash; reasoning turns escalate to the pro model.')}</div>
          </div>
          <Switch checked={cfg.autoRoute} onChange={(v) => void patch({ autoRoute: v })} label={t('settings.autoRoute', 'Auto-route (flash-first)')} testid="models-autoroute" />
        </div>
        {cfg.autoRoute && (
          <label className={styles.field}>
            {t('settings.escalationModel', 'Escalation model')}
            {modelField(cfg.escalationModel, (v) => void patch({ escalationModel: v }), 'models-escalation', 'deepseek-v4-pro')}
          </label>
        )}
        <div className={styles.row}>
          <div className={styles.rowText}>
            <div className={styles.rowLabel}>{t('settings.autoReasoning', 'Auto reasoning')}</div>
            <div className={styles.rowDesc}>{t('settings.autoReasoningNote', 'Activates extended thinking on complex queries.')}</div>
          </div>
          <Switch checked={cfg.autoReasoning} onChange={(v) => void patch({ autoReasoning: v })} label={t('settings.autoReasoning', 'Auto reasoning')} testid="models-autoreasoning" />
        </div>
      </div>
    </div>
  )
}
