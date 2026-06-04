// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/ModelSwitcher.tsx
// Status-line model picker: the label is a button that opens a popover listing
// configured models (fetched lazily on open). Selecting one switches the active
// model; the backend carries the conversation over.
import { useEffect, useState } from 'react'
import { Check, ChevronsUpDown } from 'lucide-react'
import { fetchModels, type ModelInfo } from '../lib/api'
import { useT } from '../lib/i18n'

export function ModelSwitcher({ label, onPick }: { label: string; onPick: (id: string) => void }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [models, setModels] = useState<ModelInfo[]>([])

  useEffect(() => {
    if (open) fetchModels().then(setModels).catch(() => {})
  }, [open])

  const pick = (id: string) => { setOpen(false); onPick(id) }

  return (
    <div className="modelsw">
      <button className="modelsw__trigger" data-testid="model-trigger" onClick={() => setOpen((v) => !v)}>
        <span className="modelsw__label">{label}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open && (
        <>
          <div className="modelsw__backdrop" onClick={() => setOpen(false)} />
          <div className="modelsw__menu" role="listbox">
            {models.length === 0 && <div className="modelsw__empty">{t('status.noModels', 'No models')}</div>}
            {models.map((m) => (
              <button
                key={m.id}
                role="option"
                aria-selected={m.label === label}
                className={`modelsw__item ${m.label === label ? 'modelsw__item--current' : ''}`}
                onClick={() => pick(m.id)}
              >
                <span className="modelsw__model">{m.label}</span>
                {m.label === label && <Check size={13} className="modelsw__check" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
