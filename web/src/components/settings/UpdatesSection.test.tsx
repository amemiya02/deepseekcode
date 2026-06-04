import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { UpdatesSection } from './UpdatesSection'
import * as system from '../../lib/system'

afterEach(() => vi.restoreAllMocks())

describe('UpdatesSection', () => {
  it('shows current version and up-to-date when no update', async () => {
    vi.spyOn(system, 'fetchUpdate').mockResolvedValue({ current: 'v1.0.0', latest: 'v1.0.0', updateAvailable: false, url: '' })
    render(<UpdatesSection />)
    await waitFor(() => {
      expect(screen.getByText(/v1\.0\.0/)).toBeInTheDocument()
      expect(screen.getByText(/up to date/i)).toBeInTheDocument()
    })
  })

  it('shows the UpdateBanner when an update exists', async () => {
    vi.spyOn(system, 'fetchUpdate').mockResolvedValue({ current: 'v1.0.0', latest: 'v2.0.0', updateAvailable: true, url: 'https://dl/v2' })
    render(<UpdatesSection />)
    await waitFor(() => expect(screen.getByText(/v2\.0\.0/)).toBeInTheDocument())
  })
})
