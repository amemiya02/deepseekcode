// Pure, framework-agnostic formatting helpers shared by the status bar,
// cockpit, and session list. No DOM, no React — trivially unit-testable.

const DASH = '—'

/** Render a 0..1 ratio as a rounded integer percent, or — when missing. */
export function formatPct(ratio: number | null | undefined): string {
  if (ratio == null || Number.isNaN(ratio)) return DASH
  return `${Math.round(ratio * 100)}%`
}

/** Render yuan to 4 decimal places with a ¥ prefix; ¥— when missing. */
export function formatCNY(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return `¥${DASH}`
  return `¥${value.toFixed(4)}`
}

/** Thousands-separate an integer token count; — when missing. */
export function formatTokens(value: number | null | undefined): string {
  if (value == null || Number.isNaN(value)) return DASH
  return Math.round(value).toLocaleString('en-US')
}

// Adapted from deepseek-reasonix (MIT) — components/HistoryPanel.tsx (dayLabel
// / day-bucketing logic), generalized into pure today/yesterday/earlier groups.
import type { Session } from './api'

export type DayKey = 'today' | 'yesterday' | 'earlier'
export interface SessionGroup {
  key: DayKey
  sessions: Session[]
}

function startOfDay(ms: number): number {
  const d = new Date(ms)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

/** Classify a timestamp relative to `now` into today / yesterday / earlier. */
export function relativeDay(ms: number, now: number = Date.now()): DayKey {
  const dayMs = 86_400_000
  const todayStart = startOfDay(now)
  if (ms >= todayStart) return 'today'
  if (ms >= todayStart - dayMs) return 'yesterday'
  return 'earlier'
}

/**
 * Group sessions into ordered today/yesterday/earlier buckets, newest first
 * within each bucket. Empty buckets are omitted so the UI never renders an
 * empty header.
 */
export function groupSessionsByDay(
  sessions: Session[],
  now: number = Date.now(),
): SessionGroup[] {
  const order: DayKey[] = ['today', 'yesterday', 'earlier']
  const buckets: Record<DayKey, Session[]> = { today: [], yesterday: [], earlier: [] }
  for (const s of sessions) buckets[relativeDay(s.updated_at, now)].push(s)
  for (const key of order) buckets[key].sort((a, b) => b.updated_at - a.updated_at)
  return order.filter((k) => buckets[k].length > 0).map((key) => ({ key, sessions: buckets[key] }))
}
