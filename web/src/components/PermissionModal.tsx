// Adapted from deepseek-reasonix (MIT) — components/ApprovalModal.tsx
import { useEffect, useRef } from 'react'
import { PermissionCard } from './PermissionCard'
import type { PermissionDecision, PermissionRequest } from '../lib/api'
import styles from './PermissionModal.module.css'

// PermissionModal blocks the UI for a permission request. Esc and a backdrop click
// both deny; number keys 1-4 select once/session/always/deny (reasonix's keyboard
// shelf, remapped to our four canonical PermissionDecision values).
const KEY_MAP: Record<string, PermissionDecision> = {
  '1': 'once',
  '2': 'session',
  '3': 'always',
  '4': 'deny',
  Escape: 'deny',
}

export function PermissionModal({
  open,
  request,
  onDecide,
}: {
  open: boolean
  request: PermissionRequest
  onDecide?: (decision: PermissionDecision) => void
}) {
  const bodyRef = useRef<HTMLDivElement | null>(null)

  // Focus the dialog on open so the keyboard map is live without a click.
  useEffect(() => {
    if (open) bodyRef.current?.focus()
  }, [open, request.id])

  // Global key handler (the dialog may not hold focus once a button is hovered).
  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      const tag = target?.tagName.toLowerCase()
      if (tag === 'input' || tag === 'textarea' || target?.isContentEditable) return
      const decision = KEY_MAP[event.key]
      if (!decision) return
      event.preventDefault()
      onDecide?.(decision)
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open, onDecide])

  if (!open) return null

  return (
    <div className={styles.backdrop} data-testid="perm-backdrop" onClick={() => onDecide?.('deny')}>
      <div
        ref={bodyRef}
        className={styles.body}
        role="dialog"
        aria-modal="true"
        aria-label="permission request"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
      >
        <PermissionCard request={request} onDecide={onDecide} />
      </div>
    </div>
  )
}
