import { useEffect, useState, useCallback } from 'react'
import { fetchConfig, saveConfig, type ConfigDTO } from './system'

// Shared in-flight/resolved snapshot so every settings section reuses one
// /v1/config round-trip. Subscribers are notified on every mutation.
let cache: ConfigDTO | null = null
let inflight: Promise<ConfigDTO> | null = null
const subscribers = new Set<(c: ConfigDTO) => void>()

function publish(next: ConfigDTO) {
  cache = next
  subscribers.forEach((fn) => fn(next))
}

async function ensureLoaded(): Promise<ConfigDTO> {
  if (cache) return cache
  if (!inflight) inflight = fetchConfig().then((c) => { publish(c); inflight = null; return c })
  return inflight
}

/** Test-only: drop the shared cache between tests. */
export function __resetConfigCache() { cache = null; inflight = null; subscribers.clear() }

export interface UseConfig {
  cfg: ConfigDTO | null
  error: string
  reload: () => void
  patch: (p: Partial<ConfigDTO>) => Promise<void>
  clearError: () => void
}

export function useConfig(): UseConfig {
  const [cfg, setCfg] = useState<ConfigDTO | null>(cache)
  const [error, setError] = useState('')

  useEffect(() => {
    const onChange = (c: ConfigDTO) => setCfg(c)
    subscribers.add(onChange)
    void ensureLoaded().catch((e) => setError(e instanceof Error ? e.message : String(e)))
    return () => { subscribers.delete(onChange) }
  }, [])

  const patch = useCallback(async (p: Partial<ConfigDTO>) => {
    if (cache) publish({ ...cache, ...p }) // optimistic
    try {
      const echoed = await saveConfig(p)
      publish(echoed)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  const reload = useCallback(() => { cache = null; inflight = null; void ensureLoaded() }, [])
  const clearError = useCallback(() => setError(''), [])
  return { cfg, error, reload, patch, clearError }
}
