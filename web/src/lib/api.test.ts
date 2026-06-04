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
      onTurnDone: (e) => log.push(`done:${e?.stop_reason}`),
    })
    expect(log).toEqual([
      'flash->pro:hard', 'plan:in_progress', 'cache:90/1', 'cost:0.1/50', 'retry:1/3', 'done:end_turn',
    ])
  })
})

import {
  listSessions, createSession, getSession, renameSession, deleteSession,
  getTimeline, fetchCacheLedger, fetchBalance,
} from './api'

describe('listSessions', () => {
  it('GETs /v1/sessions and returns the sessions array', async () => {
    const sessions = [{ id: 's1', title: 'Fix bug', turns: 3, updated_at: 10, created_at: 1 }]
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ sessions }) })
    global.fetch = fetchMock as unknown as typeof fetch
    const got = await listSessions()
    expect(fetchMock).toHaveBeenCalledWith('/v1/sessions')
    expect(got).toEqual(sessions)
  })
})

describe('createSession', () => {
  it('POSTs /v1/sessions and returns the new session', async () => {
    const created = { id: 'new', title: 'Untitled', turns: 0, updated_at: 5, created_at: 5 }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => created })
    global.fetch = fetchMock as unknown as typeof fetch
    const got = await createSession('/repo')
    expect(fetchMock).toHaveBeenCalledWith('/v1/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ working_dir: '/repo' }),
    })
    expect(got.id).toBe('new')
  })
})

describe('getSession', () => {
  it('GETs /v1/sessions/{id} and returns its detail', async () => {
    const detail = { id: 's1', title: 'X', turns: 2, updated_at: 9, created_at: 1, messages: [] }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => detail })
    global.fetch = fetchMock as unknown as typeof fetch
    const got = await getSession('s1')
    expect(fetchMock).toHaveBeenCalledWith('/v1/sessions/s1')
    expect(got.id).toBe('s1')
  })
})

describe('renameSession', () => {
  it('PATCHes /v1/sessions/{id} with the new title', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) })
    global.fetch = fetchMock as unknown as typeof fetch
    await renameSession('s1', 'New name')
    expect(fetchMock).toHaveBeenCalledWith('/v1/sessions/s1', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: 'New name' }),
    })
  })
})

describe('deleteSession', () => {
  it('DELETEs /v1/sessions/{id}', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) })
    global.fetch = fetchMock as unknown as typeof fetch
    await deleteSession('s1')
    expect(fetchMock).toHaveBeenCalledWith('/v1/sessions/s1', { method: 'DELETE' })
  })
})

describe('getTimeline', () => {
  it('GETs /v1/sessions/{id}/timeline and returns entries', async () => {
    const entries = [{ id: 'e1', turns: 4, compacted: false }]
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ entries }) })
    global.fetch = fetchMock as unknown as typeof fetch
    const got = await getTimeline('s1')
    expect(fetchMock).toHaveBeenCalledWith('/v1/sessions/s1/timeline')
    expect(got).toEqual(entries)
  })
})

describe('fetchCacheLedger', () => {
  it('GETs /v1/cache/ledger with a session query and returns rows', async () => {
    const rows = [{ turn: 1, hit_tokens: 100, miss_tokens: 5, evicted: false }]
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ rows }) })
    global.fetch = fetchMock as unknown as typeof fetch
    const got = await fetchCacheLedger('s1')
    expect(fetchMock).toHaveBeenCalledWith('/v1/cache/ledger?session=s1')
    expect(got).toEqual(rows)
  })
  it('includes the turn query when given', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ rows: [] }) })
    global.fetch = fetchMock as unknown as typeof fetch
    await fetchCacheLedger('s1', 2)
    expect(fetchMock).toHaveBeenCalledWith('/v1/cache/ledger?session=s1&turn=2')
  })
})

describe('fetchBalance', () => {
  it('GETs /v1/balance and returns the balance', async () => {
    const balance = { provider: 'deepseek', currency: 'CNY', amount: 12.5 }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => balance })
    global.fetch = fetchMock as unknown as typeof fetch
    const got = await fetchBalance()
    expect(fetchMock).toHaveBeenCalledWith('/v1/balance')
    expect(got.amount).toBe(12.5)
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

describe('GatewayClient cockpit SSE events', () => {
  it('parses cache_update JSON and calls onCacheUpdate', () => {
    const updates: unknown[] = []
    const fakeES = {
      addEventListener: vi.fn((type: string, cb: (e: MessageEvent) => void) => {
        if (type === 'cache_update') {
          cb(new MessageEvent('cache_update', {
            data: JSON.stringify({ turn_pct: 0.9, avg_pct: 0.8, prefixes: 1, eviction: false }),
          }))
        }
      }),
      close: vi.fn(),
    }
    // @ts-expect-error replacing global EventSource for the test
    global.EventSource = vi.fn(() => fakeES)
    new GatewayClient().openEventStream('s', { onCacheUpdate: (c) => updates.push(c) })
    expect(updates).toEqual([{ turn_pct: 0.9, avg_pct: 0.8, prefixes: 1, eviction: false }])
  })

  it('parses cost_update, routing, job_update, retry and turn_done', () => {
    const seen: Record<string, unknown> = {}
    const fakeES = {
      addEventListener: vi.fn((type: string, cb: (e: MessageEvent) => void) => {
        const payloads: Record<string, string> = {
          cost_update: JSON.stringify({ turn_cny: 0.01, session_cny: 0.2, output_tokens: 50 }),
          routing: JSON.stringify({ from: 'flash', to: 'pro', reason: 'hard task' }),
          job_update: JSON.stringify({ running: 2 }),
          retry: JSON.stringify({ attempt: 1, max: 3 }),
          turn_done: JSON.stringify({ stop_reason: 'end_turn' }),
        }
        if (type in payloads) cb(new MessageEvent(type, { data: payloads[type] }))
      }),
      close: vi.fn(),
    }
    // @ts-expect-error replacing global EventSource for the test
    global.EventSource = vi.fn(() => fakeES)
    new GatewayClient().openEventStream('s', {
      onCostUpdate: (c) => { seen.cost = c },
      onRouting: (r) => { seen.routing = r },
      onJobUpdate: (j) => { seen.job = j },
      onRetry: (r) => { seen.retry = r },
      onTurnDone: () => { seen.turnDone = true },
    })
    expect(seen.cost).toEqual({ turn_cny: 0.01, session_cny: 0.2, output_tokens: 50 })
    expect(seen.routing).toEqual({ from: 'flash', to: 'pro', reason: 'hard task' })
    expect(seen.job).toEqual({ running: 2 })
    expect(seen.retry).toEqual({ attempt: 1, max: 3 })
    expect(seen.turnDone).toBe(true)
  })
})
