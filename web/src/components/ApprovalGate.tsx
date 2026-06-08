// ApprovalGate — the single inline approval surface (spec §5), Codex/Claude-style:
// every permission request renders as a contained card pinned above the composer,
// never as a screen-blanking modal. Edit/write/patch requests show the proposed
// change as a read-only DiffView (the P3.1 obsidian island); every other tool
// (bash, glob, read_file, …) shows a compact mono preview of what would run.
// Keyboard: Enter = allow once, Backspace/Delete = reject.
import { useEffect, useRef, useState } from 'react'
import { Check, ChevronDown, SquareTerminal, X } from 'lucide-react'
import { useLocale } from '../lib/i18n'
import type { PermissionDecision, PermissionRequest } from '../lib/api'
import { buildApprovalPatch, isEditApproval } from '../lib/approval'
import { DiffView } from './DiffView'

// renderArgs turns the tool's argument object into a compact, human-readable
// preview. A single `command`/`cmd` string renders bare; anything else is JSON.
function renderArgs(args: Record<string, unknown>): string {
  const cmd = args.command ?? args.cmd
  if (typeof cmd === 'string') return cmd
  try {
    return JSON.stringify(args, null, 2)
  } catch {
    return String(args)
  }
}

export function ApprovalGate({
  request,
  onDecide,
}: {
  request: PermissionRequest
  onDecide?: (decision: PermissionDecision) => void
}) {
  const { t } = useLocale()
  const isEdit = isEditApproval(request)
  const [menuOpen, setMenuOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const decide = (d: PermissionDecision) => onDecide?.(d)

  // Enter = allow once, Backspace/Delete = reject. Scoped to the gate element.
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
    <div
      className="approval"
      data-testid="approval-gate"
      ref={rootRef}
      tabIndex={-1}
      role="group"
      aria-label={isEdit ? t('approval.aria', 'edit approval') : t('perm.aria', 'permission request')}
    >
      {isEdit ? (
        <ApprovalDiff request={request} />
      ) : (
        <div className="approval__card" data-testid="approval-cmd">
          <div className="approval__head">
            <SquareTerminal size={14} aria-hidden="true" />
            <span className="approval__tool">{request.tool}</span>
            <span className="approval__label">{t('perm.wantsToRun', 'wants to run')}</span>
          </div>
          <pre className="approval__cmd">{renderArgs(request.args)}</pre>
        </div>
      )}
      <div className="approval__actions">
        <div className="approval__accept">
          <button className="approval__btn approval__btn--accept" data-testid="approve-once" type="button" onClick={() => decide('once')}>
            <Check size={13} aria-hidden="true" /> {isEdit ? t('approval.once', 'Accept') : t('perm.once', 'Allow once')}
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
        <span className="approval__hint" aria-hidden="true">{t('approval.hint', 'Enter to accept · Backspace to reject')}</span>
      </div>
    </div>
  )
}

// ApprovalDiff isolates buildApprovalPatch so it only runs for edit requests.
function ApprovalDiff({ request }: { request: PermissionRequest }) {
  const { path, patch } = buildApprovalPatch(request)
  return <DiffView path={path} patch={patch} readOnly />
}
