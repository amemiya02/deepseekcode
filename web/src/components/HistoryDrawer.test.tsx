import { describe, it, expect, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HistoryDrawer } from './HistoryDrawer'
import type { Session } from '../lib/api'

const now = new Date('2026-06-04T12:00:00Z').getTime()
const sessions: Session[] = [
  { id: 's1', title: 'Fix login', turns: 2, updated_at: now, created_at: now },
  { id: 's2', title: 'Refactor parser', turns: 5, updated_at: new Date('2026-06-03T10:00:00Z').getTime(), created_at: 0 },
]

describe('HistoryDrawer', () => {
  it('renders nothing when closed', () => {
    render(<HistoryDrawer open={false} sessions={sessions} now={now} />)
    expect(screen.queryByTestId('history-drawer')).not.toBeInTheDocument()
  })

  it('renders all sessions grouped by day when open', () => {
    render(<HistoryDrawer open sessions={sessions} now={now} />)
    expect(screen.getByTestId('history-drawer')).toBeInTheDocument()
    expect(screen.getByTestId('group-today')).toBeInTheDocument()
    expect(screen.getByTestId('group-yesterday')).toBeInTheDocument()
  })

  it('filters across all sessions by search query', async () => {
    render(<HistoryDrawer open sessions={sessions} now={now} />)
    await userEvent.type(screen.getByTestId('history-search'), 'login')
    expect(screen.getByText('Fix login')).toBeInTheDocument()
    expect(screen.queryByText('Refactor parser')).not.toBeInTheDocument()
  })

  it('calls onResume with the session id when a row is selected', async () => {
    const onResume = vi.fn()
    render(<HistoryDrawer open sessions={sessions} now={now} onResume={onResume} />)
    const row = within(screen.getByTestId('group-today')).getByText('Fix login')
    await userEvent.click(row)
    expect(onResume).toHaveBeenCalledWith('s1')
  })

  it('calls onClose when the backdrop is clicked', async () => {
    const onClose = vi.fn()
    render(<HistoryDrawer open sessions={sessions} now={now} onClose={onClose} />)
    await userEvent.click(screen.getByTestId('history-backdrop'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
