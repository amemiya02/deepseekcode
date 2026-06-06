import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { ProvidersSection } from './ProvidersSection'
import { LocaleProvider } from '../../lib/i18n'
import * as system from '../../lib/system'

afterEach(() => vi.restoreAllMocks())

describe('ProvidersSection', () => {
  it('validates a key and shows success', async () => {
    const connect = vi.spyOn(system, 'connectKey').mockResolvedValue()
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ baseUrl: 'https://api.deepseek.com', model: 'deepseek-v4-flash' } as never)
    render(<LocaleProvider><ProvidersSection /></LocaleProvider>)
    await userEvent.type(screen.getByLabelText(/api key/i), 'sk-good')
    await userEvent.clear(screen.getByLabelText(/base url/i))
    await userEvent.type(screen.getByLabelText(/base url/i), 'https://api.deepseek.com')
    await userEvent.click(screen.getByRole('button', { name: /validate|connect/i }))
    await waitFor(() => {
      expect(connect).toHaveBeenCalledWith(expect.objectContaining({ apiKey: 'sk-good' }))
      expect(screen.getByText(/connected|valid/i)).toBeInTheDocument()
    })
  })

  it('shows the error message when validation fails', async () => {
    vi.spyOn(system, 'connectKey').mockRejectedValue(new Error('key validation failed: bad'))
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ baseUrl: 'https://api.deepseek.com', model: 'deepseek-v4-flash' } as never)
    render(<LocaleProvider><ProvidersSection /></LocaleProvider>)
    await userEvent.type(screen.getByLabelText(/api key/i), 'sk-bad')
    await userEvent.click(screen.getByRole('button', { name: /validate|connect/i }))
    await waitFor(() => expect(screen.getByText(/key validation failed/i)).toBeInTheDocument())
  })

  it('defaults the model to deepseek-v4-flash and prefills from config', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ baseUrl: 'https://api.deepseek.com', model: '' } as never)
    render(<LocaleProvider><ProvidersSection /></LocaleProvider>)
    await waitFor(() => expect((screen.getByLabelText(/model/i) as HTMLInputElement).value).toBe('deepseek-v4-flash'))
  })
})
