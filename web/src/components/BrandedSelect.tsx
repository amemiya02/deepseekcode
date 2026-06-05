import { useState } from 'react'
import { ChevronsUpDown, Check } from 'lucide-react'

export interface SelectOption { value: string; label: string }

export function BrandedSelect({
  value,
  options,
  onChange,
  ariaLabel,
  testid,
}: {
  value: string
  options: SelectOption[]
  onChange: (value: string) => void
  ariaLabel?: string
  testid?: string
}) {
  const [open, setOpen] = useState(false)
  const current = options.find((o) => o.value === value)
  const pick = (v: string) => {
    setOpen(false)
    if (v !== value) onChange(v)
  }
  return (
    <div className="modelsw brandsel">
      <button
        type="button"
        className="modelsw__trigger brandsel__trigger"
        data-testid={testid}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="modelsw__label">{current?.label ?? value}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open && (
        <>
          <div className="modelsw__backdrop" onClick={() => setOpen(false)} />
          <div className="modelsw__menu brandsel__menu" role="listbox">
            {options.map((o) => (
              <button
                type="button"
                key={o.value}
                role="option"
                aria-selected={o.value === value}
                className={`modelsw__item${o.value === value ? ' modelsw__item--current' : ''}`}
                onClick={() => pick(o.value)}
              >
                <span className="modelsw__model">{o.label}</span>
                {o.value === value && <Check size={13} className="modelsw__check" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
