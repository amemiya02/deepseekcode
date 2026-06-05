// ApprovalGate — the inline diff-as-approval gate (spec §5). For an edit/write/patch
// permission request it renders the proposed change as a read-only DiffView (the P3.1
// obsidian island) plus an action bar mapping to the four existing PermissionDecision
// levels. Keyboard: Enter = accept once, Backspace/Delete = reject.
import { useEffect, useRef, useState } from 'react'
import { Check, ChevronDown, X } from 'lucide-react'
import { useLocale } from '../lib/i18n'
import type { PermissionDecision, PermissionRequest } from '../lib/api'
import { buildApprovalPatch } from '../lib/approval'
import { DiffView } from './DiffView'

export function ApprovalGate({
  request,
  onDecide,
}: {
  request: PermissionRequest
  onDecide?: (decision: PermissionDecision) => void
}) {
  const { t } = useLocale()
  const { path, patch } = buildApprovalPatch(request)
  const [menuOpen, setMenuOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const decide = (d: PermissionDecision) => onDecide?.(d)

  // Enter = accept once, Backspace/Delete = reject. Scoped to the gate element.
  useEffect(() => {
    const el = rootRef.current
    if (!el) return
    el.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Enter') { e.preventDefault(); onDecide?.('once') }
      else if (e.key === 'Backspace' || e.key === 'Delete') { e.preventDefault(); onDecide?.('deny') }
    }
    el.addEventListener('keydown', onKey)
    return () => el.removeEventListener('keydown', onKey)
  }, [request.id, onDecide])

  return (
    <div className="approval" data-testid="approval-gate" ref={rootRef} tabIndex={-1} role="group" aria-label={t('approval.aria', 'edit approval')}>
      <DiffView path={path} patch={patch} readOnly />
      <div className="approval__actions">
        <div className="approval__accept">
          <button className="approval__btn approval__btn--accept" data-testid="approve-once" type="button" onClick={() => decide('once')}>
            <Check size={13} aria-hidden="true" /> {t('approval.once', 'Accept')}
          </button>
          <button className="approval__caret" data-testid="approve-more" type="button" aria-label={t('approval.more', 'More approval options')} aria-haspopup="menu" aria-expanded={menuOpen} onClick={() => setMenuOpen((o) => !o)}>
            <ChevronDown size={13} aria-hidden="true" />
          </button>
          {menuOpen && (
            <div className="approval__menu" role="menu">
              <button className="approval__menuitem" data-testid="approve-session" type="button" role="menuitem" onClick={() => { decide('session'); setMenuOpen(false) }}>{t('approval.session', 'Allow for session')}</button>
              <button className="approval__menuitem" data-testid="approve-always" type="button" role="menuitem" onClick={() => { decide('always'); setMenuOpen(false) }}>{t('approval.always', 'Always allow')}</button>
            </div>
          )}
        </div>
        <button className="approval__btn approval__btn--deny" data-testid="approve-deny" type="button" onClick={() => decide('deny')}>
          <X size={13} aria-hidden="true" /> {t('approval.deny', 'Reject')}
        </button>
        <span className="approval__hint" aria-hidden="true">{t('approval.hint', '⏎ accept · ⌫ reject')}</span>
      </div>
    </div>
  )
}
