import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { Toasts } from './Toasts'
import { pushToast, clearToasts } from '../../lib/toasts'
import { LocaleProvider } from '../../lib/i18n'

beforeEach(() => clearToasts())

describe('Toasts', () => {
  it('renders a pushed toast message', async () => {
    render(
      <LocaleProvider>
        <Toasts />
      </LocaleProvider>,
    )
    pushToast({ kind: 'info', message: 'Saved settings' })
    expect(await screen.findByText('Saved settings')).toBeInTheDocument()
  })

  it('removes a toast when its dismiss button is clicked', async () => {
    const user = userEvent.setup()
    render(
      <LocaleProvider>
        <Toasts />
      </LocaleProvider>,
    )
    pushToast({ kind: 'danger', message: 'It broke', durationMs: 100000 })
    const item = await screen.findByText('It broke')
    const btn = item.closest('[data-testid="toast"]')!.querySelector('button')!
    await user.click(btn)
    await waitFor(() => expect(screen.queryByText('It broke')).toBeNull())
  })
})
