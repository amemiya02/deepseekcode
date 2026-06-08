import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useGitBranches } from './useGitBranches'

// Mock the api module.
vi.mock('./api', () => ({
  fetchBranches: vi.fn(),
  checkoutBranch: vi.fn(),
}))

import { fetchBranches, checkoutBranch } from './api'
const mockFetch = vi.mocked(fetchBranches)
const mockCheckout = vi.mocked(checkoutBranch)

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('useGitBranches', () => {
  it('fetches branches on mount', async () => {
    mockFetch.mockResolvedValue({
      current: 'main',
      branches: [{ name: 'main', current: true }, { name: 'dev', current: false }],
    })

    const { result } = renderHook(() => useGitBranches())

    // Initially empty before the async fetch resolves.
    expect(result.current.current).toBe('')
    expect(result.current.branches).toEqual([])

    // Wait for the fetch to resolve.
    await vi.waitFor(() => {
      expect(result.current.current).toBe('main')
    })
    expect(result.current.branches).toEqual(['main', 'dev'])
  })

  it('checkout updates current optimistically', async () => {
    mockFetch.mockResolvedValue({
      current: 'main',
      branches: [{ name: 'main', current: true }, { name: 'dev', current: false }],
    })
    mockCheckout.mockResolvedValue({ branch: 'dev', success: true })

    const { result } = renderHook(() => useGitBranches())

    await vi.waitFor(() => {
      expect(result.current.current).toBe('main')
    })

    // After fetch confirms, set up the post-checkout fetch.
    mockFetch.mockResolvedValue({
      current: 'dev',
      branches: [{ name: 'main', current: false }, { name: 'dev', current: true }],
    })

    let success = false
    await act(async () => {
      success = await result.current.checkout('dev')
    })

    expect(success).toBe(true)
    expect(result.current.current).toBe('dev')
    expect(mockCheckout).toHaveBeenCalledWith('dev')
  })

  it('checkout reverts on failure', async () => {
    mockFetch.mockResolvedValue({
      current: 'main',
      branches: [{ name: 'main', current: true }, { name: 'dev', current: false }],
    })
    mockCheckout.mockResolvedValue({ branch: 'dev', success: false, error: 'already checked out' })

    const { result } = renderHook(() => useGitBranches())

    await vi.waitFor(() => {
      expect(result.current.current).toBe('main')
    })

    let success = true
    await act(async () => {
      success = await result.current.checkout('dev')
    })

    expect(success).toBe(false)
    expect(result.current.current).toBe('main')
    expect(result.current.error).toBe('already checked out')
  })

  it('handles fetch error gracefully', async () => {
    mockFetch.mockRejectedValue(new Error('network'))

    const { result } = renderHook(() => useGitBranches())

    // Should stay at defaults without throwing.
    await vi.waitFor(() => {
      // Give the promise a tick to reject.
    })
    expect(result.current.current).toBe('')
    expect(result.current.branches).toEqual([])
  })

  it('refresh re-fetches branches', async () => {
    mockFetch.mockResolvedValue({
      current: 'main',
      branches: [{ name: 'main', current: true }],
    })

    const { result } = renderHook(() => useGitBranches())

    await vi.waitFor(() => {
      expect(result.current.current).toBe('main')
    })

    expect(mockFetch).toHaveBeenCalledTimes(1)

    act(() => {
      result.current.refresh()
    })

    await vi.waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(2)
    })
  })
})
