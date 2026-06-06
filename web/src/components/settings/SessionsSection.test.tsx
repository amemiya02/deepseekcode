import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { SessionsSection } from './SessionsSection'
import * as useConfigMod from '../../lib/useConfig'

afterEach(() => vi.restoreAllMocks())

describe('SessionsSection', () => {
  it('renders session inputs', async () => {
    vi.spyOn(useConfigMod, 'useConfig').mockReturnValue({
      cfg: { sessionTTLDays: 90, sessionSnapshotKeep: 30, sessionAutoResumeAge: 24 } as never,
      error: '', reload: () => {}, clearError: () => {}, patch: vi.fn(),
    })
    render(<LocaleProvider><SessionsSection /></LocaleProvider>)
    expect(await screen.findByTestId('sessions-ttl')).toBeInTheDocument()
    expect(screen.getByTestId('sessions-snapshot')).toBeInTheDocument()
    expect(screen.getByTestId('sessions-resume')).toBeInTheDocument()
  })
})
