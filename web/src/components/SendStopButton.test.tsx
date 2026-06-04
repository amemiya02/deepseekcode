import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { SendStopButton } from './SendStopButton'

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('SendStopButton', () => {
  it('idle: shows Send and fires onSend', async () => {
    let sent = false
    wrap(<SendStopButton streaming={false} disabled={false} onSend={() => { sent = true }} onCancel={() => {}} />)
    await userEvent.click(screen.getByTestId('send-stop'))
    expect(sent).toBe(true)
  })

  it('streaming: shows Stop and fires onCancel', async () => {
    let cancelled = false
    wrap(<SendStopButton streaming disabled={false} onSend={() => {}} onCancel={() => { cancelled = true }} />)
    await userEvent.click(screen.getByTestId('send-stop'))
    expect(cancelled).toBe(true)
  })

  it('disabled blocks onSend when idle', async () => {
    let sent = false
    wrap(<SendStopButton streaming={false} disabled onSend={() => { sent = true }} onCancel={() => {}} />)
    expect(screen.getByTestId('send-stop')).toBeDisabled()
    await userEvent.click(screen.getByTestId('send-stop'))
    expect(sent).toBe(false)
  })
})
