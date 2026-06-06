import { describe, it, expect, vi } from 'vitest'
import { MockEventSource, cannedTurn } from './mockGateway'

describe('MockEventSource', () => {
  it('replays the canned turn in contract order ending with turn_done', async () => {
    vi.useFakeTimers()
    const seen: string[] = []
    const es = new MockEventSource('/v1/events?session_id=mock')
    for (const name of ['message_delta', 'thinking_delta', 'tool_start', 'tool_end', 'cache_update', 'cost_update', 'turn_done']) {
      es.addEventListener(name, () => seen.push(name))
    }
    es.start(cannedTurn)
    await vi.runAllTimersAsync()
    vi.useRealTimers()
    expect(seen).toContain('message_delta')
    expect(seen).toContain('tool_start')
    expect(seen).toContain('tool_end')
    expect(seen[seen.length - 1]).toBe('turn_done')
  })

  it('delivers snake_case payloads parseable as the Contract-2 shape', async () => {
    vi.useFakeTimers()
    let toolName = ''
    let toolReadOnly: boolean | undefined
    let cacheTurnPct: number | undefined
    let costTurnCny: number | undefined
    const es = new MockEventSource('/v1/events?session_id=mock')
    es.addEventListener('tool_start', (e) => {
      const d = JSON.parse((e as MessageEvent).data)
      toolName = d.name
      toolReadOnly = d.read_only
    })
    es.addEventListener('cache_update', (e) => { cacheTurnPct = JSON.parse((e as MessageEvent).data).turn_pct })
    es.addEventListener('cost_update', (e) => { costTurnCny = JSON.parse((e as MessageEvent).data).turn_cny })
    es.start(cannedTurn)
    await vi.runAllTimersAsync()
    vi.useRealTimers()
    expect(toolName).toBe('read_file')
    expect(toolReadOnly).toBe(true)
    expect(cacheTurnPct).toBeGreaterThan(0)
    expect(costTurnCny).toBeGreaterThan(0)
  })
})
