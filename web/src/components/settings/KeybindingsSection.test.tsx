import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { KeybindingsSection } from './KeybindingsSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('KeybindingsSection', () => {
  it('renders vim toggle as Switch', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { vimKeybindings: true } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><KeybindingsSection /></LocaleProvider>)
    expect(await screen.findByTestId('keys-vim')).toHaveAttribute('role', 'switch')
  })
})
