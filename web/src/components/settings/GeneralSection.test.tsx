import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { GeneralSection } from './GeneralSection'
import * as system from '../../lib/system'

afterEach(() => vi.restoreAllMocks())

const baseCfg: system.ConfigDTO = {
  theme: 'graphite', accent: 'indigo', density: 'comfortable', language: 'en',
  transcriptVerbosity: 'normal', model: 'deepseek-v4', reasoningEffort: 'high',
  baseUrl: 'https://api.deepseek.com', autoRoute: false, escalationModel: '',
  duetEnabled: false, sandboxEnabled: false, sandboxNetwork: false,
  autoReasoning: false, autoClarify: false,
  proxyMode: 'auto', proxyScheme: 'http', proxyUrl: '', noProxy: '',
  permissionDefault: 'ask',
}

describe('GeneralSection', () => {
  it('loads config and shows the language + verbosity controls', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    render(<GeneralSection />)
    await waitFor(() => expect(screen.getByLabelText(/language/i)).toBeInTheDocument())
    const verbosity = screen.getByLabelText(/verbosity/i) as HTMLSelectElement
    expect(['normal', 'verbose', 'summary']).toContain(verbosity.value)
  })

  it('saves the verbosity change via saveConfig', async () => {
    vi.spyOn(system, 'fetchConfig').mockResolvedValue({ ...baseCfg })
    const save = vi.spyOn(system, 'saveConfig').mockResolvedValue({ ...baseCfg, transcriptVerbosity: 'verbose' })
    render(<GeneralSection />)
    const verbosity = (await screen.findByLabelText(/verbosity/i)) as HTMLSelectElement
    await userEvent.selectOptions(verbosity, 'verbose')
    await waitFor(() => expect(save).toHaveBeenCalledWith(expect.objectContaining({ transcriptVerbosity: 'verbose' })))
  })
})
