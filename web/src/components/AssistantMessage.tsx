// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Message.tsx (AssistantMessage)
// While streaming, render raw text + a caret (stable, no per-token markdown reflow);
// once settled, render the Markdown body. Mirrors reasonix's "parse once, on completion".
import { useT } from '../lib/i18n'
import { Markdown } from './Markdown'

export function AssistantMessage({ text, streaming }: { text: string; streaming: boolean }) {
  const t = useT()
  return (
    <div className="msg msg--assistant">
      <div className="msg__label msg__label--assistant">{t('msg.dsc', 'dsc')}</div>
      <div className="msg__body">
        {streaming ? (
          <div className="msg__stream">{text}<span className="cursor" /></div>
        ) : (
          <Markdown text={text} />
        )}
      </div>
    </div>
  )
}
