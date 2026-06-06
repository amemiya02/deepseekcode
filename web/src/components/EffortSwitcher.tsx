// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/EffortSwitcher.tsx
import { useState } from 'react'
import { Check, ChevronsUpDown } from 'lucide-react'
import { useT } from '../lib/i18n'

export function EffortSwitcher({
  levels, current, disabled, onPick,
}: {
  levels: string[]
  current: string
  disabled: boolean
  onPick: (level: string) => void
}) {
  const t = useT()
  const [open, setOpen] = useState(false)
  if (levels.length === 0) return null

  const pick = (level: string) => {
    setOpen(false)
    if (level !== current) onPick(level)
  }

  return (
    <div className="modelsw effortsw">
      <button
        className={`modelsw__trigger effortsw__trigger ${current !== 'auto' ? 'effortsw__trigger--explicit' : ''}`}
        data-testid="effort-trigger"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="modelsw__label">{t('status.effort', 'effort: {level}', { level: current })}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open && !disabled && (
        <>
          <div className="modelsw__backdrop" onClick={() => setOpen(false)} />
          <div className="modelsw__menu effortsw__menu" role="listbox">
            {levels.map((level) => (
              <button
                key={level}
                role="option"
                aria-selected={level === current}
                className={`modelsw__item ${level === current ? 'modelsw__item--current' : ''}`}
                onClick={() => pick(level)}
              >
                <span className="modelsw__model">{level}</span>
                {level === current && <Check size={13} className="modelsw__check" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
