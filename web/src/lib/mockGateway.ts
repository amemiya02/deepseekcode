// Browser-dev mock gateway. Replays a canned agent turn through the real SSE
// event contract (Contract 2, snake_case) so the SPA builds and iterates with
// no Go rebuild, and so UI states (loading/streaming/tool/done) have a
// deterministic fixture.
//
// Activate by setting localStorage 'dsc.mock' = '1' (or VITE_MOCK=1). When
// active, installMockGateway() patches window.EventSource + window.fetch so the
// rest of the app is unchanged.

export interface MockFrame {
  name: string
  data: string
  delayMs: number
}

// cannedTurn: a representative turn exercising text, thinking, a read-only tool,
// and the hero cache/cost surfaces, terminating with turn_done. All payload keys
// are snake_case per Contract 2.
export const cannedTurn: MockFrame[] = [
  { name: 'thinking_delta', data: JSON.stringify({ text: 'Planning the change…' }), delayMs: 50 },
  { name: 'message_delta', data: JSON.stringify({ text: 'Let me read the file. ' }), delayMs: 60 },
  { name: 'tool_start', data: JSON.stringify({ id: 't1', name: 'read_file', args: { path: 'web/src/App.tsx' }, read_only: true }), delayMs: 80 },
  { name: 'tool_end', data: JSON.stringify({ id: 't1', result: 'read 95 lines', is_error: false }), delayMs: 120 },
  { name: 'message_delta', data: JSON.stringify({ text: 'The component renders three zones.' }), delayMs: 60 },
  { name: 'cache_update', data: JSON.stringify({ turn_pct: 0.94, avg_pct: 0.91, prefixes: 1, eviction: false }), delayMs: 40 },
  { name: 'cost_update', data: JSON.stringify({ turn_cny: 0.0008, session_cny: 0.012, output_tokens: 530 }), delayMs: 40 },
  { name: 'turn_done', data: JSON.stringify({ stop_reason: 'end_turn' }), delayMs: 30 },
]

type Listener = (e: MessageEvent) => void

// MockEventSource mimics the slice of the EventSource API the SPA uses:
// addEventListener(name, cb) + close(). start() drives the replay on timers.
export class MockEventSource {
  url: string
  onerror: ((e: Event) => void) | null = null
  private listeners = new Map<string, Listener[]>()
  private timers: ReturnType<typeof setTimeout>[] = []

  constructor(url: string) {
    this.url = url
  }

  addEventListener(name: string, cb: Listener): void {
    const arr = this.listeners.get(name) ?? []
    arr.push(cb)
    this.listeners.set(name, arr)
  }

  start(frames: MockFrame[] = cannedTurn): void {
    let acc = 0
    for (const f of frames) {
      acc += f.delayMs
      this.timers.push(
        setTimeout(() => {
          const e = new MessageEvent(f.name, { data: f.data })
          for (const cb of this.listeners.get(f.name) ?? []) cb(e)
        }, acc),
      )
    }
  }

  close(): void {
    for (const t of this.timers) clearTimeout(t)
    this.timers = []
  }
}

// isMockEnabled reports whether the browser-dev mock should take over.
export function isMockEnabled(): boolean {
  try {
    return localStorage.getItem('dsc.mock') === '1' || import.meta.env?.VITE_MOCK === '1'
  } catch {
    return false
  }
}

// installMockGateway patches window.EventSource so a fresh stream auto-replays
// the canned turn, and stubs window.fetch for the system endpoints with static
// fixtures. No-op when the mock is disabled.
export function installMockGateway(): void {
  if (!isMockEnabled()) return

  const RealES = window.EventSource
  // @ts-expect-error - intentional partial EventSource shim for dev only.
  window.EventSource = class extends MockEventSource {
    constructor(url: string) {
      super(url)
      queueMicrotask(() => this.start(cannedTurn))
    }
  }
  ;(window as unknown as { __realEventSource?: typeof EventSource }).__realEventSource = RealES

  const realFetch = window.fetch.bind(window)
  const fixtures: Record<string, unknown> = {
    '/v1/cache': { total_usage_turns: 3, cache_hit_tokens: 38000, cache_miss_tokens: 4200, output_tokens: 530, cost_cny: 0.012, full_body_evictions: 0, max_miss_tokens: 4200, cache_hit_rate: 0.94 },
    '/v1/config': { theme: 'graphite', accent: 'indigo', density: 'comfortable', language: 'en', transcriptVerbosity: 'normal', model: 'deepseek-v4', reasoningEffort: 'high', baseUrl: 'https://api.deepseek.com', autoRoute: true, escalationModel: 'deepseek-v4-pro', duetEnabled: false, sandboxEnabled: false, sandboxNetwork: false, autoReasoning: false, autoClarify: false },
    '/v1/doctor': { allOk: true, checks: [{ name: 'api key', ok: true, detail: 'present' }, { name: 'base url', ok: true, detail: 'reachable' }] },
    '/v1/update': { current: 'v1.0.0', latest: 'v1.0.0', updateAvailable: false, url: '' },
    '/v1/onboarding': { needsOnboarding: false, baseUrl: 'https://api.deepseek.com', model: 'deepseek-v4' },
    '/v1/mcp': { items: [] },
    '/v1/skills': { items: [] },
    '/v1/hooks': { items: [] },
    '/v1/memory': { items: [] },
  }
  window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = typeof input === 'string' ? input : input.toString()
    const path = url.split('?')[0]
    if (path in fixtures) {
      return new Response(JSON.stringify(fixtures[path]), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    if (path === '/v1/prompt') {
      return new Response(JSON.stringify({ request_id: 'mock', session_id: 'mock' }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    return realFetch(input, init)
  }
}
