import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { SandboxSection } from './SandboxSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('SandboxSection', () => {
  it('renders sandbox switches', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { sandboxEnabled: false, sandboxNetwork: false } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><SandboxSection /></LocaleProvider>)
    expect(await screen.findByTestId('sandbox-enable')).toHaveAttribute('role', 'switch')
    expect(screen.getByTestId('sandbox-network')).toHaveAttribute('role', 'switch')
  })
})
