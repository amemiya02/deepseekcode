import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'

function Hello() {
  return <p data-testid="hello">hi</p>
}

describe('react toolchain', () => {
  it('renders a TSX component and exposes jest-dom matchers', () => {
    render(<Hello />)
    expect(screen.getByTestId('hello')).toBeInTheDocument()
    expect(screen.getByTestId('hello')).toHaveTextContent('hi')
  })
})
