import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { SessionList } from './SessionList'
import type { Session } from '../lib/api'

const now = new Date('2026-06-04T12:00:00Z').getTime()
const DAY = 86_400_000
function sess(id: string, ms: number): Session {
  return { id, title: id, turns: 1, updated_at: ms, created_at: ms }
}

describe('SessionList', () => {
  it('renders Today/Yesterday/Week/Month/Older group headers for matching sessions', () => {
    render(
      <SessionList
        sessions={[
          sess('today', now),
          sess('yday', now - 1.5 * DAY),
          sess('week', now - 4 * DAY),
          sess('month', now - 15 * DAY),
          sess('old', now - 60 * DAY),
        ]}
        activeId="today"
        now={now}
      />,
    )
    expect(screen.getByTestId('group-today')).toBeInTheDocument()
    expect(screen.getByTestId('group-yesterday')).toBeInTheDocument()
    expect(screen.getByTestId('group-week')).toBeInTheDocument()
    expect(screen.getByTestId('group-month')).toBeInTheDocument()
    expect(screen.getByTestId('group-older')).toBeInTheDocument()
    const today = screen.getByTestId('group-today')
    expect(within(today).getByText('today')).toBeInTheDocument()
  })

  it('omits a group with no sessions', () => {
    render(<SessionList sessions={[sess('today', now)]} activeId="" now={now} />)
    expect(screen.getByTestId('group-today')).toBeInTheDocument()
    expect(screen.queryByTestId('group-yesterday')).not.toBeInTheDocument()
    expect(screen.queryByTestId('group-week')).not.toBeInTheDocument()
    expect(screen.queryByTestId('group-month')).not.toBeInTheDocument()
    expect(screen.queryByTestId('group-older')).not.toBeInTheDocument()
  })

  it('shows empty state when there are no sessions', () => {
    render(<SessionList sessions={[]} activeId="" now={now} />)
    expect(screen.getByTestId('sessions-empty')).toBeInTheDocument()
  })

  it('forwards now into SessionItem so relative times are deterministic', () => {
    const updated = now - 3 * 3_600_000 // 3 hours ago
    render(
      <SessionList
        sessions={[{ id: 's1', title: 'Fix login bug', turns: 2, updated_at: updated, created_at: 0 }]}
        activeId=""
        now={now}
      />,
    )
    expect(screen.getByTestId('session-time')).toHaveTextContent('3h')
  })
})
