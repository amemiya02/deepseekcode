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
  onPick: (id: string, provider?: string) => void
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

  const pick = (m: ModelInfo) => { if (m.available === false) return; setOpen(false); onPick(m.id, m.provider) }

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
            {(() => {
              const rows: React.ReactNode[] = []
              let lastProvider: string | undefined
              for (const m of models) {
                const prov = m.provider ?? ''
                if (prov && prov !== lastProvider) {
                  rows.push(<div key={`hdr-${prov}`} className="modelsw__provider" data-testid="provider-header">{prov}</div>)
                  lastProvider = prov
                }
                const isCurrent = m.id === activeId
                const unavailable = m.available === false
                const ctxStr = formatContext(m.context)
                rows.push(
                  <div
                    key={m.id}
                    role="option"
                    tabIndex={unavailable ? -1 : 0}
                    aria-selected={isCurrent}
                    aria-disabled={unavailable}
                    className={`modelsw__item${isCurrent ? ' modelsw__item--current' : ''}${unavailable ? ' modelsw__item--disabled' : ''}`}
                    onClick={() => pick(m)}
                    onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pick(m) } }}
                  >
                    <span className="modelsw__model">{m.label}</span>
                    {m.note && <span className="modelsw__note" data-testid="model-note">{m.note}</span>}
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
                  </div>,
                )
              }
              return rows
            })()}
          </div>
        </>
      )}
    </div>
  )
}
