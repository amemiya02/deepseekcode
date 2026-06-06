import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RewindMenu } from './RewindMenu'

describe('RewindMenu', () => {
  it('shows the checkpoint disclaimer', () => {
    render(<RewindMenu messageIndex={2} />)
    expect(screen.getByText(/git is the source of truth/i)).toBeInTheDocument()
  })

  it('fires onRewind with the chosen scope and message index', async () => {
    const onRewind = vi.fn()
    render(<RewindMenu messageIndex={2} onRewind={onRewind} />)
    await userEvent.click(screen.getByTestId('rewind-conversation'))
    expect(onRewind).toHaveBeenCalledWith(2, 'conversation')
  })

  it('fires onFork', async () => {
    const onFork = vi.fn()
    render(<RewindMenu messageIndex={2} onFork={onFork} />)
    await userEvent.click(screen.getByTestId('rewind-fork'))
    expect(onFork).toHaveBeenCalled()
  })

  it('fires onSummarize with mode upto and the message index', async () => {
    const onSummarize = vi.fn()
    render(<RewindMenu messageIndex={2} onSummarize={onSummarize} />)
    await userEvent.click(screen.getByTestId('rewind-summarize-upto'))
    expect(onSummarize).toHaveBeenCalledWith('upto', 2)
  })
})
