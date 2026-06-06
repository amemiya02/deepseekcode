import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { Switch } from './Switch'

describe('Switch', () => {
  it('renders a role=switch reflecting checked', () => {
    render(<Switch checked label="Vim" onChange={() => {}} />)
    const sw = screen.getByRole('switch', { name: 'Vim' })
    expect(sw).toHaveAttribute('aria-checked', 'true')
  })

  it('calls onChange with the negated value on click', () => {
    const onChange = vi.fn()
    render(<Switch checked={false} label="Vim" onChange={onChange} />)
    fireEvent.click(screen.getByRole('switch', { name: 'Vim' }))
    expect(onChange).toHaveBeenCalledWith(true)
  })

  it('does not fire onChange when disabled', () => {
    const onChange = vi.fn()
    render(<Switch checked={false} label="Vim" disabled onChange={onChange} />)
    fireEvent.click(screen.getByRole('switch', { name: 'Vim' }))
    expect(onChange).not.toHaveBeenCalled()
  })
})
