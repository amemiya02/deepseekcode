// Pure transcript model: folds a stream of Wave 1 (Contract 2) events into renderable items.
// No DOM, no React — unit-testable in isolation; the Transcript component renders the result.
export interface UserItem { type: 'user'; text: string; pills?: string[] }
export interface AssistantItem { type: 'assistant'; text: string; streaming: boolean }
export interface ThinkingItem { type: 'thinking'; text: string }
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
export type TranscriptItem = UserItem | AssistantItem | ThinkingItem | ToolItem | RoutingItem

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
  | { kind: 'turn_done'; stop_reason: string }

export function applyEvent(items: TranscriptItem[], e: TranscriptEvent): TranscriptItem[] {
  const last = items[items.length - 1]
  switch (e.kind) {
    case 'user':
      return [...items, { type: 'user', text: e.text, pills: e.pills }]
    case 'message_delta':
      if (last && last.type === 'assistant' && last.streaming) {
        const updated = items.slice()
        updated[updated.length - 1] = { ...last, text: last.text + e.text }
        return updated
      }
      return [...items, { type: 'assistant', text: e.text, streaming: true }]
    case 'thinking_delta':
      if (last && last.type === 'thinking') {
        const updated = items.slice()
        updated[updated.length - 1] = { ...last, text: last.text + e.text }
        return updated
      }
      return [...items, { type: 'thinking', text: e.text }]
    case 'tool_start':
      return [...items, { type: 'tool', id: e.id, name: e.name, args: e.args, readOnly: e.read_only, status: 'running' }]
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
      return [...items, { type: 'routing', from: e.from, to: e.to, reason: e.reason }]
    case 'turn_done':
      if (last && last.type === 'assistant' && last.streaming) {
        const updated = items.slice()
        updated[updated.length - 1] = { ...last, streaming: false }
        return updated
      }
      return items
  }
}
