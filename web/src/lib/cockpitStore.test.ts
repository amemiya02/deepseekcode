import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import * as api from './api'
import { useCockpitStore } from './cockpitStore'
import type { StreamHandlers } from './api'

// Capture the handlers passed to openEventStream so the test can drive events.
let captured: StreamHandlers = {}

describe('cockpitStore', () => {
  beforeEach(() => {
    captured = {}
    useCockpitStore.getState().reset()
    vi.spyOn(api.GatewayClient.prototype, 'openEventStream').mockImplementation(
      (_sid: string, h: StreamHandlers) => { captured = h },
    )
    vi.spyOn(api.GatewayClient.prototype, 'close').mockImplementation(() => {})
    vi.spyOn(api, 'fetchBalance').mockResolvedValue({ provider: 'deepseek', currency: 'CNY', amount: 9.9 })
  })
  afterEach(() => vi.restoreAllMocks())

  it('connect() subscribes and cache_update mutates liveCache', () => {
    useCockpitStore.getState().connect('s1')
    captured.onCacheUpdate!({ turn_pct: 0.9, avg_pct: 0.8, prefixes: 1, eviction: false })
    expect(useCockpitStore.getState().liveCache.turn_pct).toBe(0.9)
    expect(useCockpitStore.getState().liveCache.prefixes).toBe(1)
  })

  it('cost_update mutates liveCost; routing appends a hop', () => {
    useCockpitStore.getState().connect('s1')
    captured.onCostUpdate!({ turn_cny: 0.01, session_cny: 0.2, output_tokens: 50 })
    captured.onRouting!({ from: 'flash', to: 'pro', reason: 'hard' })
    expect(useCockpitStore.getState().liveCost.session_cny).toBe(0.2)
    expect(useCockpitStore.getState().routing).toHaveLength(1)
    expect(useCockpitStore.getState().routing[0].to).toBe('pro')
  })

  it('job_update and retry set indicators; turn_done clears them', () => {
    useCockpitStore.getState().connect('s1')
    captured.onJobUpdate!({ running: 2 })
    captured.onRetry!({ attempt: 1, max: 3 })
    expect(useCockpitStore.getState().jobs).toBe(2)
    expect(useCockpitStore.getState().retry).toEqual({ attempt: 1, max: 3 })
    captured.onTurnDone!({ stop_reason: 'end_turn' })
    expect(useCockpitStore.getState().jobs).toBe(0)
    expect(useCockpitStore.getState().retry).toEqual({ attempt: 0, max: 0 })
  })

  it('refreshBalance() pulls the wallet balance', async () => {
    await useCockpitStore.getState().refreshBalance()
    expect(useCockpitStore.getState().balance).toBe(9.9)
    expect(useCockpitStore.getState().currency).toBe('CNY')
  })

  it('connect() to a new session resets routing hops', () => {
    useCockpitStore.getState().connect('s1')
    captured.onRouting!({ from: 'a', to: 'b', reason: 'x' })
    expect(useCockpitStore.getState().routing).toHaveLength(1)
    useCockpitStore.getState().connect('s2')
    expect(useCockpitStore.getState().routing).toHaveLength(0)
  })
})
