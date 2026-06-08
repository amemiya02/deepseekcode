// Typed API client for the Wave 1 HTTP/SSE gateway — the SINGLE gateway client (Contract 1).
// ASSUMPTION (verify): gateway runs at the same origin in production (embedded),
// and at localhost:7432 in dev (proxied by vite.config.ts).

export interface Epoch {
  id: string
  turns: number
  compacted: boolean
}

// ── Shared cross-wave types (Contract 1: defined ONCE here) ──────────────────
export interface ModelCaps { vision: boolean; tools: boolean; reasoning: boolean }
export interface ModelEffortDesc {
  // 'levels' — model has a named effort scale (e.g. low/medium/high/max).
  // 'none'   — model has no effort control; composer hides the effort chip.
  // 'toggle' is reserved for future boolean-effort models; no Go code emits it yet.
  kind: 'levels' | 'none'
  levels?: string[]
  default?: string
}
export interface ModelInfo {
  id: string
  label: string
  provider?: string
  caps?: ModelCaps
  effort?: ModelEffortDesc
  context?: number
}
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
export interface DuetEvent { decision: string; reason: string }
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
  onDuet?: (e: DuetEvent) => void
  onCacheUpdate?: (e: CacheUpdateEvent) => void
  onCostUpdate?: (e: CostUpdateEvent) => void
  onJobUpdate?: (e: JobUpdateEvent) => void
  onRetry?: (e: RetryEvent) => void
  onTurnDone?: (e: TurnDoneEvent) => void
  onError?: (err: Event) => void
}

// UploadedFile is one saved chat attachment: `path` is workspace-relative and
// is what the prompt should reference so the agent's root-confined tools can
// read the file.
export interface UploadedFile { name: string; path: string }

// uploadFiles persists chat attachments into the workspace via POST /v1/upload
// (multipart, repeatable "file" parts) and returns the saved paths.
export async function uploadFiles(files: File[]): Promise<UploadedFile[]> {
  const form = new FormData()
  for (const f of files) form.append('file', f, f.name)
  const res = await fetch('/v1/upload', { method: 'POST', body: form })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return (data.files ?? []) as UploadedFile[]
}

export async function submitPrompt(prompt: string, sessionId?: string, mode?: string): Promise<string> {
  const body: Record<string, string> = { prompt }
  if (sessionId) body.session_id = sessionId
  // Composer permission mode (ask | auto-edit | plan | yolo) — the gateway
  // applies it to the session's agent before the turn starts.
  if (mode) body.mode = mode
  const res = await fetch('/v1/prompt', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return data.session_id as string
}

// ---- Wave 4: interactive permissions, ask, plan ----

// PermissionDecision mirrors the gateway POST /v1/permission `decision` field
// (spec §9): deny / allow-once / allow-for-session / always-save-to-config.
export type PermissionDecision = 'deny' | 'once' | 'session' | 'always'

// PermissionRequest is the canonical Wave-4 alias for PermissionRequestEvent (Contract 2).
export type PermissionRequest = PermissionRequestEvent

// AskRequest is the canonical Wave-4 alias for AskRequestEvent (Contract 2).
export type AskRequest = AskRequestEvent

// AskAnswer is the POST /v1/answer body. Per question, exactly one of
// choices/text/chat is meaningful: `choices` for option selections, `text` for
// free text, `chat: true` for the "just chat" escape (answer nothing, keep talking).
export interface AskAnswer {
  id: string
  questionIndex: number
  choices?: string[]
  text?: string
  chat?: boolean
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

// ---- Live (SSE) payload aliases — point to the canonical Contract 2 types ----
// These aliases exist so cockpit/store code can import descriptive names without
// duplicating interface definitions that would diverge from StreamHandlers callbacks.

export type LiveCache = CacheUpdateEvent
export type LiveCost = CostUpdateEvent
export type RoutingHop = RoutingEvent
export type RetryStatus = RetryEvent

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
/** POST /v1/steer — inject a mid-turn instruction into the running turn. */
export async function steerTurn(sessionId: string, prompt: string): Promise<void> {
  await postJSON('/v1/steer', { session_id: sessionId, prompt })
}
export async function respondPermission(id: string, decision: PermissionDecision): Promise<void> {
  await postJSON('/v1/permission', { id, decision })
}
export async function respondAnswer(answer: AskAnswer): Promise<void> {
  await postJSON('/v1/answer', answer as unknown as Record<string, unknown>)
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
export const EFFORT_LEVELS = ['low', 'medium', 'high', 'max'] as const

// Parse a /v1/models payload into ModelInfo[]. Accepts the capability-descriptor
// objects (current contract) AND the legacy bare-string list (back-compat).
function parseModelList(data: unknown): ModelInfo[] {
  const arr = Array.isArray(data) ? data : ((data as { models?: unknown[] }).models ?? [])
  return arr.map((m): ModelInfo => {
    if (typeof m === 'string') return { id: m, label: m }
    const d = m as Partial<ModelInfo> & { id: string }
    return {
      id: d.id,
      label: d.label ?? d.id,
      provider: d.provider,
      caps: d.caps,
      effort: d.effort,
      context: d.context,
    }
  })
}

// GET /v1/models returns {active, effort, models: string[]|descriptor[]}
// (internal/gateway/models.go). Parses both shapes into ModelInfo[].
export async function fetchModels(): Promise<ModelInfo[]> {
  const res = await fetch('/v1/models')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return parseModelList(await res.json())
}

// GET /v1/models → {models:string[]|descriptor[], active, effort}. Returns the full state
// (fetchModels above intentionally returns only the list for the picker popover).
export async function fetchModelState(): Promise<{ models: ModelInfo[]; active: string; effort: string }> {
  const res = await fetch('/v1/models')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = (await res.json()) as unknown
  const models = parseModelList(data)
  const d = (Array.isArray(data) ? {} : data) as { active?: string; effort?: string }
  return { models, active: d.active ?? '', effort: d.effort ?? 'medium' }
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
    if (handlers.onDuet) es.addEventListener('duet', j(handlers.onDuet))
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

// ---- MCP Discovery (B1) ----

export interface McpToolResult {
  server: string
  tool: string
  desc: string
}

export async function mcpSearch(query: string): Promise<McpToolResult[]> {
  const res = await fetch('/v1/mcp/search', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query }),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return (data.results ?? []) as McpToolResult[]
}

export async function mcpCall(server: string, tool: string, args?: Record<string, unknown>): Promise<unknown> {
  const res = await fetch('/v1/mcp/call', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ server, tool, args: args ?? {} }),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json()
}

// ---- Wave 5: Capabilities introspection + Runtime info (B4) ----

export interface RuntimeInfo {
  model: string
  version: string
  caps: Record<string, boolean>
}

export async function fetchCapabilities(): Promise<Record<string, boolean>> {
  const res = await fetch('/v1/capabilities')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  const data = await res.json()
  return (data.caps ?? {}) as Record<string, boolean>
}

export async function fetchRuntimeInfo(): Promise<RuntimeInfo> {
  const res = await fetch('/v1/runtime')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<RuntimeInfo>
}

// ---- Git branch introspection (B-suffix) ----

export interface GitBranch {
  name: string
  current: boolean
}

export interface GitBranchesResult {
  branches: GitBranch[]
  current: string
}

export interface CheckoutResult {
  branch: string
  success: boolean
  error?: string
}

/** GET /v1/git/branches — list local branches and the current branch name. */
export async function fetchBranches(): Promise<GitBranchesResult> {
  const res = await fetch('/v1/git/branches')
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<GitBranchesResult>
}

/** POST /v1/git/checkout — switch to the given local branch. */
export async function checkoutBranch(branch: string): Promise<CheckoutResult> {
  const res = await fetch('/v1/git/checkout', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch }),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<CheckoutResult>
}
