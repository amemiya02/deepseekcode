import { describe, it, expect, beforeEach, vi } from 'vitest'
import { fetchFiles, fetchFile, fetchChanged, addToChat, type FileEntry } from './workspace'

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('workspace API', () => {
  it('fetchFiles GETs /v1/files and returns the entries array', async () => {
    const entries: FileEntry[] = [
      { name: 'main.go', path: 'main.go', is_dir: false },
      { name: 'sub', path: 'sub', is_dir: true },
    ]
    global.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ entries }) }) as unknown as typeof fetch
    expect(await fetchFiles()).toEqual(entries)
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/v1/files')
  })

  it('fetchFiles passes the path as a query param', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ entries: [] }) }) as unknown as typeof fetch
    await fetchFiles('sub/dir')
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/v1/files?path=sub%2Fdir')
  })

  it('fetchFile GETs /v1/file?path= and returns content + flags', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true, json: async () => ({ path: 'a.go', content: 'package main', binary: false, truncated: false }),
    }) as unknown as typeof fetch
    const f = await fetchFile('a.go')
    expect(f.content).toBe('package main')
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/v1/file?path=a.go')
  })

  it('fetchChanged GETs /v1/changed and returns the entries', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true, json: async () => ({ entries: [{ path: 'a.go', status: 'M', deleted: false }] }),
    }) as unknown as typeof fetch
    const c = await fetchChanged()
    expect(c[0].path).toBe('a.go')
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/v1/changed')
  })

  it('addToChat POSTs the path to /v1/add-to-chat', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) }) as unknown as typeof fetch
    await addToChat('a.go')
    const [url, init] = (global.fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v1/add-to-chat')
    expect(init.method).toBe('POST')
    expect(String(init.body)).toContain('a.go')
  })

  it('throws on a non-ok response', async () => {
    global.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500, json: async () => ({}) }) as unknown as typeof fetch
    await expect(fetchFiles()).rejects.toThrow()
  })
})
