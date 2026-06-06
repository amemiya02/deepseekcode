import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { LocaleProvider } from '../lib/i18n'
import { ToolCard } from './ToolCard'
import type { ToolItem } from '../lib/transcript'

const base: ToolItem = {
  type: 'tool', id: 't1', name: 'read_file', args: { path: 'a.go' },
  readOnly: true, status: 'ok', result: 'line1\nline2', truncated: false,
}

const wrap = (ui: React.ReactElement) => render(<LocaleProvider>{ui}</LocaleProvider>)

describe('ToolCard', () => {
  it('renders a read-only tool quiet + collapsed (result hidden until expand)', () => {
    const { container } = wrap(<ToolCard item={base} />)
    expect(screen.getByText('read_file')).toBeInTheDocument()
    expect(screen.queryByText(/line1/)).toBeNull()
    expect(container.querySelector('.tool--quiet')).toBeInTheDocument()
  })

  it('expands a read-only tool on header click to show result', async () => {
    wrap(<ToolCard item={base} />)
    await userEvent.click(screen.getByTestId('toolcard-header'))
    expect(screen.getByText(/line1/)).toBeInTheDocument()
  })

  it('renders a writer tool expanded by default', () => {
    wrap(<ToolCard item={{ ...base, name: 'write_file', readOnly: false }} />)
    expect(screen.getByText(/line1/)).toBeInTheDocument()
  })

  it('shows a running spinner while status is running', () => {
    wrap(<ToolCard item={{ ...base, status: 'running', result: undefined }} />)
    expect(screen.getByTestId('toolcard-spinner')).toBeInTheDocument()
  })

  it('shows a truncation notice when truncated', () => {
    wrap(<ToolCard item={{ ...base, readOnly: false, truncated: true }} />)
    expect(screen.getByText(/truncated/i)).toBeInTheDocument()
  })

  it('shows error state with the result text', () => {
    wrap(<ToolCard item={{ ...base, status: 'error', result: 'denied', readOnly: false }} />)
    expect(screen.getByText('denied')).toBeInTheDocument()
  })
})
