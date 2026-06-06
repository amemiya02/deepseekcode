import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CacheCard } from './CacheCard'

describe('CacheCard', () => {
  it('renders turn% and rolling avg%', () => {
    render(<CacheCard turnPct={0.91} avgPct={0.84} prefixes={1} eviction={false} />)
    expect(screen.getByTestId('cache-turn')).toHaveTextContent('91%')
    expect(screen.getByTestId('cache-avg')).toHaveTextContent('84%')
  })

  it('shows a stability proof badge when prefixes === 1', () => {
    render(<CacheCard turnPct={0.9} avgPct={0.9} prefixes={1} eviction={false} />)
    const proof = screen.getByTestId('prefix-stable')
    expect(proof).toBeInTheDocument()
    expect(proof).toHaveTextContent('1')
  })

  it('shows an unstable warning when prefixes > 1', () => {
    render(<CacheCard turnPct={0.5} avgPct={0.5} prefixes={3} eviction={false} />)
    expect(screen.queryByTestId('prefix-stable')).not.toBeInTheDocument()
    expect(screen.getByTestId('prefix-unstable')).toHaveTextContent('3')
  })

  it('shows an eviction warning when eviction is true', () => {
    render(<CacheCard turnPct={0.2} avgPct={0.3} prefixes={2} eviction />)
    expect(screen.getByTestId('cache-eviction')).toBeInTheDocument()
  })
})
