import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { EditorSection } from './EditorSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('EditorSection', () => {
  it('renders verbosity as BrandedSelect and transparent-bg as a Switch', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { transparentBackground: false } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><EditorSection /></LocaleProvider>)
    expect(await screen.findByTestId('editor-verbosity')).toHaveAttribute('aria-haspopup', 'listbox')
    expect(screen.getByTestId('editor-transparent')).toHaveAttribute('role', 'switch')
  })
})
