import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { CodeIsland } from './CodeIsland'

describe('CodeIsland', () => {
  it('renders label, meta, actions and children inside the island body', () => {
    render(
      <CodeIsland label="ts" meta={<span>+3 −1</span>} actions={<button>Copy</button>}>
        <pre>code here</pre>
      </CodeIsland>,
    )
    const island = screen.getByTestId('code-island')
    expect(island).toHaveClass('island')
    expect(island).toHaveAttribute('data-island')
    expect(screen.getByText('ts')).toBeInTheDocument()
    expect(screen.getByText('+3 −1')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
    expect(screen.getByText('code here')).toBeInTheDocument()
  })

  it('omits the header bar when no label/meta/actions are given', () => {
    const { container } = render(<CodeIsland><pre>x</pre></CodeIsland>)
    expect(container.querySelector('.island__bar')).toBeNull()
    expect(screen.getByText('x')).toBeInTheDocument()
  })
})
