// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Message.tsx (UserMessage)
import { useT } from '../lib/i18n'
import { PastedTextFold } from './PastedTextFold'

const FOLD_LINES = 20

export function UserMessage({ text, pills = [] }: { text: string; pills?: string[] }) {
  const t = useT()
  const big = text.split('\n').length >= FOLD_LINES
  return (
    <div className="msg msg--user">
      <div className="msg__label">{t('msg.you', 'You')}</div>
      {pills.length > 0 && (
        <div className="msg__pills">
          {pills.map((p) => (
            <span className="pill" key={p}>{p}</span>
          ))}
        </div>
      )}
      {big ? <PastedTextFold text={text} /> : <div className="msg__text">{text}</div>}
    </div>
  )
}
