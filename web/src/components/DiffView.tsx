// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/DiffView.tsx
// Per-hunk accept/reject with onHunk(index, accepted). Renders inside a CodeIsland
// (spec §3 material rule): obsidian surface + brand header `path · +N −M`, dark in every
// app mode. Hunk rows + per-hunk controls + the onHunk contract are unchanged. The
// inline approve/改一下/拒绝 gate + 4-level granularity is a later phase (P3.2).
import { useRef, useState } from 'react'
import { Check, X } from 'lucide-react'
import { useT } from '../lib/i18n'
import { parseHunks, countDiffLines } from '../lib/diff'
import { CodeIsland } from './CodeIsland'

export function DiffView({
  path = '',
  patch = '',
  onHunk = () => {},
  readOnly = false,
}: {
  path?: string
  patch?: string
  onHunk?: (index: number, accepted: boolean) => void
  readOnly?: boolean
}) {
  const t = useT()
  const hunks = parseHunks(patch)
  const { added, removed } = countDiffLines(patch)
  // null = undecided, true = accepted, false = rejected
  const [decisions, setDecisions] = useState<Array<boolean | null>>(() => hunks.map(() => null))
  // Ref mirrors decisions for a synchronous guard — prevents onHunk firing twice on a
  // rapid double-click before React re-renders and disables the button.
  const committedRef = useRef<Array<boolean | null>>(hunks.map(() => null))

  const decide = (index: number, accepted: boolean) => {
    if (committedRef.current[index] !== null) return
    committedRef.current[index] = accepted
    setDecisions((prev) => {
      const next = prev.slice()
      next[index] = accepted
      return next
    })
    onHunk(index, accepted)
  }

  return (
    <CodeIsland
      label={<span className="diffview__path">{path || 'diff'}</span>}
      meta={
        <>
          <span className="island__add" data-testid="diff-added">+{added}</span>
          <span className="island__del" data-testid="diff-removed">−{removed}</span>
        </>
      }
      bodyClassName="diffview"
    >
      {hunks.map((hunk, i) => (
        <div className="hunk" data-testid="diff-hunk" data-decided={decisions[i] !== null} key={i}>
          <div className="hunk__head">
            <span className="hunk__header">{hunk.header}</span>
            {!readOnly && (
              <span className="hunk__actions">
                <button data-testid="hunk-accept" disabled={decisions[i] !== null} onClick={() => decide(i, true)} aria-label={t('diff.accept', 'Accept')}>
                  <Check size={12} /> {t('diff.accept', 'Accept')}
                </button>
                <button data-testid="hunk-reject" disabled={decisions[i] !== null} onClick={() => decide(i, false)} aria-label={t('diff.reject', 'Reject')}>
                  <X size={12} /> {t('diff.reject', 'Reject')}
                </button>
              </span>
            )}
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
    </CodeIsland>
  )
}
