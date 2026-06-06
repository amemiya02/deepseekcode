import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import { LedgerDrillDown } from './LedgerDrillDown'
import * as api from '../../lib/api'

describe('LedgerDrillDown', () => {
  beforeEach(() => {
    vi.spyOn(api, 'fetchCacheLedger').mockResolvedValue([
      { turn: 1, hit_tokens: 9000, miss_tokens: 200, evicted: false },
      { turn: 2, hit_tokens: 100, miss_tokens: 16908, evicted: true },
    ])
  })
  afterEach(() => vi.restoreAllMocks())

  it('loads the ledger for the session and renders one row per turn', async () => {
    render(<LedgerDrillDown sessionId="s1" />)
    await waitFor(() => expect(api.fetchCacheLedger).toHaveBeenCalledWith('s1', undefined))
    await waitFor(() => expect(screen.getAllByTestId('ledger-row')).toHaveLength(2))
  })

  it('thousands-separates miss tokens and flags evicted rows', async () => {
    render(<LedgerDrillDown sessionId="s1" />)
    const evicted = await screen.findByTestId('ledger-row-evicted')
    expect(within(evicted).getByText('16,908')).toBeInTheDocument()
  })

  it('shows an empty state for a session with no ledger rows', async () => {
    vi.spyOn(api, 'fetchCacheLedger').mockResolvedValue([])
    render(<LedgerDrillDown sessionId="s1" />)
    expect(await screen.findByTestId('ledger-empty')).toBeInTheDocument()
  })

  it('renders nothing and does not fetch without a sessionId', () => {
    render(<LedgerDrillDown sessionId="" />)
    expect(api.fetchCacheLedger).not.toHaveBeenCalled()
    expect(screen.queryByTestId('ledger-table')).not.toBeInTheDocument()
  })
})
