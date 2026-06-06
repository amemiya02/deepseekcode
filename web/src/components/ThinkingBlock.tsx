import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useT } from '../lib/i18n'
import { formatThinkingDuration } from '../lib/transcript'

export function ThinkingBlock({
  text, open = false, startedAt, endedAt,
}: { text: string; open?: boolean; startedAt?: number; endedAt?: number }) {
  const t = useT()
  const [expanded, setExpanded] = useState(open)
  const settled = startedAt != null && endedAt != null
  const label = settled
    ? formatThinkingDuration(startedAt!, endedAt!)
    : t('msg.thinking', 'Thinking…')
  return (
    <div className={`thinking ${settled ? '' : 'thinking--live'}`}>
      <button
        className="thinking__toggle"
        data-testid="thinking-toggle"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
      >
        <ChevronRight className={`thinking__chevron ${expanded ? 'thinking__chevron--open' : ''}`} size={12} />
        {label}
      </button>
      {expanded && <pre className="thinking__body">{text}</pre>}
    </div>
  )
}
