import { create } from 'zustand'
import {
  GatewayClient, fetchBalance,
  type LiveCache, type LiveCost, type RoutingHop, type RetryStatus,
} from './api'

const ZERO_CACHE: LiveCache = { turn_pct: 0, avg_pct: 0, prefixes: 1, eviction: false }
const ZERO_COST: LiveCost = { turn_cny: 0, session_cny: 0, output_tokens: 0 }
const ZERO_RETRY: RetryStatus = { attempt: 0, max: 0 }

// One GatewayClient instance owns the single SSE subscription shared by the
// cockpit panel and the always-mounted hero status bar.
const client = new GatewayClient()

// Refcount subscribers to the shared cockpit stream: open on the first subscriber,
// close only when the last one disconnects. Without this, unmounting one consumer
// (e.g. switching the workspace tab off the Cockpit) would tear down the SSE
// connection the still-mounted hero StatusBar depends on.
let subscribers = 0
let streamSessionId = ''

interface CockpitState {
  liveCache: LiveCache
  liveCost: LiveCost
  routing: RoutingHop[]
  jobs: number
  retry: RetryStatus
  balance: number | null
  currency: string
  connect: (sessionId: string) => void
  disconnect: () => void
  refreshBalance: () => Promise<void>
  reset: () => void
}

const initial = {
  liveCache: { ...ZERO_CACHE },
  liveCost: { ...ZERO_COST },
  routing: [] as RoutingHop[],
  jobs: 0,
  retry: { ...ZERO_RETRY },
  balance: null as number | null,
  currency: 'CNY',
}

export const useCockpitStore = create<CockpitState>((set, get) => ({
  ...initial,

  connect(sessionId) {
    if (sessionId === streamSessionId) {
      // Same session already streaming: add a subscriber without reopening
      // (reopening would reset the accumulated live view).
      subscribers += 1
      return
    }
    // New (or first) session: tear down any prior stream, reset the live view
    // (keeping the last balance), and open a fresh subscription.
    if (streamSessionId) client.close()
    streamSessionId = sessionId
    subscribers = 1
    set({
      liveCache: { ...ZERO_CACHE },
      liveCost: { ...ZERO_COST },
      routing: [],
      jobs: 0,
      retry: { ...ZERO_RETRY },
    })
    client.openEventStream(sessionId, {
      onCacheUpdate: (c) => set({ liveCache: c }),
      onCostUpdate: (c) => set({ liveCost: c }),
      onRouting: (r) => set({ routing: [...get().routing, r] }),
      onJobUpdate: (j) => set({ jobs: j.running }),
      onRetry: (r) => set({ retry: { attempt: r.attempt, max: r.max } }),
      onTurnDone: (_e) => set({ jobs: 0, retry: { ...ZERO_RETRY } }),
    })
  },

  disconnect() {
    if (subscribers > 0) subscribers -= 1
    if (subscribers === 0 && streamSessionId) {
      client.close()
      streamSessionId = ''
    }
  },

  async refreshBalance() {
    const b = await fetchBalance()
    set({ balance: b.amount, currency: b.currency })
  },

  reset() {
    if (streamSessionId) client.close()
    subscribers = 0
    streamSessionId = ''
    set({ ...initial, balance: null, currency: 'CNY' })
  },
}))
