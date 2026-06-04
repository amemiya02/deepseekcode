import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { OnboardingWizard } from './OnboardingWizard'
import * as system from '../lib/system'
import * as theme from '../lib/theme'

afterEach(() => vi.restoreAllMocks())

describe('OnboardingWizard', () => {
  it('advances from key step to theme step only after a valid key', async () => {
    const connect = vi.spyOn(system, 'connectKey').mockResolvedValue()
    render(<OnboardingWizard open onComplete={() => {}} />)
    await userEvent.type(screen.getByLabelText(/api key/i), 'sk-good')
    await userEvent.click(screen.getByRole('button', { name: /next|continue/i }))
    await waitFor(() => {
      expect(connect).toHaveBeenCalled()
      expect(screen.getByText(/choose.*theme|theme/i)).toBeInTheDocument()
    })
  })

  it('blocks advancing when the key is invalid', async () => {
    vi.spyOn(system, 'connectKey').mockRejectedValue(new Error('key validation failed: bad'))
    render(<OnboardingWizard open onComplete={() => {}} />)
    await userEvent.type(screen.getByLabelText(/api key/i), 'sk-bad')
    await userEvent.click(screen.getByRole('button', { name: /next|continue/i }))
    await waitFor(() => expect(screen.getByText(/key validation failed/i)).toBeInTheDocument())
    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument()
  })

  it('completes after theme + permission default and applies the theme', async () => {
    vi.spyOn(system, 'connectKey').mockResolvedValue()
    const apply = vi.spyOn(theme, 'setThemeSettings').mockImplementation(() => {})
    vi.spyOn(system, 'saveConfig').mockResolvedValue({} as system.ConfigDTO)
    const onComplete = vi.fn()
    render(<OnboardingWizard open onComplete={onComplete} />)
    await userEvent.type(screen.getByLabelText(/api key/i), 'sk-good')
    await userEvent.click(screen.getByRole('button', { name: /next|continue/i }))
    await screen.findByText(/theme/i)
    await userEvent.click(screen.getByRole('button', { name: /next|continue/i }))
    await userEvent.click(screen.getByRole('button', { name: /finish|done|start/i }))
    await waitFor(() => expect(onComplete).toHaveBeenCalled())
    expect(apply).toHaveBeenCalled()
  })
})
