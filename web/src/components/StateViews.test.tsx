import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { StateView } from './StateViews'

describe('StateView', () => {
  it('renders the loading variant with a status role', () => {
    render(<StateView kind="loading" message="Loading settings…" />)
    const el = screen.getByRole('status')
    expect(el.textContent).toContain('Loading settings…')
  })

  it('renders the empty variant with title and hint', () => {
    render(<StateView kind="empty" title="No sessions yet" hint="Start a chat to begin." />)
    expect(screen.getByText('No sessions yet')).toBeInTheDocument()
    expect(screen.getByText('Start a chat to begin.')).toBeInTheDocument()
  })

  it('renders the error variant with an alert role and retry button', async () => {
    const onRetry = vi.fn()
    render(<StateView kind="error" message="gateway error 500" onRetry={onRetry} />)
    const alert = screen.getByRole('alert')
    expect(alert.textContent).toContain('gateway error 500')
    await userEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(onRetry).toHaveBeenCalledOnce()
  })
})
