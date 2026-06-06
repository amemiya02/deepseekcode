import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { Cockpit } from './Cockpit'
import * as api from '../../lib/api'
import { useCockpitStore } from '../../lib/cockpitStore'

describe('Cockpit', () => {
  beforeEach(() => {
    useCockpitStore.getState().reset()
    vi.spyOn(api.GatewayClient.prototype, 'openEventStream').mockImplementation(() => {})
    vi.spyOn(api.GatewayClient.prototype, 'close').mockImplementation(() => {})
    vi.spyOn(api, 'fetchBalance').mockResolvedValue({ provider: 'deepseek', currency: 'CNY', amount: 5 })
    vi.spyOn(api, 'fetchCacheLedger').mockResolvedValue([])
  })
  afterEach(() => vi.restoreAllMocks())

  it('renders all four cockpit cards', async () => {
    render(<Cockpit sessionId="s1" />)
    await waitFor(() => {
      expect(screen.getByTestId('cache-card')).toBeInTheDocument()
      expect(screen.getByTestId('cost-card')).toBeInTheDocument()
      expect(screen.getByTestId('routing-card')).toBeInTheDocument()
      expect(screen.getByTestId('ledger-card')).toBeInTheDocument()
    })
  })

  it('opens the SSE stream for the session', () => {
    render(<Cockpit sessionId="s1" />)
    expect(api.GatewayClient.prototype.openEventStream).toHaveBeenCalled()
  })
})
