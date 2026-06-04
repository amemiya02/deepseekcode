import { useT } from '../lib/i18n'

export type AutonomyMode = 'ask' | 'auto-edit' | 'plan' | 'yolo'

const ORDER: AutonomyMode[] = ['ask', 'auto-edit', 'plan', 'yolo']

export function AutonomyToggle({ mode, onChange }: { mode: AutonomyMode; onChange: (mode: AutonomyMode) => void }) {
  const t = useT()
  const label: Record<AutonomyMode, string> = {
    ask: t('autonomy.ask', 'Ask'),
    'auto-edit': t('autonomy.autoEdit', 'Auto-edit'),
    plan: t('autonomy.plan', 'Plan'),
    yolo: t('autonomy.yolo', 'Yolo'),
  }
  const next = () => onChange(ORDER[(ORDER.indexOf(mode) + 1) % ORDER.length])
  return (
    <button
      className={`autonomy autonomy--${mode}`}
      data-testid="autonomy-toggle"
      onClick={next}
      title={t('composer.modeHint', 'Shift+Tab to cycle')}
    >
      <span className="autonomy__dot" />
      {label[mode]}
    </button>
  )
}
