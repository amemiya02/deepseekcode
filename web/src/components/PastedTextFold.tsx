import { useState } from 'react'
import { useT } from '../lib/i18n'

export function PastedTextFold({ text }: { text: string }) {
  const t = useT()
  const [expanded, setExpanded] = useState(false)
  const lineCount = text.split('\n').length
  return (
    <div className="fold">
      <button className="fold__toggle" data-testid="fold-toggle" onClick={() => setExpanded((v) => !v)} aria-expanded={expanded}>
        {expanded ? t('paste.hide', 'Hide') : t('paste.summary', 'Pasted {lines} lines', { lines: lineCount })}
      </button>
      {expanded && <pre className="fold__body">{text}</pre>}
    </div>
  )
}
