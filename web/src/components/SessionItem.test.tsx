import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SessionItem } from './SessionItem'
import type { Session } from '../lib/api'

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
})
