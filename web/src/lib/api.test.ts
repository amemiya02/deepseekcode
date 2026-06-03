import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { submitPrompt, fetchCacheReport, GatewayClient } from './api'

// ---- submitPrompt ----
describe('submitPrompt', () => {
  it('posts to /v1/prompt and returns request_id', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ request_id: 'req-abc' }),
    })
    global.fetch = fetchMock as unknown as typeof fetch

    const id = await submitPrompt('fix the bug', 'sess-1')
    expect(id).toBe('req-abc')
    expect(fetchMock).toHaveBeenCalledWith('/v1/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: 'fix the bug', session_id: 'sess-1' }),
    })
  })

  it('throws when response is not ok', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch
    await expect(submitPrompt('x')).rejects.toThrow('gateway error 500')
  })
})

// ---- fetchCacheReport ----
describe('fetchCacheReport', () => {
  it('returns parsed Report from /v1/cache including output_tokens and cost_cny', async () => {
    const report = {
      total_usage_turns: 3,
      cache_hit_tokens: 9000,
      cache_miss_tokens: 1200,
      output_tokens: 450,
      cost_cny: 0.012,
      full_body_evictions: 1,
      max_miss_tokens: 8754,
      cache_hit_rate: 0.88,
    }
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => report,
    }) as unknown as typeof fetch

    const got = await fetchCacheReport()
    expect(got.full_body_evictions).toBe(1)
    expect(got.cache_hit_rate).toBeCloseTo(0.88)
    expect(got.output_tokens).toBe(450)
    expect(got.cost_cny).toBeCloseTo(0.012)
  })
})

// ---- GatewayClient.openEventStream ----
describe('GatewayClient.openEventStream', () => {
  it('constructs EventSource with correct URL and calls onDelta for delta events', () => {
    const events: string[] = []
    const fakeES = {
      addEventListener: vi.fn((type: string, cb: (e: MessageEvent) => void) => {
        if (type === 'delta') cb(new MessageEvent('delta', { data: 'hello' }))
      }),
      close: vi.fn(),
    }
    const ESMock = vi.fn(() => fakeES)
    // @ts-expect-error replacing global EventSource for test
    global.EventSource = ESMock

    const client = new GatewayClient()
    client.openEventStream('sess-1', {
      onDelta: (text) => events.push(text),
    })
    expect(ESMock).toHaveBeenCalledWith('/v1/events?session_id=sess-1')
    expect(events).toEqual(['hello'])
  })

  it('calls onTool with the tool name from SSE data', () => {
    const tools: string[] = []
    const fakeES = {
      addEventListener: vi.fn((type: string, cb: (e: MessageEvent) => void) => {
        if (type === 'tool') cb(new MessageEvent('tool', { data: 'bash' }))
      }),
      close: vi.fn(),
    }
    // @ts-expect-error replacing global EventSource for test
    global.EventSource = vi.fn(() => fakeES)

    const client = new GatewayClient()
    client.openEventStream('sess-2', {
      onTool: (name) => tools.push(name),
    })
    expect(tools).toEqual(['bash'])
  })

  it('calls onStep with the summary from SSE data', () => {
    const steps: string[] = []
    const fakeES = {
      addEventListener: vi.fn((type: string, cb: (e: MessageEvent) => void) => {
        if (type === 'step') cb(new MessageEvent('step', { data: 'planning done' }))
      }),
      close: vi.fn(),
    }
    // @ts-expect-error replacing global EventSource for test
    global.EventSource = vi.fn(() => fakeES)

    const client = new GatewayClient()
    client.openEventStream('sess-3', {
      onStep: (summary) => steps.push(summary),
    })
    expect(steps).toEqual(['planning done'])
  })

  it('calls onDone when done event fires', () => {
    let doneCalled = false
    const fakeES = {
      addEventListener: vi.fn((type: string, cb: () => void) => {
        if (type === 'done') cb()
      }),
      close: vi.fn(),
    }
    // @ts-expect-error replacing global EventSource for test
    global.EventSource = vi.fn(() => fakeES)

    const client = new GatewayClient()
    client.openEventStream('sess-4', {
      onDone: () => { doneCalled = true },
    })
    expect(doneCalled).toBe(true)
  })

  it('wires onError to es.onerror', () => {
    const errors: Event[] = []
    const fakeES: Record<string, unknown> = {
      addEventListener: vi.fn(),
      close: vi.fn(),
      onerror: null,
    }
    // @ts-expect-error replacing global EventSource for test
    global.EventSource = vi.fn(() => fakeES)

    const client = new GatewayClient()
    const handler = (e: Event) => errors.push(e)
    client.openEventStream('sess-5', {
      onError: handler,
    })
    expect(fakeES.onerror).toBe(handler)
  })
})
