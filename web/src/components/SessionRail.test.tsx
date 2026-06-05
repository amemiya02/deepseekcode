import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SessionRail } from './SessionRail'
import type { Session } from '../lib/api'

const now = new Date('2026-06-04T12:00:00Z').getTime()
const sessions: Session[] = [
  { id: 's1', title: 'Fix login', turns: 2, updated_at: now, created_at: now },
  { id: 's2', title: 'Refactor parser', turns: 5, updated_at: now, created_at: now },
]

describe('SessionRail', () => {
  it('renders every session by title', () => {
    render(<SessionRail sessions={sessions} activeId="s1" now={now} />)
    expect(screen.getByText('Fix login')).toBeInTheDocument()
    expect(screen.getByText('Refactor parser')).toBeInTheDocument()
  })

  it('filters by the search query (case-insensitive)', async () => {
    render(<SessionRail sessions={sessions} activeId="" now={now} />)
    await userEvent.type(screen.getByTestId('session-search'), 'parser')
    expect(screen.queryByText('Fix login')).not.toBeInTheDocument()
    expect(screen.getByText('Refactor parser')).toBeInTheDocument()
  })

  it('calls onNew when the New button is clicked', async () => {
    const onNew = vi.fn()
    render(<SessionRail sessions={sessions} activeId="" now={now} onNew={onNew} />)
    await userEvent.click(screen.getByTestId('session-new'))
    expect(onNew).toHaveBeenCalledTimes(1)
  })

  it('shows a session count footer', () => {
    render(<SessionRail sessions={sessions} activeId="" now={now} />)
    expect(screen.getByTestId('session-count')).toHaveTextContent('2 sessions')
  })

  it('shows singular session count for one session', () => {
    const one: Session[] = [{ id: 's1', title: 'Fix login', turns: 2, updated_at: now, created_at: now }]
    render(<SessionRail sessions={one} activeId="" now={now} />)
    expect(screen.getByTestId('session-count')).toHaveTextContent('1 session')
  })

  it('does not show a footer when sessions is empty', () => {
    render(<SessionRail sessions={[]} now={now} />)
    expect(screen.queryByTestId('session-count')).toBeNull()
  })

  it('renders skeletons while loading', () => {
    render(<SessionRail sessions={[]} loading now={now} />)
    expect(screen.getByTestId('session-skeleton')).toBeInTheDocument()
    expect(screen.queryByTestId('sessions-empty')).toBeNull()
  })

  it('renders an empty hint when there are no sessions', () => {
    render(<SessionRail sessions={[]} now={now} />)
    expect(screen.getByTestId('sessions-empty')).toBeInTheDocument()
  })
})
