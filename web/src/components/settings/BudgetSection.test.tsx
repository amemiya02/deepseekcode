import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { BudgetSection } from './BudgetSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('BudgetSection', () => {
  it('renders budget inputs', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { maxReadBytes: 5242880, maxWriteBytes: 5242880 } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><BudgetSection /></LocaleProvider>)
    expect(await screen.findByTestId('budget-read')).toBeInTheDocument()
    expect(screen.getByTestId('budget-write')).toBeInTheDocument()
  })
})
