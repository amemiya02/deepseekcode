import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useT } from '../lib/i18n'

export function ThinkingBlock({ text, open = false }: { text: string; open?: boolean }) {
  const t = useT()
  const [expanded, setExpanded] = useState(open)
  return (
    <div className="thinking">
      <button
        className="thinking__toggle"
        data-testid="thinking-toggle"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
      >
        <ChevronRight className={`thinking__chevron ${expanded ? 'thinking__chevron--open' : ''}`} size={12} />
        {t('msg.thinking', 'Thinking')}
      </button>
      {expanded && <pre className="thinking__body">{text}</pre>}
    </div>
  )
}
