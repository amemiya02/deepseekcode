import { describe, it, expect, vi, beforeEach } from 'vitest'
import { rewind, fork, branch, switchSession, summarize } from './checkpoint'

function mockFetch(json: unknown, ok = true, status = 200) {
  return vi.fn().mockResolvedValue({
    ok,
    status,
    json: async () => json,
  } as Response)
}

describe('checkpoint client', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('rewind posts session_id, keep_messages, scope', async () => {
    const f = mockFetch({ removed_messages: 2, restored_files: 0 })
    vi.stubGlobal('fetch', f)
    const res = await rewind('s1', 2, 'conversation')
    expect(res.removed_messages).toBe(2)
    const [url, init] = f.mock.calls[0]
    expect(url).toBe('/v1/rewind')
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      session_id: 's1', keep_messages: 2, scope: 'conversation',
    })
  })

  it('branch returns the child session id', async () => {
    vi.stubGlobal('fetch', mockFetch({ session_id: 'child', parent_id: 's1', branch_point: 2 }))
    const res = await branch('s1', 2)
    expect(res.session_id).toBe('child')
    expect(res.branch_point).toBe(2)
  })

  it('fork posts only session_id', async () => {
    const f = mockFetch({ session_id: 'c2', parent_id: 's1', branch_point: 4 })
    vi.stubGlobal('fetch', f)
    await fork('s1')
    expect(JSON.parse((f.mock.calls[0][1] as RequestInit).body as string)).toEqual({ session_id: 's1' })
  })

  it('switchSession returns the replayed transcript', async () => {
    vi.stubGlobal('fetch', mockFetch({ session_id: 's1', messages: [{ role: 'user', text: 'hi' }] }))
    const res = await switchSession('s1')
    expect(res.messages).toHaveLength(1)
    expect(res.messages[0].role).toBe('user')
  })

  it('summarize posts mode + index', async () => {
    const f = mockFetch({ summary_idx: 0 })
    vi.stubGlobal('fetch', f)
    const res = await summarize('s1', 'upto', 3, 'earlier')
    expect(res.summary_idx).toBe(0)
    expect(JSON.parse((f.mock.calls[0][1] as RequestInit).body as string)).toEqual({
      session_id: 's1', mode: 'upto', index: 3, summary: 'earlier',
    })
  })

  it('throws on non-ok response', async () => {
    vi.stubGlobal('fetch', mockFetch({}, false, 500))
    await expect(rewind('s1', 0, 'both')).rejects.toThrow()
  })
})
