import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { Markdown } from './markdown'

describe('Markdown', () => {
  it('renders a heading', () => {
    render(<Markdown text={'# Hello world'} />)
    expect(screen.getByRole('heading', { name: 'Hello world' })).toBeInTheDocument()
  })

  it('renders GFM tables', () => {
    render(<Markdown text={'| a | b |\n|---|---|\n| 1 | 2 |'} />)
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByText('a')).toBeInTheDocument()
  })

  it('renders inline code', () => {
    render(<Markdown text={'use `npm run build` now'} />)
    expect(screen.getByText('npm run build')).toBeInTheDocument()
  })

  it('renders a fenced code block', () => {
    const { container } = render(<Markdown text={'```ts\nconst x = 1\n```'} />)
    const pre = container.querySelector('pre')
    expect(pre).toBeInTheDocument()
    expect(pre!.textContent).toMatch(/const x = 1/)
  })
})
