// Typed API client for the Wave 1 HTTP/SSE gateway — the SINGLE gateway client (Contract 1).
// ASSUMPTION (verify): gateway runs at the same origin in production (embedded),
// and at localhost:7432 in dev (proxied by vite.config.ts).

export interface Epoch {
  id: string
  turns: number
  compacted: boolean
}

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

// ── Shared cross-wave types (Contract 1: defined ONCE here) ──────────────────
export interface ModelInfo { id: string; label: string }
export type PlanStatus = 'pending' | 'in_progress' | 'done'
export interface PlanItem { text: string; status: PlanStatus }

// FileEntry is owned by web/src/lib/workspace.ts (Wave 5); it is re-exported here
// so consumers can import it from the single client. Shape: { name, path, is_dir }.
export type { FileEntry } from './workspace'

// ── Contract 2 SSE event payloads (snake_case JSON objects) ──────────────────
export interface ToolStartEvent { id: string; name: string; args: Record<string, unknown>; read_only: boolean }
export interface ToolDeltaEvent { id: string; delta: string }
export interface ToolEndEvent { id: string; result: string; is_error: boolean }
export interface PlanUpdateEvent { items: PlanItem[] }
export interface PermissionOption { label: string; description: string }
export interface PermissionRequestEvent { id: string; tool: string; args: Record<string, unknown>; options: PermissionOption[] }
export interface AskQuestion { question: string; header: string; multiple: boolean; options: PermissionOption[] }
export interface AskRequestEvent { id: string; questions: AskQuestion[] }
export interface RoutingEvent { from: string; to: string; reason: string }
export interface CacheUpdateEvent { turn_pct: number; avg_pct: number; prefixes: number; eviction: boolean }
export interface CostUpdateEvent { turn_cny: number; session_cny: number; output_tokens: number }
export interface JobUpdateEvent { running: number }
export interface RetryEvent { attempt: number; max: number }
export interface TurnDoneEvent { stop_reason: string }

export interface StreamHandlers {
  // Legacy listeners (kept for back-compat; remove once all callers migrate).
  onDelta?: (text: string) => void
  onTool?: (name: string) => void
  onStep?: (summary: string) => void
  onDone?: () => void
  // Contract 2 listeners.
  onMessageDelta?: (text: string) => void
  onThinkingDelta?: (text: string) => void
  onToolStart?: (e: ToolStartEvent) => void
  onToolDelta?: (e: ToolDeltaEvent) => void
  onToolEnd?: (e: ToolEndEvent) => void
  onPlanUpdate?: (e: PlanUpdateEvent) => void
  onPermissionRequest?: (e: PermissionRequestEvent) => void
  onAskRequest?: (e: AskRequestEvent) => void
  onRouting?: (e: RoutingEvent) => void
  onCacheUpdate?: (e: CacheUpdateEvent) => void
  onCostUpdate?: (e: CostUpdateEvent) => void
  onJobUpdate?: (e: JobUpdateEvent) => void
  onRetry?: (e: RetryEvent) => void
  onTurnDone?: (e: TurnDoneEvent) => void
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
  return data.session_id as string
}

// ---- Wave 3: session & cockpit types (Contract 1 shared types) ----

// Contract-1 shared model type — defined ONCE. If an earlier wave already
// exports ModelInfo from this file, do NOT add this duplicate.
// (ModelInfo is already exported above as a cross-wave type.)

export interface Session {
  id: string
  title: string
  turns: number
  updated_at: number // epoch ms
  created_at: number // epoch ms
}

export interface SessionTurn {
  role: 'user' | 'assistant' | 'tool'
  text: string
}

export interface SessionDetail extends Session {
  messages: SessionTurn[]
}

// Timeline entries reuse the existing Epoch shape.
export type TimelineEntry = Epoch

export interface CacheLedgerRow {
  turn: number
  hit_tokens: number
  miss_tokens: number
  evicted: boolean
}

export interface Balance {
  provider: string
  currency: string
  amount: number
}

// ---- Live (SSE) payloads — Contract 2 snake_case fields ----

export interface LiveCache {
  turn_pct: number // 0..1 this-turn cache-hit rate
  avg_pct: number // 0..1 session rolling avg
  prefixes: number // count of distinct cached prefixes (1 == stable)
  eviction: boolean // full-body eviction this turn
}

export interface LiveCost {
  turn_cny: number
  session_cny: number
  output_tokens: number
}

export interface RoutingHop {
  from: string
  to: string
  reason: string
}

export interface JobStatus {
  running: number
}

export interface RetryStatus {
  attempt: number
  max: number
}

export async function fetchCacheReport(): Promise<CacheReport> {
  const res = await fetch('/v1/cache')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<CacheReport>
}

async function postJSON(path: string, body: Record<string, unknown>): Promise<void> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
}

export async function cancelTurn(sessionId: string): Promise<void> {
  await postJSON('/v1/cancel', { session_id: sessionId })
}
export async function setModel(sessionId: string, model: string): Promise<void> {
  await postJSON('/v1/model', { session_id: sessionId, model })
}
export async function setEffort(sessionId: string, effort: string): Promise<void> {
  await postJSON('/v1/effort', { session_id: sessionId, effort })
}
export async function setOutputStyle(sessionId: string, style: string): Promise<void> {
  await postJSON('/v1/output-style', { session_id: sessionId, style })
}
export async function fetchModels(): Promise<ModelInfo[]> {
  const res = await fetch('/v1/models')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return (await res.json()) as ModelInfo[]
}

export class GatewayClient {
  private es: EventSource | null = null

  openEventStream(sessionId: string, handlers: StreamHandlers): void {
    this.es?.close()
    const es = new EventSource(`/v1/events?session_id=${encodeURIComponent(sessionId)}`)
    this.es = es
    // Legacy listeners (raw string data).
    if (handlers.onDelta) es.addEventListener('delta', (e) => handlers.onDelta!(e.data))
    if (handlers.onTool) es.addEventListener('tool', (e) => handlers.onTool!(e.data))
    if (handlers.onStep) es.addEventListener('step', (e) => handlers.onStep!(e.data))
    if (handlers.onDone) es.addEventListener('done', () => handlers.onDone!())
    // Contract 2 listeners (JSON-object data).
    const j = <T>(cb?: (e: T) => void) => (ev: MessageEvent) => { if (cb) cb(JSON.parse(ev.data) as T) }
    if (handlers.onMessageDelta) es.addEventListener('message_delta', j<{ text: string }>((d) => handlers.onMessageDelta!(d.text)))
    if (handlers.onThinkingDelta) es.addEventListener('thinking_delta', j<{ text: string }>((d) => handlers.onThinkingDelta!(d.text)))
    if (handlers.onToolStart) es.addEventListener('tool_start', j(handlers.onToolStart))
    if (handlers.onToolDelta) es.addEventListener('tool_delta', j(handlers.onToolDelta))
    if (handlers.onToolEnd) es.addEventListener('tool_end', j(handlers.onToolEnd))
    if (handlers.onRouting) es.addEventListener('routing', j(handlers.onRouting))
    if (handlers.onPlanUpdate) es.addEventListener('plan_update', j(handlers.onPlanUpdate))
    if (handlers.onPermissionRequest) es.addEventListener('permission_request', j(handlers.onPermissionRequest))
    if (handlers.onAskRequest) es.addEventListener('ask_request', j(handlers.onAskRequest))
    if (handlers.onCacheUpdate) es.addEventListener('cache_update', j(handlers.onCacheUpdate))
    if (handlers.onCostUpdate) es.addEventListener('cost_update', j(handlers.onCostUpdate))
    if (handlers.onJobUpdate) es.addEventListener('job_update', j(handlers.onJobUpdate))
    if (handlers.onRetry) es.addEventListener('retry', j(handlers.onRetry))
    if (handlers.onTurnDone) es.addEventListener('turn_done', j(handlers.onTurnDone))
    if (handlers.onError) es.onerror = handlers.onError
  }

  close(): void {
    this.es?.close()
    this.es = null
  }
}

// ---- Wave 3: session CRUD + timeline ----

export async function listSessions(): Promise<Session[]> {
  const res = await fetch('/v1/sessions')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return (data.sessions ?? []) as Session[]
}

export async function createSession(workingDir: string): Promise<Session> {
  const res = await fetch('/v1/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ working_dir: workingDir }),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<Session>
}

export async function getSession(id: string): Promise<SessionDetail> {
  const res = await fetch(`/v1/sessions/${id}`)
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<SessionDetail>
}

export async function renameSession(id: string, title: string): Promise<void> {
  const res = await fetch(`/v1/sessions/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
}

export async function deleteSession(id: string): Promise<void> {
  const res = await fetch(`/v1/sessions/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
}

export async function getTimeline(id: string): Promise<TimelineEntry[]> {
  const res = await fetch(`/v1/sessions/${id}/timeline`)
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return (data.entries ?? []) as TimelineEntry[]
}

// ---- Wave 3: cockpit reads ----

export async function fetchCacheLedger(session: string, turn?: number): Promise<CacheLedgerRow[]> {
  const q = new URLSearchParams({ session })
  if (turn != null) q.set('turn', String(turn))
  const res = await fetch(`/v1/cache/ledger?${q.toString()}`)
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return (data.rows ?? []) as CacheLedgerRow[]
}

export async function fetchBalance(): Promise<Balance> {
  const res = await fetch('/v1/balance')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<Balance>
}
