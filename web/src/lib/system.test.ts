import { describe, it, expect, vi, afterEach } from 'vitest'
import { fetchConfig, saveConfig, fetchDoctor, fetchUpdate, fetchOnboarding, connectKey } from './system'

afterEach(() => vi.restoreAllMocks())

function mockFetch(status: number, body: unknown) {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
  )
}

describe('system client', () => {
  it('fetchConfig returns the DTO', async () => {
    mockFetch(200, { theme: 'graphite', accent: 'indigo', density: 'comfortable', model: 'deepseek-v4', autoRoute: false })
    const cfg = await fetchConfig()
    expect(cfg.theme).toBe('graphite')
    expect(cfg.accent).toBe('indigo')
    expect(cfg.model).toBe('deepseek-v4')
  })

  it('saveConfig PUTs and returns the echoed DTO', async () => {
    const spy = mockFetch(200, { theme: 'lumen', accent: 'terracotta', density: 'compact', model: 'deepseek-v4', autoRoute: true })
    const out = await saveConfig({ theme: 'lumen' })
    expect(out.theme).toBe('lumen')
    expect(spy).toHaveBeenCalledWith('/v1/config', expect.objectContaining({ method: 'PUT' }))
  })

  it('fetchDoctor returns checks', async () => {
    mockFetch(200, { allOk: false, checks: [{ name: 'api key', ok: false, detail: 'missing' }] })
    const d = await fetchDoctor()
    expect(d.allOk).toBe(false)
    expect(d.checks[0].name).toBe('api key')
  })

  it('fetchUpdate maps updateAvailable', async () => {
    mockFetch(200, { current: 'v1.0.0', latest: 'v2.0.0', updateAvailable: true, url: 'https://x' })
    const u = await fetchUpdate()
    expect(u.updateAvailable).toBe(true)
    expect(u.url).toBe('https://x')
  })

  it('fetchOnboarding reports needsOnboarding', async () => {
    mockFetch(200, { needsOnboarding: true, baseUrl: 'https://api.deepseek.com', model: 'deepseek-v4' })
    const o = await fetchOnboarding()
    expect(o.needsOnboarding).toBe(true)
  })

  it('connectKey throws the server message on 400', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('key validation failed: bad', { status: 400 }))
    await expect(connectKey({ apiKey: 'x', baseUrl: 'y', model: 'z' })).rejects.toThrow(/key validation failed/)
  })
})
