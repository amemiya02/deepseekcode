import { describe, it, expect } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RoutingCard } from './RoutingCard'
import type { RoutingHop } from '../../lib/api'

describe('RoutingCard', () => {
  it('renders each hop with from→to and reason', () => {
    const hops: RoutingHop[] = [
      { from: 'flash', to: 'pro', reason: 'hard task' },
      { from: 'pro', to: 'flash', reason: 'trivial follow-up' },
    ]
    render(<RoutingCard hops={hops} />)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(within(items[0]).getByText('flash')).toBeInTheDocument()
    expect(within(items[0]).getByText('pro')).toBeInTheDocument()
    expect(within(items[0]).getByText('hard task')).toBeInTheDocument()
  })

  it('shows an empty state when there are no hops', () => {
    render(<RoutingCard hops={[]} />)
    expect(screen.getByTestId('routing-empty')).toBeInTheDocument()
  })
})
