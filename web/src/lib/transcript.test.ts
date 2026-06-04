import { describe, it, expect } from 'vitest'
import { applyEvent, type TranscriptItem, type TranscriptEvent } from './transcript'

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
