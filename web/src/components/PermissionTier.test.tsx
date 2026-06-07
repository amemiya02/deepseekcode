import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { PermissionTier } from './PermissionTier'

describe('PermissionTier', () => {
  it('renders the three tiers', () => {
    render(<PermissionTier value="workspace-write" onChange={() => {}} />)
    expect(screen.getByText(/read-only/i)).toBeInTheDocument()
    expect(screen.getByText(/workspace-write/i)).toBeInTheDocument()
    expect(screen.getByText(/full/i)).toBeInTheDocument()
  })

  it('marks the active tier with aria-checked', () => {
    render(<PermissionTier value="full" onChange={() => {}} />)
    expect(screen.getByTestId('tier-full')).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByTestId('tier-read-only')).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByTestId('tier-workspace-write')).toHaveAttribute('aria-checked', 'false')
  })

  it('fires onChange when a tier is clicked', async () => {
    const onChange = vi.fn()
    render(<PermissionTier value="read-only" onChange={onChange} />)
    await userEvent.click(screen.getByTestId('tier-full'))
    expect(onChange).toHaveBeenCalledWith('full')
  })

  it('does not fire onChange when disabled', async () => {
    const onChange = vi.fn()
    render(<PermissionTier value="read-only" onChange={onChange} disabled />)
    await userEvent.click(screen.getByTestId('tier-full'))
    expect(onChange).not.toHaveBeenCalled()
  })
})
