// Pure transcript model: folds a stream of Wave 1 (Contract 2) events into renderable items.
// No DOM, no React — unit-testable in isolation; the Transcript component renders the result.
export interface UserItem { type: 'user'; text: string; pills?: string[] }
export interface AssistantItem { type: 'assistant'; text: string; streaming: boolean }
export interface ThinkingItem { type: 'thinking'; text: string; startedAt?: number; endedAt?: number }
export interface ToolItem {
  type: 'tool'
  id: string
  name: string
  args: Record<string, unknown>
  readOnly: boolean
  status: 'running' | 'ok' | 'error'
  result?: string
  truncated?: boolean
}
export interface RoutingItem { type: 'routing'; from: string; to: string; reason: string }
export interface DuetItem { type: 'duet'; decision: string; reason: string }
export type TranscriptItem = UserItem | AssistantItem | ThinkingItem | ToolItem | RoutingItem | DuetItem

// Events use the Contract-2 snake_case field names so the reducer can be fed
// straight from the GatewayClient handlers without a rename layer.
export type TranscriptEvent =
  | { kind: 'user'; text: string; pills?: string[] }
  | { kind: 'message_delta'; text: string }
  | { kind: 'thinking_delta'; text: string }
  | { kind: 'tool_start'; id: string; name: string; args: Record<string, unknown>; read_only: boolean }
  | { kind: 'tool_delta'; id: string; delta: string }
  | { kind: 'tool_end'; id: string; result: string; is_error: boolean; truncated?: boolean }
  | { kind: 'routing'; from: string; to: string; reason: string }
  | { kind: 'duet'; decision: string; reason: string }
  | { kind: 'turn_done'; stop_reason: string }

export type Clock = () => number
const defaultNow: Clock = () => (typeof performance !== 'undefined' ? performance.now() : Date.now())

export function formatThinkingDuration(startedAt: number, endedAt: number): string {
  const ms = Math.max(0, endedAt - startedAt)
  if (ms < 1000) return 'Thought for <1s'
  return `Thought for ${Math.round(ms / 1000)}s`
}

// Stamp endedAt on the trailing thinking item if it is still open.
function closeOpenThinking(items: TranscriptItem[], at: number): TranscriptItem[] {
  const last = items[items.length - 1]
  if (last && last.type === 'thinking' && last.endedAt == null) {
    const updated = items.slice()
    updated[updated.length - 1] = { ...last, endedAt: at }
    return updated
  }
  return items
}

// Strip cache/cost telemetry lines that the backend appends to message deltas.
// Pattern: "turn=N epoch=N cause=... hit=N miss=N residual=N cost=¥X saved=¥X"
const TELEMETRY_RE = /\s*turn=\d+\s+epoch=\d+\s+cause=\S+\s+hit=\d+\s+miss=\d+\s+residual=\d+\s+cost=¥[\d.]+\s+saved=¥[\d.]+.*$/

export function applyEvent(
  items: TranscriptItem[],
  e: TranscriptEvent,
  now: Clock = defaultNow,
): TranscriptItem[] {
  const last = items[items.length - 1]
  switch (e.kind) {
    case 'user':
      return [...closeOpenThinking(items, now()), { type: 'user', text: e.text, pills: e.pills }]
    case 'message_delta': {
      const clean = e.text.replace(TELEMETRY_RE, '')
      if (!clean) return items
      const base = closeOpenThinking(items, now())
      const tail = base[base.length - 1]
      if (tail && tail.type === 'assistant' && tail.streaming) {
        const updated = base.slice()
        updated[updated.length - 1] = { ...tail, text: tail.text + clean }
        return updated
      }
      return [...base, { type: 'assistant', text: clean, streaming: true }]
    }
    case 'thinking_delta':
      if (last && last.type === 'thinking' && last.endedAt == null) {
        const updated = items.slice()
        updated[updated.length - 1] = { ...last, text: last.text + e.text }
        return updated
      }
      return [...items, { type: 'thinking', text: e.text, startedAt: now() }]
    case 'tool_start':
      return [...closeOpenThinking(items, now()), { type: 'tool', id: e.id, name: e.name, args: e.args, readOnly: e.read_only, status: 'running' }]
    case 'tool_delta': {
      const idx = items.findIndex((it) => it.type === 'tool' && it.id === e.id)
      if (idx === -1) return items
      const tool = items[idx] as ToolItem
      const updated = items.slice()
      updated[idx] = { ...tool, result: (tool.result ?? '') + e.delta }
      return updated
    }
    case 'tool_end': {
      const idx = items.findIndex((it) => it.type === 'tool' && it.id === e.id)
      if (idx === -1) return items
      const tool = items[idx] as ToolItem
      const updated = items.slice()
      updated[idx] = { ...tool, status: e.is_error ? 'error' : 'ok', result: e.result, truncated: e.truncated }
      return updated
    }
    case 'routing':
      return [...closeOpenThinking(items, now()), { type: 'routing', from: e.from, to: e.to, reason: e.reason }]
    case 'duet':
      return [...closeOpenThinking(items, now()), { type: 'duet', decision: e.decision, reason: e.reason }]
    case 'turn_done': {
      const base = closeOpenThinking(items, now())
      const tail = base[base.length - 1]
      if (tail && tail.type === 'assistant' && tail.streaming) {
        const updated = base.slice()
        updated[updated.length - 1] = { ...tail, streaming: false }
        return updated
      }
      return base
    }
  }
}
