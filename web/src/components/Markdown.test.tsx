import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { Markdown } from './Markdown'

describe('Markdown', () => {
  it('renders a heading and inline emphasis', () => {
    render(<Markdown text={'# Title\n\nhello **world**'} />)
    expect(screen.getByRole('heading', { name: 'Title' })).toBeInTheDocument()
    expect(screen.getByText('world')).toBeInTheDocument()
  })

  it('renders a fenced code block as a <pre>', () => {
    const { container } = render(<Markdown text={'```go\nfmt.Println("hi")\n```'} />)
    expect(container.querySelector('pre')).toBeInTheDocument()
    expect(screen.getByText(/fmt\.Println/)).toBeInTheDocument()
  })

  it('renders a GFM table', () => {
    render(<Markdown text={'| a | b |\n|---|---|\n| 1 | 2 |'} />)
    expect(screen.getByRole('table')).toBeInTheDocument()
  })
})
