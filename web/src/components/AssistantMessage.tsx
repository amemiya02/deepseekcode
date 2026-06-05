// Adapted from deepseek-reasonix (MIT) — desktop/frontend/src/components/Message.tsx (AssistantMessage)
// While streaming, render raw text + a caret (stable, no per-token markdown reflow);
// once settled, render the Markdown body. Mirrors reasonix's "parse once, on completion".
import { useT } from '../lib/i18n'
import { Markdown } from './Markdown'
import { Logo } from './Logo'

export function AssistantMessage({ text, streaming }: { text: string; streaming: boolean }) {
  const t = useT()
  return (
    <div className="msg msg--assistant">
      <div className="msg__avatar msg__avatar--assistant" data-testid="msg-avatar" aria-hidden="true">
        <Logo size={16} />
      </div>
      <div className="msg__main">
        <div className="msg__head">
          <span className="msg__name msg__name--assistant">{t('msg.dsc', 'dsc')}</span>
        </div>
        <div className="msg__body">
          {streaming ? (
            <div className="msg__stream">{text}<span className="cursor" /></div>
          ) : (
            <Markdown text={text} />
          )}
        </div>
      </div>
    </div>
  )
}
