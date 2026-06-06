import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { DuetSection } from './DuetSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('DuetSection', () => {
  it('renders duet enable as Switch', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { duetEnabled: true } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><DuetSection /></LocaleProvider>)
    expect(await screen.findByTestId('duet-enable')).toHaveAttribute('role', 'switch')
  })
})
