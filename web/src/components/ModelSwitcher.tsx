// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/ModelSwitcher.tsx
// Status-line model picker: the label is a button that opens a popover listing
// configured models (fetched lazily on open, or from the parent's capability-rich
// prop). Selecting one switches the active model; the backend carries the conversation.
import { useEffect, useState } from 'react'
import { Brain, Check, ChevronsUpDown, Eye, Wrench } from 'lucide-react'
import { fetchModels, type ModelInfo } from '../lib/api'
import { useT } from '../lib/i18n'

function formatContext(n?: number): string {
  if (!n) return ''
  if (n >= 1_000_000) return `${Math.round(n / 1_000_000)}M`
  if (n >= 1_000) return `${Math.round(n / 1_000)}K`
  return String(n)
}

export function ModelSwitcher({
  label,
  activeId,
  models: modelsProp,
  onPick,
}: {
  label: string
  activeId?: string
  models?: ModelInfo[]
  onPick: (id: string) => void
}) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const [fetched, setFetched] = useState<ModelInfo[]>([])

  // Use parent-supplied capability-rich list when available; fall back to self-fetch.
  const models = modelsProp && modelsProp.length > 0 ? modelsProp : fetched

  useEffect(() => {
    // Only self-fetch when the parent did not provide a capability-rich list.
    if (open && !(modelsProp && modelsProp.length > 0)) {
      fetchModels().then(setFetched).catch(() => {})
    }
  }, [open, modelsProp])

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
            {models.map((m) => {
              const isCurrent = m.id === activeId
              const ctxStr = formatContext(m.context)
              return (
                <div
                  key={m.id}
                  role="option"
                  tabIndex={0}
                  aria-selected={isCurrent}
                  className={`modelsw__item${isCurrent ? ' modelsw__item--current' : ''}`}
                  onClick={() => pick(m.id)}
                  onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pick(m.id) } }}
                >
                  <span className="modelsw__model">{m.label}</span>
                  {ctxStr && (
                    <span className="modelsw__ctx" data-testid="model-ctx">{ctxStr}</span>
                  )}
                  {m.caps && (
                    <span className="modelsw__caps" aria-hidden="true">
                      {m.caps.reasoning && <Brain size={11} data-testid="cap-reasoning" />}
                      {m.caps.tools && <Wrench size={11} data-testid="cap-tools" />}
                      {m.caps.vision && <Eye size={11} data-testid="cap-vision" />}
                    </span>
                  )}
                  {isCurrent && <Check size={13} className="modelsw__check" />}
                </div>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}
