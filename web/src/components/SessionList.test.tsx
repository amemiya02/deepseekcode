import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { SessionList } from './SessionList'
import type { Session } from '../lib/api'

const now = new Date('2026-06-04T12:00:00Z').getTime()
function sess(id: string, ms: number): Session {
  return { id, title: id, turns: 1, updated_at: ms, created_at: ms }
}

describe('SessionList', () => {
  it('renders Today/Yesterday/Earlier group headers for matching sessions', () => {
    render(
      <SessionList
        sessions={[
          sess('today', now),
          sess('yday', new Date('2026-06-03T10:00:00Z').getTime()),
          sess('old', new Date('2026-05-01T10:00:00Z').getTime()),
        ]}
        activeId="today"
        now={now}
      />,
    )
    expect(screen.getByTestId('group-today')).toBeInTheDocument()
    expect(screen.getByTestId('group-yesterday')).toBeInTheDocument()
    expect(screen.getByTestId('group-earlier')).toBeInTheDocument()
    const today = screen.getByTestId('group-today')
    expect(within(today).getByText('today')).toBeInTheDocument()
  })

  it('omits a group with no sessions', () => {
    render(<SessionList sessions={[sess('today', now)]} activeId="" now={now} />)
    expect(screen.getByTestId('group-today')).toBeInTheDocument()
    expect(screen.queryByTestId('group-yesterday')).not.toBeInTheDocument()
  })

  it('shows empty state when there are no sessions', () => {
    render(<SessionList sessions={[]} activeId="" now={now} />)
    expect(screen.getByTestId('sessions-empty')).toBeInTheDocument()
  })
})
