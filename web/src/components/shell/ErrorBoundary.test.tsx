import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ErrorBoundary } from './ErrorBoundary'
import { LocaleProvider } from '../../lib/i18n'

function Boom(): JSX.Element {
  throw new Error('boom')
}

let errSpy: ReturnType<typeof vi.spyOn>
beforeEach(() => {
  // React logs caught errors to console.error; silence it for clean test output.
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => errSpy.mockRestore())

describe('ErrorBoundary', () => {
  it('renders children when they do not throw', () => {
    render(
      <LocaleProvider>
        <ErrorBoundary>
          <span>ok</span>
        </ErrorBoundary>
      </LocaleProvider>,
    )
    expect(screen.getByText('ok')).toBeInTheDocument()
    expect(screen.queryByText('Something went wrong')).toBeNull()
  })

  it('shows the fallback when a child throws during render', () => {
    render(
      <LocaleProvider>
        <ErrorBoundary>
          <Boom />
        </ErrorBoundary>
      </LocaleProvider>,
    )
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(screen.getByText('boom')).toBeInTheDocument()
  })
})
