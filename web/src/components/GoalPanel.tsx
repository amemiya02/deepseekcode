import { useLocale } from '../lib/i18n'
import { Pause, Play, CheckCircle, Target } from 'lucide-react'
import styles from './GoalPanel.module.css'

export interface Goal {
  text: string
  state: 'active' | 'paused' | 'completed'
}

export function GoalPanel({
  goal,
  onPause,
  onResume,
  onComplete,
}: {
  goal: Goal
  onPause: () => void
  onResume: () => void
  onComplete: () => void
}) {
  const { t } = useLocale()

  const stateLabel = {
    active: t('goal.state.active', 'active'),
    paused: t('goal.state.paused', 'paused'),
    completed: t('goal.state.completed', 'completed'),
  }[goal.state]

  const stateClass = {
    active: styles.stateActive,
    paused: styles.statePaused,
    completed: styles.stateCompleted,
  }[goal.state]

  return (
    <div className={styles.panel} data-testid="goal-panel" role="region" aria-label={t('goal.aria', 'goal panel')}>
      <div className={styles.head}>
        <Target size={16} className={styles.icon} aria-hidden="true" />
        <span className={styles.text}>{goal.text}</span>
        <span className={`${styles.state} ${stateClass}`}>{stateLabel}</span>
      </div>
      {goal.state !== 'completed' && (
        <div className={styles.actions}>
          {goal.state === 'active' ? (
            <button type="button" className={styles.btn} onClick={onPause} aria-label={t('goal.pause', 'pause')}>
              <Pause size={14} aria-hidden="true" />
              <span>{t('goal.pause', 'pause')}</span>
            </button>
          ) : (
            <button type="button" className={styles.btn} onClick={onResume} aria-label={t('goal.resume', 'resume')}>
              <Play size={14} aria-hidden="true" />
              <span>{t('goal.resume', 'resume')}</span>
            </button>
          )}
          <button type="button" className={`${styles.btn} ${styles.btnComplete}`} onClick={onComplete} aria-label={t('goal.complete', 'complete')}>
            <CheckCircle size={14} aria-hidden="true" />
            <span>{t('goal.complete', 'complete')}</span>
          </button>
        </div>
      )}
    </div>
  )
}
