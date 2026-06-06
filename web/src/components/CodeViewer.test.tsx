import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { CodeViewer } from './CodeViewer'

describe('CodeViewer', () => {
  it('renders file content in the fallback pre', () => {
    render(<CodeViewer path="a.go" content="package main" binary={false} truncated={false} />)
    expect(screen.getByTestId('code-fallback').textContent).toContain('package main')
  })

  it('shows a binary notice instead of content', () => {
    render(<CodeViewer path="blob.bin" content="" binary={true} truncated={false} />)
    expect(screen.getByText(/binary file/i)).toBeInTheDocument()
  })

  it('shows a truncation notice', () => {
    render(<CodeViewer path="big.txt" content="x" binary={false} truncated={true} />)
    expect(screen.getByText(/truncated/i)).toBeInTheDocument()
  })

  it('shows an empty state when no path is selected', () => {
    render(<CodeViewer path="" content="" binary={false} truncated={false} />)
    expect(screen.getByText(/select a file/i)).toBeInTheDocument()
  })
})
