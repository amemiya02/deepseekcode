// Typed API client for the Plan 4 HTTP/SSE gateway.
// ASSUMPTION (verify): gateway runs at the same origin in production (embedded),
// and at localhost:7432 in dev (proxied by vite.config.ts).

export interface CacheReport {
  total_usage_turns: number
  cache_hit_tokens: number
  cache_miss_tokens: number
  output_tokens: number
  cost_cny: number
  full_body_evictions: number
  max_miss_tokens: number
  cache_hit_rate: number
}

export interface StreamHandlers {
  onDelta?: (text: string) => void
  onTool?: (name: string) => void
  onStep?: (summary: string) => void
  onDone?: () => void
  onError?: (err: Event) => void
}

export async function submitPrompt(prompt: string, sessionId?: string): Promise<string> {
  const body: Record<string, string> = { prompt }
  if (sessionId) body.session_id = sessionId
  const res = await fetch('/v1/prompt', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return data.request_id as string
}

export async function fetchCacheReport(): Promise<CacheReport> {
  const res = await fetch('/v1/cache')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<CacheReport>
}

export class GatewayClient {
  private es: EventSource | null = null

  openEventStream(sessionId: string, handlers: StreamHandlers): void {
    this.es?.close()
    const es = new EventSource(`/v1/events?session_id=${encodeURIComponent(sessionId)}`)
    this.es = es
    if (handlers.onDelta) es.addEventListener('delta', (e) => handlers.onDelta!(e.data))
    if (handlers.onTool) es.addEventListener('tool', (e) => handlers.onTool!(e.data))
    if (handlers.onStep) es.addEventListener('step', (e) => handlers.onStep!(e.data))
    if (handlers.onDone) es.addEventListener('done', () => handlers.onDone!())
    if (handlers.onError) es.onerror = handlers.onError
  }

  close(): void {
    this.es?.close()
    this.es = null
  }
}
