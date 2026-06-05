import { useState } from 'react'
import { ChevronsUpDown, Check } from 'lucide-react'
import { ORDER, MODES, type AutonomyMode } from '../lib/autonomy'

export function ModeMenu({ mode, onChange }: { mode: AutonomyMode; onChange: (m: AutonomyMode) => void }) {
  const [open, setOpen] = useState(false)
  const pick = (m: AutonomyMode) => { setOpen(false); if (m !== mode) onChange(m) }
  return (
    <div className="modelsw modemenu">
      <button
        className={`modelsw__trigger modemenu__trigger autonomy--${mode}`}
        data-testid="mode-trigger"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="autonomy__dot" />
        <span className="modelsw__label">{MODES[mode].label}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open && (
        <>
          <div className="modelsw__backdrop" onClick={() => setOpen(false)} />
          <div className="modelsw__menu modemenu__menu" role="listbox">
            {ORDER.map((m) => (
              <button
                key={m}
                className={`modelsw__item modemenu__item autonomy--${m}`}
                data-testid={`mode-option-${m}`}
                role="option"
                aria-selected={m === mode}
                onClick={() => pick(m)}
              >
                <span className="autonomy__dot" />
                <span className="modemenu__name">{MODES[m].label}</span>
                <span className="modemenu__desc">{MODES[m].desc}</span>
                {m === mode && <Check size={13} className="modemenu__check" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
