import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { UpdateBanner } from './UpdateBanner'

describe('UpdateBanner', () => {
  it('renders nothing when no update is available', () => {
    const { container } = render(
      <UpdateBanner info={{ current: 'v1.0.0', latest: 'v1.0.0', updateAvailable: false, url: '' }} />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('shows the version and opens the download via the Wails bridge', async () => {
    const open = vi.fn()
    ;(window as unknown as { runtime?: { BrowserOpenURL: (u: string) => void } }).runtime = { BrowserOpenURL: open }
    render(<UpdateBanner info={{ current: 'v1.0.0', latest: 'v2.0.0', updateAvailable: true, url: 'https://dl/v2' }} />)
    expect(screen.getByText(/v2\.0\.0/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /download|update/i }))
    expect(open).toHaveBeenCalledWith('https://dl/v2')
  })

  it('dismisses when the close button is clicked', async () => {
    const onDismiss = vi.fn()
    render(
      <UpdateBanner
        info={{ current: 'v1.0.0', latest: 'v2.0.0', updateAvailable: true, url: 'https://dl/v2' }}
        onDismiss={onDismiss}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: /dismiss|close/i }))
    expect(onDismiss).toHaveBeenCalledOnce()
  })
})
