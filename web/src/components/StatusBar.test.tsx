import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { StatusBar } from './StatusBar'

const base = {
  status: 'idle' as const,
  model: 'deepseek-chat',
  effort: 'medium',
  ctxPct: 0.42,
  turnCachePct: 0.9,
  avgCachePct: 0.83,
  eviction: false,
  turnCny: 0.012,
  sessionCny: 0.21,
  balance: 12.5,
  currency: 'CNY',
  jobs: 0,
  retryAttempt: 0,
  retryMax: 0,
}

describe('StatusBar', () => {
  it('renders model, effort, ctx%, turn/avg cache%, ¥ turn/session, balance', () => {
    render(<StatusBar {...base} />)
    expect(screen.getByText('deepseek-chat')).toBeInTheDocument()
    expect(screen.getByText('medium')).toBeInTheDocument()
    expect(screen.getByTestId('ctx-pct')).toHaveTextContent('42%')
    expect(screen.getByTestId('turn-cache')).toHaveTextContent('90%')
    expect(screen.getByTestId('avg-cache')).toHaveTextContent('83%')
    expect(screen.getByTestId('turn-cost')).toHaveTextContent('¥0.0120')
    expect(screen.getByTestId('session-cost')).toHaveTextContent('¥0.2100')
    expect(screen.getByTestId('balance')).toHaveTextContent('12.5')
  })

  it('reflects status with the matching dot data-status', () => {
    render(<StatusBar {...base} status="streaming" />)
    expect(screen.getByTestId('status-dot')).toHaveAttribute('data-status', 'streaming')
  })

  it('shows an eviction warning when eviction is true', () => {
    render(<StatusBar {...base} eviction />)
    expect(screen.getByTestId('eviction-warning')).toBeInTheDocument()
  })

  it('hides the eviction warning when eviction is false', () => {
    render(<StatusBar {...base} eviction={false} />)
    expect(screen.queryByTestId('eviction-warning')).not.toBeInTheDocument()
  })

  it('shows a retry indicator only when retryMax > 0', () => {
    const { rerender } = render(<StatusBar {...base} />)
    expect(screen.queryByTestId('retry')).not.toBeInTheDocument()
    rerender(<StatusBar {...base} retryAttempt={2} retryMax={3} />)
    expect(within(screen.getByTestId('retry')).getByText(/2\/3/)).toBeInTheDocument()
  })

  it('shows a jobs indicator only when jobs > 0', () => {
    const { rerender } = render(<StatusBar {...base} />)
    expect(screen.queryByTestId('jobs')).not.toBeInTheDocument()
    rerender(<StatusBar {...base} jobs={3} />)
    expect(screen.getByTestId('jobs')).toHaveTextContent('3')
  })
})
