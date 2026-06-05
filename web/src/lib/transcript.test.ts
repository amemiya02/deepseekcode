import { describe, it, expect } from 'vitest'
import { applyEvent, formatThinkingDuration, type TranscriptItem, type TranscriptEvent } from './transcript'

function run(events: TranscriptEvent[]): TranscriptItem[] {
  return events.reduce<TranscriptItem[]>((items, e) => applyEvent(items, e), [])
}

describe('applyEvent', () => {
  it('coalesces message_delta into one streaming assistant item', () => {
    const items = run([
      { kind: 'message_delta', text: 'Hel' },
      { kind: 'message_delta', text: 'lo' },
    ])
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ type: 'assistant', text: 'Hello', streaming: true })
  })

  it('coalesces thinking_delta into one thinking item separate from assistant', () => {
    const items = run([
      { kind: 'thinking_delta', text: 'plan ' },
      { kind: 'thinking_delta', text: 'it' },
      { kind: 'message_delta', text: 'done' },
    ])
    expect(items).toHaveLength(2)
    expect(items[0]).toMatchObject({ type: 'thinking', text: 'plan it' })
    expect(items[1]).toMatchObject({ type: 'assistant', text: 'done' })
  })

  it('tool_start creates a running tool item; tool_end completes it by id', () => {
    const items = run([
      { kind: 'tool_start', id: 't1', name: 'read_file', args: { path: 'a' }, read_only: true },
      { kind: 'tool_end', id: 't1', result: 'ok', is_error: false },
    ])
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ type: 'tool', id: 't1', name: 'read_file', status: 'ok', readOnly: true, result: 'ok' })
  })

  it('tool_delta appends streaming output by id', () => {
    const items = run([
      { kind: 'tool_start', id: 't1', name: 'bash', args: {}, read_only: false },
      { kind: 'tool_delta', id: 't1', delta: 'line1\n' },
      { kind: 'tool_delta', id: 't1', delta: 'line2' },
    ])
    expect(items[0]).toMatchObject({ type: 'tool', result: 'line1\nline2', status: 'running' })
  })

  it('tool_end with is_error sets status error', () => {
    const items = run([
      { kind: 'tool_start', id: 't2', name: 'bash', args: {}, read_only: false },
      { kind: 'tool_end', id: 't2', result: 'boom', is_error: true },
    ])
    expect(items[0]).toMatchObject({ type: 'tool', status: 'error', result: 'boom' })
  })

  it('routing appends a routing item', () => {
    const items = run([{ kind: 'routing', from: 'flash', to: 'pro', reason: 'hard' }])
    expect(items[0]).toMatchObject({ type: 'routing', from: 'flash', to: 'pro', reason: 'hard' })
  })

  it('a duet event appends a DuetItem', () => {
    const out = applyEvent([], { kind: 'duet', decision: 'deny', reason: 'scope' })
    expect(out).toEqual([{ type: 'duet', decision: 'deny', reason: 'scope' }])
  })

  it('user appends a user item with pills', () => {
    const items = run([{ kind: 'user', text: 'fix it', pills: ['a.go'] }])
    expect(items[0]).toMatchObject({ type: 'user', text: 'fix it', pills: ['a.go'] })
  })

  it('turn_done clears streaming on the last assistant item', () => {
    const items = run([
      { kind: 'message_delta', text: 'x' },
      { kind: 'turn_done', stop_reason: 'end_turn' },
    ])
    expect(items[0]).toMatchObject({ type: 'assistant', streaming: false })
  })
})

describe('thinking timing', () => {
  const clock = () => { let t = 1000; return () => (t += 500) } // +0.5s each call

  it('stamps startedAt when a thinking item is created', () => {
    const now = clock()
    const out = applyEvent([], { kind: 'thinking_delta', text: 'hmm' }, now)
    const item = out[0]
    expect(item.type).toBe('thinking')
    if (item.type === 'thinking') expect(item.startedAt).toBe(1500)
  })

  it('does not restamp startedAt on subsequent thinking deltas', () => {
    const now = clock()
    let s = applyEvent([], { kind: 'thinking_delta', text: 'a' }, now)
    s = applyEvent(s, { kind: 'thinking_delta', text: 'b' }, now)
    const item = s[0]
    if (item.type === 'thinking') expect(item.startedAt).toBe(1500)
  })

  it('stamps endedAt when a non-thinking item follows', () => {
    const now = clock()
    let s = applyEvent([], { kind: 'thinking_delta', text: 'a' }, now) // started 1500
    s = applyEvent(s, { kind: 'message_delta', text: 'hi' }, now)       // ends 2000
    const item = s[0]
    if (item.type === 'thinking') expect(item.endedAt).toBe(2000)
  })

  it('stamps endedAt on turn_done', () => {
    const now = clock()
    let s = applyEvent([], { kind: 'thinking_delta', text: 'a' }, now)
    s = applyEvent(s, { kind: 'turn_done', stop_reason: 'end' }, now)
    const item = s[0]
    if (item.type === 'thinking') expect(typeof item.endedAt).toBe('number')
  })
})

describe('formatThinkingDuration', () => {
  it('formats whole seconds', () => expect(formatThinkingDuration(1000, 4200)).toBe('Thought for 3s'))
  it('formats sub-second as <1s', () => expect(formatThinkingDuration(1000, 1400)).toBe('Thought for <1s'))
  it('rounds to nearest second', () => expect(formatThinkingDuration(0, 2600)).toBe('Thought for 3s'))
})
