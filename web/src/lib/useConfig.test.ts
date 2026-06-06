import { renderHook, act, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import * as system from './system'
import { useConfig, __resetConfigCache } from './useConfig'

const cfg = { accent: 'indigo', density: 'comfortable', vimKeybindings: true } as unknown as system.ConfigDTO

afterEach(() => { __resetConfigCache(); vi.restoreAllMocks() })

describe('useConfig', () => {
  it('loads the config once and exposes it', async () => {
    const fetchSpy = vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...cfg })
    const { result } = renderHook(() => useConfig())
    await waitFor(() => expect(result.current.cfg).not.toBeNull())
    expect(result.current.cfg?.accent).toBe('indigo')
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })

  it('patch() optimistically updates and calls saveConfig', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...cfg })
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...cfg, accent: 'rose' })
    const { result } = renderHook(() => useConfig())
    await waitFor(() => expect(result.current.cfg).not.toBeNull())
    await act(async () => { await result.current.patch({ accent: 'rose' }) })
    expect(save).toHaveBeenCalledWith({ accent: 'rose' })
    expect(result.current.cfg?.accent).toBe('rose')
  })

  it('exposes an error string when saveConfig rejects (does not throw)', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...cfg })
    vi.spyOn(system, 'saveConfig').mockRejectedValue(new Error('boom'))
    const { result } = renderHook(() => useConfig())
    await waitFor(() => expect(result.current.cfg).not.toBeNull())
    await act(async () => { await result.current.patch({ accent: 'rose' }) })
    expect(result.current.error).toBe('boom')
  })
})
