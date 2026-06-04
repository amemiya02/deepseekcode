// Contract 4: owned by Wave 2, reused by Wave 4. Props { streaming, disabled, onSend, onCancel }.
import { ArrowUp, Square } from 'lucide-react'
import { useT } from '../lib/i18n'

export function SendStopButton({
  streaming, disabled, onSend, onCancel,
}: {
  streaming: boolean
  disabled: boolean
  onSend: () => void
  onCancel: () => void
}) {
  const t = useT()
  if (streaming) {
    return (
      <button className="composer__btn composer__btn--stop" data-testid="send-stop" onClick={onCancel} aria-label={t('composer.stop', 'Stop')}>
        <Square size={14} fill="currentColor" />
      </button>
    )
  }
  return (
    <button
      className="composer__btn composer__btn--send"
      data-testid="send-stop"
      onClick={onSend}
      disabled={disabled}
      aria-label={t('composer.send', 'Send')}
    >
      <ArrowUp size={16} />
    </button>
  )
}
