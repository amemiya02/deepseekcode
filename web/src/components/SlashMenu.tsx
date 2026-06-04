// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/SlashMenu.tsx
// Presentational "/" autocomplete: the Composer owns filtering, activeIndex, and
// key handling; this renders the list and reports hover/pick. mousedown (not click)
// so picking doesn't blur the textarea first.
import { useEffect, useRef } from 'react'
import { useT } from '../lib/i18n'

export interface SlashCommand {
  name: string
  description: string
  kind: 'builtin' | 'custom' | 'mcp' | 'skill'
  hint?: string
}

export function SlashMenu({
  items, activeIndex, onPick, onHover,
}: {
  items: SlashCommand[]
  activeIndex: number
  onPick: (c: SlashCommand) => void
  onHover: (i: number) => void
}) {
  const t = useT()
  const activeRef = useRef<HTMLButtonElement>(null)
  useEffect(() => { activeRef.current?.scrollIntoView({ block: 'nearest' }) }, [activeIndex])
  const kindTag = (kind: SlashCommand['kind']) =>
    kind === 'custom' ? t('slash.project', 'project')
      : kind === 'mcp' ? t('slash.mcp', 'mcp')
        : kind === 'skill' ? t('slash.skill', 'skill') : ''
  return (
    <div className="slashmenu" role="listbox">
      {items.map((c, i) => (
        <button
          key={c.kind + ':' + c.name}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`slashmenu__item ${i === activeIndex ? 'slashmenu__item--active' : ''}`}
          onMouseDown={(e) => { e.preventDefault(); onPick(c) }}
          onMouseMove={() => onHover(i)}
        >
          <span className="slashmenu__name">/{c.name}</span>
          {c.hint && <span className="slashmenu__hint">{c.hint}</span>}
          <span className="slashmenu__desc">{c.description}</span>
          {kindTag(c.kind) && <span className="slashmenu__kind">{kindTag(c.kind)}</span>}
        </button>
      ))}
    </div>
  )
}
