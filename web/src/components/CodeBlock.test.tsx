import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { CodeBlock } from './CodeBlock'

describe('CodeBlock', () => {
  it('renders a language label and copy button', () => {
    render(<CodeBlock className="language-ts">const x = 1</CodeBlock>)
    expect(screen.getByTestId('codecard-lang')).toHaveTextContent('ts')
    expect(screen.getByTestId('codecard-copy')).toBeInTheDocument()
  })

  it('falls back to "code" when no language class', () => {
    render(<CodeBlock>plain</CodeBlock>)
    expect(screen.getByTestId('codecard-lang')).toHaveTextContent('code')
  })

  it('copies the raw text on click', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    render(<CodeBlock className="language-js">hello()</CodeBlock>)
    fireEvent.click(screen.getByTestId('codecard-copy'))
    expect(writeText).toHaveBeenCalledWith('hello()')
  })

  it('renders inside a CodeIsland', () => {
    const { container } = render(
      <CodeBlock className="language-ts">const x = 1</CodeBlock>,
    )
    expect(container.querySelector('.island')).not.toBeNull()
    expect(screen.getByTestId('codecard-lang')).toHaveTextContent('ts')
    expect(screen.getByTestId('codecard-copy')).toBeInTheDocument()
  })
})
