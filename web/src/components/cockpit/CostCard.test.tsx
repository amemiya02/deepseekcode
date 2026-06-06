import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CostCard } from './CostCard'

describe('CostCard', () => {
  it('renders ¥ turn, ¥ session and output tokens', () => {
    render(<CostCard turnCny={0.012} sessionCny={0.34} outputTokens={1234} balance={12.5} currency="CNY" />)
    expect(screen.getByTestId('cost-turn')).toHaveTextContent('¥0.0120')
    expect(screen.getByTestId('cost-session')).toHaveTextContent('¥0.3400')
    expect(screen.getByTestId('cost-output')).toHaveTextContent('1,234')
  })

  it('renders the wallet balance with currency', () => {
    render(<CostCard turnCny={0} sessionCny={0} outputTokens={0} balance={8.75} currency="CNY" />)
    expect(screen.getByTestId('cost-balance')).toHaveTextContent('8.75')
    expect(screen.getByTestId('cost-balance')).toHaveTextContent('CNY')
  })

  it('shows an em dash when balance is null', () => {
    render(<CostCard turnCny={0} sessionCny={0} outputTokens={0} balance={null} currency="CNY" />)
    expect(screen.getByTestId('cost-balance')).toHaveTextContent('—')
  })
})
