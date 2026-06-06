import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import { StatusBarLive } from './StatusBarLive'
import * as api from '../lib/api'
import type { StreamHandlers } from '../lib/api'
import { useCockpitStore } from '../lib/cockpitStore'

let captured: StreamHandlers = {}

describe('StatusBarLive', () => {
  beforeEach(() => {
    captured = {}
    useCockpitStore.getState().reset()
    vi.spyOn(api.GatewayClient.prototype, 'openEventStream').mockImplementation(
      (_sid: string, h: StreamHandlers) => { captured = h },
    )
    vi.spyOn(api.GatewayClient.prototype, 'close').mockImplementation(() => {})
    vi.spyOn(api, 'fetchBalance').mockResolvedValue({ provider: 'deepseek', currency: 'CNY', amount: 7 })
  })
  afterEach(() => vi.restoreAllMocks())

  it('renders the model/effort passed as props', async () => {
    render(<StatusBarLive sessionId="s1" model="deepseek-chat" effort="high" status="idle" />)
    await waitFor(() => expect(screen.getByText('deepseek-chat')).toBeInTheDocument())
    expect(screen.getByText('high')).toBeInTheDocument()
  })

  it('reflects a live cache_update in the turn cache cell', async () => {
    render(<StatusBarLive sessionId="s1" model="m" effort="e" status="streaming" />)
    await waitFor(() => expect(captured.onCacheUpdate).toBeTruthy())
    act(() => {
      captured.onCacheUpdate!({ turn_pct: 0.95, avg_pct: 0.9, prefixes: 1, eviction: true })
    })
    await waitFor(() => expect(screen.getByTestId('turn-cache')).toHaveTextContent('95%'))
    expect(screen.getByTestId('eviction-warning')).toBeInTheDocument()
  })
})
