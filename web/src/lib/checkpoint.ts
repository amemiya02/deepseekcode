// Typed client for the Wave-5 checkpoint/branch endpoints (spec §9, Contract 1).
// Framework-agnostic: no React imports. All calls go through postJSON, which
// throws on a non-2xx response so callers can surface a toast rather than
// silently proceeding from a failed rewind.

export type RewindScope = 'code' | 'conversation' | 'both'

export interface RewindResult {
  removed_messages: number
  restored_files: number
}

export interface BranchResult {
  session_id: string
  parent_id: string
  branch_point: number
}

export interface TranscriptMessage {
  role: string
  text: string
}

export interface SwitchResult {
  session_id: string
  messages: TranscriptMessage[]
}

export interface SummarizeResult {
  summary_idx: number
}

async function postJSON<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`gateway error ${res.status}`)
  return res.json() as Promise<T>
}

export function rewind(
  sessionId: string,
  keepMessages: number,
  scope: RewindScope,
): Promise<RewindResult> {
  return postJSON<RewindResult>('/v1/rewind', {
    session_id: sessionId,
    keep_messages: keepMessages,
    scope,
  })
}

export function branch(sessionId: string, branchPoint: number): Promise<BranchResult> {
  return postJSON<BranchResult>('/v1/branch', {
    session_id: sessionId,
    branch_point: branchPoint,
  })
}

export function fork(sessionId: string): Promise<BranchResult> {
  return postJSON<BranchResult>('/v1/fork', { session_id: sessionId })
}

export function switchSession(sessionId: string): Promise<SwitchResult> {
  return postJSON<SwitchResult>('/v1/switch', { session_id: sessionId })
}

export function summarize(
  sessionId: string,
  mode: 'from' | 'upto',
  index: number,
  summary: string,
): Promise<SummarizeResult> {
  return postJSON<SummarizeResult>('/v1/summarize', {
    session_id: sessionId,
    mode,
    index,
    summary,
  })
}
