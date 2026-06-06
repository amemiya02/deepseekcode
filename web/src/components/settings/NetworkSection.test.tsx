import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { NetworkSection } from './NetworkSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('NetworkSection', () => {
  it('renders proxy selects as BrandedSelect', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { proxyMode: 'auto', proxyScheme: 'http', proxyUrl: '', noProxy: '' } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><NetworkSection /></LocaleProvider>)
    expect(await screen.findByTestId('network-mode')).toHaveAttribute('aria-haspopup', 'listbox')
  })
})
