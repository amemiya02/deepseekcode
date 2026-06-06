// Adapted from deepseek-reasonix (MIT) — components/ApprovalModal.tsx, components/PromptShelf.tsx
import { useLocale } from '../lib/i18n'
import type { PermissionDecision, PermissionRequest } from '../lib/api'
import styles from './PermissionCard.module.css'

// PermissionCard is the inline gate for a tool call awaiting approval. It mirrors
// reasonix's ApprovalModal action set (allow once / session / persistent / deny)
// but exposes them as PermissionDecision values the gateway understands.
export function PermissionCard({
  request,
  onDecide,
}: {
  request: PermissionRequest
  onDecide?: (decision: PermissionDecision) => void
}) {
  const { t } = useLocale()
  const summary = renderArgs(request.args)
  const decide = (d: PermissionDecision) => onDecide?.(d)

  return (
    <div className={styles.card} role="group" aria-label={t('perm.aria', 'permission request')}>
      <div className={styles.head}>
        <span className={styles.tool}>{request.tool}</span>
        <span className={styles.label}>{t('perm.wantsToRun', 'wants to run')}</span>
      </div>
      <pre className={styles.args}>{summary}</pre>
      <div className={styles.actions}>
        <button
          className={`${styles.btn} ${styles.danger}`}
          data-testid="perm-deny"
          type="button"
          onClick={() => decide('deny')}
        >
          {t('perm.deny', 'Deny')}
        </button>
        <button className={styles.btn} data-testid="perm-once" type="button" onClick={() => decide('once')}>
          {t('perm.once', 'Allow once')}
        </button>
        <button className={styles.btn} data-testid="perm-session" type="button" onClick={() => decide('session')}>
          {t('perm.session', 'Allow session')}
        </button>
        <button
          className={`${styles.btn} ${styles.accent}`}
          data-testid="perm-always"
          type="button"
          onClick={() => decide('always')}
        >
          {t('perm.always', 'Always allow')}
        </button>
      </div>
    </div>
  )
}

// renderArgs turns the tool's argument object into a compact, human-readable line.
// A single `command`/`cmd` string renders bare; anything else is pretty JSON.
function renderArgs(args: Record<string, unknown>): string {
  const cmd = args.command ?? args.cmd
  if (typeof cmd === 'string') return cmd
  try {
    return JSON.stringify(args, null, 2)
  } catch {
    return String(args)
  }
}
