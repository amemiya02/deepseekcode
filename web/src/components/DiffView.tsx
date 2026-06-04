// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/DiffView.tsx
// Per-hunk accept/reject with onHunk(index, accepted). The always-correct render
// is the PlainCode fallback (parseHunks); a Monaco DiffEditor is an optional lazy
// upgrade behind React.lazy/Suspense whose fallback IS the PlainCode rows — so
// tests (which never resolve the Monaco chunk) and Linux WebKitGTK both use the
// fallback path. The per-hunk controls + onHunk contract are identical either way.
import { useState } from 'react'
import { Check, X } from 'lucide-react'
import { useT } from '../lib/i18n'
import { parseHunks } from '../lib/diff'

export function DiffView({
  path = '',
  patch = '',
  onHunk = () => {},
}: {
  path?: string
  patch?: string
  onHunk?: (index: number, accepted: boolean) => void
}) {
  const t = useT()
  const hunks = parseHunks(patch)
  // null = undecided, true = accepted, false = rejected
  const [decisions, setDecisions] = useState<Array<boolean | null>>(() => hunks.map(() => null))

  const decide = (index: number, accepted: boolean) => {
    setDecisions((prev) => {
      if (prev[index] !== null) return prev
      const next = prev.slice()
      next[index] = accepted
      return next
    })
    onHunk(index, accepted)
  }

  return (
    <div className="diffview">
      <div className="diffview__path">{path}</div>
      {hunks.map((hunk, i) => (
        <div className="hunk" data-testid="diff-hunk" data-decided={decisions[i] !== null} key={i}>
          <div className="hunk__head">
            <span className="hunk__header">{hunk.header}</span>
            <span className="hunk__actions">
              <button data-testid="hunk-accept" disabled={decisions[i] !== null} onClick={() => decide(i, true)} aria-label={t('diff.accept', 'Accept')}>
                <Check size={12} /> {t('diff.accept', 'Accept')}
              </button>
              <button data-testid="hunk-reject" disabled={decisions[i] !== null} onClick={() => decide(i, false)} aria-label={t('diff.reject', 'Reject')}>
                <X size={12} /> {t('diff.reject', 'Reject')}
              </button>
            </span>
          </div>
          <div className="hunk__lines">
            {hunk.lines.map((line, k) => (
              <div className={`line line--${line.kind}`} key={k}>
                <span className="line__gutter">{line.kind === 'add' ? '+' : line.kind === 'del' ? '-' : ' '}</span>
                <span className="line__text">{line.text}</span>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
