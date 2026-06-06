import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { LocaleProvider } from '../../lib/i18n'
import { AboutSection } from './AboutSection'
import * as system from '../../lib/system'

afterEach(() => vi.restoreAllMocks())

describe('AboutSection', () => {
  it('shows the current version and an up-to-date status', async () => {
    vi.spyOn(system, 'fetchUpdate').mockResolvedValue({
      current: '1.2.3', latest: '1.2.3', updateAvailable: false, url: '',
    })
    render(<LocaleProvider><AboutSection /></LocaleProvider>)
    await waitFor(() => expect(screen.getByText('1.2.3')).toBeInTheDocument())
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('renders a repository link', async () => {
    vi.spyOn(system, 'fetchUpdate').mockResolvedValue({
      current: '1.2.3', latest: '1.2.3', updateAvailable: false, url: '',
    })
    render(<LocaleProvider><AboutSection /></LocaleProvider>)
    await waitFor(() => expect(screen.getByText('1.2.3')).toBeInTheDocument())
    expect(screen.getByRole('link', { name: /github|repository|仓库/i })).toBeInTheDocument()
  })
})
