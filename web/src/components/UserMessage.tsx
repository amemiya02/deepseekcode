// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Message.tsx (UserMessage)
import { useState } from 'react'
import { useT } from '../lib/i18n'
import { PastedTextFold } from './PastedTextFold'
import { RewindMenu } from './RewindMenu'
import type { RewindScope } from '../lib/checkpoint'

const FOLD_LINES = 20

export interface UserMessageProps {
  text: string
  pills?: string[]
  messageIndex?: number
  onRewind?: (keepMessages: number, scope: RewindScope) => void
  onFork?: () => void
  onSummarize?: (mode: 'from' | 'upto', index: number) => void
}

export function UserMessage({ text, pills = [], messageIndex = 0, onRewind, onFork, onSummarize }: UserMessageProps) {
  const t = useT()
  const big = text.split('\n').length >= FOLD_LINES
  const [menuOpen, setMenuOpen] = useState(false)
  const hasActions = onRewind != null || onFork != null || onSummarize != null
  return (
    <div className="msg msg--user">
      <div className="msg__avatar msg__avatar--user" data-testid="msg-avatar" aria-hidden="true">
        <span className="msg__avatar-dot" />
      </div>
      <div className="msg__main">
        <div className="msg__head">
          <span className="msg__name">{t('msg.you', 'You')}</span>
          {hasActions && (
            <button
              className="msg__rewind-trigger"
              data-testid="rewind-trigger"
              aria-label={t('rewind.open', 'Open rewind menu')}
              onClick={() => setMenuOpen((o) => !o)}
            >
              &#8635;
            </button>
          )}
        </div>
        {pills.length > 0 && (
          <div className="msg__pills">
            {pills.map((p) => (
              <span className="pill" key={p}>{p}</span>
            ))}
          </div>
        )}
        <div className="msg__bubble">
          {big ? <PastedTextFold text={text} /> : <div className="msg__text">{text}</div>}
        </div>
        {hasActions && menuOpen && (
          <RewindMenu
            messageIndex={messageIndex}
            onRewind={onRewind}
            onFork={onFork}
            onSummarize={onSummarize}
          />
        )}
      </div>
    </div>
  )
}
