import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { PermissionsSection } from './PermissionsSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('PermissionsSection', () => {
  it('renders permissionDefault as BrandedSelect and autoClarify as Switch', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { permissionDefault: 'ask', autoClarify: false } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><PermissionsSection /></LocaleProvider>)
    expect(await screen.findByTestId('perm-default')).toHaveAttribute('aria-haspopup', 'listbox')
    expect(screen.getByTestId('perm-autoclarify')).toHaveAttribute('role', 'switch')
  })
})
