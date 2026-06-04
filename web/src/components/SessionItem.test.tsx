import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SessionItem } from './SessionItem'
import { listSessions, type Session } from '../lib/api'

const base: Session = { id: 's1', title: 'Fix login bug', turns: 3, updated_at: 0, created_at: 0 }

describe('SessionItem', () => {
  it('renders title and turn count', () => {
    render(<SessionItem session={base} active={false} />)
    expect(screen.getByText('Fix login bug')).toBeInTheDocument()
    expect(screen.getByText('3 turns')).toBeInTheDocument()
  })

  it('uses singular turn when turns === 1', () => {
    render(<SessionItem session={{ ...base, turns: 1 }} active={false} />)
    expect(screen.getByText('1 turn')).toBeInTheDocument()
  })

  it('marks the active row with aria-current', () => {
    render(<SessionItem session={base} active />)
    expect(screen.getByTestId('session-item')).toHaveAttribute('aria-current', 'true')
  })

  it('calls onSelect when the row is clicked', async () => {
    const onSelect = vi.fn()
    render(<SessionItem session={base} active={false} onSelect={onSelect} />)
    await userEvent.click(screen.getByTestId('session-item'))
    expect(onSelect).toHaveBeenCalledWith('s1')
  })

  it('calls onDelete without selecting when the delete control is clicked', async () => {
    const onSelect = vi.fn()
    const onDelete = vi.fn()
    render(<SessionItem session={base} active={false} onSelect={onSelect} onDelete={onDelete} />)
    await userEvent.click(screen.getByTestId('session-delete'))
    expect(onDelete).toHaveBeenCalledWith('s1')
    expect(onSelect).not.toHaveBeenCalled()
  })

  // Contract regression: run the EXACT JSON the Go gateway emits for GET /v1/sessions
  // (see internal/gateway/sessions.go) through the real listSessions() parser into the
  // real component. A field mismatch (backend `turn_count` vs frontend `turns`) used to
  // render the literal placeholder "{n} turns" — this guards the cross-stack contract.
  it('renders the live gateway wire shape: turns is interpolated, never "{n}"', async () => {
    const wire = {
      sessions: [{ id: 'sess-1', title: 'sess-1', turns: 0, created_at: 1780565710666, updated_at: 1780565710666 }],
    }
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => wire }) as unknown as Response))
    try {
      const sessions = await listSessions()
      render(<SessionItem session={sessions[0]} active={false} />)
      expect(screen.getByText('0 turns')).toBeInTheDocument()
      expect(screen.queryByText(/\{n\}/)).toBeNull()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
