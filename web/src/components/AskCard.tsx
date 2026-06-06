// Adapted from deepseek-reasonix (MIT) — components/AskCard.tsx, components/PromptShelf.tsx
import { useState } from 'react'
import { useLocale } from '../lib/i18n'
import type { AskAnswer, AskQuestion, AskRequest } from '../lib/api'
import styles from './AskCard.module.css'

// AskCard renders the agent's `ask` request as a shelf of choice cards plus a
// free-text fallback and a "just chat" escape. Single-select questions answer
// immediately; multi-select accumulates and waits for an explicit submit.
export function AskCard({
  request,
  onAnswer,
  onDismiss,
}: {
  request: AskRequest
  onAnswer?: (answer: AskAnswer) => void
  onDismiss?: () => void
}) {
  const { t } = useLocale()
  // Per-question accumulators for multi-select and free text.
  const [selected, setSelected] = useState<Record<number, string[]>>({})
  const [drafts, setDrafts] = useState<Record<number, string>>({})

  const isSelected = (qi: number, label: string) => (selected[qi] ?? []).includes(label)

  const pickSingle = (qi: number, label: string) =>
    onAnswer?.({ id: request.id, questionIndex: qi, choices: [label] })

  const toggleMulti = (qi: number, label: string) =>
    setSelected((prev) => {
      const cur = prev[qi] ?? []
      return {
        ...prev,
        [qi]: cur.includes(label) ? cur.filter((l) => l !== label) : [...cur, label],
      }
    })

  const submitMulti = (qi: number) =>
    onAnswer?.({ id: request.id, questionIndex: qi, choices: selected[qi] ?? [] })

  const submitText = (qi: number) =>
    onAnswer?.({ id: request.id, questionIndex: qi, text: drafts[qi] ?? '' })

  return (
    <div className={styles.card} role="group" aria-label={t('ask.aria', 'agent question')}>
      {request.questions.map((q: AskQuestion, qi: number) => (
        <div className={styles.q} key={qi}>
          {q.header && <div className={styles.header}>{q.header}</div>}
          <div className={styles.question}>{q.question}</div>
          <div className={styles.options}>
            {q.options.map((opt) => (
              <button
                key={opt.label}
                type="button"
                className={`${styles.opt}${q.multiple && isSelected(qi, opt.label) ? ` ${styles.selected}` : ''}`}
                onClick={() => (q.multiple ? toggleMulti(qi, opt.label) : pickSingle(qi, opt.label))}
              >
                <span className={styles.optLabel}>{opt.label}</span>
                {opt.description && <span className={styles.optDesc}>{opt.description}</span>}
              </button>
            ))}
          </div>
          {q.multiple && (
            <button
              type="button"
              className={`${styles.btn} ${styles.accent}`}
              data-testid={`ask-submit-${qi}`}
              onClick={() => submitMulti(qi)}
            >
              {t('ask.submit', 'Submit selection')}
            </button>
          )}
          <div className={styles.free}>
            <input
              className={styles.input}
              data-testid={`ask-text-${qi}`}
              placeholder={t('ask.customPlaceholder', 'Or type an answer…')}
              value={drafts[qi] ?? ''}
              onChange={(e) => setDrafts((prev) => ({ ...prev, [qi]: e.target.value }))}
            />
            <button
              type="button"
              className={styles.btn}
              data-testid={`ask-text-submit-${qi}`}
              onClick={() => submitText(qi)}
            >
              {t('ask.send', 'Send')}
            </button>
          </div>
        </div>
      ))}
      <button type="button" className={styles.link} data-testid="ask-chat" onClick={() => onDismiss?.()}>
        {t('ask.justChat', 'Just chat instead')}
      </button>
    </div>
  )
}
