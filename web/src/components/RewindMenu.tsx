import { RotateCcw, FileCode, GitFork, ListCollapse } from 'lucide-react'
import type { RewindScope } from '../lib/checkpoint'
import { t } from '../lib/i18n'
import styles from './RewindMenu.module.css'

export interface RewindMenuProps {
  messageIndex: number
  onRewind?: (keepMessages: number, scope: RewindScope) => void
  onFork?: () => void
  onSummarize?: (mode: 'from' | 'upto', index: number) => void
}

// RewindMenu is the per-user-message checkpoint control: rewind the
// conversation/code/both to before this message, fork a clean copy from here,
// or collapse the range above/below into a summary. It is pure-props; the parent
// (App, mounting it on user messages per Contract 4) wires the callbacks to
// web/src/lib/checkpoint.ts. keepMessages == messageIndex: rewinding "to before
// message N" retains the leading N messages.
export function RewindMenu({
  messageIndex,
  onRewind,
  onFork,
  onSummarize,
}: RewindMenuProps) {
  return (
    <div className={styles.menu} role="menu">
      <button
        role="menuitem"
        className={styles.item}
        data-testid="rewind-conversation"
        onClick={() => onRewind?.(messageIndex, 'conversation')}
      >
        <RotateCcw size={14} aria-hidden />
        {t('rewind.conversation', 'Rewind conversation here')}
      </button>
      <button
        role="menuitem"
        className={styles.item}
        data-testid="rewind-code"
        onClick={() => onRewind?.(messageIndex, 'code')}
      >
        <FileCode size={14} aria-hidden />
        {t('rewind.code', 'Restore code here')}
      </button>
      <button
        role="menuitem"
        className={styles.item}
        data-testid="rewind-both"
        onClick={() => onRewind?.(messageIndex, 'both')}
      >
        <RotateCcw size={14} aria-hidden />
        {t('rewind.both', 'Rewind code + conversation')}
      </button>
      <button
        role="menuitem"
        className={styles.item}
        data-testid="rewind-fork"
        onClick={() => onFork?.()}
      >
        <GitFork size={14} aria-hidden />
        {t('rewind.fork', 'Fork from here')}
      </button>
      <button
        role="menuitem"
        className={styles.item}
        data-testid="rewind-summarize-upto"
        onClick={() => onSummarize?.('upto', messageIndex)}
      >
        <ListCollapse size={14} aria-hidden />
        {t('rewind.summarizeUpto', 'Summarize up to here')}
      </button>
      <button
        role="menuitem"
        className={styles.item}
        data-testid="rewind-summarize-from"
        onClick={() => onSummarize?.('from', messageIndex)}
      >
        <ListCollapse size={14} aria-hidden />
        {t('rewind.summarizeFrom', 'Summarize from here')}
      </button>
      <p className={styles.disclaimer}>
        {t(
          'rewind.disclaimer',
          'Code rewind restores only files the agent snapshotted. Edits from bash or external tools are not tracked — git is the source of truth for the working tree.',
        )}
      </p>
    </div>
  )
}
