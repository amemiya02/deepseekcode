import type { Mode } from '../lib/layoutStore'

export interface ModeTabsProps {
  mode: Mode
  onChange: (mode: Mode) => void
}

const tabs: { id: Mode; label: string; disabled?: boolean; hint?: string }[] = [
  { id: 'code', label: 'Code' },
  { id: 'write', label: 'Write', disabled: true, hint: 'soon' },
]

export function ModeTabs({ mode, onChange }: ModeTabsProps) {
  return (
    <div className="mode-tabs" role="tablist" aria-label="Mode">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          role="tab"
          aria-selected={mode === tab.id}
          aria-disabled={tab.disabled || undefined}
          disabled={tab.disabled}
          className={`mode-tab${mode === tab.id ? ' mode-tab--active' : ''}`}
          onClick={() => !tab.disabled && onChange(tab.id)}
        >
          {tab.label}
          {tab.hint && <span className="mode-tab__hint">{tab.hint}</span>}
        </button>
      ))}
    </div>
  )
}
