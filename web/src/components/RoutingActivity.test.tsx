import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { RoutingActivity } from './RoutingActivity'

describe('RoutingActivity', () => {
  it('renders from, to, and reason', () => {
    render(<RoutingActivity from="flash" to="pro" reason="hard task" />)
    expect(screen.getByText('flash')).toBeInTheDocument()
    expect(screen.getByText('pro')).toBeInTheDocument()
    expect(screen.getByText('hard task')).toBeInTheDocument()
  })
})
