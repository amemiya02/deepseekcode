import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { GitBranchPicker } from './GitBranchPicker'

describe('GitBranchPicker', () => {
  it('shows current branch and lists branches', () => {
    render(<GitBranchPicker current="main" branches={['main', 'feat/x']} onSelect={() => {}} />)
    expect(screen.getByText('main')).toBeInTheDocument()
  })

  it('shows branch icon in trigger', () => {
    render(<GitBranchPicker current="main" branches={['main', 'feat/x']} onSelect={() => {}} />)
    expect(screen.getByTestId('git-branch-icon')).toBeInTheDocument()
  })

  it('opens dropdown on click and lists all branches', async () => {
    const user = userEvent.setup()
    render(<GitBranchPicker current="main" branches={['main', 'feat/x', 'dev']} onSelect={() => {}} />)
    await user.click(screen.getByTestId('branch-trigger'))
    expect(screen.getByText('feat/x')).toBeInTheDocument()
    expect(screen.getByText('dev')).toBeInTheDocument()
  })

  it('calls onSelect when a branch is clicked', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(<GitBranchPicker current="main" branches={['main', 'feat/x']} onSelect={onSelect} />)
    await user.click(screen.getByTestId('branch-trigger'))
    await user.click(screen.getByText('feat/x'))
    expect(onSelect).toHaveBeenCalledWith('feat/x')
  })

  it('closes dropdown after selection', async () => {
    const user = userEvent.setup()
    render(<GitBranchPicker current="main" branches={['main', 'feat/x']} onSelect={() => {}} />)
    await user.click(screen.getByTestId('branch-trigger'))
    expect(screen.getByText('feat/x')).toBeInTheDocument()
    await user.click(screen.getByText('feat/x'))
    expect(screen.queryByText('feat/x')).not.toBeInTheDocument()
  })
})
