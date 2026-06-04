import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchFiles, fetchFile, fetchChanged, addToChat } from './workspace'

function mockFetch(json: unknown, ok = true, status = 200) {
  return vi.fn().mockResolvedValue({ ok, status, json: async () => json } as Response)
}

describe('workspace client', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('fetchFiles passes the path query', async () => {
    const f = mockFetch({ entries: [{ name: 'a.go', path: 'a.go', is_dir: false }] })
    vi.stubGlobal('fetch', f)
    const res = await fetchFiles('pkg')
    expect(res.entries[0].name).toBe('a.go')
    expect(f.mock.calls[0][0]).toContain('/v1/files?path=pkg')
  })

  it('fetchFile returns content + flags', async () => {
    vi.stubGlobal('fetch', mockFetch({ path: 'a.go', content: 'x', binary: false, truncated: false }))
    const res = await fetchFile('a.go')
    expect(res.content).toBe('x')
    expect(res.binary).toBe(false)
  })

  it('fetchChanged returns entries', async () => {
    vi.stubGlobal('fetch', mockFetch({ entries: [{ path: 'a.go', status: ' M', deleted: false }] }))
    const res = await fetchChanged()
    expect(res.entries[0].status).toBe(' M')
  })

  it('addToChat posts the ref payload', async () => {
    const f = mockFetch({ label: 'a.go', content: '@a.go' })
    vi.stubGlobal('fetch', f)
    const res = await addToChat({ path: 'a.go' })
    expect(res.label).toBe('a.go')
    expect(JSON.parse((f.mock.calls[0][1] as RequestInit).body as string)).toEqual({ path: 'a.go' })
  })

  it('throws on non-ok', async () => {
    vi.stubGlobal('fetch', mockFetch({}, false, 400))
    await expect(fetchFile('../x')).rejects.toThrow()
  })
})
