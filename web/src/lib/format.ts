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
// / day-bucketing logic), generalized into pure five-bucket groups.
import type { Session } from './api'

export type DayKey = 'today' | 'yesterday' | 'week' | 'month' | 'older'
export interface SessionGroup {
  key: DayKey
  sessions: Session[]
}

function startOfDay(ms: number): number {
  const d = new Date(ms)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

const DAY_MS = 86_400_000

/** Classify a timestamp relative to `now` into today/yesterday/week/month/older. */
export function relativeDay(ms: number, now: number = Date.now()): DayKey {
  const todayStart = startOfDay(now)
  if (ms >= todayStart) return 'today'
  if (ms >= todayStart - DAY_MS) return 'yesterday'
  if (ms >= todayStart - 7 * DAY_MS) return 'week'
  if (ms >= todayStart - 30 * DAY_MS) return 'month'
  return 'older'
}

/**
 * Group sessions into ordered day buckets, newest first within each bucket.
 * Empty buckets are omitted so the UI never renders an empty header.
 */
export function groupSessionsByDay(
  sessions: Session[],
  now: number = Date.now(),
): SessionGroup[] {
  const order: DayKey[] = ['today', 'yesterday', 'week', 'month', 'older']
  const buckets: Record<DayKey, Session[]> = { today: [], yesterday: [], week: [], month: [], older: [] }
  for (const s of sessions) buckets[relativeDay(s.updated_at, now)].push(s)
  for (const key of order) buckets[key].sort((a, b) => b.updated_at - a.updated_at)
  return order.filter((k) => buckets[k].length > 0).map((key) => ({ key, sessions: buckets[key] }))
}

/**
 * Compact relative timestamp for session rows: now / {m}m / {h}h / {d}d, then a
 * short month-day, with the year appended when it differs from `now`'s year.
 * Pure — `now` injected, never argless Date.
 */
export function formatRelativeTime(ms: number, now: number = Date.now()): string {
  const diff = now - ms
  if (diff < 60_000) return 'now'
  const min = Math.floor(diff / 60_000)
  if (min < 60) return `${min}m`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}h`
  const day = Math.floor(hr / 24)
  if (day < 7) return `${day}d`
  const d = new Date(ms)
  const month = d.toLocaleString('en-US', { month: 'short' })
  const sameYear = d.getFullYear() === new Date(now).getFullYear()
  return sameYear ? `${month} ${d.getDate()}` : `${month} ${d.getDate()} ${d.getFullYear()}`
}
