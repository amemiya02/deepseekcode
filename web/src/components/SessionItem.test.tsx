import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SessionItem } from './SessionItem'
import { listSessions, type Session } from '../lib/api'
import { formatRelativeTime } from '../lib/format'

const now = new Date('2026-06-04T12:00:00Z').getTime()
const base: Session = { id: 's1', title: 'Fix login bug', turns: 3, updated_at: now - 3 * 3_600_000, created_at: 0 }

describe('SessionItem', () => {
  it('renders title and a relative timestamp', () => {
    render(<SessionItem session={base} active={false} now={now} />)
    expect(screen.getByText('Fix login bug')).toBeInTheDocument()
    expect(screen.getByTestId('session-time')).toHaveTextContent('3h')
  })

  it('marks the active row with aria-current', () => {
    render(<SessionItem session={base} active now={now} />)
    expect(screen.getByTestId('session-item')).toHaveAttribute('aria-current', 'true')
  })

  it('calls onSelect when the row is clicked', async () => {
    const onSelect = vi.fn()
    render(<SessionItem session={base} active={false} now={now} onSelect={onSelect} />)
    await userEvent.click(screen.getByTestId('session-item'))
    expect(onSelect).toHaveBeenCalledWith('s1')
  })

  it('calls onDelete without selecting when the delete control is clicked', async () => {
    const onSelect = vi.fn()
    const onDelete = vi.fn()
    render(<SessionItem session={base} active={false} now={now} onSelect={onSelect} onDelete={onDelete} />)
    await userEvent.click(screen.getByTestId('session-delete'))
    expect(onDelete).toHaveBeenCalledWith('s1')
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('shows the rename button on hover and enters rename mode when clicked', async () => {
    const onRename = vi.fn()
    render(<SessionItem session={base} active={false} now={now} onRename={onRename} />)
    await userEvent.click(screen.getByTestId('session-rename'))
    expect(screen.getByTestId('session-rename-input')).toBeInTheDocument()
  })

  it('calls onRename with new title when Enter is pressed', async () => {
    const onRename = vi.fn()
    render(<SessionItem session={base} active={false} now={now} onRename={onRename} />)
    await userEvent.click(screen.getByTestId('session-rename'))
    const input = screen.getByTestId('session-rename-input')
    await userEvent.clear(input)
    await userEvent.type(input, 'New title{Enter}')
    await waitFor(() => expect(onRename).toHaveBeenCalledWith('s1', 'New title'))
  })

  it('cancels rename on Escape and does not call onRename', async () => {
    const onRename = vi.fn()
    render(<SessionItem session={base} active={false} now={now} onRename={onRename} />)
    await userEvent.click(screen.getByTestId('session-rename'))
    await userEvent.keyboard('{Escape}')
    expect(onRename).not.toHaveBeenCalled()
    expect(screen.queryByTestId('session-rename-input')).not.toBeInTheDocument()
  })

  it('commits rename when the confirm button is clicked', async () => {
    const onRename = vi.fn()
    render(<SessionItem session={base} active={false} now={now} onRename={onRename} />)
    await userEvent.click(screen.getByTestId('session-rename'))
    const input = screen.getByTestId('session-rename-input')
    await userEvent.clear(input)
    await userEvent.type(input, 'Committed')
    await userEvent.click(screen.getByTestId('session-rename-commit'))
    await waitFor(() => expect(onRename).toHaveBeenCalledWith('s1', 'Committed'))
  })

  // Contract regression: run the EXACT JSON the Go gateway emits for GET /v1/sessions
  // (see internal/gateway/sessions.go) through the real listSessions() parser into the
  // real component. A field mismatch (backend `turn_count` vs frontend `turns`) used to
  // render the literal placeholder "{n} turns" — this guards the cross-stack contract.
  it('renders the live gateway wire shape: title present AND no "{n}" placeholder', async () => {
    const wire = {
      sessions: [{ id: 'sess-1', title: 'sess-1', turns: 0, created_at: 1780565710666, updated_at: 1780565710666 }],
    }
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => wire }) as unknown as Response))
    try {
      const sessions = await listSessions()
      render(<SessionItem session={sessions[0]} active={false} now={now} />)
      expect(screen.getByText('sess-1')).toBeInTheDocument()
      expect(screen.queryByText(/\{n\}/)).toBeNull()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
