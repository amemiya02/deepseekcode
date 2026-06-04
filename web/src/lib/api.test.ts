import { describe, it, expect, vi, beforeEach } from 'vitest'
import { GatewayClient, submitPrompt, setModel, setEffort, setOutputStyle, cancelTurn, fetchModels } from './api'

describe('submitPrompt', () => {
  it('posts to /v1/prompt and returns session_id', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ session_id: 'sess-1' }),
    }) as unknown as typeof fetch
    const id = await submitPrompt('hello')
    expect(id).toBe('sess-1')
    expect((global.fetch as ReturnType<typeof vi.fn>)).toHaveBeenCalledWith(
      '/v1/prompt',
      expect.objectContaining({ method: 'POST' }),
    )
  })
})

// A fake EventSource that fires every event type whose name is a key of `fire`.
function fakeES(fire: Record<string, unknown>) {
  return {
    addEventListener: vi.fn((type: string, cb: (e: MessageEvent) => void) => {
      if (type in fire) cb(new MessageEvent(type, { data: JSON.stringify(fire[type]) }))
    }),
    close: vi.fn(),
    onerror: null as null | ((e: Event) => void),
  }
}

describe('GatewayClient — Contract 2 SSE schema', () => {
  it('parses message_delta and thinking_delta JSON into text', () => {
    const msgs: string[] = []
    const think: string[] = []
    // @ts-expect-error test global
    global.EventSource = vi.fn(() => fakeES({ message_delta: { text: 'hi' }, thinking_delta: { text: 'plan' } }))
    new GatewayClient().openEventStream('s', {
      onMessageDelta: (t) => msgs.push(t),
      onThinkingDelta: (t) => think.push(t),
    })
    expect(msgs).toEqual(['hi'])
    expect(think).toEqual(['plan'])
  })

  it('parses tool_start (read_only), tool_delta (delta) and tool_end (is_error)', () => {
    const starts: Array<{ name: string; readOnly: boolean }> = []
    const deltas: string[] = []
    const ends: Array<{ id: string; isError: boolean }> = []
    // @ts-expect-error test global
    global.EventSource = vi.fn(() => fakeES({
      tool_start: { id: 't1', name: 'read_file', args: { path: 'a.go' }, read_only: true },
      tool_delta: { id: 't1', delta: 'chunk' },
      tool_end: { id: 't1', result: 'ok', is_error: false },
    }))
    new GatewayClient().openEventStream('s', {
      onToolStart: (e) => starts.push({ name: e.name, readOnly: e.read_only }),
      onToolDelta: (e) => deltas.push(e.delta),
      onToolEnd: (e) => ends.push({ id: e.id, isError: e.is_error }),
    })
    expect(starts).toEqual([{ name: 'read_file', readOnly: true }])
    expect(deltas).toEqual(['chunk'])
    expect(ends).toEqual([{ id: 't1', isError: false }])
  })

  it('parses routing, plan_update, cache_update, cost_update, retry, turn_done', () => {
    const log: string[] = []
    // @ts-expect-error test global
    global.EventSource = vi.fn(() => fakeES({
      routing: { from: 'flash', to: 'pro', reason: 'hard' },
      plan_update: { items: [{ text: 'step', status: 'in_progress' }] },
      // C-2: turn_pct/avg_pct are 0–1 RATIOS (NOT already-percent). The UI ×100s them.
      cache_update: { turn_pct: 0.9, avg_pct: 0.88, prefixes: 1, eviction: false },
      cost_update: { turn_cny: 0.1, session_cny: 1.2, output_tokens: 50 },
      retry: { attempt: 1, max: 3 },
      turn_done: { stop_reason: 'end_turn' },
    }))
    new GatewayClient().openEventStream('s', {
      onRouting: (e) => log.push(`${e.from}->${e.to}:${e.reason}`),
      onPlanUpdate: (e) => log.push(`plan:${e.items[0].status}`),
      // Render the ratio as a percent here exactly as the cockpit will (×100, rounded).
      onCacheUpdate: (e) => log.push(`cache:${Math.round(e.turn_pct * 100)}/${e.prefixes}`),
      onCostUpdate: (e) => log.push(`cost:${e.turn_cny}/${e.output_tokens}`),
      onRetry: (e) => log.push(`retry:${e.attempt}/${e.max}`),
      onTurnDone: (e) => log.push(`done:${e.stop_reason}`),
    })
    expect(log).toEqual([
      'flash->pro:hard', 'plan:in_progress', 'cache:90/1', 'cost:0.1/50', 'retry:1/3', 'done:end_turn',
    ])
  })
})

describe('REST control helpers', () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) }) as unknown as typeof fetch
  })
  it('submitPrompt returns session_id from the response', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ session_id: 'sess-9' }) }) as unknown as typeof fetch
    expect(await submitPrompt('go', 'sess-9')).toBe('sess-9')
  })
  it('cancelTurn posts session_id to /v1/cancel', async () => {
    await cancelTurn('s9')
    expect((global.fetch as ReturnType<typeof vi.fn>)).toHaveBeenCalledWith('/v1/cancel', expect.objectContaining({ method: 'POST' }))
  })
  it('setModel posts model to /v1/model', async () => {
    await setModel('s', 'deepseek-pro')
    const body = ((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1] as { body: string }).body
    expect(body).toContain('deepseek-pro')
  })
  it('setEffort posts effort to /v1/effort', async () => {
    await setEffort('s', 'high')
    expect((global.fetch as ReturnType<typeof vi.fn>)).toHaveBeenCalledWith('/v1/effort', expect.objectContaining({ method: 'POST' }))
  })
  it('setOutputStyle posts style to /v1/output-style', async () => {
    await setOutputStyle('s', 'concise')
    expect((global.fetch as ReturnType<typeof vi.fn>)).toHaveBeenCalledWith('/v1/output-style', expect.objectContaining({ method: 'POST' }))
  })
  it('fetchModels returns the parsed list', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => [{ id: 'a', label: 'A' }] }) as unknown as typeof fetch
    expect(await fetchModels()).toEqual([{ id: 'a', label: 'A' }])
  })
})
