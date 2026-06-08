import { useState, useEffect, useCallback } from 'react'
import { fetchBranches, checkoutBranch } from './api'

export interface UseGitBranchesResult {
  /** Current branch name, or empty string while loading. */
  current: string
  /** All local branch names. Empty while loading. */
  branches: string[]
  /** Switch to a different local branch. Returns true on success. */
  checkout: (branch: string) => Promise<boolean>
  /** Whether a checkout is in flight. */
  switching: boolean
  /** Last checkout error message, cleared on next attempt. */
  error: string
  /** Re-fetch branches (e.g. after an external git operation). */
  refresh: () => void
}

/**
 * Fetches local git branches from the gateway and provides a checkout action.
 * Optimistic: the current branch updates immediately on checkout, then
 * re-fetches to confirm. Falls back gracefully when the gateway has no
 * workspace root (empty lists, no error).
 */
export function useGitBranches(): UseGitBranchesResult {
  const [current, setCurrent] = useState('')
  const [branches, setBranches] = useState<string[]>([])
  const [switching, setSwitching] = useState(false)
  const [error, setError] = useState('')
  const [seq, setSeq] = useState(0)

  useEffect(() => {
    let cancelled = false
    fetchBranches()
      .then((res) => {
        if (cancelled) return
        setCurrent(res.current)
        setBranches(res.branches.map((b) => b.name))
      })
      .catch(() => {
        // Gateway not ready or no workspace — stay at defaults.
      })
    return () => { cancelled = true }
  }, [seq])

  const refresh = useCallback(() => setSeq((n) => n + 1), [])

  const checkout = useCallback(
    async (branch: string): Promise<boolean> => {
      setSwitching(true)
      setError('')
      // Optimistic update.
      setCurrent(branch)
      try {
        const res = await checkoutBranch(branch)
        if (!res.success) {
          // Revert on failure.
          setCurrent(current)
          setError(res.error ?? 'checkout failed')
          setSwitching(false)
          return false
        }
        // Re-fetch to confirm.
        const updated = await fetchBranches()
        setCurrent(updated.current)
        setBranches(updated.branches.map((b) => b.name))
        setSwitching(false)
        return true
      } catch {
        setCurrent(current)
        setError('checkout failed')
        setSwitching(false)
        return false
      }
    },
    [current],
  )

  return { current, branches, checkout, switching, error, refresh }
}
